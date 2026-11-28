package home

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
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

// greetingAt is the greeting for t and the next moment it changes. name is
// the person's first name from their Apple Music profile, "" for none.
func greetingAt(t time.Time, name string) (text string, until time.Time) {
	boundary := func(hour, days int) time.Time {
		y, m, d := t.Date()
		return time.Date(y, m, d+days, hour, 0, 0, 0, t.Location())
	}
	switch h := t.Hour(); {
	case h < morningFrom:
		return goodEvening(name), boundary(morningFrom, 0)
	case h < afternoonFrom:
		return goodMorning(name), boundary(afternoonFrom, 0)
	case h < eveningFrom:
		return goodAfternoon(name), boundary(eveningFrom, 0)
	}
	return goodEvening(name), boundary(morningFrom, 1)
}

// The greetings are whole sentences, one per part of the day and with or
// without a name, so a language can set the name where it wants it.

func goodMorning(name string) string {
	if name == "" {
		return i18n.T("Good morning")
	}
	return i18n.T("Good morning, {name}", "name", name)
}

func goodAfternoon(name string) string {
	if name == "" {
		return i18n.T("Good afternoon")
	}
	return i18n.T("Good afternoon, {name}", "name", name)
}

func goodEvening(name string) string {
	if name == "" {
		return i18n.T("Good evening")
	}
	return i18n.T("Good evening, {name}", "name", name)
}

// homeSettings are Settings → General's choices for this page. known is
// false until the settings have been read: the defaults stand in (the
// greeting on), and the greeting holds its line rather than guess the name.
func (h *home) homeSettings() (s config.Home, known bool) {
	cfg := store.UseSettings(h.env.Queries, h.env.Core)
	if !cfg.HasData {
		return config.Defaults().Home, false
	}
	return cfg.Data.Home, true
}

// profileName is the first name the greeting uses, from Go's SQLite copy
// (never a request: ProfileService fetches it after the first paint). ready
// is false until that read has answered, so the greeting holds its line
// instead of changing its words a frame later.
func (h *home) profileName() (name string, ready bool) {
	res := store.Use(h.env.Queries, keyGreetingName, h.prof.GreetingName)
	return res.Data.Name, res.HasData || res.Err != nil
}

// greeting is the masthead's opening statement at the display size, unless
// Settings → General turned it off: "Good evening", or "Good evening, Alex"
// when the person has an Apple Music profile and allows the name (s, the
// settings, known once read). It is one text, so a screen reader says it
// whole and before the page's heading, which stays "For You", level 1, and
// the window title; the name is never read on its own. The view is asked
// back for the moment the greeting changes, so nothing runs between.
func (h *home) greeting(c *ui.Context, s config.Home, known bool) {
	name, ready := "", known
	if known && s.UseName {
		name, ready = h.profileName()
	}
	now := h.now()
	text, until := greetingAt(now, name)
	c.After(until.Sub(now) + time.Millisecond)
	if !ready {
		line := theme.Current().Type["display"].LineHeight
		ui.Column(c.Key("greeting")).Height(line).Margin(0, 0, 4, 0).Shrink(0)
		return
	}
	theme.Text(c, text, "display").TextColor(theme.C("ink")).MaxLines(2).MinWidth(0).Margin(0, 0, 4, 0)
}
