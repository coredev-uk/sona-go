//go:build linux || (cgo && (darwin || windows))

package gst

/*
#include <stdlib.h>
#include "filehead.h"
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

// FilePipeline plays a plain audio file (MP3, AAC or ALAC; a library
// upload) whose bytes are fed in order as they arrive:
//
//	source (bytes) ! decodebin ! <output tail: equaliser, volume> ! sink
//
// The whole file is kept in memory (never on disk); the demuxer reads any
// range of it, waiting for bytes not yet fed. The sink gets no audio before
// the first Play (see filehead.c's gate_probe), so Pause reaches PAUSED once
// the decoder is set up (EventAsyncDone): the duration is known and Seek
// works from then on.
//
// A live pipeline (size Live: a broadcaster's stream) has no length and no
// duration, cannot seek, and keeps only the bytes not yet decoded.
type FilePipeline struct {
	f      *C.sona_file
	sink   string
	events chan Event
	quit   chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
	life   sync.RWMutex // as Pipeline.life
	closed bool

	// mu serialises output moves and guards what the sink was set up for:
	// the chosen device, the app name and the resulting streamProps.
	mu      sync.Mutex
	device  string
	appName string
	route   string
	// routeSeen is the routeEpoch watch saw last; routeErr carries a failed
	// move to watch, which reports it.
	routeSeen uint64
	routeErr  chan error
}

// Live is the size NewFile takes for a live stream.
const Live int64 = -1

// NewFile builds a file pipeline for size bytes (Live for a live stream)
// and brings it to READY. o.Decoder and o.MaxQueue are ignored (decodebin
// picks the decoder).
func NewFile(o Options, size int64) (*FilePipeline, error) {
	if err := Init(); err != nil {
		return nil, err
	}
	if size == 0 || size < Live {
		return nil, errors.New("gst: empty file")
	}
	if o.AppName == "" {
		o.AppName = "Sona"
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
		p, err := buildFile(s, o, size)
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

func buildFile(sink string, o Options, size int64) (*FilePipeline, error) {
	seen := routeEpoch()
	props := streamProps(sink, o.AppName, o.Device)
	csink, capp, cprops := C.CString(sink), C.CString(o.AppName), C.CString(props)
	defer func() {
		for _, s := range []*C.char{csink, capp, cprops} {
			C.free(unsafe.Pointer(s))
		}
	}()
	var buf [256]C.char
	live := C.int(0)
	if size == Live {
		live, size = 1, 0
	}
	f := C.sona_file_new(csink, capp, cprops, C.uint64_t(size), live, &buf[0], C.size_t(len(buf)))
	if f == nil {
		return nil, fmt.Errorf("gst: %s", C.GoString(&buf[0]))
	}
	p := &FilePipeline{
		f: f, sink: sink, events: make(chan Event, 64), quit: make(chan struct{}),
		device: o.Device, appName: o.AppName, route: props, routeSeen: seen, routeErr: make(chan error, 1),
	}
	if C.sona_file_set_state(f, C.int(StateReady)) == 0 {
		C.sona_file_free(f)
		return nil, fmt.Errorf("gst: %s: state change to READY failed", sink)
	}
	p.wg.Add(1)
	go p.watch()
	return p, nil
}

func (p *FilePipeline) use() bool {
	p.life.RLock()
	if p.closed {
		p.life.RUnlock()
		return false
	}
	return true
}

// Sink names the sink in use.
func (p *FilePipeline) Sink() string { return p.sink }

// Events delivers bus events until Close (as Pipeline.Events).
// EventAsyncDone follows the first Pause once the decoder is set up.
func (p *FilePipeline) Events() <-chan Event { return p.events }

func (p *FilePipeline) watch() {
	defer p.wg.Done()
	defer close(p.events)
	for {
		select {
		case <-p.quit:
			return
		default:
		}
		if e := routeEpoch(); e != p.routeSeen {
			// A move may wait for a gap between buffers: off this loop.
			p.routeSeen = e
			go p.followRoute()
		}
		select {
		case err := <-p.routeErr:
			select {
			case p.events <- Event{Kind: EventWarning, Err: err}:
			default: // warnings drop when the receiver lags
			}
		default:
		}
		var m C.sona_file_msg
		kind := C.sona_file_pop(p.f, C.int64_t(200*time.Millisecond), &m)
		var ev Event
		switch kind {
		case C.SONA_FILE_MSG_EOS:
			ev = Event{Kind: EventEOS}
		case C.SONA_FILE_MSG_ERROR, C.SONA_FILE_MSG_WARNING:
			text := C.GoString(m.text)
			C.sona_file_free_text(m.text)
			ev = Event{Kind: EventError, Err: errors.New("gst: " + text)}
			if kind == C.SONA_FILE_MSG_WARNING {
				ev.Kind = EventWarning
			}
		case C.SONA_FILE_MSG_STATE:
			ev = Event{Kind: EventState, State: State(m.b)}
		case C.SONA_FILE_MSG_ASYNC_DONE:
			ev = Event{Kind: EventAsyncDone}
		default:
			continue
		}
		select {
		case p.events <- ev:
		default:
			if ev.Kind != EventWarning {
				p.events <- ev
			}
		}
	}
}

func (p *FilePipeline) setState(s State) error {
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	if C.sona_file_set_state(p.f, C.int(s)) == 0 {
		return fmt.Errorf("gst: state change to %d failed", int(s))
	}
	return nil
}

// Play and Pause change the state. The first Play lets audio through to
// the sink.
func (p *FilePipeline) Play() error  { return p.setState(StatePlaying) }
func (p *FilePipeline) Pause() error { return p.setState(StatePaused) }

// State returns the current and pending state without waiting.
func (p *FilePipeline) State() (cur, pending State) {
	var c, pend C.int
	if !p.use() {
		return StateNull, StateNull
	}
	defer p.life.RUnlock()
	C.sona_file_get_state(p.f, &c, &pend)
	return State(c), State(pend)
}

// Feed appends data at off, which must be where the previous Feed ended.
func (p *FilePipeline) Feed(off int64, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	if C.sona_file_write(p.f, C.uint64_t(off), (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data))) != 0 {
		return fmt.Errorf("gst: file bytes at %d out of order, past the end, or (live) too far ahead of playback", off)
	}
	return nil
}

// Complete marks the file fully fed.
func (p *FilePipeline) Complete() {
	if p.use() {
		C.sona_file_complete(p.f)
		p.life.RUnlock()
	}
}

// Level reports the bytes fed, the file size (0 when live) and the
// demuxer's read offset.
func (p *FilePipeline) Level() (have, size, pos int64) {
	var h, s, r C.uint64_t
	if p.use() {
		C.sona_file_level(p.f, &h, &s, &r)
		p.life.RUnlock()
	}
	return int64(h), int64(s), int64(r)
}

// Seek moves playback to pos (flushing, sample-accurate). The pipeline
// must be PAUSED or PLAYING.
func (p *FilePipeline) Seek(pos time.Duration) error {
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	if C.sona_file_seek(p.f, C.int64_t(pos.Nanoseconds())) != 0 {
		return fmt.Errorf("gst: seek to %v failed", pos)
	}
	return nil
}

// Position returns the position being played, if known.
func (p *FilePipeline) Position() (time.Duration, bool) {
	var ns C.int64_t
	if !p.use() {
		return 0, false
	}
	defer p.life.RUnlock()
	if C.sona_file_position(p.f, &ns) == 0 {
		return 0, false
	}
	return time.Duration(ns), true
}

// Duration returns the file's length once the demuxer knows it.
func (p *FilePipeline) Duration() (time.Duration, bool) {
	var ns C.int64_t
	if !p.use() {
		return 0, false
	}
	defer p.life.RUnlock()
	if C.sona_file_duration(p.f, &ns) == 0 {
		return 0, false
	}
	return time.Duration(ns), true
}

// SetVolume sets the linear volume.
func (p *FilePipeline) SetVolume(v float64) {
	if p.use() {
		C.sona_tail_set_volume(C.sona_file_tail(p.f), C.double(v))
		p.life.RUnlock()
	}
}

// SetEqualizer sets the pregain and band gains (dB), as Pipeline.SetEqualizer.
func (p *FilePipeline) SetEqualizer(pregain float64, bands [EQBands]float64) {
	var cb [EQBands]C.double
	for i, b := range bands {
		cb[i] = C.double(b)
	}
	if p.use() {
		C.sona_tail_set_eq(C.sona_file_tail(p.f), C.double(pregain), &cb[0])
		p.life.RUnlock()
	}
}

// Output reads the live output's settings back, as Pipeline.Output.
func (p *FilePipeline) Output() (OutputSettings, bool) {
	if !p.use() {
		return OutputSettings{}, false
	}
	defer p.life.RUnlock()
	return readTail(C.sona_file_tail(p.f))
}

// Close stops and frees the pipeline; later calls return ErrClosed.
func (p *FilePipeline) Close() {
	p.once.Do(func() {
		p.life.Lock()
		p.closed = true
		p.life.Unlock()
		close(p.quit)
		C.sona_file_wake(p.f)
		go func() {
			for range p.events {
			}
		}()
		p.wg.Wait()
		C.sona_file_free(p.f)
	})
}

// SetDevice moves output to device (Options.Device; "" follows the default)
// by replacing the sink; decoding and the position carry on.
func (p *FilePipeline) SetDevice(device, appName string) error {
	if appName == "" {
		appName = "Sona"
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.device, p.appName = device, appName
	return p.moveOutput()
}

// followRoute moves the output when the device it should play to is not
// the one its sink was set up for (as Pipeline.followRoute).
func (p *FilePipeline) followRoute() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if streamProps(p.sink, p.appName, p.device) == p.route {
		return
	}
	if err := p.moveOutput(); err != nil && !errors.Is(err, ErrClosed) {
		select {
		case p.routeErr <- err:
		default: // one unreported failure is enough
		}
	}
}

// moveOutput replaces the sink with one for the device to play to now;
// with mu held.
func (p *FilePipeline) moveOutput() error {
	props := streamProps(p.sink, p.appName, p.device)
	cprops := C.CString(props)
	defer C.free(unsafe.Pointer(cprops))
	if !p.use() {
		return ErrClosed
	}
	defer p.life.RUnlock()
	if C.sona_file_set_sink_props(p.f, cprops) != 0 {
		return fmt.Errorf("gst: moving the output to %q failed", p.device)
	}
	p.route = props
	return nil
}
