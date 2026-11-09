//go:build linux || (cgo && (darwin || windows))

package cdmhost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/avc"
	"github.com/coredev-uk/sona-go/internal/playback/fmp4"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
)

// The child's side of music videos (video.go): a feeder turns fragments
// into Annex B access units, a decoder runs them through the CDM's video
// decoder a few pictures ahead of the audio, and a presenter sends each
// picture to the parent when the audio heard reaches it (planFrames).

const (
	// maxQueuedUnits bounds the access units waiting for the decoder
	// (about 10 s at 25 fps): FeedVideo blocks beyond it.
	maxQueuedUnits = 256
	// maxQueuedPictures bounds the decoded pictures waiting for their
	// time (under half a second; an 864×486 picture is 630 KB).
	maxQueuedPictures = 10
	// maxDecodeErrors ends a video after this many access units in a row
	// failed to decode.
	maxDecodeErrors = 5
)

// videoUnit is one access unit waiting for the decoder.
type videoUnit struct {
	data []byte
	subs []widevine.Subsample
	iv   [16]byte
	ivn  int // IV length; 0 for a clear unit
	pts  time.Duration
}

// videoState is the current video; gen 0 when the current track has none.
// mu guards everything but decMu; decMu is held while a unit is decoded
// and while the decoder is reset, so a reset never meets a unit of the
// previous video. Lock order: server.mu, then mu; decMu before mu.
type videoState struct {
	decMu sync.Mutex

	mu     sync.Mutex
	cond   *sync.Cond
	closed bool
	gen    uint64
	epoch  uint64 // changes on every load and stop
	in     *fmp4.Init
	avc    *avc.Config
	kid    []byte
	start  time.Duration
	shift  time.Duration // subtracted from presentation times (VideoMedia.Shift)
	units  []videoUnit
	frames []widevine.VideoFrame
	ended  bool // EndOfVideo received: drain the decoder when units run out
	// drained: the decoder has handed out its last picture.
	drained bool
	shown   bool  // a picture was sent since the load
	failed  error // the decoder failed; nothing more is decoded
	errs    int   // consecutive decode errors
	wake    chan struct{}
}

func newVideoState() *videoState {
	v := &videoState{wake: make(chan struct{}, 1)}
	v.cond = sync.NewCond(&v.mu)
	return v
}

// poke wakes the presenter; with mu held or not.
func (v *videoState) poke() {
	select {
	case v.wake <- struct{}{}:
	default:
	}
}

// clearLocked drops the video's queues; with mu held.
func (v *videoState) clearLocked() {
	v.epoch++
	v.units, v.frames = nil, nil
	v.ended, v.drained, v.shown, v.failed, v.errs = false, false, false, nil, 0
	v.cond.Broadcast()
	v.poke()
}

// stopVideo drops the video: the current track has none (a Load of
// another track, or Stop). With server.mu held or not.
func (s *server) stopVideo() {
	v := s.video
	if v == nil {
		return // a server built without video (tests)
	}
	v.mu.Lock()
	v.gen = 0
	v.clearLocked()
	v.mu.Unlock()
}

// videoConfig is the CDM decoder configuration for a video init segment,
// with its H.264 configuration record (Annex B conversion).
func videoConfig(in *fmp4.Init) (widevine.VideoConfig, *avc.Config, error) {
	if !in.IsVideo() {
		return widevine.VideoConfig{}, nil, errors.New("cdmhost: the video init segment has no video track")
	}
	if in.Codec != "avc1" && in.Codec != "avc3" {
		return widevine.VideoConfig{}, nil, fmt.Errorf("cdmhost: video codec %q: the CDM's decoder takes H.264 only", in.Codec)
	}
	if in.Protected && in.Scheme != "cenc" && in.Scheme != "cbcs" {
		return widevine.VideoConfig{}, nil, fmt.Errorf("cdmhost: unsupported encryption scheme %q", in.Scheme)
	}
	c, err := avc.ParseConfig(in.VideoConfig)
	if err != nil {
		return widevine.VideoConfig{}, nil, err
	}
	p := widevine.H264Profile(c.Profile)
	if p == widevine.ProfileUnknown {
		return widevine.VideoConfig{}, nil, fmt.Errorf("cdmhost: H.264 profile %d", c.Profile)
	}
	if in.Width <= 0 || in.Height <= 0 || in.Width > MaxVideoSize || in.Height > MaxVideoSize {
		return widevine.VideoConfig{}, nil, fmt.Errorf("cdmhost: video size %dx%d", in.Width, in.Height)
	}
	// As Chromium does for H.264 from MP4: no extra data, the parameter
	// sets travel in band (avc.Config.AnnexB).
	return widevine.VideoConfig{
		Codec: widevine.CodecH264, Profile: p, Width: in.Width, Height: in.Height,
		CBCS: in.Scheme == "cbcs",
	}, c, nil
}

// loadVideo makes r.Track's video current from r.Start: with an init
// segment it initialises the CDM's decoder for it, without (a seek) it
// flushes the decoder and keeps the video's configuration.
func (s *server) loadVideo(ctx context.Context, r request) error {
	if s.cdm == nil {
		return errors.New("cdmhost: CDM not initialised")
	}
	var (
		in  *fmp4.Init
		cfg widevine.VideoConfig
		c   *avc.Config
	)
	if len(r.Data) > 0 {
		var err error
		if in, err = fmp4.ParseInit(r.Data); err != nil {
			return err
		}
		if cfg, c, err = videoConfig(in); err != nil {
			return err
		}
	}
	s.mu.Lock()
	current := r.Track == s.track && s.pipe != nil
	s.mu.Unlock()
	if !current {
		return ErrStale
	}
	v := s.video
	v.decMu.Lock()
	defer v.decMu.Unlock()
	v.mu.Lock()
	if in == nil && v.in == nil {
		v.mu.Unlock()
		return errors.New("cdmhost: video load without init segment")
	}
	if in != nil {
		v.in, v.avc = in, c
	}
	v.gen, v.kid, v.start, v.shift = r.Track, r.KID, r.Start, r.Shift
	v.clearLocked()
	v.mu.Unlock()
	var err error
	if in != nil {
		err = s.cdm.InitializeVideoDecoder(ctx, cfg)
	} else {
		err = s.cdm.ResetVideoDecoder(ctx)
	}
	if err != nil {
		v.mu.Lock()
		if v.gen == r.Track {
			v.gen = 0
			if in != nil {
				v.in, v.avc = nil, nil
			}
			v.clearLocked()
		}
		v.mu.Unlock()
		return err
	}
	s.log.Debug("cdmhost: video loaded", "track", r.Track, "start", r.Start, "new", in != nil)
	return nil
}

// endVideo marks track's last fragment.
func (s *server) endVideo(r request) error {
	v := s.video
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.gen != r.Track || v.gen == 0 {
		return ErrStale
	}
	v.ended = true
	v.cond.Broadcast()
	return nil
}

// videoFeeder queues video fragments in arrival order.
func (s *server) videoFeeder() {
	var sc videoScratch
	for {
		select {
		case r := <-s.vfeeds:
			if err := s.feedVideo(r, &sc); err != nil {
				s.reply(fail(r.ID, err))
			} else {
				s.reply(reply{ID: r.ID})
			}
		case <-s.quit:
			return
		}
	}
}

// videoScratch is reused between fragments.
type videoScratch struct {
	subs []fmp4.Subsample
}

// feedVideo parses fragment r.Data of the current video into Annex B
// access units and queues them for the decoder, waiting while the queue is
// full.
func (s *server) feedVideo(r request, sc *videoScratch) error {
	v := s.video
	v.mu.Lock()
	in, c, gen, shift := v.in, v.avc, v.gen, v.shift
	v.mu.Unlock()
	if gen == 0 || gen != r.Track || in == nil {
		return ErrStale
	}
	samples, err := fmp4.ParseFragment(r.Data, in)
	if err != nil {
		return err
	}
	units := make([]videoUnit, 0, len(samples))
	for i := range samples {
		smp := &samples[i]
		if len(smp.Data) == 0 {
			continue
		}
		var subs []fmp4.Subsample
		if smp.Encrypted() {
			subs = smp.Subsamples
			if subs == nil {
				subs = []fmp4.Subsample{} // encrypted whole: AnnexB refuses it
			}
		}
		data, out, err := c.AnnexB(nil, smp.Data, subs, sc.subs)
		if err != nil {
			return fmt.Errorf("cdmhost: video sample %d: %w", i, err)
		}
		u := videoUnit{data: data, pts: videoTime(smp, in) - shift}
		if smp.Encrypted() {
			u.ivn = len(fmp4.PaddedIV(&u.iv, smp.IV))
			u.subs = make([]widevine.Subsample, len(out))
			for j, ss := range out {
				u.subs[j] = widevine.Subsample{Clear: ss.Clear, Cipher: ss.Cipher}
			}
		}
		sc.subs = out
		units = append(units, u)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	for v.gen == r.Track && !v.closed && len(v.units) >= maxQueuedUnits {
		v.cond.Wait()
	}
	if v.gen != r.Track || v.closed {
		return ErrStale
	}
	v.units = append(v.units, units...)
	v.cond.Broadcast()
	return nil
}

// videoDecoder runs queued access units through the CDM's decoder while
// fewer than maxQueuedPictures pictures wait for their time.
func (s *server) videoDecoder() {
	v := s.video
	for {
		v.mu.Lock()
		for !v.closed && !v.decodableLocked() {
			v.cond.Wait()
		}
		if v.closed {
			v.mu.Unlock()
			return
		}
		epoch := v.epoch
		v.mu.Unlock()

		v.decMu.Lock()
		v.mu.Lock()
		if v.epoch != epoch || v.gen == 0 {
			v.mu.Unlock()
			v.decMu.Unlock()
			continue
		}
		gen, kid, cbcs := v.gen, v.kid, v.in.Scheme == "cbcs"
		crypt, skip := v.in.CryptByte, v.in.SkipByte
		var b *widevine.Buffer
		if len(v.units) > 0 {
			u := v.units[0]
			v.units[0] = videoUnit{}
			v.units = v.units[1:]
			v.cond.Broadcast() // room for the feeder
			b = &widevine.Buffer{Data: u.data, Timestamp: u.pts}
			if u.ivn > 0 {
				b.KeyID, b.IV, b.Subsamples = kid, u.iv[:u.ivn], u.subs
				b.CBCS, b.CryptBlocks, b.SkipBlocks = cbcs, crypt, skip
				if cbcs && crypt == 0 && skip == 0 {
					b.CryptBlocks = 1
				}
			}
		}
		v.mu.Unlock()

		f, err := s.cdm.DecodeVideo(b)

		v.mu.Lock()
		if v.epoch == epoch {
			switch {
			case err == nil:
				v.errs = 0
				v.frames = append(v.frames, f)
				v.poke()
			case errors.Is(err, widevine.ErrNeedMoreData):
				if b == nil {
					v.drained = true
				}
			case errors.Is(err, widevine.ErrDecode) && b != nil && v.errs+1 < maxDecodeErrors:
				v.errs++
				s.log.Warn("cdmhost: video unit not decoded", "track", gen, "at", b.Timestamp, "err", err)
			default:
				v.failed = err
			}
		}
		failed := v.failed
		v.mu.Unlock()
		v.decMu.Unlock()
		if failed != nil && err != nil && v.isEpoch(epoch) {
			s.log.Error("cdmhost: video decode failed", "track", gen, "err", err)
			s.emit(Event{Kind: EventError, Track: gen, Err: fmt.Sprintf("video: %v", err)})
		}
	}
}

// isEpoch reports whether epoch is still the current load.
func (v *videoState) isEpoch(epoch uint64) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.epoch == epoch
}

// videoPresenter sends each decoded picture to the parent when it is due
// against the audio being heard (planFrames).
func (s *server) videoPresenter() {
	v := s.video
	t := time.NewTimer(presentPoll)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-v.wake:
			if !t.Stop() {
				select {
				case <-t.C:
				default:
				}
			}
		case <-t.C:
		}
		v.mu.Lock()
		gen := v.gen
		v.mu.Unlock()
		pos, havePos := s.audioPosition(gen)

		v.mu.Lock()
		var send *widevine.VideoFrame
		wait := presentPoll
		if v.gen == gen && gen != 0 && len(v.frames) > 0 {
			pts := make([]time.Duration, len(v.frames))
			for i := range v.frames {
				pts[i] = v.frames[i].Timestamp
			}
			p := planFrames(pts, v.start, pos, havePos, v.shown)
			for i := range p.drop {
				v.frames[i] = widevine.VideoFrame{}
			}
			v.frames = v.frames[p.drop:]
			if p.show {
				f := v.frames[0]
				v.frames[0] = widevine.VideoFrame{}
				v.frames = v.frames[1:]
				send = &f
				v.shown = true
			}
			if p.drop > 0 || p.show {
				v.cond.Broadcast() // room for the decoder
			}
			wait = p.wait
		}
		v.mu.Unlock()
		if send != nil {
			s.send(frame{Video: &VideoFrame{Track: gen, Position: send.Timestamp, Width: send.Width, Height: send.Height, Data: send.Data}})
			wait = presentMin
		}
		t.Reset(wait)
	}
}

// audioPosition is the position of track gen's audio being heard, when
// gen is the track playing.
func (s *server) audioPosition(gen uint64) (time.Duration, bool) {
	if gen == 0 {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pipe == nil || s.file != nil || s.track != gen {
		return 0, false
	}
	g, pos, ok := s.pipe.Position()
	if !ok || g != gen {
		return 0, false
	}
	return pos, true
}

// closeVideo stops the video goroutines.
func (s *server) closeVideo() {
	v := s.video
	if v == nil {
		return
	}
	v.mu.Lock()
	v.closed = true
	v.cond.Broadcast()
	v.mu.Unlock()
}

// decodableLocked reports whether the decoder has work: a current video
// that has not failed, room for another picture, and a unit to decode or
// a drain to run. With mu held.
func (v *videoState) decodableLocked() bool {
	return v.gen != 0 && v.failed == nil && len(v.frames) < maxQueuedPictures &&
		(len(v.units) > 0 || (v.ended && !v.drained))
}
