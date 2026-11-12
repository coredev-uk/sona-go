package settings

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// shortcutGroups is how shortcuts are listed in Settings, in this order
// (shared/module.ts shortcutGroups).
var shortcutGroups = []module.ShortcutGroup{"playback", "view", "pages", "navigation", "general"}

// The section's geometry (Shortcuts.tsx): the groups run in two columns
// where the section is at least 46rem wide.
const (
	twoColumnsWidth = 736 // @min-[46rem]
	columnGap       = 40  // gap-x-10
	recorderWidth   = 112 // min-w-28
	shortcutGap     = 24  // gap-6 between a row's name and its keys
	shortcutHeight  = 32  // min-h-8, h-8
	shortcutRowPad  = 6   // py-1.5
	groupPadBottom  = 32  // pb-8
	groupHeading    = 30  // the h3 and its mb-2, for the columns' balance
	shortcutRowFull = 45  // a row and its line, likewise
)

// shortcutsState is the section's own state: the filter.
type shortcutsState struct {
	filter string
}

// recorder is one row's own state (ShortcutRow's useState): whether its
// keys are being recorded, why a key was refused, and a chord waiting for
// the user to settle a conflict.
type recorder struct {
	recording bool
	refused   string
	// refusals counts the refusals, so the same refusal twice is a new
	// announcement.
	refusals int
	pending  *pendingChord
	// refocus asks the next frame to put the focus back on the recorder:
	// what it was in goes with the choice or the reset button.
	refocus bool
	// slash is a "?" the recorder took as it came (kit.IsShiftSlash): on GTK
	// and AppKit it has no Key, which no Shortcut of its own can ask for.
	slash bool
}

// pendingChord is a chord waiting for the user to settle a conflict.
type pendingChord struct {
	chord string
	with  []module.Shortcut
}

func (r *recorder) stop() { r.recording, r.refused, r.slash = false, "", false }

// shortcutView is what a frame of the section works from.
type shortcutView struct {
	all        []module.Shortcut
	overrides  config.Shortcuts
	bind       bindings
	conflicted map[string]bool
}

// shortcutsSection is Settings → Shortcuts: every registered shortcut by
// group with its keys. Click a binding and press the new keys; a key
// another shortcut uses in an overlapping scope asks first and moves it
// here. Changes save to Go's settings (shortcuts) and apply at once.
func (s *settings) shortcutsSection(c *ui.Context) {
	cfg := s.cfg()
	if !cfg.HasData {
		if cfg.Err != nil {
			kit.QueryError(c.Key("err"), s.env.Queries, i18n.T("Couldn't read Sona's settings"), cfg, store.KeySettings)
			return
		}
		s.loading(c, 8)
		return
	}
	ss := stateOf[shortcutsState](s, "shortcuts")
	all := shortcutsOf(s.shortcuts())
	bind := resolveBindings(all, cfg.Data.Shortcuts)
	v := &shortcutView{all: all, overrides: cfg.Data.Shortcuts, bind: bind, conflicted: conflictingIds(all, bind)}

	query := strings.TrimSpace(ss.filter)
	shown := filterShortcuts(all, bind, query)
	ui.Column(c.Key("shortcuts")).FillWidth().Gap(24).Children(func() {
		s.shortcutTools(c, ss, len(v.overrides) == 0)
		s.shortcutColumns(c, groupShortcuts(shown), v)
		s.shortcutStatus(c, len(shown) == 0, query)
	})
}

// shortcutsOf are the registered shortcuts of Env.Shortcuts, with their
// default keys.
func shortcutsOf(list []module.BoundShortcut) []module.Shortcut {
	out := make([]module.Shortcut, len(list))
	for i, b := range list {
		out[i] = b.Shortcut
	}
	return out
}

// shortcutLabel is what a shortcut does, in words.
func shortcutLabel(sh module.Shortcut) string {
	if sh.Label != nil {
		return sh.Label()
	}
	return sh.ID
}

// filterShortcuts are the shortcuts whose action or keys hold query (any
// case): the keys as spoken, "Ctrl+Left Arrow".
func filterShortcuts(all []module.Shortcut, b bindings, query string) []module.Shortcut {
	f := strings.ToLower(query)
	if f == "" {
		return all
	}
	var out []module.Shortcut
	for _, sh := range all {
		if strings.Contains(strings.ToLower(shortcutLabel(sh)), f) ||
			slices.ContainsFunc(b[sh.ID], func(c string) bool { return strings.Contains(strings.ToLower(kit.ChordText(c)), f) }) {
			out = append(out, sh)
		}
	}
	return out
}

// shortcutTools is the filter and Reset all. Reset all stays a button the
// keyboard can reach when there is nothing to reset (a Reason, so that after
// a reset it keeps the focus instead of dropping it): dimmed, and a click does
// nothing.
func (s *settings) shortcutTools(c *ui.Context, ss *shortcutsState, nothingToReset bool) {
	ui.Row(c.Key("tools")).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Column(c.Key("filter")).Grow(1).MinWidth(0).Children(func() {
			kit.SearchField(c, &ss.filter, kit.SearchFieldOpts{
				Placeholder: i18n.T("Filter by action or key"), A11yLabel: i18n.T("Filter shortcuts"),
			})
		})
		// With nothing to reset it keeps the focus and says so.
		reason := ""
		if nothingToReset {
			reason = i18n.T("No shortcut has been changed.")
		}
		b := kit.Button(c.Key("reset-all"), i18n.T("Reset all"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Reason: reason})
		if b.Clicked() && !nothingToReset {
			kit.Confirm(kit.ConfirmOptions{
				Title:        i18n.T("Reset every shortcut?"),
				Body:         i18n.T("All shortcuts go back to their default keys."),
				ConfirmLabel: i18n.T("Reset all"),
			}, func(ok bool) {
				if ok {
					s.save(func(st *config.Settings) { st.Shortcuts = config.Shortcuts{} })
				}
			})
		}
	})
}

// shortcutGroup is the shortcuts of one group, in registration order.
type shortcutGroup struct {
	group module.ShortcutGroup
	list  []module.Shortcut
}

// groupShortcuts splits shown by group, in the order the groups are listed;
// a group with nothing in it is left out.
func groupShortcuts(shown []module.Shortcut) []shortcutGroup {
	var out []shortcutGroup
	for _, g := range shortcutGroups {
		var list []module.Shortcut
		for _, sh := range shown {
			if sh.Group == g {
				list = append(list, sh)
			}
		}
		if len(list) > 0 {
			out = append(out, shortcutGroup{g, list})
		}
	}
	return out
}

// shortcutColumns lays the groups out: one column, or where the section is
// wide enough two, the groups running on from the first to the second with
// the break where the columns come out nearest in height (CSS columns
// with break-inside: avoid).
func (s *settings) shortcutColumns(c *ui.Context, groups []shortcutGroup, v *shortcutView) {
	wrap := ui.Column(c.Key("columns")).FillWidth()
	wide := watchWidth(c, wrap) >= twoColumnsWidth && len(groups) > 1
	wrap.Children(func() {
		if !wide {
			for _, g := range groups {
				s.shortcutGroup(c, g, v)
			}
			return
		}
		k := splitColumns(groups)
		ui.Row(c.Key("two")).FillWidth().Gap(columnGap).AlignItems(ui.Start).Children(func() {
			for i, part := range [][]shortcutGroup{groups[:k], groups[k:]} {
				ui.Column(c.Key(i)).Grow(1).Basis(0).MinWidth(0).Children(func() {
					for _, g := range part {
						s.shortcutGroup(c, g, v)
					}
				})
			}
		})
	})
}

// splitColumns is how many of the groups the first of two columns holds:
// the split whose taller column is the shorter, the height of a group being
// its heading and its rows.
func splitColumns(groups []shortcutGroup) int {
	height := func(g shortcutGroup) float32 {
		return groupHeading + groupPadBottom + shortcutRowFull*float32(len(g.list))
	}
	var total float32
	for _, g := range groups {
		total += height(g)
	}
	best, tallest := len(groups), total
	var first float32
	for k := 1; k < len(groups); k++ {
		first += height(groups[k-1])
		if m := max(first, total-first); m < tallest {
			best, tallest = k, m
		}
	}
	return best
}

// watchWidth is e's width in the last frame, and asks for another frame when
// it is not the one the frame before saw, so what is laid out from a
// measured width settles.
func watchWidth(c *ui.Context, e ui.Element) float32 {
	last := ui.Local(e, "watched", func() float32 { return -1 })
	w := e.Bounds().W
	if w != *last {
		*last = w
		c.After(time.Millisecond)
	}
	return w
}

// shortcutGroup is a group: its heading (an h3 under the section's h2) over
// its rows.
func (s *settings) shortcutGroup(c *ui.Context, g shortcutGroup, v *shortcutView) {
	title := groupTitle(g.group)
	c = c.Key("group:" + string(g.group))
	ui.Column(c).FillWidth().Padding(0, 0, groupPadBottom, 0).Role(ui.RoleGroup).Label(title).Children(func() {
		theme.Text(c, title, "heading").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(3).Margin(0, 0, 8, 0)
		for _, sh := range g.list {
			s.shortcutRow(c, sh, v)
		}
	})
}

// shortcutRow is ShortcutRow: the action, its keys as a recorder button,
// the reset button, and under them the live note and a conflict to settle.
func (s *settings) shortcutRow(c *ui.Context, sh module.Shortcut, v *shortcutView) {
	id := "shortcut-" + sh.ID
	c = c.Key(id)
	s.seen[id] = true
	rec := stateOf[recorder](s, "recorder:"+sh.ID)
	chords := v.bind[sh.ID]
	label := shortcutLabel(sh)

	var others []module.Shortcut
	if v.conflicted[sh.ID] {
		for _, ch := range chords {
			for _, o := range conflictsFor(v.all, v.bind, sh.ID, ch) {
				if !slices.ContainsFunc(others, func(x module.Shortcut) bool { return x.ID == o.ID }) {
					others = append(others, o)
				}
			}
		}
	}
	// note is the live region's text: what to do while recording, or why the
	// keys are marked.
	note := func() (msg string, danger bool) {
		switch {
		case rec.recording && rec.refused != "":
			return rec.refused, false
		case rec.recording:
			return i18n.T("Press the new keys. Esc cancels, Backspace removes the shortcut."), false
		case len(others) > 0:
			return i18n.T("Also used by {otherNames}. Only the first one runs.", "otherNames", quoted(others)), true
		}
		return "", false
	}

	ui.Column(c).FillWidth().Children(func() {
		s.line(c)
		in := ui.Column(c.Key("in")).FillWidth().Margin(0, -rowWashInset).Padding(shortcutRowPad, rowWashInset).
			Radius(theme.Current().Radius["md"])
		s.wash(in, id)
		in.Children(func() {
			var recorderButton ui.Element
			ui.Row(c.Key("top")).FillWidth().MinHeight(shortcutHeight).AlignItems(ui.Center).Justify(ui.SpaceBetween).
				Gap(shortcutGap).Children(func() {
				theme.Text(c, label, "body").TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
				ui.Row(c.Key("controls")).Shrink(0).AlignItems(ui.Center).Gap(6).Children(func() {
					recorderButton = s.recorderButton(c, sh, rec, v, label, v.conflicted[sh.ID], note)
					s.resetButton(c, sh, rec, v, label)
				})
			})
			s.shortcutNote(c, rec, note)
			s.shortcutChoice(c, sh, rec)
			s.land(c, id, in, recorderButton)
		})
	})
}

// quoted is the labels of shortcuts in quotes, as “Next”, “Previous”.
func quoted(list []module.Shortcut) string {
	names := make([]string, len(list))
	for i, o := range list {
		names[i] = "“" + shortcutLabel(o) + "”"
	}
	return strings.Join(names, ", ")
}

// chordsName is a shortcut's chords as the keycaps read, "Ctrl B or Ctrl F",
// so the recorder's name contains its visible text (2.5.3) for speech input.
func chordsName(chords []string) string {
	items := make([]string, len(chords))
	for i, ch := range chords {
		items[i] = strings.Join(kit.ChordParts(ch), " ")
	}
	return i18n.ListOr(items)
}

// recorderButton is the button showing the shortcut's keys. Click it (or
// press Enter or Space) and press the new keys: the keys go to it and to no
// shortcut. Esc cancels, Backspace and Delete remove the keys, Tab and
// Shift+Tab are refused (they move the focus), a modifier alone is
// ignored, and losing the focus stops the recording.
func (s *settings) recorderButton(c *ui.Context, sh module.Shortcut, rec *recorder, v *shortcutView, label string,
	conflicted bool, note func() (string, bool),
) ui.Element {
	chords := v.bind[sh.ID]
	keys := i18n.T("not set")
	switch {
	case rec.recording:
		keys = i18n.T("Press keys…")
	case len(chords) > 0:
		keys = chordsName(chords)
	}
	b := ui.ButtonBase(c.Key("recorder")).Label(label + " " + keys).Role(ui.RoleToggleButton).Cursor(ui.CursorPointer)
	if rec.recording && !b.Focused() {
		rec.stop() // the focus left: a click elsewhere, another window
	}
	if rec.refocus {
		rec.refocus = false
		b.Focus()
	}
	if b.Clicked() {
		rec.pending = nil
		rec.recording = !rec.recording
		rec.refused = ""
	}
	if rec.recording {
		chord, ok := captureChord(b)
		if rec.slash && !ok {
			chord, ok = "?", true
		}
		rec.slash = false
		if ok {
			s.recorded(rec, sh, v, chord)
		}
		// The shell's dispatcher saw the "?" first, and lets it go.
		b.HandleInput(func(ev ui.InputEvent) bool {
			if !kit.IsShiftSlash(ev) {
				return false
			}
			kit.TakeShiftSlash()
			rec.slash = true
			return true
		})
	}
	b.Checked(rec.recording)
	if msg, _ := note(); rec.recording || conflicted {
		b.Description(msg)
	}

	border, fill := theme.C("control-line").Alpha(0.4), ui.Transparent
	switch {
	case rec.recording:
		border, fill = theme.C("ink"), theme.C("raised")
	case b.Hovered():
		border, fill = theme.C("control-line"), theme.C("raised")
	case conflicted:
		border = theme.C("danger").Alpha(0.5)
	}
	b.Height(shortcutHeight).MinWidth(recorderWidth).PaddingX(8).Gap(6).AlignItems(ui.Center).Justify(ui.End).
		Radius(theme.Current().Radius["md"]).Border(1, border).Background(fill)
	theme.Fade(b, "feedback")
	b.Children(func() {
		switch {
		case rec.recording:
			theme.Text(c, i18n.T("Press keys…"), "secondary").TextColor(theme.C("ink-muted"))
		case len(chords) > 0:
			kit.Keycaps(c.Key("keys"), chords...)
		default:
			theme.Text(c, i18n.T("Not set"), "secondary").TextColor(theme.C("ink-faint"))
		}
	})
	return b
}

// captureChord is the chord of the key pressed while the recorder has the
// focus (eventChord): the recorder asks for every key with every set of
// modifiers as a shortcut of its own, which a key goes to before the
// window's, so a chord the shell runs is recorded instead of run. A lone
// modifier is no key press and records nothing; a key no chord can name
// (spell) is not asked for.
func captureChord(b ui.Element) (chord string, ok bool) {
	for _, key := range recordable {
		for _, mods := range modifierCombos {
			// Every one is asked for, even after a key came: what a frame
			// does not ask for the next one does not claim.
			if b.Shortcut(mods, key) && !ok {
				chord, ok = spell(mods, key)
			}
		}
	}
	return chord, ok
}

// recorded is what a chord pressed while recording does.
func (s *settings) recorded(rec *recorder, sh module.Shortcut, v *shortcutView, chord string) {
	switch {
	case chord == "Escape":
		rec.stop()
	case chord == "Backspace" || chord == "Delete":
		rec.stop()
		s.bindShortcut(sh.ID, nil, false)
	case routerChord(chord, ui.Cmd):
		// The router takes these before any shortcut: it would go back or
		// forward whatever they were bound to.
		rec.refused = i18n.T("{key} is Sona's own key for going back and forward. Choose another key.", "key", kit.ChordText(chord))
		rec.refusals++
	case chord == "Tab" || chord == "Shift+Tab":
		// They move the focus: not shortcuts.
		rec.refused = i18n.T("{key} moves between controls. Choose another key.", "key", kit.ChordText(chord))
		rec.refusals++
	default:
		rec.stop()
		if clash := conflictsFor(v.all, v.bind, sh.ID, chord); len(clash) > 0 {
			rec.pending = &pendingChord{chord: chord, with: clash}
		} else {
			s.bindShortcut(sh.ID, []string{chord}, false)
		}
	}
}

// bindShortcut binds id to chords and saves (rebind): with steal, the
// shortcuts on those chords lose them.
func (s *settings) bindShortcut(id string, chords []string, steal bool) {
	all := shortcutsOf(s.shortcuts())
	s.save(func(st *config.Settings) { st.Shortcuts = rebind(all, st.Shortcuts, id, chords, steal) })
}

// resetButton is the row's reset to the default keys. It is there while the
// keys are changed; the space stays, as the button is invisible, not gone.
func (s *settings) resetButton(c *ui.Context, sh module.Shortcut, rec *recorder, v *shortcutView, label string) {
	if _, overridden := v.overrides[sh.ID]; !overridden {
		ui.Box(c.Key("reset")).Size(shortcutHeight, shortcutHeight).Shrink(0)
		return
	}
	name := i18n.T("Reset {label} to no key", "label", label)
	if defaults := defaultChords(sh); len(defaults) > 0 {
		words := make([]string, len(defaults))
		for i, ch := range defaults {
			words[i] = kit.ChordText(ch)
		}
		name = i18n.T("Reset {label} to {defaults}", "label", label, "defaults", i18n.ListOr(words))
	}
	b := kit.IconButton(c.Key("reset"), "rotate-ccw", name, kit.ButtonOpts{
		Variant: kit.Ghost, Size: kit.SizeSM, Muted: true, Tip: i18n.T("Reset to default"),
	})
	if b.Clicked() {
		s.bindShortcut(sh.ID, defaultChords(sh), false)
		// This button goes once the keys are back to the default.
		rec.refocus = true
	}
}

// shortcutNote is the live region under a row: what to do while recording
// or why the keys are marked. A change is announced; a refusal is announced
// again if it comes again.
func (s *settings) shortcutNote(c *ui.Context, rec *recorder, note func() (string, bool)) {
	msg, danger := note()
	holder := ui.Column(c.Key("note")).FillWidth()
	holder.Children(func() {
		if msg == "" {
			return
		}
		color := theme.C("ink-muted")
		if danger {
			color = theme.C("danger")
		}
		theme.Text(c, msg, "secondary").TextColor(color).Margin(6, 0, 0, 0)
	})
	key := strconv.Itoa(rec.refusals) + ":" + msg
	told := ui.Local(holder, "told", func() string { return key })
	if *told != key {
		*told = key
		if msg != "" {
			c.Announce(msg)
		}
	}
}

// shortcutChoice is the alert a conflicting chord asks with: the chord is
// used by other shortcuts; Cancel, or Use it here, which moves the key
// here. Escape cancels; either returns the focus to the recorder.
func (s *settings) shortcutChoice(c *ui.Context, sh module.Shortcut, rec *recorder) {
	p := rec.pending
	if p == nil {
		return
	}
	sentence := i18n.T("<0/> is used by {pendingNames}.", "pendingNames", quoted(p.with))
	spoken := strings.Replace(sentence, "<0/>", kit.ChordText(p.chord), 1)
	settle := func(steal bool) {
		if steal {
			s.bindShortcut(sh.ID, []string{p.chord}, true)
		}
		rec.pending = nil
		rec.refocus = true // the choice's buttons go with it
	}
	box := ui.Row(c.Key("choice")).FillWidth().Margin(8, 0, 0, 0).Padding(8, 12).Gap(16).AlignItems(ui.Center).
		Justify(ui.SpaceBetween).Radius(theme.Current().Radius["md"]).Background(theme.C("hover")).
		Role(ui.RoleStatus).Label(spoken)
	kit.AnnounceMounted(c, box, spoken)
	if box.Shortcut(0, ui.KeyEscape) {
		settle(false)
	}
	box.Children(func() {
		before, after, _ := strings.Cut(sentence, "<0/>")
		ui.Row(c.Key("sentence")).Grow(1).Shrink(1).MinWidth(0).Wrap().AlignItems(ui.Center).Gap(4).Children(func() {
			if before = strings.TrimSpace(before); before != "" {
				theme.Text(c.Key("before"), before, "secondary").TextColor(theme.C("ink"))
			}
			kit.Keycaps(c.Key("chord"), p.chord)
			theme.Text(c.Key("after"), strings.TrimSpace(after), "secondary").TextColor(theme.C("ink"))
		})
		ui.Row(c.Key("buttons")).Shrink(0).Gap(8).Children(func() {
			if kit.Button(c.Key("cancel"), i18n.T("Cancel"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM}).Clicked() {
				settle(false)
			}
			if kit.Button(c.Key("use"), i18n.T("Use it here"), kit.ButtonOpts{Size: kit.SizeSM}).AutoFocus().Clicked() {
				settle(true)
			}
		})
	})
}

// shortcutStatus is the line under the groups when the filter finds
// nothing: a status, read as it changes (the section opening on a filter
// that finds nothing says it as it builds).
func (s *settings) shortcutStatus(c *ui.Context, none bool, query string) {
	msg := ""
	if none {
		msg = i18n.T("No shortcut matches “{query}”.", "query", query)
	}
	col := ui.Column(c.Key("status")).FillWidth().AlignItems(ui.Center).Role(ui.RoleStatus)
	kit.AnnounceMounted(c, col, msg)
	col.Children(func() {
		if msg != "" {
			theme.Text(c, msg, "secondary").TextColor(theme.C("ink-muted")).Padding(24, 0)
		}
	})
}
