package services

import (
	"context"
	"sync"

	"github.com/coredev-uk/sona-go/internal/desktop/notify"
)

// clickNotices shows the desktop notifications of the services that tell
// about a release (RemindersService, ArtistTrackerService) and keeps them,
// so a click is delivered after the call that showed one has returned:
// clicking one raises the window and emits "app:open" for its target. The
// notifications stay on screen when the service closes.
type clickNotices struct {
	e *Env

	mu    sync.Mutex
	shown []Notifier
}

// show shows a notification on its own notifier. image is an artwork
// template ("" for none), sized as the track notifications' art; label names
// the body click (it is the text of a notification's default action, which
// some systems show).
func (c *clickNotices) show(ctx context.Context, summary, body, image, label string, target OpenTarget) error {
	var file string
	if image != "" && c.e.Art != nil {
		if p, err := c.e.Art.LocalPath(ctx, image, mprisArtSize, mprisArtSize); err == nil {
			file = p
		} else {
			c.e.Logger.Debug("notification artwork", "err", err)
		}
	}
	n, err := c.e.Platform.Notifications(ctx, func(key string) {
		if key == notify.DefaultAction {
			c.open(target)
		}
	})
	if err != nil {
		return err
	}
	err = n.Show(ctx, notify.Notification{
		Summary: summary,
		Body:    body,
		Image:   file,
		Actions: []notify.Action{{Key: notify.DefaultAction, Label: label}},
		Urgency: notify.UrgencyNormal,
	})
	if err != nil {
		n.Close()
		return err
	}
	c.mu.Lock()
	c.shown = append(c.shown, n)
	c.mu.Unlock()
	return nil
}

// open brings the window up on target.
func (c *clickNotices) open(target OpenTarget) {
	if c.e.Focus != nil {
		c.e.Focus()
	}
	if c.e.Emit != nil {
		c.e.Emit.Emit(EventAppOpen, target)
	}
}

// close closes the notifiers: the notifications already shown stay on
// screen.
func (c *clickNotices) close() {
	c.mu.Lock()
	shown := c.shown
	c.shown = nil
	c.mu.Unlock()
	for _, n := range shown {
		n.Close()
	}
}
