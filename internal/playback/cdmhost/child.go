//go:build linux || (darwin && cgo)

package cdmhost

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/coredev-uk/sona-go/internal/playback/cdm"
	"github.com/coredev-uk/sona-go/internal/playback/equalizer"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/gst"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
)

// ChildMain turns the process into the CDM host when EnvChild is set: it
// enters the sandbox (on Linux this re-executes the process once, see
// enterSandbox), serves the parent on stdin/stdout and exits when stdin
// closes. Otherwise it returns immediately. Call it first thing in main.
func ChildMain() {
	if os.Getenv(EnvChild) != "1" {
		return
	}
	// Before the CDM or GStreamer load.
	sb := enterSandbox()
	os.Exit(runChild(sb))
}

func runChild(sb Sandbox) int {
	// The protocol owns the original stdout; anything else writing to fd 1
	// (GStreamer, the CDM) is redirected to stderr so it cannot corrupt it.
	fd, err := unix.Dup(1)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cdmhost: dup stdout:", err)
		return 1
	}
	if err := unix.Dup2(2, 1); err != nil {
		fmt.Fprintln(os.Stderr, "cdmhost: redirect stdout:", err)
		return 1
	}
	out := os.NewFile(uintptr(fd), "cdmhost-protocol")
	var level slog.Level
	_ = level.UnmarshalText([]byte(os.Getenv(envLogLevel)))
	s := &server{
		log:     slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})),
		enc:     gob.NewEncoder(out),
		feeds:   make(chan request, 8),
		volume:  1,
		quit:    make(chan struct{}),
		sandbox: sb,
		cdmOnly: os.Getenv(envCDMOnly) == "1",
	}
	if s.sandbox.Reason != "" {
		s.log.Warn("cdmhost: sandbox incomplete", "reason", s.sandbox.Reason)
	}
	go s.feeder()
	go s.monitor()
	dec := gob.NewDecoder(os.Stdin)
	for {
		var r request
		if err := dec.Decode(&r); err != nil {
			if !errors.Is(err, io.EOF) {
				s.log.Error("cdmhost: read request", "err", err)
			}
			break
		}
		if r.Op == opFeed {
			s.feeds <- r
			continue
		}
		s.reply(s.handle(r))
	}
	close(s.quit)
	s.shutdown()
	return 0
}

// server is the child's state. mu guards the playback fields; the CDM and
// pipeline are safe for concurrent use.
type server struct {
	log   *slog.Logger
	encMu sync.Mutex
	enc   *gob.Encoder
	feeds chan request
	quit  chan struct{}

	cdm     *widevine.CDM
	info    Info
	sandbox Sandbox // the child's part of the sandbox (sandbox_<os>.go)
	cdmOnly bool    // loads the CDM and nothing else (envCDMOnly)

	mu        sync.Mutex
	sink      string
	pipe      *gst.Pipeline
	file      *gst.FilePipeline // the current library upload, if one plays (child_file.go)
	fs        fileState
	track     uint64
	init      *fmp4.Init
	start     time.Duration // media time feeding starts from (priming + position)
	duration  time.Duration // the current track's duration (Media.Duration)
	next      nextTrack     // the track queued to follow (opNext)
	volume    float64
	eq        equalizer.Curve // applied to every pipeline
	device    string          // PipeWire sink for every pipeline; "" = default
	trackGain bool            // apply the track's Sound Check gain
	gain      float64         // the current track's Sound Check gain (0 = unity)
	playing   bool            // wanted state
	started   bool            // audio advancing since the last Load/Play
	prev      time.Duration
	havePrev  bool
	ended     bool // EndOfTrack received for track
	buffering bool
	underrun  time.Time
	lastPos   time.Time
}

// nextTrack is the track queued to follow the current one; gen 0 for none.
type nextTrack struct {
	gen   uint64
	init  *fmp4.Init
	start time.Duration // media time feeding starts from (its priming)
	dur   time.Duration
	gain  float64
	ended bool // EndOfTrack received
}

func (s *server) send(f frame) {
	s.encMu.Lock()
	defer s.encMu.Unlock()
	if err := s.enc.Encode(f); err != nil {
		s.log.Error("cdmhost: write", "err", err)
	}
}

func (s *server) reply(r reply) { s.send(frame{Reply: &r}) }
func (s *server) emit(ev Event) { s.send(frame{Event: &ev}) }

func fail(id uint64, err error) reply {
	r := reply{ID: id, Err: err.Error()}
	switch {
	case errors.Is(err, ErrStale), errors.Is(err, gst.ErrStale), errors.Is(err, gst.ErrFlushing), errors.Is(err, gst.ErrClosed):
		r.Code = codeStale
	case errors.Is(err, widevine.ErrNoUsable):
		r.Code = codeNoUsableKey
	case errors.Is(err, ErrEnded), errors.Is(err, gst.ErrEnded):
		r.Code = codeEnded
	case errors.Is(err, ErrSandbox):
		r.Code = codeSandbox
	case errors.Is(err, ErrCDMLoad):
		r.Code = codeCDMLoad
	}
	return r
}

func (s *server) handle(r request) reply {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ok := reply{ID: r.ID}
	switch r.Op {
	case opInit:
		s.sink, s.device = r.Sink, r.Device
		if r.EQ != nil {
			s.eq = *r.EQ
		}
		info, err := s.open(r.CDMPath, r.Clear)
		if err != nil {
			rep := fail(r.ID, err)
			rep.Info = &info
			return rep
		}
		ok.Info = &info
	case opAudio:
		if err := gst.Init(); err != nil {
			return fail(r.ID, err)
		}
		ok.Info = &Info{PID: os.Getpid(), GStreamer: gst.Version(), Sandbox: s.sandbox}
	case opCert:
		if err := s.needCDM(); err != nil {
			return fail(r.ID, err)
		}
		if err := s.cdm.SetServerCertificate(ctx, r.Data); err != nil {
			return fail(r.ID, err)
		}
	case opSession:
		if err := s.needCDM(); err != nil {
			return fail(r.ID, err)
		}
		pssh, err := widevine.InitData(r.KID)
		if err != nil {
			return fail(r.ID, err)
		}
		id, challenge, err := s.cdm.CreateSession(ctx, pssh)
		if err != nil {
			return fail(r.ID, err)
		}
		ok.Session, ok.Challenge = id, challenge
	case opUpdate:
		if err := s.needCDM(); err != nil {
			return fail(r.ID, err)
		}
		if err := s.cdm.UpdateSession(ctx, r.Session, r.Data); err != nil {
			return fail(r.ID, err)
		}
		// The expiry is the licence's duration as the CDM enforces it, for
		// checking Apple's renewal policy.
		exp := "none"
		if t := s.cdm.Expiry(r.Session); !t.IsZero() {
			exp = t.Format(time.RFC3339)
		}
		s.log.Debug("cdmhost: licence installed", "session", r.Session, "expiry", exp)
	case opCloseSession:
		if err := s.needCDM(); err != nil {
			return fail(r.ID, err)
		}
		if err := s.cdm.CloseSession(ctx, r.Session); err != nil {
			return fail(r.ID, err)
		}
	case opLoad:
		if err := s.load(r); err != nil {
			return fail(r.ID, err)
		}
	case opLoadFile:
		if err := s.loadFile(r); err != nil {
			return fail(r.ID, err)
		}
	case opFileData:
		if err := s.feedFile(r); err != nil {
			return fail(r.ID, err)
		}
	case opFileEnd:
		if err := s.endFile(r); err != nil {
			return fail(r.ID, err)
		}
	case opNext:
		if err := s.queueNext(r); err != nil {
			return fail(r.ID, err)
		}
	case opEnd:
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case s.pipe == nil:
			return fail(r.ID, ErrStale)
		case r.Track == s.track:
			s.ended = true
		case r.Track == s.next.gen && r.Track != 0:
			s.next.ended = true
		default:
			return fail(r.ID, ErrStale)
		}
		if err := s.pipe.EndOfStream(r.Track); err != nil {
			return fail(r.ID, err)
		}
	case opPlay:
		s.mu.Lock()
		defer s.mu.Unlock()
		o := s.outLocked()
		if o == nil {
			return fail(r.ID, errors.New("cdmhost: nothing loaded"))
		}
		s.playing, s.started, s.havePrev, s.buffering = true, false, false, false
		if s.file != nil && !s.fs.prerolled {
			break // watchFile plays once prerolled
		}
		if err := o.Play(); err != nil {
			return fail(r.ID, err)
		}
	case opPause:
		s.mu.Lock()
		defer s.mu.Unlock()
		s.playing, s.buffering = false, false
		if o := s.outLocked(); o != nil {
			if err := o.Pause(); err != nil {
				return fail(r.ID, err)
			}
			ok.Position, ok.HasPos = s.positionLocked()
		}
	case opStop:
		s.mu.Lock()
		defer s.mu.Unlock()
		s.track, s.playing, s.started, s.ended, s.buffering = r.Track, false, false, false, false
		s.next = nextTrack{}
		s.closeFileLocked()
		if s.pipe != nil {
			s.pipe.Stop(r.Track)
			if err := s.pipe.Pause(); err != nil {
				return fail(r.ID, err)
			}
		}
	case opVolume:
		s.mu.Lock()
		defer s.mu.Unlock()
		s.volume = r.Volume
		s.applyVolumeLocked()
	case opPosition:
		s.mu.Lock()
		defer s.mu.Unlock()
		ok.Position, ok.HasPos = s.positionLocked()
	case opEqualizer:
		if r.EQ == nil {
			return fail(r.ID, errors.New("cdmhost: equalizer request without settings"))
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		s.eq = *r.EQ
		if s.pipe != nil {
			s.pipe.SetEqualizer(s.eq.Pregain, s.eq.Gains)
		}
		if s.file != nil {
			s.file.SetEqualizer(s.eq.Pregain, s.eq.Gains)
		}
	case opTrackGain:
		s.mu.Lock()
		defer s.mu.Unlock()
		s.trackGain = r.Enabled
		if s.pipe != nil {
			s.pipe.SetTrackGain(r.Enabled)
		}
		s.applyVolumeLocked()
	case opDevice:
		s.mu.Lock()
		defer s.mu.Unlock()
		s.device = r.Device // later pipelines play there too
		if s.pipe != nil {
			// The output moves at once; decoding and the position carry on.
			if err := s.pipe.SetDevice(r.Device); err != nil {
				return fail(r.ID, err)
			}
		}
		if s.file != nil {
			if err := s.file.SetDevice(r.Device, ""); err != nil {
				return fail(r.ID, err)
			}
		}
	default:
		return fail(r.ID, fmt.Errorf("cdmhost: unknown op %q", r.Op))
	}
	return ok
}

func (s *server) needCDM() error {
	if s.cdm == nil {
		return errors.New("cdmhost: CDM not initialised")
	}
	return nil
}

// open loads the CDM library at lib (the parent chose it: the downloaded
// CDM, internal/playback/cdm) and builds the output pipeline, unless the
// child only loads the CDM (cdmOnly). A clear host loads no CDM: it plays
// plain audio only (previews, uploads), and every CDM request fails. On
// Linux the CDM is loaded only under the seccomp filter that keeps it off
// the network (ErrSandbox; Sandbox.loadsCDM). A library that is missing
// or fails to load or initialise is ErrCDMLoad.
func (s *server) open(lib string, clear bool) (Info, error) {
	info := Info{PID: os.Getpid(), Sandbox: s.sandbox}
	if !s.cdmOnly {
		info.GStreamer = gst.Version()
	}
	if s.cdm != nil {
		return s.info, nil
	}
	if !clear {
		if lib == "" {
			return info, fmt.Errorf("%w: no CDM library given", ErrCDMLoad)
		}
		if st, err := os.Stat(lib); err != nil || !st.Mode().IsRegular() {
			return info, fmt.Errorf("%w: %s is not a readable file", ErrCDMLoad, lib)
		}
		info.CDMPath, info.CDMVersion = lib, cdm.LibraryVersion(lib)
		if !s.sandbox.loadsCDM() {
			return info, fmt.Errorf("%w: %s", ErrSandbox, s.sandbox.Reason)
		}
		c, err := widevine.Open(lib, s.log)
		if err != nil {
			return info, fmt.Errorf("%w: %w", ErrCDMLoad, err)
		}
		s.cdm = c
		info.CDMVersion, info.CDMInterface, info.HostInterface = c.Version(), c.Interface(), c.HostInterface()
		go func() {
			for m := range c.Messages() {
				s.emit(Event{Kind: EventMessage, Session: m.SessionID, Type: int(m.Type), Data: m.Data})
			}
		}()
		if s.cdmOnly {
			s.info = info
			return info, nil
		}
	}
	s.mu.Lock()
	err := s.ensurePipeline()
	if s.pipe != nil {
		info.Decoder, info.Sink = s.pipe.Decoder(), s.pipe.Sink()
	}
	s.mu.Unlock()
	s.info = info
	return info, err
}

// ensurePipeline builds the pipeline if there is none. Call with mu held.
func (s *server) ensurePipeline() error {
	if s.pipe != nil {
		return nil
	}
	if s.cdmOnly {
		return errors.New("cdmhost: this child only loads the CDM; it has no audio pipeline")
	}
	p, err := gst.New(gst.Options{Sink: s.sink, Device: s.device})
	if err != nil {
		return err
	}
	p.SetEqualizer(s.eq.Pregain, s.eq.Gains)
	p.SetTrackGain(s.trackGain)
	s.pipe = p
	s.applyVolumeLocked()
	go s.watch(p)
	return nil
}

// applyVolumeLocked sets the output level: the user's volume, times the
// upload's Sound Check gain while that is on (the CDM pipeline's mixer
// applies each track's gain itself). Call with mu held.
func (s *server) applyVolumeLocked() {
	if s.pipe != nil {
		s.pipe.SetVolume(s.volume)
	}
	if s.file != nil {
		v := s.volume
		if s.trackGain && s.gain > 0 {
			v *= s.gain
		}
		s.file.SetVolume(v)
	}
}

// positionLocked is the current track's position. Call with mu held.
func (s *server) positionLocked() (time.Duration, bool) {
	if s.file != nil {
		return s.file.Position()
	}
	if s.pipe == nil {
		return 0, false
	}
	gen, pos, ok := s.pipe.Position()
	if !ok || (gen != s.track && gen != s.next.gen) {
		return 0, false
	}
	return pos, true
}

// watch forwards pipeline events for p's lifetime.
func (s *server) watch(p *gst.Pipeline) {
	for ev := range p.Events() {
		switch ev.Kind {
		case gst.EventEOS:
			s.mu.Lock()
			track, ended := s.track, s.ended && s.pipe == p
			pos, _ := s.positionLocked()
			s.mu.Unlock()
			if ended {
				s.emit(Event{Kind: EventEnded, Track: track, Position: pos, At: time.Now()})
			}
		case gst.EventError:
			s.log.Error("cdmhost: pipeline error", "err", ev.Err, "track", ev.Gen)
			s.mu.Lock()
			track := s.track
			switch {
			case s.pipe != p:
			case ev.Gen != 0 && ev.Gen == s.next.gen:
				// The queued track failed to decode: the current one ends
				// as if nothing followed, and the parent plays on.
				track = ev.Gen
				s.next = nextTrack{}
				p.Unqueue()
			case ev.Gen == 0:
				// The output failed for good: rebuilt on the next Load.
				s.pipe, s.init = nil, nil
				go p.Close()
			}
			s.mu.Unlock()
			s.emit(Event{Kind: EventError, Track: track, Err: ev.Err.Error()})
		case gst.EventWarning:
			s.log.Warn("cdmhost: pipeline warning", "err", ev.Err)
		}
	}
}

// load starts a new track generation at r.Start, flushing whatever played
// or was queued.
func (s *server) load(r request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		if len(r.Data) == 0 {
			return s.seekFileLocked(r)
		}
		s.closeFileLocked()
	}
	in, dur, gain := s.init, s.duration, s.gain
	if len(r.Data) > 0 { // a new track; a seek keeps the track's
		var err error
		if in, err = fmp4.ParseInit(r.Data); err != nil {
			return err
		}
		dur, gain = r.Duration, r.Gain
	}
	if in == nil {
		return errors.New("cdmhost: load without init segment")
	}
	if err := checkScheme(in); err != nil {
		return err
	}
	if err := s.ensurePipeline(); err != nil {
		return err
	}
	t := gstTrack(r.Track, in, dur, gain)
	t.Start = r.Start
	s.track, s.init, s.start, s.duration, s.gain = r.Track, in, t.Priming+r.Start, dur, gain
	s.next = nextTrack{}
	s.started, s.havePrev, s.ended, s.buffering = false, false, false, false
	s.pipe.Load(t)
	if s.playing {
		return s.pipe.Play()
	}
	return s.pipe.Pause()
}

// gstTrack describes a track to the pipeline. Its priming is trimmed only
// with a known duration (a VOD track; live streams play every sample).
func gstTrack(gen uint64, in *fmp4.Init, dur time.Duration, gain float64) gst.Track {
	t := gst.Track{
		Gen:      gen,
		Caps:     gst.Caps{AudioConfig: in.AudioConfig, Rate: in.SampleRate, Channels: in.Channels},
		Duration: dur,
		Gain:     gain,
	}
	if dur > 0 {
		t.Priming = in.Priming()
	}
	return t
}

// queueNext queues r.Track to follow the current track (track 0: drop
// the queued one).
func (s *server) queueNext(r request) error {
	var in *fmp4.Init
	if r.Track != 0 {
		var err error
		if in, err = fmp4.ParseInit(r.Data); err != nil {
			return err
		}
		if err := checkScheme(in); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pipe == nil || s.file != nil {
		return ErrEnded
	}
	if r.Track == 0 {
		s.next = nextTrack{}
		s.pipe.Unqueue()
		return nil
	}
	t := gstTrack(r.Track, in, r.Duration, r.Gain)
	if err := s.pipe.Queue(t, r.Fade); err != nil {
		s.next = nextTrack{}
		return err
	}
	s.next = nextTrack{gen: r.Track, init: in, start: t.Priming, dur: r.Duration, gain: r.Gain}
	return nil
}

// feeder decrypts and queues fragments in arrival order.
func (s *server) feeder() {
	var scratch feedScratch
	for {
		select {
		case r := <-s.feeds:
			if err := s.feed(r, &scratch); err != nil {
				s.reply(fail(r.ID, err))
			} else {
				s.reply(reply{ID: r.ID})
			}
		case <-s.quit:
			return
		}
	}
}

// feedScratch is reused between fragments to avoid per-sample allocation.
type feedScratch struct {
	bufs []widevine.Buffer
	out  []pending
	ivs  [][16]byte
	subs []widevine.Subsample
}

// pending is one access unit on its way to the pipeline.
type pending struct {
	buf      gst.Buffer
	pts, dur time.Duration
	dec      int // index into feedScratch.bufs, -1 when the sample is clear
	nsub     int
}

// preroll is how much audio before the segment start is decoded and then
// clipped, so the decoder's overlap state is primed at the target.
const preroll = 50 * time.Millisecond

// checkScheme rejects encryption the feeder cannot decrypt.
func checkScheme(in *fmp4.Init) error {
	if in.Protected && in.Scheme != "cenc" && in.Scheme != "cbcs" {
		return fmt.Errorf("cdmhost: unsupported encryption scheme %q", in.Scheme)
	}
	return nil
}

// switchInit adopts a stream's new init segment (a live programme
// boundary) for the following fragments of the same track, without
// flushing what is queued. The audio configuration must not change.
func (s *server) switchInit(r request) error {
	in, err := fmp4.ParseInit(r.Init)
	if err != nil {
		return err
	}
	if err := checkScheme(in); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Track != s.track || s.init == nil {
		return ErrStale
	}
	if !bytes.Equal(in.AudioConfig, s.init.AudioConfig) {
		return errors.New("cdmhost: audio configuration changed mid-stream")
	}
	s.init = in
	return nil
}

// feed decrypts one fragment with the CDM straight into GStreamer buffers
// and pushes them, blocking while the pipeline queue is full. A stream
// fragment may carry a new init segment (applied first) and a Shift
// subtracted from its timestamps, which maps them onto the track timeline.
func (s *server) feed(r request, sc *feedScratch) error {
	if len(r.Init) > 0 {
		if err := s.switchInit(r); err != nil {
			return err
		}
	}
	s.mu.Lock()
	in, start, pipe := s.init, s.start, s.pipe
	switch {
	case r.Track == s.track:
	case r.Track == s.next.gen && r.Track != 0:
		in, start = s.next.init, s.next.start
	default:
		in = nil
	}
	s.mu.Unlock()
	if pipe == nil || in == nil {
		return ErrStale
	}
	samples, err := fmp4.ParseFragment(r.Data, in)
	if err != nil {
		return err
	}
	toDur := func(units int64) time.Duration { return fmp4.Time(units, in.Timescale) }
	cbcs := in.Scheme == "cbcs"
	crypt, skip := in.CryptByte, in.SkipByte
	if cbcs && crypt == 0 && skip == 0 {
		crypt = 1 // no pattern (tenc v0, typical for audio): every block is encrypted
	}
	kid := in.KID
	if len(r.KID) == 16 {
		kid = r.KID
	}
	sc.bufs, sc.out, sc.subs = sc.bufs[:0], sc.out[:0], sc.subs[:0]
	if cap(sc.ivs) < len(samples) {
		sc.ivs = make([][16]byte, len(samples))
	}
	for i := range samples {
		smp := &samples[i]
		pts, dur := toDur(smp.PTS())-r.Shift, toDur(int64(smp.Duration))
		if pts+dur < start-preroll || len(smp.Data) == 0 {
			continue
		}
		p := pending{buf: gst.Alloc(len(smp.Data)), pts: pts, dur: dur, dec: -1}
		if !smp.Encrypted() {
			copy(p.buf.Data, smp.Data)
		} else {
			for _, ss := range smp.Subsamples {
				sc.subs = append(sc.subs, widevine.Subsample{Clear: ss.Clear, Cipher: ss.Cipher})
			}
			p.dec, p.nsub = len(sc.bufs), len(smp.Subsamples)
			sc.bufs = append(sc.bufs, widevine.Buffer{
				KeyID:       kid,
				IV:          fmp4.PaddedIV(&sc.ivs[i], smp.IV),
				Data:        smp.Data,
				Out:         p.buf.Data,
				Timestamp:   pts,
				CBCS:        cbcs,
				CryptBlocks: crypt,
				SkipBlocks:  skip,
			})
		}
		sc.out = append(sc.out, p)
	}
	// Point each buffer at its run of the (now final) subsample table.
	off := 0
	for _, p := range sc.out {
		if p.dec >= 0 {
			sc.bufs[p.dec].Subsamples = sc.subs[off : off+p.nsub : off+p.nsub]
			off += p.nsub
		}
	}
	freeFrom := func(i int) {
		for _, p := range sc.out[i:] {
			p.buf.Free()
		}
	}
	if len(sc.bufs) > 0 {
		if s.cdm == nil {
			freeFrom(0)
			return errors.New("cdmhost: CDM not initialised")
		}
		if err := s.cdm.Decrypt(sc.bufs); err != nil {
			freeFrom(0)
			return err
		}
	}
	for i, p := range sc.out {
		n := len(p.buf.Data)
		if p.dec >= 0 {
			n = sc.bufs[p.dec].N
		}
		if err := pipe.Push(r.Track, p.buf, n, p.pts, p.dur); err != nil {
			freeFrom(i + 1)
			return err
		}
	}
	return nil
}

// monitor detects first audio, periodic position, and underruns.
func (s *server) monitor() {
	t := time.NewTicker(20 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			return
		case now := <-t.C:
			if ev, ok := s.tick(now); ok {
				s.emit(ev)
			}
		}
	}
}

const (
	positionEvery  = 5 * time.Second
	underrunAfter  = 100 * time.Millisecond
	resumeBuffered = 2 * time.Second
)

func (s *server) tick(now time.Time) (Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.outLocked()
	if !s.playing || o == nil {
		return Event{}, false
	}
	if s.file == nil && s.next.gen != 0 {
		if gen, pos, ok := s.pipe.Position(); ok && gen == s.next.gen {
			// The queued track is heard: it is current from here on.
			n := s.next
			s.track, s.init, s.start, s.duration, s.gain, s.ended = n.gen, n.init, n.start, n.dur, n.gain, n.ended
			s.next = nextTrack{}
			s.started, s.lastPos, s.underrun = true, now, time.Time{}
			return Event{Kind: EventAdvance, Track: n.gen, Position: pos, At: now}, true
		}
	}
	pos, ok := s.positionLocked()
	if !s.started {
		if !ok {
			return Event{}, false
		}
		if !s.havePrev {
			s.prev, s.havePrev = pos, true
			return Event{}, false
		}
		if pos <= s.prev {
			return Event{}, false
		}
		s.started, s.lastPos = true, now
		ev := Event{Kind: EventStarted, Track: s.track, Position: pos, At: now}
		if s.file != nil {
			ev.Duration, _ = s.file.Duration()
			s.fs.durSent = ev.Duration > 0
		}
		return ev, true
	}
	if s.file != nil && !s.fs.durSent && ok {
		// Some formats (MP3 without a header) know their length only
		// after a while: report it as soon as it is.
		if d, known := s.file.Duration(); known {
			s.fs.durSent, s.lastPos = true, now
			return Event{Kind: EventPosition, Track: s.track, Position: pos, At: now, Duration: d}, true
		}
	}
	var queued time.Duration
	if s.file != nil {
		queued = s.fileQueuedLocked()
	} else {
		queued = s.pipe.Queued(s.track)
	}
	switch {
	case s.buffering && (queued >= resumeBuffered || s.ended):
		s.buffering = false
		_ = o.Play()
		s.started, s.havePrev = false, false // re-anchor with EventStarted
		return Event{Kind: EventBuffering, Track: s.track, Position: pos, At: now}, true
	case !s.buffering && !s.ended && queued == 0:
		if s.underrun.IsZero() {
			s.underrun = now
		}
		if now.Sub(s.underrun) >= underrunAfter {
			s.buffering, s.underrun = true, time.Time{}
			_ = o.Pause()
			return Event{Kind: EventBuffering, Track: s.track, Position: pos, At: now, Buffering: true}, true
		}
	default:
		s.underrun = time.Time{}
	}
	if !s.buffering && ok && now.Sub(s.lastPos) >= positionEvery {
		s.lastPos = now
		return Event{Kind: EventPosition, Track: s.track, Position: pos, At: now}, true
	}
	return Event{}, false
}

func (s *server) shutdown() {
	s.mu.Lock()
	p := s.pipe
	s.pipe = nil
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	s.mu.Unlock()
	if p != nil {
		p.Close()
	}
	if s.cdm != nil {
		s.cdm.Close()
	}
}
