package home

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The greeting's parts of the day, by the hour on the user's clock: morning
// from 05:00, afternoon from 12:00, evening from 18:00 until 05:00.
const (
	morningFrom   = 5
	afternoonFrom = 12
	eveningFrom   = 18
)

// greetingAt is the greeting for t and the next moment it changes.
func greetingAt(t time.Time) (text string, until time.Time) {
	boundary := func(hour, days int) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d+days, hour, 0, 0, 0, t.Location())
	}
	switch h := t.Hour(); {
	case h < morningFrom:
		return i18n.T("Good evening"), boundary(morningFrom, 0)
	case h < afternoonFrom:
		return i18n.T("Good morning"), boundary(afternoonFrom, 0)
	case h < eveningFrom:
		return i18n.T("Good afternoon"), boundary(eveningFrom, 0)
	}
	return i18n.T("Good evening"), boundary(morningFrom, 1)
}

// greeting is the line above the page's heading, unless Settings → General
// turned it off. It is decoration: the heading is still the page's, for
// screen readers and the window title. The view is asked back for the
// moment the greeting changes, so nothing runs between.
func (h *home) greeting(c *ui.Context) {
	cfg := store.UseSettings(h.env.Queries, h.env.Core)
	if !cfg.HasData || !cfg.Data.Home.Greeting {
		return
	}
	now := h.now()
	text, until := greetingAt(now)
	c.After(until.Sub(now) + time.Millisecond)
	theme.Text(c, text, "sub").TextColor(theme.C("ink-muted")).SingleLine().Margin(0, 0, 4, 0)
}
