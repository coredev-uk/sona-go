// Package player is the player bar: the playing song's cover, title and
// artist, the transport, the seek bar and the volume, across the bottom of
// the window, and the shortcuts for what the bar does. The queue, lyrics,
// Now Playing and the video view are other features.
//
// Every command goes through services.PlaybackService, off the main
// thread, and its failure comes back as a toast worded by i18n.ErrorText.
// The bar draws what store.Playback mirrors; it keeps no playback state of
// its own besides the seek bar's drag.
package player

import (
	"context"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// transport is what the bar asks of the playback service
// (*services.PlaybackService).
type transport interface {
	Toggle(ctx context.Context) error
	Next(ctx context.Context) error
	Prev(ctx context.Context) error
	Seek(ctx context.Context, positionMs int64) error
	SetVolume(ctx context.Context, volume float64) error
	SetShuffle(on bool)
	SetRepeat(mode string) error
	SetAutoPlay(on bool) error
	SetSleep(mode string, minutes int) error
	Jump(ctx context.Context, index int) error
	Remove(ctx context.Context, index int) error
	Move(from, to int) error
	MoveNext(from int) error
	MoveToEnd(from int) error
	Clear()
	kit.QueueService
}

// player is the feature's state.
type player struct {
	env *module.Env
	svc transport
	// volume reports whether Sona sets the playback volume here
	// (PlatformService capabilities).
	volume func() bool
	now    func() time.Time

	// seek is the seek bar while the user drags it: the position shown,
	// committed when the pointer lets go.
	seek struct {
		active bool
		secs   float64
	}

	// ui is the panels' state (view.go); np is Now Playing's (nowplaying_state.go).
	ui *view
	np nowPlayingState
	// cmds are the queue commands (playing, replacing and queueing).
	cmds *kit.QueueCommands
	// queue is the queue panel's own state (queue_*.go).
	queue queueUI
	// sleepUI is the sleep timer's state (sleep_*.go).
	sleep sleepUI
	// announce is the track announcer's state (announcer.go).
	announce announcer
	// extras is the bar's other state (bar_extras.go).
	extras barExtras

	// ui is the panels' state (view.go); np is Now Playing's (nowplaying_state.go).
	ui *view
	np nowPlayingState

	mu     sync.Mutex
	failed []failure // command failures waiting to be shown (the view shows them)
}

// failure is a command that failed: what it was, and Go's error code.
type failure struct {
	what string
	code services.ErrorCode
}

// New is the player bar's module.
func New(env *module.Env) module.Module {
	pb := services.Service[services.PlaybackService](env.Core)
	plat := services.Service[services.PlatformService](env.Core)
	p := newPlayer(env, pb, func() bool { return plat.Capabilities().Playback.Volume })
	if env.Queue != nil {
		p.cmds = env.Queue
	}
	p.np.queueView = p.upNext
	p.np.rating = func(c *ui.Context, cur *services.QueueItem) { p.ratingButtons(c, cur, false) }
	auth := services.Service[services.AuthService](env.Core)
	return module.Module{
		ID:        "player",
		Shortcuts: append(p.shortcuts(), p.nowPlayingShortcuts()...),
		Actions:   p.actions(),
		Slots: module.Slots{
			Bottom: p.bar, Aside: p.queueAside, Overlay: p.overlaySlot(),
			Layer: p.roomLayer, LayerOpen: p.roomOpen,
		},
		Init: func() { p.init(); p.followAccount(auth) },
	}
}

func newPlayer(env *module.Env, svc transport, volume func() bool) *player {
	p := &player{env: env, svc: svc, volume: volume, now: time.Now, ui: newView()}
	p.cmds = kit.NewQueueCommands(kit.QueueDeps{
		Svc: svc, Play: env.Play, Post: env.Post,
		Unavailable: func() string { return unavailable(env.Play.State(), ctlQueue) },
	})
	return p
}

// run starts a command off the main thread; its failure is shown as a toast
// titled what.
func (p *player) run(what func() string, fn func(ctx context.Context) error) {
	title := what()
	go func() {
		if err := fn(context.Background()); err != nil {
			p.fail(title, err)
		}
	}()
}

// fail queues a failure for the view and redraws.
func (p *player) fail(what string, err error) {
	f := failure{what: what, code: services.CodeOf(err)}
	p.env.Post(func() {
		p.mu.Lock()
		p.failed = append(p.failed, f)
		p.mu.Unlock()
	})
}

// toasts shows the failures that arrived since the last frame: the
// command in the title, the code's words below.
func (p *player) toasts(c *ui.Context) {
	p.mu.Lock()
	failed := p.failed
	p.failed = nil
	p.mu.Unlock()
	for _, f := range failed {
		c.AddToast(ui.Toast{ID: f.what, Title: f.what, Description: i18n.ErrorText(f.code), Type: "error"})
	}
}

// The commands. Each says what it was doing (its toast's title) as a
// literal, so the catalogue's extraction finds it.

func (p *player) toggle() {
	p.run(func() string { return i18n.T("Couldn't play or pause") }, p.svc.Toggle)
}

func (p *player) next() {
	p.run(func() string { return i18n.T("Couldn't skip") }, p.svc.Next)
}

func (p *player) previous() {
	p.run(func() string { return i18n.T("Couldn't go back") }, p.svc.Prev)
}

// seekTo seeks to a position in the playing song: the mirror shows it at
// once, until Go's next snapshot.
func (p *player) seekTo(pos time.Duration) {
	ms := pos.Milliseconds()
	now := p.now().UnixMilli()
	p.env.Play.Patch(func(s *services.PlaybackState) { s.PositionMs, s.At = ms, now })
	p.run(func() string { return i18n.T("Couldn't seek") }, func(ctx context.Context) error { return p.svc.Seek(ctx, ms) })
}

// seekBy seeks by delta from the live position, within the song; a song
// that cannot seek (a live station) or has no length is left alone.
func (p *player) seekBy(delta time.Duration) {
	s := p.env.Play.State()
	if s.Current == nil || p.refuse(ctlSeek) || s.DurationMs <= 0 {
		return
	}
	to := min(max(p.env.Play.Position()+delta, 0), time.Duration(s.DurationMs)*time.Millisecond)
	p.seekTo(to)
}

// setVolume sets the volume (clamped to 0…1) and shows it at once.
func (p *player) setVolume(v float64) {
	v = min(max(v, 0), 1)
	p.env.Play.Patch(func(s *services.PlaybackState) { s.Volume = v })
	p.run(func() string { return i18n.T("Couldn't change the volume") }, func(ctx context.Context) error { return p.svc.SetVolume(ctx, v) })
}

// toggleMute mutes, or returns to the volume before.
func (p *player) toggleMute() {
	v := p.env.Play.State().Volume
	if v > 0 {
		p.env.Play.SetUnmuted(v)
		p.setVolume(0)
		return
	}
	p.setVolume(p.env.Play.Unmuted())
}

func (p *player) toggleShuffle() {
	s := p.env.Play.State()
	if s.Current == nil || p.refuse(ctlShuffle) {
		return
	}
	on := !s.Shuffle
	p.env.Play.Patch(func(s *services.PlaybackState) { s.Shuffle = on })
	p.run(func() string { return i18n.T("Couldn't change shuffle") }, func(context.Context) error {
		p.svc.SetShuffle(on)
		return nil
	})
}

// nextRepeat is off → all → one → off.
func nextRepeat(mode string) string {
	switch mode {
	case "off":
		return "all"
	case "all":
		return "one"
	}
	return "off"
}

func (p *player) cycleRepeat() {
	s := p.env.Play.State()
	if s.Current == nil || p.refuse(ctlRepeat) {
		return
	}
	mode := nextRepeat(s.Repeat)
	p.env.Play.Patch(func(s *services.PlaybackState) { s.Repeat = mode })
	p.run(func() string { return i18n.T("Couldn't change repeat") }, func(context.Context) error { return p.svc.SetRepeat(mode) })
}

// init subscribes (v0.2.0's index.ts eventHandlers): a new track on
// playback:state invalidates the library's "recent" queries.
func (p *player) init() {
	var last string
	store.On(p.env.Bus, services.EventPlaybackState, func(s services.PlaybackState) {
		id := ""
		if s.Current != nil {
			id = s.Current.ID
		}
		if id != "" && id != last {
			p.env.Queries.Invalidate(store.Key{"library", "recent"})
		}
		last = id
	})
	p.initBar()
}

// upNext is the Now Playing view's Up next pane: the list, then its foot,
// the summary (leave closes the view as a station link is followed) and the
// queue's tools.
func (p *player) upNext(c *ui.Context, leave func()) {
	ui.Column(c).Grow(1).Basis(0).MinHeight(0).Children(func() {
		p.queueList(c, true)
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() { p.queueSummary(c, leave) })
			p.queueTools(c)
		})
	})
}
