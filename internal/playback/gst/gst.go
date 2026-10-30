// Package gst is Sona's audio output: a GStreamer pipeline
//
//	appsrc (raw AAC + codec_data) ! avdec_aac ! audioconvert ! audioresample ! volume ! pipewiresink
//
// fed with PTS-stamped AAC access units. Pushes block while the appsrc queue
// is full (backpressure); Flush starts a new generation and segment so seeks
// and track changes clip precisely and never play superseded data.
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
	MaxQueue time.Duration // appsrc queue limit before Push blocks, default 12s
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
	EventEOS       EventKind = iota + 1 // the last buffer has been rendered
	EventError                          // fatal pipeline error
	EventWarning                        //
	EventState                          // pipeline state changed to Event.State
	EventAsyncDone                      // preroll after a state change or flush finished
)

// Event is a bus message.
type Event struct {
	Kind  EventKind
	State State
	Err   error
}

// Caps describes the AAC stream.
type Caps struct {
	AudioConfig []byte // AudioSpecificConfig (codec_data)
	Rate        int
	Channels    int
}

// Pipeline is one output pipeline.
type Pipeline struct {
	p       *C.sona_gst
	decoder string
	sink    string
	events  chan Event
	quit    chan struct{}
	wg      sync.WaitGroup
	once    sync.Once
	// life keeps the C pipeline alive while calls are in flight: methods
	// hold it shared, Close exclusively before freeing.
	life   sync.RWMutex
	closed bool
}

// ErrClosed reports a call on a closed Pipeline.
var ErrClosed = errors.New("gst: pipeline closed")

// use pins the pipeline for one call; release with p.life.RUnlock.
func (p *Pipeline) use() bool {
	p.life.RLock()
	if p.closed {
		p.life.RUnlock()
		return false
	}
	return true
}

// New builds a pipeline and brings it to READY (so the sink has connected
// to the sound server).
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
	sinks := Sinks
	if o.Sink != "" {
		sinks = []string{o.Sink}
	}
	var errs []error
	for _, s := range sinks {
		if o.Sink == "" && !Have(s) {
			continue
		}
		p, err := build(dec, s, o)
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
func streamProps(sink, app string) string {
	esc := strings.NewReplacer(`"`, ``, `\`, ``).Replace(app)
	switch sink {
	case "pipewiresink":
		return fmt.Sprintf(`props,media.role=(string)Music,media.category=(string)Playback,media.name=(string)"%s",application.name=(string)"%s",application.id=(string)sona,node.description=(string)"%s"`, esc, esc, esc)
	case "pulsesink":
		return fmt.Sprintf(`props,media.role=(string)music,application.name=(string)"%s",application.id=(string)sona`, esc)
	}
	return ""
}

func build(dec, sink string, o Options) (*Pipeline, error) {
	cdec, csink := C.CString(dec), C.CString(sink)
	capp, cprops := C.CString(o.AppName), C.CString(streamProps(sink, o.AppName))
	defer func() {
		for _, s := range []*C.char{cdec, csink, capp, cprops} {
			C.free(unsafe.Pointer(s))
		}
	}()
	var buf [256]C.char
	cp := C.sona_gst_new(cdec, csink, capp, cprops, C.uint64_t(o.MaxQueue.Nanoseconds()), &buf[0], C.size_t(len(buf)))
	if cp == nil {
		return nil, fmt.Errorf("gst: %s", C.GoString(&buf[0]))
	}
	p := &Pipeline{p: cp, decoder: dec, sink: sink, events: make(chan Event, 64), quit: make(chan struct{})}
	if err := p.setState(StateReady); err != nil {
		C.sona_gst_free_pipeline(cp)
		return nil, fmt.Errorf("gst: %s: %w", sink, err)
	}
	p.wg.Add(1)
	go p.watch()
	return p, nil
}

// Decoder and Sink name the elements in use.
func (p *Pipeline) Decoder() string { return p.decoder }
func (p *Pipeline) Sink() string    { return p.sink }

// Events delivers bus events until Close. Warnings are dropped when the
// receiver lags; other events are never dropped while capacity remains.
func (p *Pipeline) Events() <-chan Event { return p.events }

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
		var ev Event
		switch kind {
		case C.SONA_MSG_EOS:
			ev = Event{Kind: EventEOS}
		case C.SONA_MSG_ERROR, C.SONA_MSG_WARNING:
			text := C.GoString(m.text)
			if m.debug != nil {
				text += " (" + C.GoString(m.debug) + ")"
			}
			C.sona_gst_free(unsafe.Pointer(m.text))
			C.sona_gst_free(unsafe.Pointer(m.debug))
			ev = Event{Kind: EventError, Err: errors.New("gst: " + text)}
			if kind == C.SONA_MSG_WARNING {
				ev.Kind = EventWarning
			}
		case C.SONA_MSG_STATE:
			ev = Event{Kind: EventState, State: State(m.b)}
		case C.SONA_MSG_ASYNC_DONE:
			ev = Event{Kind: EventAsyncDone}
		default:
			continue
		}
		select {
		case p.events <- ev:
		default:
			if ev.Kind != EventWarning {
				p.events <- ev // block rather than lose EOS or errors
			}
		}
	}
}

func (p *Pipeline) setState(s State) error {
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	if r := C.sona_gst_set_state(p.p, C.int(s)); r == 0 /* GST_STATE_CHANGE_FAILURE */ {
		return fmt.Errorf("state change to %d failed", int(s))
	}
	return nil
}

// Play, Pause and Ready change the pipeline state. Play and Pause complete
// asynchronously once the sink has prerolled.
func (p *Pipeline) Play() error  { return p.setState(StatePlaying) }
func (p *Pipeline) Pause() error { return p.setState(StatePaused) }
func (p *Pipeline) Ready() error { return p.setState(StateReady) }

// State returns the current and pending pipeline state without waiting.
func (p *Pipeline) State() (cur, pending State) {
	var c, pend C.int
	if !p.use() {
		return StateNull, StateNull
	}
	defer p.life.RUnlock()
	C.sona_gst_get_state(p.p, 0, &c, &pend)
	return State(c), State(pend)
}

// Flush drops queued and playing audio and starts generation gen: the next
// Push of gen opens a segment at start, so samples before start are clipped.
// caps nil keeps the previous caps.
func (p *Pipeline) Flush(gen uint64, start time.Duration, caps *Caps) {
	var asc *C.uint8_t
	var ascLen C.size_t
	var rate, ch C.int
	if caps != nil && len(caps.AudioConfig) > 0 {
		asc = (*C.uint8_t)(unsafe.Pointer(&caps.AudioConfig[0]))
		ascLen = C.size_t(len(caps.AudioConfig))
		rate, ch = C.int(caps.Rate), C.int(caps.Channels)
	}
	if !p.use() {
		return
	}
	defer p.life.RUnlock()
	C.sona_gst_flush(p.p, C.uint64_t(gen), C.int64_t(start.Nanoseconds()), asc, ascLen, rate, ch)
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

// Push queues the first n bytes of b as an access unit of generation gen,
// blocking while the queue is full. It always takes ownership of b.
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
	return pushErr(C.sona_gst_push(p.p, C.uint64_t(gen), b.ptr, C.size_t(n), C.int64_t(pts.Nanoseconds()), C.int64_t(dur.Nanoseconds())))
}

// EndOfStream marks the end of generation gen's data; EventEOS follows once
// it has played out.
func (p *Pipeline) EndOfStream(gen uint64) error {
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	return pushErr(C.sona_gst_end_of_stream(p.p, C.uint64_t(gen)))
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

// SetVolume sets the linear volume (0..1; values up to 10 amplify).
func (p *Pipeline) SetVolume(v float64) {
	if p.use() {
		C.sona_gst_set_volume(p.p, C.double(v))
		p.life.RUnlock()
	}
}

// Position returns the stream time being played, if known.
func (p *Pipeline) Position() (time.Duration, bool) {
	var ns C.int64_t
	if !p.use() {
		return 0, false
	}
	defer p.life.RUnlock()
	if C.sona_gst_position(p.p, &ns) == 0 {
		return 0, false
	}
	return time.Duration(ns), true
}

// Queued returns the duration of data waiting in appsrc.
func (p *Pipeline) Queued() time.Duration {
	if !p.use() {
		return 0
	}
	defer p.life.RUnlock()
	return time.Duration(C.sona_gst_queued_ns(p.p))
}

// Close stops and frees the pipeline. It waits for calls in flight, first
// waking a Push blocked on a full queue; later calls return ErrClosed.
func (p *Pipeline) Close() {
	p.once.Do(func() {
		p.Flush(^uint64(0), 0, nil) // no generation matches: pushes return
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
