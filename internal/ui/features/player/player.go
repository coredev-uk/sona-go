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
	"github.com/coredev-uk/sona-go/internal/ui/module"
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
	// unmuted is the volume to return to on unmute (v0.2.0's `unmuted`).
	unmuted float64

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
	return module.Module{
		ID:        "player",
		Shortcuts: p.shortcuts(),
		Slots:     module.Slots{Bottom: p.bar},
	}
}

func newPlayer(env *module.Env, svc transport, volume func() bool) *player {
	return &player{env: env, svc: svc, volume: volume, now: time.Now, unmuted: 0.8}
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
	if s.Current == nil || s.DurationMs <= 0 || unavailable(s, ctlSeek) != "" {
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
		p.unmuted = v
		p.setVolume(0)
		return
	}
	p.setVolume(p.unmuted)
}

func (p *player) toggleShuffle() {
	s := p.env.Play.State()
	if s.Current == nil || unavailable(s, ctlShuffle) != "" {
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
	if s.Current == nil || unavailable(s, ctlRepeat) != "" {
		return
	}
	mode := nextRepeat(s.Repeat)
	p.env.Play.Patch(func(s *services.PlaybackState) { s.Repeat = mode })
	p.run(func() string { return i18n.T("Couldn't change repeat") }, func(context.Context) error { return p.svc.SetRepeat(mode) })
}
