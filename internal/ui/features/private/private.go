// Package private is private listening: Go's private mode mirrored from
// "private:changed" (the Settings row, the tray and `sona private` change it
// too), the player bar's switch (the Private pill) and a command for the
// palette, bindable in Settings → Shortcuts. It has no default keys: it is a
// deliberate switch, rarely flipped, and a stray key press would change what
// Apple records.
//
// The mirror is a store query (store.UsePrivate) so the account menu and
// Settings read the same state; the switch itself is kit.SetPrivate.
package private

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// controlOrder places the pill first in the player bar's right group
// (v0.2.0: PrivateButton, then the sleep timer, lyrics, the queue).
const controlOrder = 10

// private is the feature's state.
type private struct {
	env *module.Env
}

// New is the private listening module.
func New(env *module.Env) module.Module {
	p := &private{env: env}
	env.PlayerControls.Add(controlOrder, p.pill)
	return module.Module{
		ID:        "private",
		Shortcuts: []module.Shortcut{p.toggleShortcut()},
		Init:      p.init,
	}
}

// init mirrors Go's state: every "private:changed", and a snapshot that only
// fills the mirror while it is still empty, so an event that lands first wins.
func (p *private) init() {
	var snapshot func() services.PrivateState
	if p.env.Core != nil {
		snapshot = services.Service[services.PrivateService](p.env.Core).State
	}
	store.Mirror(p.env.Bus, p.env.Queries, p.env.Post, store.KeyPrivate, snapshot, services.EventPrivateChanged)
}

// state is the mirror, with no fetch: false until Go has answered.
func (p *private) state() (services.PrivateState, bool) {
	return store.Peek[services.PrivateState](p.env.Queries, store.KeyPrivate)
}

// set switches private listening; Go's "private:changed" follows.
func (p *private) set(on bool) { kit.SetPrivate(p.env.Queries, p.env.Core, on) }

// toggleShortcut is the palette command "Turn on/off private listening"
// (private.toggle): unavailable until Go reports the state and while
// SONA_PRIVATE=1 forces it on.
func (p *private) toggleShortcut() module.Shortcut {
	return module.Shortcut{
		ID: "private.toggle", Group: "playback", Icon: "hat-glasses",
		Label: func() string {
			if s, ok := p.state(); ok && s.On {
				return i18n.T("Turn off private listening")
			}
			return i18n.T("Turn on private listening")
		},
		Keywords: func() string { return i18n.T("private incognito history scrobble discord listening") },
		When: func() bool {
			s, ok := p.state()
			return ok && !store.PrivateForced(s)
		},
		Run: func() {
			if s, ok := p.state(); ok {
				p.set(!s.On)
			}
		},
	}
}

// Window widths, in DIPs, from which the pill shows "Private" (v0.2.0's
// min-[1100px]) and its development-build note (Tailwind's xl).
const (
	wordFrom = 1100
	noteFrom = 1280
)

// pill is the player bar's private listening switch. Off: a muted glyph like
// its neighbours. On: a chip with a 1 DIP dashed control-line outline, the
// glyph and "Private" in ink (never amber, which means "playing"), so it reads
// at a glance; the word drops below 1100 DIPs and the development-build note
// below 1280, where the dashed chip alone carries the state. Forced on by SONA_PRIVATE=1 it is unavailable
// to assistive technology (AccessDisabled) but stays focusable (clicks do
// nothing) so its tooltip can say why, and while Go has not answered it is
// dimmed.
func (p *private) pill(c *ui.Context) {
	r := store.UsePrivate(p.env.Queries, p.env.Core)
	st, known := r.Data, r.HasData
	on := known && st.On
	forced := known && store.PrivateForced(st)
	dev := known && st.Source == "development"

	// The accessible name is what the switch is and where an unchosen state
	// comes from; the toggle state carries on/off.
	name := i18n.T("Private listening")
	switch {
	case forced:
		// translators: No Apple Music web string; Apple's apps offer the inverse, 'Use Listening History'. SONA_PRIVATE=1 is an environment variable, keep as is.
		name = i18n.T("Private listening (forced by SONA_PRIVATE=1)")
	case dev:
		// translators: A development build of Sona, not a release
		name = i18n.T("Private listening (development build)")
	}
	tip := i18n.T("Turn on private listening")
	switch {
	case forced:
		tip = i18n.T("On because Sona was started with SONA_PRIVATE=1")
	case on && dev:
		tip = i18n.T("Private listening is on for this development build. Turning it off lasts until Sona quits.")
	case on:
		tip = i18n.T("Private listening is on: plays stay out of your listening history and recommendations, and aren't scrobbled, shown on Discord or added to History.")
	}
	var word string
	if w, _ := c.Size(); on && w >= wordFrom {
		// translators: Short label of the private listening switch while it is on
		word = i18n.T("Private")
		if dev && w >= noteFrom {
			// translators: Label of the private listening switch while a development build of Sona keeps it on
			word = i18n.T("Private (development build)")
		}
	}

	e := ui.ButtonBase(c.Key("private"))
	bg, fg := ui.Transparent, theme.C("ink-muted")
	switch {
	case on:
		fg = theme.C("ink")
		if e.Hovered() {
			bg = theme.C("accent-soft")
		}
	case e.Hovered():
		bg, fg = theme.C("accent-soft"), theme.C("ink")
	}
	e.Height(28).MinWidth(28).Gap(4).PaddingX(6).Radius(theme.Current().Radius["md"]).AlignItems(ui.Center).Justify(ui.Center).
		Background(bg).TextColor(fg).Role(ui.RoleToggleButton).Checked(on).Label(name)
	if word == "" {
		// The glyph alone is a square button, dashed outline included.
		e.Width(28).PaddingX(0)
	}
	if on {
		// "Off the record": a dashed outline, the mark of something that is
		// not being kept. Drawn by the element itself, in ink, never amber.
		e.Border(1, theme.C("control-line")).BorderStyle(ui.BorderDashed)
	}
	if !known {
		e.Opacity(0.4)
	}
	e.AccessDisabled(forced)
	theme.Fade(e, "feedback")
	e.Children(func() {
		ui.Icon(c, theme.Icon("hat-glasses")).FontSize(16)
		if word != "" {
			t := theme.Current().Type["meta"]
			ui.Text(c, word).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(500).SingleLine()
		}
	})
	kit.Tooltip(c, e, tip)
	if e.Clicked() && known && !forced {
		p.set(!on)
	}
}
