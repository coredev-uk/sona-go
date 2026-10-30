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
	"syscall"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/gst"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
)

// ChildMain turns the process into the CDM host when EnvChild is set: it
// serves the parent on stdin/stdout and exits when stdin closes. Otherwise it
// returns immediately. Call it first thing in main.
func ChildMain() {
	if os.Getenv(EnvChild) != "1" {
		return
	}
	os.Exit(runChild())
}

func runChild() int {
	// The protocol owns the original stdout; anything else writing to fd 1
	// (GStreamer, the CDM) is redirected to stderr so it cannot corrupt it.
	fd, err := syscall.Dup(1)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cdmhost: dup stdout:", err)
		return 1
	}
	if err := syscall.Dup3(2, 1, 0); err != nil {
		fmt.Fprintln(os.Stderr, "cdmhost: redirect stdout:", err)
		return 1
	}
	out := os.NewFile(uintptr(fd), "cdmhost-protocol")
	var level slog.Level
	_ = level.UnmarshalText([]byte(os.Getenv(envLogLevel)))
	s := &server{
		log:    slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})),
		enc:    gob.NewEncoder(out),
		feeds:  make(chan request, 8),
		volume: 1,
		quit:   make(chan struct{}),
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

	cdm  *widevine.CDM
	info Info

	mu        sync.Mutex
	sink      string
	pipe      *gst.Pipeline
	track     uint64
	init      *fmp4.Init
	start     time.Duration
	volume    float64
	playing   bool // wanted state
	started   bool // audio advancing since the last Load/Play
	prev      time.Duration
	havePrev  bool
	ended     bool // EndOfTrack received for track
	buffering bool
	underrun  time.Time
	lastPos   time.Time
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
	}
	return r
}

func (s *server) handle(r request) reply {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ok := reply{ID: r.ID}
	switch r.Op {
	case opInit:
		s.sink = r.Sink
		info, err := s.open(r.CDMPath)
		if err != nil {
			rep := fail(r.ID, err)
			rep.Info = &info
			return rep
		}
		ok.Info = &info
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
	case opEnd:
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Track != s.track || s.pipe == nil {
			return fail(r.ID, ErrStale)
		}
		s.ended = true
		if err := s.pipe.EndOfStream(r.Track); err != nil {
			return fail(r.ID, err)
		}
	case opPlay:
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.pipe == nil {
			return fail(r.ID, errors.New("cdmhost: nothing loaded"))
		}
		s.playing, s.started, s.havePrev, s.buffering = true, false, false, false
		if err := s.pipe.Play(); err != nil {
			return fail(r.ID, err)
		}
	case opPause:
		s.mu.Lock()
		defer s.mu.Unlock()
		s.playing, s.buffering = false, false
		if s.pipe != nil {
			if err := s.pipe.Pause(); err != nil {
				return fail(r.ID, err)
			}
			ok.Position, ok.HasPos = s.pipe.Position()
		}
	case opStop:
		s.mu.Lock()
		defer s.mu.Unlock()
		s.track, s.playing, s.started, s.ended, s.buffering = r.Track, false, false, false, false
		if s.pipe != nil {
			s.pipe.Flush(r.Track, 0, nil)
			if err := s.pipe.Pause(); err != nil {
				return fail(r.ID, err)
			}
		}
	case opVolume:
		s.mu.Lock()
		defer s.mu.Unlock()
		s.volume = r.Volume
		if s.pipe != nil {
			s.pipe.SetVolume(r.Volume)
		}
	case opPosition:
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.pipe != nil {
			ok.Position, ok.HasPos = s.pipe.Position()
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

// open loads the CDM and builds the output pipeline.
func (s *server) open(override string) (Info, error) {
	info := Info{PID: os.Getpid(), GStreamer: gst.Version()}
	if s.cdm != nil {
		return s.info, nil
	}
	path, version, err := widevine.FindCDM(override)
	if err != nil {
		return info, err
	}
	info.CDMPath, info.CDMVersion = path, version
	cdm, err := widevine.Open(path, s.log)
	if err != nil {
		return info, err
	}
	s.cdm = cdm
	info.CDMVersion, info.CDMInterface, info.HostInterface = cdm.Version(), cdm.Interface(), cdm.HostInterface()
	go func() {
		for m := range cdm.Messages() {
			s.emit(Event{Kind: EventMessage, Session: m.SessionID, Type: int(m.Type), Data: m.Data})
		}
	}()
	s.mu.Lock()
	err = s.ensurePipeline()
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
	p, err := gst.New(gst.Options{Sink: s.sink})
	if err != nil {
		return err
	}
	p.SetVolume(s.volume)
	s.pipe = p
	go s.watch(p)
	return nil
}

// watch forwards pipeline events for p's lifetime.
func (s *server) watch(p *gst.Pipeline) {
	for ev := range p.Events() {
		switch ev.Kind {
		case gst.EventEOS:
			s.mu.Lock()
			track, ended := s.track, s.ended && s.pipe == p
			pos, _ := p.Position()
			s.mu.Unlock()
			if ended {
				s.emit(Event{Kind: EventEnded, Track: track, Position: pos, At: time.Now()})
			}
		case gst.EventError:
			s.log.Error("cdmhost: pipeline error", "err", ev.Err)
			s.mu.Lock()
			track := s.track
			if s.pipe == p {
				// Rebuilt on the next Load.
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

// load starts a new track generation at r.Start, flushing whatever played.
func (s *server) load(r request) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	in := s.init
	if len(r.Data) > 0 {
		var err error
		if in, err = fmp4.ParseInit(r.Data); err != nil {
			return err
		}
	}
	if in == nil {
		return errors.New("cdmhost: load without init segment")
	}
	if err := checkScheme(in); err != nil {
		return err
	}
	fresh := s.pipe == nil
	if err := s.ensurePipeline(); err != nil {
		return err
	}
	var caps *gst.Caps
	if fresh || s.init == nil || !bytes.Equal(in.AudioConfig, s.init.AudioConfig) {
		caps = &gst.Caps{AudioConfig: in.AudioConfig, Rate: in.SampleRate, Channels: in.Channels}
	}
	s.track, s.init, s.start = r.Track, in, r.Start
	s.started, s.havePrev, s.ended, s.buffering = false, false, false, false
	s.pipe.Flush(r.Track, r.Start, caps)
	if s.playing {
		return s.pipe.Play()
	}
	return s.pipe.Pause()
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
	track, in, start, pipe := s.track, s.init, s.start, s.pipe
	s.mu.Unlock()
	if r.Track != track || pipe == nil || in == nil {
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
	if !s.playing || s.pipe == nil {
		return Event{}, false
	}
	pos, ok := s.pipe.Position()
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
		return Event{Kind: EventStarted, Track: s.track, Position: pos, At: now}, true
	}
	queued := s.pipe.Queued()
	switch {
	case s.buffering && (queued >= resumeBuffered || s.ended):
		s.buffering = false
		_ = s.pipe.Play()
		s.started, s.havePrev = false, false // re-anchor with EventStarted
		return Event{Kind: EventBuffering, Track: s.track, Position: pos, At: now}, true
	case !s.buffering && !s.ended && queued == 0:
		if s.underrun.IsZero() {
			s.underrun = now
		}
		if now.Sub(s.underrun) >= underrunAfter {
			s.buffering, s.underrun = true, time.Time{}
			_ = s.pipe.Pause()
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
	s.mu.Unlock()
	if p != nil {
		p.Close()
	}
	if s.cdm != nil {
		s.cdm.Close()
	}
}
