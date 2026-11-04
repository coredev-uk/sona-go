package settings

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// What a row is made of (SettingRow.tsx): the 1 DIP line above it, the title
// and what it does on the left, the control on the right (under the text
// when stacked), and the wash Find a setting puts on the row it lands on.
const (
	rowPadY       = 16
	rowWashInset  = 12  // the wash reaches this far past the text column
	rowGap        = 40  // between the text and the control
	rowTextWidth  = 440 // max-w-[60ch] at 13 DIP
	selectWidth   = 176 // SELECT_WIDTH: min-w-44 (a select grows with a long value)
	foundDuration = 2 * time.Second
	landDuration  = 3 * time.Second
)

// text is a Text in one of the theme's type styles ("body", "meta"…).
func text(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// rowOpts describe one setting row.
type rowOpts struct {
	// ID is the row's find ID (SettingEntry.ID, v0.2.0's data-setting): the
	// row Find a setting lands on.
	ID    string
	Title string
	// Desc says what the setting does, under the title.
	Desc string
	// Ink is the part of Desc set in ink instead of muted (SettingRow's
	// <b> in the window size row's figures).
	Ink string
	// Notes builds more of the text column under Desc (a live note, a
	// link); it is built inside the column.
	Notes func()
	// Stacked puts the control under the text, full width (wide inputs).
	Stacked bool
}

// row is a SettingRow: control builds the control (inside the row) and
// returns the element a find lands the focus on, usually its first
// control; nil for none. Name the control with Title (Label in Name) and
// it is described by Desc: row passes Desc to the returned element's
// accessible description.
func (s *settings) row(c *ui.Context, o rowOpts, control func() ui.Element) {
	c = c.Key("row:" + o.ID)
	s.seen[o.ID] = true
	col := ui.Column(c).FillWidth()
	col.Children(func() {
		s.line(c)
		in := ui.Row(c.Key("in")).FillWidth().Margin(0, -rowWashInset).Padding(rowPadY, rowWashInset).
			AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(rowGap).Radius(theme.Current().Radius["md"])
		if o.Stacked {
			in = ui.Column(c.Key("in")).FillWidth().Margin(0, -rowWashInset).Padding(rowPadY, rowWashInset).
				Gap(12).Radius(theme.Current().Radius["md"])
		}
		s.wash(in, o.ID)
		in.Children(func() {
			var title ui.Element
			ui.Column(c.Key("text")).Grow(1).Shrink(1).MinWidth(0).MaxWidth(rowTextWidth).Children(func() {
				title = text(c, o.Title, "label").TextColor(theme.C("ink"))
				if o.Desc != "" {
					descText(c, o.Desc, o.Ink)
				}
				if o.Notes != nil {
					o.Notes()
				}
			})
			var first ui.Element
			if o.Stacked {
				ui.Column(c.Key("control")).FillWidth().Children(func() { first = control() })
			} else {
				ui.Row(c.Key("control")).Shrink(0).AlignItems(ui.Center).Gap(8).Children(func() { first = control() })
			}
			if !isNone(first) && o.Desc != "" {
				first.Description(o.Desc)
			}
			// A click on the title goes to the control, as a label's does
			// (MyGo has no label-for): the control takes the focus.
			if !isNone(first) && title.Clicked() {
				first.Focus()
			}
			s.land(c, o.ID, in, first)
		})
	})
}

// descText is a row's description: muted, with ink (when it appears in desc)
// in ink.
func descText(c *ui.Context, desc, ink string) {
	before, after, found := strings.Cut(desc, ink)
	if ink == "" || !found {
		text(c, desc, "secondary").TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0)
		return
	}
	t := theme.Current().Type["secondary"]
	ui.RichText(c.Key("desc")).FontSize(t.Size).FixedLineHeight(t.LineHeight).TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0).Children(func() {
		ui.Text(c.Key("before"), before)
		ui.Text(c.Key("ink"), ink).TextColor(theme.C("ink"))
		ui.Text(c.Key("after"), after)
	})
}

// unavailableRow is an UnavailableRow: a setting this platform can't back
// keeps its place and title and says why there is no control. Find can land
// on it (it takes the focus, but is not a Tab stop). An empty reason says
// the platform doesn't have it yet.
func (s *settings) unavailableRow(c *ui.Context, id, title, reason string) {
	c = c.Key("row:" + id)
	s.seen[id] = true
	if reason == "" {
		reason = i18n.T("Not available on {platform} yet.", "platform", s.platformName())
	}
	ui.Column(c).FillWidth().Children(func() {
		s.line(c)
		in := ui.Column(c.Key("in")).FillWidth().Margin(0, -rowWashInset).Padding(rowPadY, rowWashInset).
			Radius(theme.Current().Radius["md"]).Role(ui.RoleGroup).Label(title).Description(reason)
		s.wash(in, id)
		in.Children(func() {
			text(c, title, "label").TextColor(theme.C("ink-muted"))
			ui.Row(c.Key("why")).Gap(6).AlignItems(ui.Start).Margin(2, 0, 0, 0).MaxWidth(rowTextWidth).Children(func() {
				ui.Icon(c, theme.Icon("circle-slash")).FontSize(14).TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0).Shrink(0)
				text(c, reason, "secondary").TextColor(theme.C("ink-muted")).Shrink(1).MinWidth(0)
			})
		})
		s.land(c, id, in, ui.Element{})
	})
}

// line is the 1 DIP line above a row.
func (s *settings) line(c *ui.Context) {
	ui.Box(c.Key("line")).FillWidth().Height(1).Shrink(0).Background(theme.C("line"))
}

// group is a SettingsGroup: a section within a page section: a heading (an
// h3 under the section's h2), optionally a line about the whole group, then
// its rows.
func (s *settings) group(c *ui.Context, id, title, desc string, rows func()) {
	c = c.Key("group:" + id)
	ui.Column(c).FillWidth().Gap(12).Role(ui.RoleGroup).Label(title).Children(func() {
		ui.Column(c.Key("head")).FillWidth().Children(func() {
			text(c, title, "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(3)
			if desc != "" {
				text(c, desc, "secondary").TextColor(theme.C("ink-muted")).Margin(4, 0, 0, 0).MaxWidth(rowTextWidth + 40)
			}
		})
		ui.Column(c.Key("rows")).FillWidth().Children(rows)
	})
}

// stack is SettingsSections: a section's groups (and any untitled block
// before them) at one spacing.
func (s *settings) stack(c *ui.Context, build func()) {
	ui.Column(c.Key("stack")).FillWidth().Gap(40).Children(build)
}

// loading is SettingsLoading: a section still loading, in its rows' shape:
// flat bars at the real text sizes, announced busy.
func (s *settings) loading(c *ui.Context, rows int) {
	ui.Column(c.Key("loading")).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		ui.Box(c.Key("h")).Width(112).Height(16).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover")).Margin(0, 0, 12, 0)
		for i := range rows {
			k := c.Key(i)
			ui.Column(k).FillWidth().Children(func() {
				s.line(k)
				ui.Row(k.Key("r")).FillWidth().Padding(rowPadY, 0).AlignItems(ui.Center).Justify(ui.SpaceBetween).Gap(rowGap).Children(func() {
					ui.Column(k.Key("l")).Gap(8).Children(func() {
						ui.Box(k.Key("t")).Width(160).Height(14).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover"))
						ui.Box(k.Key("d")).Width(float32(220 + (i*53)%140)).Height(12).Radius(theme.Current().Radius["sm"]).Background(theme.C("hover"))
					})
					ui.Box(k.Key("c")).Width(selectWidth).Height(32).Radius(theme.Current().Radius["md"]).Background(theme.C("hover"))
				})
			})
		}
	})
}

// platformName is the platform's name as people know it (platform.ts), from
// Capabilities.OS.
func (s *settings) platformName() string {
	switch os := s.caps().Data.OS; os {
	case "darwin":
		return i18n.T("macOS")
	case "windows":
		return i18n.T("Windows")
	case "linux":
		return i18n.T("Linux")
	default:
		return os
	}
}

// landing is a pending Find a setting landing (find.ts focusSetting): the
// row's ID, how long the page waits for the row to be built, and the wash.
type landing struct {
	id    string
	until time.Time // wait for the row until
}

// found is the row washed after a landing.
type found struct {
	id    string
	until time.Time
}

// focusSetting moves the focus to a setting's control (the row when it has
// none), scrolls it into view and washes the row for a moment (find.ts
// focusSetting). The row may not be built yet (the section is changing, or
// loading its data), so the request waits for it, frame by frame, for up to
// three seconds; a later request replaces an earlier one.
func (s *settings) focusSetting(id string) {
	s.pending = &landing{id: id, until: s.now().Add(landDuration)}
}

// land is called by a row as it builds: if Find asked for it, it scrolls
// the row in, focuses its control (or the row) and starts the wash.
func (s *settings) land(c *ui.Context, id string, row ui.Element, control ui.Element) {
	p := s.pending
	if p == nil || p.id != id {
		return
	}
	s.pending = nil
	row.ScrollIntoView()
	if !isNone(control) {
		control.Focus()
	} else {
		row.Focusable().Focus()
	}
	s.found = found{id: id, until: s.now().Add(foundDuration)}
	c.After(foundDuration)
}

// wash colours a row while it is the one Find landed on: the hover colour,
// eased in and out over 150 ms (data-found).
func (s *settings) wash(row ui.Element, id string) {
	on := float32(0)
	if s.found.id == id && s.now().Before(s.found.until) {
		on = 1
	}
	if a := row.Animate("wash", on, 150*time.Millisecond); a > 0 {
		row.Background(theme.C("hover").Alpha(a))
	}
}

// isNone reports whether e is the zero Element: no control to focus.
func isNone(e ui.Element) bool { return e == (ui.Element{}) }

// switchRow is a row with a switch: the control most settings are. The
// switch is named by the title; set gets the new value on a click.
func (s *settings) switchRow(c *ui.Context, o rowOpts, on, disabled bool, set func(bool)) {
	s.row(c, o, func() ui.Element {
		v := on
		sw := kit.Switch(c, &v, kit.SwitchOpts{A11yLabel: o.Title, Disabled: disabled})
		if sw.Changed() {
			set(v)
		}
		return sw
	})
}

// selectRow is a row with a select (SELECT_WIDTH: 176 DIP at least, 288 at
// most, growing with a long value instead of cutting it). set gets the
// chosen value.
func (s *settings) selectRow(c *ui.Context, o rowOpts, value string, items []kit.SelectItem, disabled bool, set func(string)) {
	s.row(c, o, func() ui.Element {
		v := value
		sel := kit.Select(c, &v, items, kit.SelectOpts{Small: true, A11yLabel: o.Title, Disabled: disabled, Width: selectWidthFor(items, value)})
		if sel.Changed() && v != value {
			set(v)
		}
		return sel
	})
}

// selectWidthFor is the width of a select showing the chosen item: 176 to
// 288 DIP, by the length of its label.
func selectWidthFor(items []kit.SelectItem, value string) float32 {
	n := 0
	for _, it := range items {
		if it.Value == value {
			n = utf8.RuneCountInString(it.Label)
		}
	}
	return min(max(float32(n)*7.5+56, selectWidth), 288)
}
