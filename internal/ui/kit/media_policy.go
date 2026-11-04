package kit

import (
	"context"
	"fmt"
	"sync"
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
	now  func() time.Time
	// codec is media.Codec ("" when nothing decodes motion artwork).
	codec func() string

	// The main thread only.
	onBattery bool
	hidden    bool
	covers    int
	offPower  func()
	owner     *motionSlot // the tile whose motion plays
	last      string      // the newest video's outcome, for Diagnostics

	mu        sync.Mutex // the registry, which the sweep reads
	slots     map[*motionSlot]struct{}
	quit      chan struct{}
	sweepOnce sync.Once
	closeOnce sync.Once
}

// NewMotionPolicy returns the policy, following "power:changed" on bus. Call
// it on the main thread (store.On's rule); post is the UI's Poster.
func NewMotionPolicy(bus *store.Bus, post store.Poster, o MotionOptions) *MotionPolicy {
	if post == nil {
		post = func(fn func()) { fn() }
	}
	p := &MotionPolicy{opts: o, post: post, now: time.Now, codec: media.Codec, slots: map[*motionSlot]struct{}{}, quit: make(chan struct{})}
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

// Close stops following the power source and closes every decoder.
func (p *MotionPolicy) Close() {
	if p == nil {
		return
	}
	if p.offPower != nil {
		p.offPower()
	}
	p.closeOnce.Do(func() { close(p.quit) })
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
// but their stills when it is false.
func (p *MotionPolicy) Allowed() bool {
	if p == nil || p.open == nil || p.codec() == "" {
		return false
	}
	if p.opts.Setting != nil && !p.opts.Setting() {
		return false
	}
	return !theme.ReduceEffects() && !theme.ReducedMotion() && !p.onBattery
}

// SetVisible says whether the window shows (not minimised, not hidden to the
// tray): playing videos pause while it does not, and keep their decoders.
func (p *MotionPolicy) SetVisible(v bool) {
	if p != nil {
		p.hidden = !v
	}
}

// Visible reports what SetVisible last said (true at first).
func (p *MotionPolicy) Visible() bool { return p != nil && !p.hidden }

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
	r.Last = p.last
	switch p.codec() {
	case "avc":
		r.H264 = "probably"
	case "hevc":
		r.HEVC = "probably"
	}
	return r
}

// outcome records how a video went: the newest one is what Diagnostics shows.
func (p *MotionPolicy) outcome(kind string, err error) {
	p.last = kind
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

// register adds a slot to the registry and starts the sweep.
func (p *MotionPolicy) register(s *motionSlot) {
	p.mu.Lock()
	p.slots[s] = struct{}{}
	p.mu.Unlock()
	p.sweepOnce.Do(func() { go p.sweep() })
}

func (p *MotionPolicy) unregister(s *motionSlot) {
	p.mu.Lock()
	delete(p.slots, s)
	p.mu.Unlock()
}

// sweep closes the decoders of slots that stopped being built, on the main
// thread. While the window is hidden (SetVisible) they stay: nothing builds
// then, and the decoders only wait.
func (p *MotionPolicy) sweep() {
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		select {
		case <-p.quit:
			return
		case <-t.C:
		}
		stale := p.stale()
		if len(stale) == 0 {
			continue
		}
		p.post(func() { p.release(stale) })
	}
}

// stale are the slots nothing has built for idleAfter.
func (p *MotionPolicy) stale() (out []*motionSlot) {
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	for s := range p.slots {
		if now.Sub(time.Unix(0, s.built.Load())) > idleAfter {
			out = append(out, s)
		}
	}
	return out
}

// release closes the decoders of the slots still unbuilt, on the main
// thread; while the window is hidden (SetVisible) they stay, waiting.
func (p *MotionPolicy) release(slots []*motionSlot) {
	if p.hidden {
		return
	}
	for _, s := range slots {
		if p.now().Sub(time.Unix(0, s.built.Load())) > idleAfter {
			s.release()
		}
	}
}
