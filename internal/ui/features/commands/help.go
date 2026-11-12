package commands

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// helpSheet is the "?" sheet: every shortcut by group with its current keys,
// under a filter that narrows them as you type.
type helpSheet struct {
	open bool
	// frames counts the frames the sheet has been open; the filter takes the
	// focus on the second, once the dialog has noted where the focus was.
	frames int
	// slash is "?" where it comes with no Key (GTK, AppKit), which the
	// sheet observes to close on its own chord.
	slash kit.ShiftSlash
	// chord notes a press of Shift and the slash key, whatever has the focus.
	chord bool
	// query is the filter's text; last the one the list was built for and
	// announced the count of.
	query, last string
	// scroll is how far the list is scrolled: it goes back to the top with
	// the query, and Up at the top returns the focus to the filter.
	scroll ui.ScrollState
}

// narrow is the window width below which the groups list in one column.
const narrow = 720

const (
	// helpRowHeight is a shortcut's row.
	helpRowHeight = 32
	// notSetWidth is about how wide "Not set" is in 12 DIP type.
	notSetWidth = 48
)

// helpGroup is one group of the sheet with the shortcuts that show in it
// and the width of its key column.
type helpGroup struct {
	group module.ShortcutGroup
	list  []module.BoundShortcut
	keysW float32
}

// show draws the sheet while it is open: a dialog (820 wide, at most 80 % of
// the window high, 10 % from the top so the filter stays where it is as the
// list shortens) with the title and the filter, the groups, and a foot with
// the line about Settings and the keys that move around. The filter takes
// the focus on opening; Down moves into the list, which scrolls with the
// arrows and Page Down, and Up at its top comes back.
func (h *helpSheet) show(c *ui.Context, env *module.Env) {
	if !h.open {
		h.frames, h.query, h.last, h.chord = 0, "", "", false
		h.scroll = ui.ScrollState{}
		h.slash.Take(c) // a "?" that came as it closed another way is not the next opening's
		return
	}
	w, wh := c.Size()
	closeOwn := func() {
		keys := env.KeysOf("commands.help")
		// "?" pressed in the filter is text to MyGo, so the chord never
		// reaches the sheet from there, where the focus starts: the dialog
		// notes the press itself (observe), and it is the chord while the
		// filter holds nothing but it; a "?" after other text is text.
		q := strings.TrimSpace(h.query)
		chord := slices.ContainsFunc(keys, kit.IsShiftSlashChord) && (q == "?" || (h.chord && q == ""))
		h.chord = false
		if h.slash.Pressed(c, keys...) || chord {
			h.open = false
		}
	}
	panel := kit.Dialog(c.Key("help"), &h.open, kit.DialogOpts{
		Title: i18n.T("Keyboard shortcuts"), Description: plainAbout(), HideHeader: true, MaxWidth: 820, Top: wh * 0.1, Padding: 0, Gap: 0, Bare: true, Raised: true,
	}, func() {
		closeOwn() // the sheet's own chord closes it again, which only the dialog sees
		var field ui.Element
		ui.Row(c.Key("help:head")).FillWidth().Padding(18, 56, 16, 24).Gap(16).AlignItems(ui.Center).Children(func() {
			theme.Text(c.Key("title"), i18n.T("Keyboard shortcuts"), "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(2).Grow(1).MinWidth(0)
			ui.Row(c.Key("help:filter")).Width(268).MinWidth(150).Shrink(1).Children(func() {
				field = kit.SearchField(c.Key("filter"), &h.query, kit.SearchFieldOpts{Placeholder: i18n.T("Filter shortcuts")})
			})
		})
		down := field.Shortcut(0, ui.KeyDown)
		ui.Box(c.Key("help:sep")).Height(1).FillWidth().Background(theme.C("line"))

		shown := h.filter(env)
		h.announce(c, shown)
		list := ui.Scroll(c.Key("help:list")).TrackScroll(&h.scroll).Role(ui.RoleGroup).Label(i18n.T("Shortcuts")).
			FillWidth().MinHeight(min(wh*0.8, 720)/2).MaxHeight(min(wh*0.8, 720)-104).Padding(8, 24, 4)
		list.Children(func() {
			if len(shown) == 0 {
				h.none(c)
				return
			}
			h.groups(c, shown, w)
		})
		switch {
		case down:
			list.Focus()
		case h.scroll.Y <= 0 && list.Shortcut(0, ui.KeyUp):
			field.Focus()
		}
		if h.frames++; h.frames == 2 {
			field.Focus()
		} else if h.frames < 2 {
			c.AnimationFrame()
		}

		ui.Box(c.Key("help:sep2")).Height(1).FillWidth().Background(theme.C("line"))
		ui.Row(c.Key("help:foot")).Height(40).FillWidth().Padding(0, 24).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Row(c.Key("about")).Grow(1).MinWidth(0).Children(func() { h.about(c, env) })
			hint := func(key, label string, chord string) {
				ui.Row(c.Key("hint:" + key)).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
					kit.Keycaps(c, chord)
					theme.Text(c.Key("l"), label, "meta").TextColor(theme.C("ink-faint")).SingleLine()
				})
			}
			hint("list", i18n.T("Browse"), "ArrowDown")
			hint("close", i18n.T("Close"), "Escape")
		})
	})
	panel.ObserveInput(h.observe)
}

// observe is the dialog's ObserveInput: it lets the ShiftSlash see "?" by
// position and notes any press of Shift and the slash key (by position or
// with its Key) for closeOwn.
func (h *helpSheet) observe(ev ui.InputEvent) {
	h.slash.Observe(ev)
	if ev.Kind == ui.InputKeyDown && ev.Mods == ui.Shift && !ev.Repeat &&
		(ev.Key == ui.KeySlash || (ev.Key == ui.KeyUnknown && ev.Code == ui.CodeSlash)) {
		h.chord = true
	}
}

// about is the line in the foot: "Change any of them in Settings.", the
// word a link to the shortcut settings where that page exists.
func (h *helpSheet) about(c *ui.Context, env *module.Env) {
	msg := i18n.T("Change any of them in <0>Settings</0>.")
	before, rest, _ := strings.Cut(msg, "<0>")
	name, after, _ := strings.Cut(rest, "</0>")
	// Spaces at the seams become the row's gap: the text measures without them.
	gap := float32(0)
	if strings.HasSuffix(before, " ") || strings.HasPrefix(name, " ") {
		gap = 4
	}
	before = strings.TrimSpace(before)
	ui.Row(c.Key("about:line")).AlignItems(ui.Center).Gap(gap).Children(func() {
		theme.Text(c.Key("a"), before, "secondary").TextColor(theme.C("ink-muted")).SingleLine().Shrink(0)
		ui.Row(c.Key("link")).AlignItems(ui.Center).Children(func() { h.link(c, env, name, after) })
	})
}

// link is the word that links to Settings (plain text where that page does
// not exist), and what follows it.
func (h *helpSheet) link(c *ui.Context, env *module.Env, name, after string) {
	if env.Routed("/settings") {
		link := ui.ButtonBase(c.Key("settings")).Role(ui.RoleLink).Label(name).Cursor(ui.CursorPointer)
		link.Children(func() {
			theme.Text(c, name, "secondary").TextColor(theme.C("ink")).Underline().SingleLine().Shrink(0)
		})
		if link.Clicked() {
			h.open = false
			env.Nav.Push("/settings?section=shortcuts")
		}
	} else {
		theme.Text(c.Key("s"), name, "secondary").TextColor(theme.C("ink-muted")).SingleLine().Shrink(0)
	}
	theme.Text(c.Key("b"), after, "secondary").TextColor(theme.C("ink-muted")).SingleLine().Shrink(0)
}

// filter is the groups with the shortcuts the query leaves, in the order of
// groups; groups with none are left out. A shortcut matches when every word
// of the query is in its label, its keywords, its group's title or the
// chords it answers to now (spelt as the caps show them and as they are
// spoken), ignoring case and accents.
func (h *helpSheet) filter(env *module.Env) []helpGroup {
	var all []module.BoundShortcut
	if env.Shortcuts != nil {
		all = env.Shortcuts()
	}
	words := strings.Fields(i18n.Fold(h.query))
	var out []helpGroup
	for _, g := range groups {
		var list []module.BoundShortcut
		for _, s := range all {
			if s.Group == g && matchesWords(s, words) {
				list = append(list, s)
			}
		}
		if len(list) == 0 {
			continue
		}
		var keysW float32
		for _, s := range list {
			if len(s.Keys) == 0 {
				keysW = max(keysW, notSetWidth)
			}
			keysW = max(keysW, keycapsWidth(s.Keys))
		}
		out = append(out, helpGroup{group: g, list: list, keysW: keysW})
	}
	return out
}

// matchesWords reports whether every word is in the shortcut's text.
func matchesWords(s module.BoundShortcut, words []string) bool {
	if len(words) == 0 {
		return true
	}
	var b strings.Builder
	b.WriteString(s.Label())
	if s.Keywords != nil {
		b.WriteString(" " + s.Keywords())
	}
	b.WriteString(" " + groupTitle(s.Group))
	for _, chord := range s.Keys {
		b.WriteString(" " + kit.ChordText(chord) + " " + strings.Join(kit.ChordParts(chord), " "))
	}
	text := i18n.Fold(b.String())
	for _, w := range words {
		if !strings.Contains(text, w) {
			return false
		}
	}
	return true
}

// keycapsWidth is about how wide kit.Keycaps draws chords: a 20 DIP cap or
// its text with 10 DIPs round it (12 DIP type, tabular), 2 between caps and
// 6 between chords. It sizes a group's key column, so the caps of its rows
// start on one edge, without measuring text.
func keycapsWidth(chords []string) float32 {
	var w float32
	for i, chord := range chords {
		if i > 0 {
			w += 6
		}
		for j, part := range kit.ChordParts(chord) {
			if j > 0 {
				w += 2
			}
			w += max(20, 10+7*float32(utf8.RuneCountInString(part)))
		}
	}
	return w
}

// announce says how many shortcuts a typed query leaves, once for each
// change of the query.
func (h *helpSheet) announce(c *ui.Context, shown []helpGroup) {
	if h.query == h.last {
		return
	}
	h.last = h.query
	h.scroll.Y = 0
	q := strings.TrimSpace(h.query)
	if q == "" {
		return
	}
	n := 0
	for _, g := range shown {
		n += len(g.list)
	}
	if n == 0 {
		c.Announce(i18n.T("No shortcuts match “{term}”", "term", q))
		return
	}
	c.Announce(i18n.T("{count, plural, one {# shortcut} other {# shortcuts}}", "count", n))
}

// none is the list when the filter leaves nothing.
func (h *helpSheet) none(c *ui.Context) {
	ui.Column(c.Key("help:none")).FillWidth().Padding(56, 0).Gap(4).AlignItems(ui.Center).Children(func() {
		theme.Text(c.Key("t"), i18n.T("No shortcuts match “{term}”", "term", strings.TrimSpace(h.query)), "label").TextColor(theme.C("ink")).Role(ui.RoleStatus)
		theme.Text(c.Key("d"), i18n.T("Try part of a command's name, or a key."), "secondary").TextColor(theme.C("ink-muted"))
	})
}

// groups lists the groups in two columns (one when the window is narrow),
// the first filled before the second and a group never split.
func (h *helpSheet) groups(c *ui.Context, sections []helpGroup, width float32) {
	rows := 0
	for _, s := range sections {
		rows += len(s.list) + 1
	}
	cols := [][]helpGroup{sections}
	if width >= narrow {
		cols = [][]helpGroup{nil, nil}
		n := 0
		for _, s := range sections {
			col := 0
			if n >= rows/2 {
				col = 1
			}
			cols[col] = append(cols[col], s)
			n += len(s.list) + 1
		}
	}
	ui.Row(c.Key("help:cols")).Gap(40).AlignItems(ui.Start).FillWidth().Children(func() {
		for i, col := range cols {
			ui.Column(c.Key("col:" + string(rune('0'+i)))).Grow(1).Basis(0).MinWidth(0).Children(func() {
				for _, s := range col {
					h.section(c, s)
				}
			})
		}
	})
}

// section is one group: its title over a hairline, then a row for each
// shortcut, its label and its caps (or "Not set") in the group's key column.
func (h *helpSheet) section(c *ui.Context, g helpGroup) {
	ui.Column(c.Key("g:"+string(g.group))).FillWidth().Padding(0, 0, 24).Role(ui.RoleGroup).Label(groupTitle(g.group)).Children(func() {
		theme.Text(c.Key("h"), groupTitle(g.group), "body").FontWeight(600).TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(3).Padding(0, 0, 6)
		ui.Box(c.Key("rule")).Height(1).FillWidth().Background(theme.C("line"))
		for _, s := range g.list {
			ui.Row(c.Key("s:"+s.ID)).MinHeight(helpRowHeight).FillWidth().Padding(2, 0).Gap(16).AlignItems(ui.Center).Role(ui.RoleListItem).Label(s.Label()).Children(func() {
				theme.Text(c.Key("l"), s.Label(), "body").TextColor(theme.C("ink")).Grow(1).MinWidth(0)
				ui.Row(c.Key("keys")).Width(g.keysW).MinWidth(g.keysW).Shrink(0).AlignItems(ui.Center).Children(func() {
					if len(s.Keys) > 0 {
						kit.Keycaps(c.Key("k"), s.Keys...)
					} else {
						theme.Text(c.Key("n"), i18n.T("Not set"), "meta").TextColor(theme.C("ink-faint")).SingleLine()
					}
				})
			})
		}
	})
}

// plainAbout is the line in the foot without its link markup, for the
// dialog's description.
func plainAbout() string {
	return strings.NewReplacer("<0>", "", "</0>", "").Replace(i18n.T("Change any of them in <0>Settings</0>."))
}
