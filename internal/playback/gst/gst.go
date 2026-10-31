//go:build linux

// Package gst is Sona's audio output. Two decode lanes and a mixer feed one
// output pipeline:
//
//	lane ×2: appsrc (AAC access units) ! avdec_aac ! audioconvert ! audioresample ! appsink
//	mixer:   joins the lanes' decoded tracks gaplessly or with a crossfade
//	output:  appsrc (PCM) ! tail (equaliser, volume; tail.c) ! pipewiresink
//
// A track is loaded into a lane with Load (which flushes everything: play,
// seek) or Queue (which follows the current track without a gap). Each
// lane's segment clips the track to its audio: the encoder's priming at the
// start and its padding at the end are never decoded into the output, so
// consecutive tracks join sample-exact. Pushes block while a lane's queue is
// full (backpressure); a generation number per track makes superseded data
// harmless. The output is rebuilt in place when its sink fails or the device
// changes; PipeWire relinking the stream is not a failure.
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include <stdlib.h>
#include "gst.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// Decoders and sinks tried in order when Options leaves them empty.
var (
	Decoders = []string{"avdec_aac", "fdkaacdec", "faad"}
	Sinks    = []string{"pipewiresink", "pulsesink", "autoaudiosink"}
)

// Push errors.
var (
	ErrStale    = errors.New("gst: buffer belongs to a flushed generation")
	ErrFlushing = errors.New("gst: flushed while pushing")
	ErrEOS      = errors.New("gst: pipeline is at end of stream")
	ErrFlow     = errors.New("gst: pipeline refused buffer")
	// ErrEnded reports a Queue after the current track has played out (or
	// is already crossfading): the queued track cannot follow it any more.
	ErrEnded = errors.New("gst: the current track has ended")
)

var (
	initOnce sync.Once
	initErr  error
)

// Init initialises GStreamer once per process.
func Init() error {
	initOnce.Do(func() {
		var buf [256]C.char
		if C.sona_gst_init(&buf[0], C.size_t(len(buf))) != 0 {
			initErr = fmt.Errorf("gst: init: %s", C.GoString(&buf[0]))
		}
	})
	return initErr
}

// Version returns the GStreamer version string.
func Version() string {
	if Init() != nil {
		return ""
	}
	v := C.sona_gst_version()
	defer C.sona_gst_free(unsafe.Pointer(v))
	return C.GoString(v)
}

// Have reports whether an element factory is installed.
func Have(factory string) bool {
	if Init() != nil {
		return false
	}
	c := C.CString(factory)
	defer C.free(unsafe.Pointer(c))
	return C.sona_gst_have(c) != 0
}

// Options configures a Pipeline.
type Options struct {
	Decoder  string        // "" picks the first installed of Decoders
	Sink     string        // "" picks the first of Sinks that reaches READY
	AppName  string        // client/application name, default "Sona"
	MaxQueue time.Duration // per-lane queue of access units before Push blocks, default 12s
	// Device is the PipeWire sink (node.name) to play to; "" follows the
	// default. Only pipewiresink can target one.
	Device string
}

// State is a GstState.
type State int

const (
	StateNull    State = 1
	StateReady   State = 2
	StatePaused  State = 3
	StatePlaying State = 4
)

// EventKind classifies a bus event.
type EventKind int

const (
	EventEOS       EventKind = iota + 1 // the last track has been rendered
	EventError                          // fatal error of track Event.Gen (0: the output)
	EventWarning                        //
	EventState                          // output state changed to Event.State
	EventAsyncDone                      // preroll after a state change or flush finished
)

// Event is a bus message.
type Event struct {
	Kind  EventKind
	State State
	Gen   uint64 // EventError: the track whose lane failed; 0 for the output
	Err   error
}

// Caps describes an AAC stream.
type Caps struct {
	AudioConfig []byte // AudioSpecificConfig (codec_data)
	Rate        int
	Channels    int
}

// Track is a track to play. Times are on the track's timeline: position 0
// is its first audible sample, Priming earlier in the access units' media
// time.
type Track struct {
	Gen   uint64 // generation its pushes carry
	Caps  Caps
	Start time.Duration // where to start in the track
	// Priming is the encoder delay ahead of the audio (media time of
	// position 0); Duration the track's length, 0 if unknown. Audio
	// outside [0, Duration) is dropped by the decoder.
	Priming  time.Duration
	Duration time.Duration
	Gain     float64 // Sound Check gain (linear), applied while SetTrackGain is on; 0 = unity
}

// Pipeline is the output with its lanes and mixer.
type Pipeline struct {
	p       *C.sona_gst
	lanes   [2]*lane
	decoder string
	sink    string
	opts    Options
	events  chan Event
	quit    chan struct{}
	wg      sync.WaitGroup
	once    sync.Once
	// life keeps the C pipeline alive while calls are in flight: methods
	// hold it shared, Close exclusively before freeing.
	life   sync.RWMutex
	closed bool

	// mu guards the mixer and what it plays; the mixer goroutine holds it
	// while it fills a buffer (lane reads wait at most pullWait).
	mu      sync.Mutex
	cond    *sync.Cond
	mix     mixer
	epoch   uint64
	want    State
	stopped bool
	build   atomic.Uint64 // the output build whose messages count
	// played: the current output has been PLAYING. Until then a pause
	// keeps it in READY: pipewiresink 1.6 deadlocks its PipeWire loop
	// when it prerolls (PAUSED) without ever having rendered, and any
	// later flush or teardown hangs on that loop.
	played atomic.Bool
	// Output failures, for the rebuild budget.
	rebuilds []time.Time
	// The stall watchdog's last output position and when it last moved
	// (watch goroutine only).
	stallPos time.Duration
	stallAt  time.Time
	lastPos  time.Duration
	lastAt   time.Time
}

// ErrClosed reports a call on a closed Pipeline.
var ErrClosed = errors.New("gst: pipeline closed")

const (
	chunkFrames = 1024                  // frames per output buffer
	pullWait    = 10 * time.Millisecond // a lane read's wait for decoded audio
	// Output rebuilds tolerated in rebuildWindow before an output failure
	// is reported as fatal.
	maxRebuilds   = 3
	rebuildWindow = 30 * time.Second
	// stallAfter is how long the output may stand still while it should
	// play and has audio queued before it is rebuilt.
	stallAfter = 3 * time.Second
	// raceSlack is how far the output may run ahead of real time between
	// two checks before it counts as skipping.
	raceSlack = 500 * time.Millisecond
)

// use pins the pipeline for one call; release with p.life.RUnlock.
func (p *Pipeline) use() bool {
	p.life.RLock()
	if p.closed {
		p.life.RUnlock()
		return false
	}
	return true
}

// New builds the lanes and the output and brings the output to READY (so
// the sink has connected to the sound server).
func New(o Options) (*Pipeline, error) {
	if err := Init(); err != nil {
		return nil, err
	}
	if o.AppName == "" {
		o.AppName = "Sona"
	}
	if o.MaxQueue <= 0 {
		o.MaxQueue = 12 * time.Second
	}
	dec := o.Decoder
	if dec == "" {
		for _, d := range Decoders {
			if Have(d) {
				dec = d
				break
			}
		}
		if dec == "" {
			return nil, fmt.Errorf("gst: no AAC decoder installed (tried %s)", strings.Join(Decoders, ", "))
		}
	}
	o.Decoder = dec
	sinks := Sinks
	if o.Sink != "" {
		sinks = []string{o.Sink}
	}
	var errs []error
	for _, s := range sinks {
		if o.Sink == "" && !Have(s) {
			continue
		}
		p, err := build(s, o)
		if err == nil {
			return p, nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return nil, fmt.Errorf("gst: no audio sink installed (tried %s)", strings.Join(sinks, ", "))
	}
	return nil, errors.Join(errs...)
}

// streamProps returns the sink's stream-properties: PipeWire wants its own
// property names and capitalised roles, PulseAudio lower-case roles.
func streamProps(sink, app, device string) string {
	clean := strings.NewReplacer(`"`, ``, `\`, ``)
	esc := clean.Replace(app)
	switch sink {
	case "pipewiresink":
		props := fmt.Sprintf(`props,media.role=(string)Music,media.category=(string)Playback,media.name=(string)"%s",application.name=(string)"%s",application.id=(string)sona,node.description=(string)"%s"`, esc, esc, esc)
		if device != "" {
			// What pipewiresink's target-object sets: the session manager
			// links the stream to this node (or the default if it is gone).
			props += fmt.Sprintf(`,target.object=(string)"%s"`, clean.Replace(device))
		}
		return props
	case "pulsesink":
		return fmt.Sprintf(`props,media.role=(string)music,application.name=(string)"%s",application.id=(string)sona`, esc)
	}
	return ""
}

func build(sink string, o Options) (*Pipeline, error) {
	csink, capp, cprops := C.CString(sink), C.CString(o.AppName), C.CString(streamProps(sink, o.AppName, o.Device))
	defer func() {
		for _, s := range []*C.char{csink, capp, cprops} {
			C.free(unsafe.Pointer(s))
		}
	}()
	var buf [256]C.char
	cp := C.sona_gst_new(csink, capp, cprops, &buf[0], C.size_t(len(buf)))
	if cp == nil {
		return nil, fmt.Errorf("gst: %s: %s", sink, C.GoString(&buf[0]))
	}
	p := &Pipeline{p: cp, decoder: o.Decoder, sink: sink, opts: o, events: make(chan Event, 64), quit: make(chan struct{}), want: StateReady}
	p.cond = sync.NewCond(&p.mu)
	p.build.Store(1)
	cdec := C.CString(o.Decoder)
	defer C.free(unsafe.Pointer(cdec))
	for i := range p.lanes {
		l := C.sona_lane_new(cp, C.int(i), cdec, C.uint64_t(o.MaxQueue.Nanoseconds()), &buf[0], C.size_t(len(buf)))
		if l == nil {
			for _, ln := range p.lanes[:i] {
				C.sona_lane_free(ln.l)
			}
			C.sona_gst_free_pipeline(cp)
			return nil, fmt.Errorf("gst: %s", C.GoString(&buf[0]))
		}
		p.lanes[i] = &lane{l: l}
	}
	p.wg.Add(2)
	go p.watch()
	go p.run()
	return p, nil
}

// Decoder and Sink name the elements in use.
func (p *Pipeline) Decoder() string { return p.decoder }
func (p *Pipeline) Sink() string    { return p.sink }

// Events delivers bus events until Close. Warnings are dropped when the
// receiver lags; other events are never dropped while capacity remains.
func (p *Pipeline) Events() <-chan Event { return p.events }

func (p *Pipeline) send(ev Event) {
	select {
	case p.events <- ev:
	default:
		if ev.Kind != EventWarning {
			select {
			case p.events <- ev: // block rather than lose EOS or errors
			case <-p.quit:
			}
		}
	}
}

func (p *Pipeline) watch() {
	defer p.wg.Done()
	defer close(p.events)
	for {
		select {
		case <-p.quit:
			return
		default:
		}
		var m C.sona_msg
		kind := C.sona_gst_pop(p.p, C.int64_t(200*time.Millisecond), &m)
		var text string
		if m.text != nil {
			text = C.GoString(m.text)
			if m.debug != nil {
				text += " (" + C.GoString(m.debug) + ")"
			}
			C.sona_gst_free(unsafe.Pointer(m.text))
			C.sona_gst_free(unsafe.Pointer(m.debug))
		}
		if m.lane >= 0 {
			p.laneMessage(int(m.lane), kind, text)
			continue
		}
		if kind != C.SONA_MSG_NONE && kind != C.SONA_MSG_WAKE && uint64(m.build) < p.build.Load() {
			continue // from an output that was replaced
		}
		switch kind {
		case C.SONA_MSG_NONE, C.SONA_MSG_WAKE:
			p.checkStall()
		case C.SONA_MSG_EOS:
			p.send(Event{Kind: EventEOS})
		case C.SONA_MSG_ERROR:
			p.outputFailed(errors.New("gst: " + text))
		case C.SONA_MSG_WARNING:
			p.send(Event{Kind: EventWarning, Err: errors.New("gst: " + text)})
		case C.SONA_MSG_RELINK:
			// PipeWire took the stream's buffers away to relink it (default
			// device change, its sink removed). pipewiresink reports that as
			// fatal; it is not, but its clock may then jump, which makes it
			// drop audio. A new output links afresh at the same position.
			p.send(Event{Kind: EventWarning, Err: errors.New("gst: the sound server relinked the output stream; reconnecting")})
			if err := p.rebuild(nil); err != nil {
				p.outputFailed(err)
			}
		case C.SONA_MSG_STATE:
			if State(m.b) == StatePlaying {
				p.played.Store(true)
			}
			p.send(Event{Kind: EventState, State: State(m.b)})
		case C.SONA_MSG_ASYNC_DONE:
			p.send(Event{Kind: EventAsyncDone})
		case C.SONA_MSG_LATENCY:
			if p.use() {
				C.sona_gst_out_recalc_latency(p.p)
				p.life.RUnlock()
			}
		case C.SONA_MSG_CLOCK_LOST:
			if p.use() {
				C.sona_gst_out_clock_lost(p.p)
				p.life.RUnlock()
			}
		}
	}
}

// laneMessage handles a message of lane i.
func (p *Pipeline) laneMessage(i int, kind C.int, text string) {
	l := p.lanes[i]
	switch kind {
	case C.SONA_MSG_ERROR:
		gen := l.gen.Load()
		if p.use() {
			C.sona_lane_reset(l.l)
			p.life.RUnlock()
		}
		p.send(Event{Kind: EventError, Gen: gen, Err: errors.New("gst: " + text)})
	case C.SONA_MSG_WARNING:
		p.send(Event{Kind: EventWarning, Err: errors.New("gst: " + text)})
	}
}

// outputFailed rebuilds the output after a sink error; repeated failures
// are reported as fatal.
func (p *Pipeline) outputFailed(cause error) {
	now := time.Now()
	p.mu.Lock()
	recent := p.rebuilds[:0]
	for _, t := range p.rebuilds {
		if now.Sub(t) < rebuildWindow {
			recent = append(recent, t)
		}
	}
	p.rebuilds = recent
	spent := len(recent) >= maxRebuilds
	if !spent {
		p.rebuilds = append(p.rebuilds, now)
	}
	p.mu.Unlock()
	if !spent {
		p.send(Event{Kind: EventWarning, Err: fmt.Errorf("%w; rebuilding the output", cause)})
		if err := p.rebuild(nil); err == nil {
			return
		} else {
			cause = errors.Join(cause, err)
		}
	}
	p.send(Event{Kind: EventError, Err: cause})
}

// checkStall rebuilds the output when it should play and has audio
// queued but its position has not moved for stallAfter (a sink that lost
// its device and never got one back), or when it races ahead of real time
// (a sink whose clock jumped, dropping audio). Called from watch, at least
// every 200 ms.
func (p *Pipeline) checkStall() {
	p.mu.Lock()
	want := p.want
	p.mu.Unlock()
	now := time.Now()
	pos, ok := p.outPosition()
	queued := time.Duration(0)
	if p.use() {
		queued = time.Duration(C.sona_gst_out_queued_ns(p.p))
		p.life.RUnlock()
	}
	prevPos, prevAt := p.lastPos, p.lastAt
	p.lastPos, p.lastAt = pos, now
	if want == StatePlaying && ok && !prevAt.IsZero() && pos-prevPos > now.Sub(prevAt)+raceSlack {
		p.stallPos, p.stallAt = pos, now
		p.outputFailed(fmt.Errorf("gst: the output skipped %v", (pos - prevPos - now.Sub(prevAt)).Round(time.Millisecond)))
		return
	}
	if want != StatePlaying || !ok || pos != p.stallPos || queued == 0 || p.stallAt.IsZero() {
		p.stallPos, p.stallAt = pos, now
		return
	}
	if now.Sub(p.stallAt) >= stallAfter {
		p.stallAt = now
		p.outputFailed(errors.New("gst: the output stopped playing"))
	}
}

// rebuild replaces the output (props nil keeps the sink's properties) and
// restores the wanted state; the mixer carries on into the new one.
func (p *Pipeline) rebuild(props *string) error {
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	var cprops *C.char
	if props != nil {
		cprops = C.CString(*props)
		defer C.free(unsafe.Pointer(cprops))
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.played.Store(false)
	var buf [256]C.char
	b := C.sona_gst_out_rebuild(p.p, cprops, C.int(p.outState(p.want)), &buf[0], C.size_t(len(buf)))
	if b == 0 {
		return fmt.Errorf("gst: rebuild output: %s", C.GoString(&buf[0]))
	}
	p.build.Store(uint64(b))
	return nil
}

// SetDevice moves the output to a PipeWire sink (node.name; "" follows
// the default) by rebuilding the output in place: playback continues at
// its position.
func (p *Pipeline) SetDevice(device string) error {
	p.mu.Lock()
	p.opts.Device = device
	p.mu.Unlock()
	props := streamProps(p.sink, p.opts.AppName, device)
	return p.rebuild(&props)
}

// outState is the state the output is set to for the wanted state s.
func (p *Pipeline) outState(s State) State {
	if s == StatePaused && !p.played.Load() {
		return StateReady // see played
	}
	return s
}

func (p *Pipeline) setState(s State) error {
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	p.mu.Lock()
	p.want = s
	out := p.outState(s)
	p.mu.Unlock()
	if r := C.sona_gst_set_state(p.p, C.int(out)); r == 0 /* GST_STATE_CHANGE_FAILURE */ {
		return fmt.Errorf("state change to %d failed", int(s))
	}
	return nil
}

// Play, Pause and Ready change the output state. Play and Pause complete
// asynchronously once the sink has prerolled.
func (p *Pipeline) Play() error  { return p.setState(StatePlaying) }
func (p *Pipeline) Pause() error { return p.setState(StatePaused) }
func (p *Pipeline) Ready() error { return p.setState(StateReady) }

// State returns the output's current and pending state without waiting.
func (p *Pipeline) State() (cur, pending State) {
	var c, pend C.int
	if !p.use() {
		return StateNull, StateNull
	}
	defer p.life.RUnlock()
	C.sona_gst_get_state(p.p, 0, &c, &pend)
	return State(c), State(pend)
}

// frames converts a duration to the nearest frame at Rate.
func frames(d time.Duration) int64 {
	return (int64(d)*Rate + int64(time.Second)/2) / int64(time.Second)
}

// startTime and stopTime are segment bounds that GStreamer's sample
// clipping turns into exactly frame f: it drops whole frames before the
// start and after the stop, with buffer times rounded down to nanoseconds,
// so the start is rounded up and the stop moved half a frame early.
func startTime(f int64) C.int64_t { return C.int64_t((f*int64(time.Second) + Rate - 1) / Rate) }

func stopTime(f int64) C.int64_t {
	return C.int64_t((f*int64(time.Second) - int64(time.Second)/2) / Rate)
}

// duration converts frames at Rate to a duration.
func duration(f int64) time.Duration { return time.Duration(f * int64(time.Second) / Rate) }

// laneOf returns the lane fed with track gen.
func (p *Pipeline) laneOf(gen uint64) *lane {
	for _, l := range p.lanes {
		if l.gen.Load() == gen {
			return l
		}
	}
	return nil
}

// flushLane points l at track t (gen 0: idle); with mu held.
func (p *Pipeline) flushLane(l *lane, t *Track) {
	l.release()
	if t == nil {
		l.gen.Store(0)
		C.sona_lane_flush(l.l, 0, 0, -1, nil, 0, 0, 0)
		return
	}
	l.gen.Store(t.Gen)
	l.expect = t.Gen
	var asc *C.uint8_t
	if len(t.Caps.AudioConfig) > 0 {
		asc = (*C.uint8_t)(unsafe.Pointer(&t.Caps.AudioConfig[0]))
	}
	stop := C.int64_t(-1)
	if t.Duration > 0 {
		stop = stopTime(frames(t.Priming) + frames(t.Duration))
	}
	C.sona_lane_flush(l.l, C.uint64_t(t.Gen), startTime(frames(t.Priming)+frames(t.Start)), stop,
		asc, C.size_t(len(t.Caps.AudioConfig)), C.int(t.Caps.Rate), C.int(t.Caps.Channels))
}

func newTrack(t *Track, l *lane) *track {
	tr := &track{gen: t.Gen, r: l, gain: float32(t.Gain), start: frames(t.Start)}
	if t.Duration > 0 {
		tr.length = max(frames(t.Duration)-tr.start, 0)
	}
	return tr
}

// Load drops everything queued or playing and starts track t: its
// generation becomes current and audio before t.Start is clipped.
func (p *Pipeline) Load(t Track) {
	if !p.use() {
		return
	}
	defer p.life.RUnlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.epoch++
	C.sona_gst_out_flush(p.p, C.uint64_t(p.epoch))
	p.flushLane(p.lanes[1], nil)
	p.flushLane(p.lanes[0], &t)
	p.mix.load(newTrack(&t, p.lanes[0]))
	p.cond.Broadcast()
}

// Queue makes t follow the current track: sample-exact when fade is 0,
// else crossfading over fade. A track queued earlier is replaced. It fails
// with ErrEnded once the current track has played out or started fading.
func (p *Pipeline) Queue(t Track, fade time.Duration) error {
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mix.cur == nil || p.mix.drained || p.mix.fading {
		return ErrEnded
	}
	l := p.lanes[0]
	if p.mix.cur.r == l {
		l = p.lanes[1]
	}
	p.mix.unqueue()
	p.flushLane(l, &t)
	if !p.mix.queue(newTrack(&t, l), frames(fade)) {
		p.flushLane(l, nil)
		return ErrEnded
	}
	p.cond.Broadcast()
	return nil
}

// Unqueue drops the queued track, unless it is already fading in.
func (p *Pipeline) Unqueue() {
	if !p.use() {
		return
	}
	defer p.life.RUnlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if n := p.mix.next; n != nil && !p.mix.fading {
		p.mix.unqueue()
		p.flushLane(n.r.(*lane), nil)
	}
}

// Stop drops everything and makes gen current with nothing to play.
func (p *Pipeline) Stop(gen uint64) {
	if !p.use() {
		return
	}
	defer p.life.RUnlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stop(gen)
}

// stop is Stop with mu held.
func (p *Pipeline) stop(gen uint64) {
	p.epoch++
	C.sona_gst_out_flush(p.p, C.uint64_t(p.epoch))
	p.flushLane(p.lanes[1], nil)
	l := p.lanes[0]
	l.release()
	l.gen.Store(gen)
	C.sona_lane_flush(l.l, C.uint64_t(gen), 0, -1, nil, 0, 0, 0)
	p.mix = mixer{trackGain: p.mix.trackGain}
}

// SetTrackGain turns the tracks' Sound Check gain on or off; the change
// ramps in over one buffer.
func (p *Pipeline) SetTrackGain(on bool) {
	p.mu.Lock()
	p.mix.trackGain = on
	p.mu.Unlock()
}

// TrackGain reports the current track's Sound Check gain (0 = unity) and
// whether it is applied.
func (p *Pipeline) TrackGain() (gain float64, on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mix.cur != nil {
		gain = float64(p.mix.cur.gain)
	}
	return gain, p.mix.trackGain
}

// run is the mixer: it fills output buffers from the lanes and pushes them,
// blocking while the output queue is full.
func (p *Pipeline) run() {
	defer p.wg.Done()
	for {
		p.mu.Lock()
		for !p.stopped && (p.mix.cur == nil || p.mix.drained) {
			p.cond.Wait()
		}
		if p.stopped {
			p.mu.Unlock()
			return
		}
		epoch := p.epoch
		var out C.sona_pcm
		C.sona_gst_out_alloc(p.p, chunkFrames, &out)
		dst := unsafe.Slice((*float32)(unsafe.Pointer(out.data)), chunkFrames*Channels)
		n, done := p.mix.fill(dst)
		pts := duration(p.mix.out)
		p.mix.commit(n)
		p.mu.Unlock()
		if n > 0 {
			C.sona_gst_out_push(p.p, C.uint64_t(epoch), &out, C.size_t(n), C.int64_t(pts.Nanoseconds()))
		} else {
			C.sona_pcm_release(&out)
		}
		if done {
			C.sona_gst_out_eos(p.p, C.uint64_t(epoch))
		}
	}
}

// lane is one decode lane and the mixer's reader of it.
type lane struct {
	l   *C.sona_lane
	gen atomic.Uint64 // the track generation pushes go to (0: idle)
	// Reader state, used with Pipeline.mu held.
	expect uint64 // the generation the mixer reads
	pcm    C.sona_pcm
	held   bool
	off    int // frames of pcm consumed
}

func (l *lane) release() {
	if l.held {
		C.sona_pcm_release(&l.pcm)
		l.held = false
	}
}

func (l *lane) peek(max int) ([]float32, bool) {
	for tries := 0; ; tries++ {
		if l.held && l.off < int(l.pcm.frames) {
			n := min(max, int(l.pcm.frames)-l.off)
			all := unsafe.Slice((*float32)(unsafe.Pointer(l.pcm.data)), int(l.pcm.frames)*Channels)
			return all[l.off*Channels : (l.off+n)*Channels], false
		}
		l.release()
		if tries > 8 {
			return nil, false
		}
		switch C.sona_lane_pull(l.l, C.int64_t(pullWait.Nanoseconds()), &l.pcm) {
		case C.SONA_PULL_OK:
			if uint64(l.pcm.gen) != l.expect {
				C.sona_pcm_release(&l.pcm) // decoded before the flush that made expect current
				continue
			}
			l.held, l.off = true, 0
		case C.SONA_PULL_EOS:
			return nil, uint64(l.pcm.gen) == l.expect
		default:
			return nil, false
		}
	}
}

func (l *lane) advance(n int) { l.off += n }

// Push queues the first n bytes of b as an access unit of track gen,
// blocking while its lane's queue is full. It always takes ownership of b.
func (p *Pipeline) Push(gen uint64, b Buffer, n int, pts, dur time.Duration) error {
	if n < 0 || n > len(b.Data) {
		b.Free()
		return fmt.Errorf("gst: push %d of %d bytes", n, len(b.Data))
	}
	if !p.use() {
		b.Free()
		return ErrClosed
	}
	defer p.life.RUnlock()
	l := p.laneOf(gen)
	if l == nil || gen == 0 {
		b.Free()
		return ErrStale
	}
	return pushErr(C.sona_lane_push(l.l, C.uint64_t(gen), b.ptr, C.size_t(n), C.int64_t(pts.Nanoseconds()), C.int64_t(dur.Nanoseconds())))
}

// EndOfStream marks the end of track gen's data. EventEOS follows once it
// has played out, unless another track follows it.
func (p *Pipeline) EndOfStream(gen uint64) error {
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	l := p.laneOf(gen)
	if l == nil || gen == 0 {
		return ErrStale
	}
	return pushErr(C.sona_lane_eos(l.l, C.uint64_t(gen)))
}

func pushErr(r C.int) error {
	switch r {
	case C.SONA_PUSH_OK:
		return nil
	case C.SONA_PUSH_STALE:
		return ErrStale
	case C.SONA_PUSH_FLUSHING:
		return ErrFlushing
	case C.SONA_PUSH_EOS:
		return ErrEOS
	}
	return ErrFlow
}

// Buffer is C memory for one access unit. Fill Data, then hand it to Push
// (which takes ownership) or Free it.
type Buffer struct {
	ptr  *C.uint8_t
	Data []byte
}

// Alloc returns an n-byte Buffer.
func Alloc(n int) Buffer {
	ptr := C.sona_gst_alloc(C.size_t(n))
	return Buffer{ptr: ptr, Data: unsafe.Slice((*byte)(unsafe.Pointer(ptr)), n)}
}

// Free releases a Buffer that was not pushed.
func (b Buffer) Free() { C.sona_gst_free(unsafe.Pointer(b.ptr)) }

// outPosition is the output's stream time.
func (p *Pipeline) outPosition() (time.Duration, bool) {
	if !p.use() {
		return 0, false
	}
	defer p.life.RUnlock()
	var ns C.int64_t
	if C.sona_gst_position(p.p, &ns) == 0 {
		return 0, false
	}
	return time.Duration(ns), true
}

// Position returns the track being heard and its position, if known.
func (p *Pipeline) Position() (gen uint64, pos time.Duration, ok bool) {
	t, ok := p.outPosition()
	if !ok {
		return 0, 0, false
	}
	f := frames(t)
	p.mu.Lock()
	defer p.mu.Unlock()
	if f > p.mix.out {
		// Not played yet: a position the output reports from before the
		// last Load, until the new audio reaches it.
		return 0, 0, false
	}
	gen, at, ok := p.mix.at(f)
	if !ok {
		return 0, 0, false
	}
	p.mix.prune(f)
	return gen, duration(at), true
}

// Queued returns the duration of track gen's access units waiting in its
// lane.
func (p *Pipeline) Queued(gen uint64) time.Duration {
	if !p.use() {
		return 0
	}
	defer p.life.RUnlock()
	l := p.laneOf(gen)
	if l == nil {
		return 0
	}
	return time.Duration(C.sona_lane_queued_ns(l.l))
}

// Close stops and frees the pipeline. It waits for calls in flight, first
// waking pushes blocked on a full queue; later calls return ErrClosed.
func (p *Pipeline) Close() {
	p.once.Do(func() {
		p.mu.Lock()
		p.stopped = true
		p.stop(0) // no track is current: pushes return
		p.cond.Broadcast()
		p.mu.Unlock()
		p.life.Lock()
		p.closed = true
		p.life.Unlock()
		close(p.quit)
		C.sona_gst_wake(p.p)
		go func() {
			for range p.events {
			}
		}()
		p.wg.Wait()
		for _, l := range p.lanes {
			l.release()
			C.sona_lane_free(l.l)
		}
		C.sona_gst_free_pipeline(p.p)
	})
}

// Info describes the output stack for Diagnostics.
type Info struct {
	Version string
	Decoder string
	Sink    string
}

// Probe builds a default pipeline to report the GStreamer version and the
// decoder and sink Sona would use, then tears it down.
func Probe() (Info, error) {
	info := Info{Version: Version()}
	p, err := New(Options{})
	if err != nil {
		return info, err
	}
	defer p.Close()
	info.Decoder, info.Sink = p.decoder, p.sink
	return info, nil
}
