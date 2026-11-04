package kit

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/media"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// MotionOptions are what a MotionPolicy reads.
type MotionOptions struct {
	// Files serves the motion artwork files (*motion.Cache); nil: none is
	// offered and nothing plays.
	Files media.MotionFiles
	// Setting is Settings → Appearance → Motion artwork
	// (config.Settings.MotionArtwork.Enabled); nil: on.
	Setting func() bool
	// Power is the power source now (MotionService.Power), read once to
	// seed what "power:changed" keeps current; nil: mains power.
	Power func() services.PowerState
	// Art is the window's artwork loader, which knows the screen's scale: the
	// size a tile's video is requested at (TileMotion takes no loader). nil:
	// scale 1.
	Art *ArtLoader
	// Log is told how motion went (never a file path or a title); nil:
	// nowhere.
	Log func(msg string, args ...any)
}

// MotionPolicy decides whether motion artwork may play, and owns what plays:
// the covers' and tiles' decoders, which it opens off the main thread and
// closes once nothing builds them (a view that went away has no unmount).
//
// Motion plays only when asked for and nothing argues against it: the setting
// is on, Reduce effects and the desktop's reduced motion are off, the
// computer is not on battery (Go's "power:changed", UPower's devices, not its
// OnBattery) and the platform has a decoder (media.Codec). A playing video
// also pauses while it is out of view, while the window is hidden
// (SetVisible) and while the page is covered (Cover): under Now Playing,
// full-screen lyrics or a modal dialog, which keep drawing what lies under
// them (React paused when the page went inert). Its methods run on the main
// thread; nil is a policy that allows nothing.
type MotionPolicy struct {
	opts MotionOptions
	post store.Poster
	// open makes a player of a motion file (tests replace it).
	open func(ctx context.Context, urlPath string, px int) (*ui.VideoPlayer, error)
	// probe is media.Codec: what decodes motion artwork, "" for nothing. It
	// starts GStreamer and scans its plugins, so it runs on a goroutine, once.
	probe func() string

	// The main thread only.
	codec     string // the probe's answer, once codecDone
	codecDone bool
	probing   bool
	onBattery bool
	covers    int
	offPower  func()
	owner     *motionSlot // the tile whose motion plays
	last      string      // the newest video's outcome, for Diagnostics
	lastAt    int64       // when it was recorded, Unix milliseconds

	hidden atomic.Bool // the window is hidden (the sweep reads it)
	quit   chan struct{}
	closed sync.Once

	mu       sync.Mutex // the registry and the clock, which the sweep reads
	now      func() time.Time
	slots    map[*motionSlot]struct{}
	sweeping bool
}

// NewMotionPolicy returns the policy, following "power:changed" on bus. Call
// it on the main thread (store.On's rule); post is the UI's Poster.
func NewMotionPolicy(bus *store.Bus, post store.Poster, o MotionOptions) *MotionPolicy {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	p := &MotionPolicy{opts: o, post: post, now: time.Now, probe: media.Codec, slots: map[*motionSlot]struct{}{}, quit: make(chan struct{})}
	if o.Power != nil {
		p.onBattery = o.Power().OnBattery
	}
	if bus != nil {
		p.offPower = store.On(bus, services.EventPower, func(s services.PowerState) { p.onBattery = s.OnBattery })
	}
	if o.Files != nil {
		p.open = func(ctx context.Context, urlPath string, px int) (*ui.VideoPlayer, error) {
			return media.OpenMotion(ctx, o.Files, urlPath, px)
		}
	}
	return p
}

// NewTestMotionPolicy is a policy for the tests of the features that build
// motion covers: motion is allowed (the setting on, an "avc" decoder known, no
// battery) and every motion file is opened by open instead of the motion
// cache and GStreamer, so a test sees which URL and size a cover asked for.
// post is the test's main thread (uitest's Main.Post); the caller closes the
// policy.
func NewTestMotionPolicy(post store.Poster, open func(ctx context.Context, urlPath string, px int) (*ui.VideoPlayer, error)) *MotionPolicy {
	p := NewMotionPolicy(nil, post, MotionOptions{})
	p.open = open
	p.setCodec("avc")
	return p
}

// Close stops following the power source and closes every decoder.
func (p *MotionPolicy) Close() {
	if p == nil {
		return
	}
	if p.offPower != nil {
		p.offPower()
	}
	p.closed.Do(func() { close(p.quit) })
	p.mu.Lock()
	slots := make([]*motionSlot, 0, len(p.slots))
	for s := range p.slots {
		slots = append(slots, s)
	}
	p.mu.Unlock()
	for _, s := range slots {
		s.release()
	}
}

// Allowed reports whether motion may play at all now: the setting, reduce
// effects, reduced motion, power and a decoder. Covers and tiles draw nothing
// but their stills when it is false, which it is until the decoder probe has
// answered (the first call starts it on a goroutine and the answer redraws).
func (p *MotionPolicy) Allowed() bool {
	if p == nil || p.open == nil || !p.codecKnown() || p.codec == "" {
		return false
	}
	if p.opts.Setting != nil && !p.opts.Setting() {
		return false
	}
	return !theme.ReduceEffects() && !theme.ReducedMotion() && !p.onBattery
}

// Probed reports whether the decoder probe has answered, starting it the
// first time (Allowed does too). Until then Report's codec fields are blank
// because nothing is known, not because nothing decodes.
func (p *MotionPolicy) Probed() bool { return p != nil && p.codecKnown() }

// codecKnown reports whether the decoder probe has answered, starting it the
// first time: GStreamer's start-up (its plugin registry may be scanned) must
// not run on the main thread.
func (p *MotionPolicy) codecKnown() bool {
	if !p.codecDone && !p.probing {
		p.probing = true
		go func() {
			name := p.probe()
			p.post(func() { p.codec, p.codecDone = name, true })
		}()
	}
	return p.codecDone
}

// setCodec is the probe's answer, given (tests).
func (p *MotionPolicy) setCodec(name string) { p.codec, p.codecDone, p.probing = name, true, true }

// SetVisible says whether the window shows (not minimised, not hidden to the
// tray): playing videos pause while it does not, and keep their decoders.
func (p *MotionPolicy) SetVisible(v bool) {
	if p != nil {
		p.hidden.Store(!v)
	}
}

// Visible reports what SetVisible last said (true at first).
func (p *MotionPolicy) Visible() bool { return p != nil && !p.hidden.Load() }

// Cover marks the page as covered (Now Playing, full-screen lyrics, a modal
// dialog) until release is called: the videos and equalisers of the page
// pause. Calls nest.
func (p *MotionPolicy) Cover() (release func()) {
	if p == nil {
		return func() {}
	}
	p.covers++
	var once sync.Once
	return func() { once.Do(func() { p.covers-- }) }
}

// Covered reports whether something covers the page.
func (p *MotionPolicy) Covered() bool { return p != nil && p.covers > 0 }

// Report is the native side of motion artwork as Diagnostics sends it
// (MotionService.Report): what decides whether motion plays, which codec
// the decoder has and how the newest video went.
func (p *MotionPolicy) Report() services.MotionReport {
	r := services.MotionReport{Visible: p.Visible(), ReduceEffects: theme.ReduceEffects(), ReducedMotion: theme.ReducedMotion()}
	if p == nil {
		return r
	}
	r.Allowed = p.Allowed()
	r.Setting = p.opts.Setting == nil || p.opts.Setting()
	r.OnBattery = p.onBattery
	p.mu.Lock()
	for s := range p.slots {
		if s.live {
			r.Playing++
		}
	}
	p.mu.Unlock()
	r.Last, r.LastAt = p.last, p.lastAt
	switch p.codec {
	case "avc":
		r.H264 = "probably"
	case "hevc":
		r.HEVC = "probably"
	}
	return r
}

// outcome records how a video went: the newest one is what Diagnostics shows.
func (p *MotionPolicy) outcome(kind string, err error) {
	p.last, p.lastAt = kind, p.clock().UnixMilli()
	if err != nil {
		p.last = fmt.Sprintf("%s: %v", kind, err)
	}
	if p.opts.Log != nil {
		if err != nil {
			p.opts.Log("motion: "+kind, "err", err)
		} else {
			p.opts.Log("motion: " + kind)
		}
	}
}

// sweepEvery is how often decoders nothing builds are looked for, and
// idleAfter how long a slot may go unbuilt before its decoder is closed (a
// view that is gone does not tell: React unmounted it).
const (
	sweepEvery = 500 * time.Millisecond
	idleAfter  = 1500 * time.Millisecond
)

// register adds a slot to the registry and starts the sweep if it is not
// running.
func (p *MotionPolicy) register(s *motionSlot) {
	p.mu.Lock()
	p.slots[s] = struct{}{}
	start := !p.sweeping
	p.sweeping = true
	p.mu.Unlock()
	if start {
		go p.sweep()
	}
}

func (p *MotionPolicy) unregister(s *motionSlot) {
	p.mu.Lock()
	delete(p.slots, s)
	p.mu.Unlock()
}

// clock is the policy's time (tests move it).
func (p *MotionPolicy) clock() time.Time {
	p.mu.Lock()
	f := p.now
	p.mu.Unlock()
	return f()
}

// sweep closes the decoders of slots that stopped being built, on the main
// thread. It runs while there are slots and ends with the last; while the
// window is hidden (SetVisible) it does nothing and wakes nobody: nothing
// builds then, and the decoders only wait.
func (p *MotionPolicy) sweep() {
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		select {
		case <-p.quit:
			return
		case <-t.C:
		}
		if p.hidden.Load() {
			continue
		}
		stale, empty := p.stale()
		if empty {
			return
		}
		if len(stale) > 0 {
			p.post(func() { p.release(stale) })
		}
	}
}

// stale are the slots nothing has built for idleAfter; empty reports that
// there are no slots at all, which also ends the sweep (register starts it
// again).
func (p *MotionPolicy) stale() (out []*motionSlot, empty bool) {
	now := p.clock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.slots) == 0 {
		p.sweeping = false
		return nil, true
	}
	for s := range p.slots {
		if now.Sub(time.Unix(0, s.built.Load())) > idleAfter {
			out = append(out, s)
		}
	}
	return out, false
}

// release closes the decoders of the slots still unbuilt, on the main
// thread; while the window is hidden (SetVisible) they stay, waiting.
func (p *MotionPolicy) release(slots []*motionSlot) {
	if p.hidden.Load() {
		return
	}
	for _, s := range slots {
		if p.clock().Sub(time.Unix(0, s.built.Load())) > idleAfter {
			s.release()
		}
	}
}

// ownerStale is how long a tile that plays may go unbuilt before another may
// take over (it scrolled away, or its page went).
const ownerStale = 600 * time.Millisecond

// claim says whether tile s may play: one tile's motion plays at a time, the
// hovered one before one that only has the keyboard focus (lib/motion.ts's
// hovered ?? focused). A hovered tile takes over from a focused one at once;
// an owner that failed, or was not built lately, is gone. A tile that cannot
// play now is woken by the owner's release (see motionSlot.release), so it
// polls nothing.
func (p *MotionPolicy) claim(s *motionSlot, hover bool) bool {
	rank := 1
	if hover {
		rank = 2
	}
	if o := p.owner; o != nil && o != s {
		gone := o.state == slotFailed || p.clock().Sub(time.Unix(0, o.built.Load())) > ownerStale
		if !gone && o.rank >= rank {
			return false
		}
		o.release()
	}
	p.owner, s.rank = s, rank
	return true
}
