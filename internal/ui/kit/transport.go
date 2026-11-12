package kit

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// PlaybackService is what Transport asks of PlaybackService
// (*services.PlaybackService).
type PlaybackService interface {
	Toggle(ctx context.Context) error
	Next(ctx context.Context) error
	Prev(ctx context.Context) error
	Seek(ctx context.Context, positionMs int64) error
	SetVolume(ctx context.Context, volume float64) error
}

// Transport is the playback command set the bar, the video view and the mini
// player share: play or pause, previous, next, seek and volume. Each command
// shows its result in the mirror at once (until Go's next snapshot) and runs
// the service off the main thread; a failure goes to Fail with what the
// command was doing, in the app's words. What Fail does (a toast, a notice)
// and which commands apply (the guards) stay with the caller.
type Transport struct {
	Svc  PlaybackService
	Play *store.Playback
	// Fail is called off the main thread with a failed command's title; nil
	// toasts it (ToastFailure).
	Fail func(what string, err error)
}

// run starts fn off the main thread.
func (t Transport) run(what string, fn func(ctx context.Context) error) {
	go func() {
		if err := fn(context.Background()); err != nil {
			if t.Fail == nil {
				ToastFailure(what, err)
				return
			}
			t.Fail(what, err)
		}
	}()
}

// The commands each say what they were doing (the failure's title) as a
// literal, so the catalogue's extraction finds it.

// Toggle plays or pauses.
func (t Transport) Toggle() {
	t.run(i18n.T("Couldn't play or pause"), t.Svc.Toggle)
}

// Next skips to the next song.
func (t Transport) Next() {
	t.run(i18n.T("Couldn't skip"), t.Svc.Next)
}

// Prev goes back.
func (t Transport) Prev() {
	t.run(i18n.T("Couldn't go back"), t.Svc.Prev)
}

// SeekTo seeks to a position in what plays; now stamps the mirror's position.
func (t Transport) SeekTo(pos time.Duration, now time.Time) {
	ms := pos.Milliseconds()
	at := now.UnixMilli()
	t.Play.Patch(func(s *services.PlaybackState) { s.PositionMs, s.At = ms, at })
	t.run(i18n.T("Couldn't seek"), func(ctx context.Context) error { return t.Svc.Seek(ctx, ms) })
}

// SeekBy seeks by delta from the live position, within the song. Nothing that
// plays, no length, or refuse reporting true (called once there is something
// to seek in; a guard that says why it refuses) leaves it alone.
func (t Transport) SeekBy(delta time.Duration, now time.Time, refuse func() bool) {
	s := t.Play.State()
	if s.Current == nil || (refuse != nil && refuse()) || s.DurationMs <= 0 {
		return
	}
	t.SeekTo(min(max(t.Play.Position()+delta, 0), time.Duration(s.DurationMs)*time.Millisecond), now)
}

// SetVolume sets the volume (clamped to 0…1) and shows it at once.
func (t Transport) SetVolume(v float64) {
	v = min(max(v, 0), 1)
	t.Play.Patch(func(s *services.PlaybackState) { s.Volume = v })
	t.run(i18n.T("Couldn't change the volume"), func(ctx context.Context) error { return t.Svc.SetVolume(ctx, v) })
}

// ToggleMute mutes, or returns to the volume before.
func (t Transport) ToggleMute() {
	if v := t.Play.State().Volume; v > 0 {
		t.Play.SetUnmuted(v)
		t.SetVolume(0)
		return
	}
	t.SetVolume(t.Play.Unmuted())
}
