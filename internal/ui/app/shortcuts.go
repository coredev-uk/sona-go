package app

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// A chord is a key with exact modifiers, written as v0.2.0 wrote them
// ("Ctrl+K", "Shift+ArrowRight", "Alt+1", "Space", "["). The dispatcher
// follows v0.2.0's app/shortcuts.ts: one handler for every registered
// shortcut, the user's bindings (config.Settings.Shortcuts) over the
// defaults, and for a key the first shortcut bound to it whose scope and
// When allow it takes it.
//
// What the browser's rules left to the page, MyGo's input handling does: a
// focused text input keeps the keys it types, and a focused button or
// slider keeps Enter, Space and the arrows it operates with (a shortcut
// only sees what no focused element took), so a page-scoped key never
// fires from a text field, and Space on a focused button presses it.

// chord is one parsed key chord.
type chord struct {
	mods ui.Modifiers
	key  ui.Key
}

// parseChord reads one chord (kit.ParseChord's spelling); ok is false for
// anything that is not one.
func parseChord(s string) (chord, bool) {
	mods, key, ok := kit.ParseChord(s)
	return chord{mods, key}, ok
}

// parseBinding reads a binding: chords separated by spaces, or "none" (no
// keys). ok is false when it is empty or any part is not a chord.
func parseBinding(s string) (chords []chord, ok bool) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, false
	}
	if len(fields) == 1 && strings.EqualFold(fields[0], "none") {
		return nil, true
	}
	for _, f := range fields {
		c, ok := parseChord(f)
		if !ok {
			return nil, false
		}
		chords = append(chords, c)
	}
	return chords, true
}

// chordsOf is a shortcut's keys: the user's binding when it parses, else
// its defaults (a blank or invalid override keeps them).
func chordsOf(s module.Shortcut, overrides config.Shortcuts) []chord {
	if b, ok := overrides[s.ID]; ok {
		if chords, ok := parseBinding(b); ok {
			return chords
		}
	}
	var out []chord
	for _, k := range s.Keys {
		if c, ok := parseChord(k); ok {
			out = append(out, c)
		}
	}
	return out
}

// keysOf is a shortcut's keys as strings: the user's binding when it
// parses ("none" is no keys), else its defaults.
func keysOf(s module.Shortcut, overrides config.Shortcuts) []string {
	if b, ok := overrides[s.ID]; ok {
		if _, ok := parseBinding(b); ok {
			if strings.EqualFold(strings.TrimSpace(b), "none") {
				return nil
			}
			return strings.Fields(b)
		}
	}
	return s.Keys
}

// bound is a shortcut with the chords it answers to.
type bound struct {
	module.Shortcut
	chords []chord
}

// dispatcher is the window's one key handler.
type dispatcher struct {
	shortcuts []module.Shortcut
	overrides config.Shortcuts
	list      []bound
	listed    []module.BoundShortcut // what Env.Shortcuts returns, built on rebind
	chords    []chord                // distinct, in registration order
}

func newDispatcher(list []module.Shortcut, overrides config.Shortcuts) *dispatcher {
	d := &dispatcher{shortcuts: list}
	d.rebind(overrides)
	return d
}

// rebind applies the user's bindings (a settings change applies at once).
func (d *dispatcher) rebind(overrides config.Shortcuts) {
	d.overrides = overrides
	d.list, d.chords = d.list[:0], d.chords[:0]
	d.listed = make([]module.BoundShortcut, 0, len(d.shortcuts))
	for _, s := range d.shortcuts {
		d.listed = append(d.listed, module.BoundShortcut{Shortcut: s, Keys: keysOf(s, overrides)})
		b := bound{Shortcut: s, chords: chordsOf(s, overrides)}
		d.list = append(d.list, b)
		for _, c := range b.chords {
			if !slices.Contains(d.chords, c) {
				d.chords = append(d.chords, c)
			}
		}
	}
}

// bindings is every registered shortcut with the keys it answers to now
// (Env.Shortcuts).
func (d *dispatcher) bindings() []module.BoundShortcut { return d.listed }

// dispatch asks MyGo, each frame, for every chord, and runs the first
// shortcut allowed for one that was pressed. A shortcut that is
// Unavailable takes the key too, but only says why.
func (d *dispatcher) dispatch(c *ui.Context) {
	for _, ch := range d.chords {
		if !c.Shortcut(ch.mods, ch.key) {
			continue
		}
		for i := range d.list {
			s := &d.list[i]
			if !slices.Contains(s.chords, ch) || !allowed(s.Shortcut) {
				continue
			}
			if s.Unavailable != nil {
				if why := s.Unavailable(); why != "" {
					kit.Unavailable(why)
					break
				}
			}
			if s.Run != nil {
				s.Run()
			}
			break
		}
	}
}

// allowed reports whether a shortcut may take a key now: its When holds.
// (A scope other than page and global names a context that When checks;
// v0.2.0's dispatcher did not match it against anything else.)
func allowed(s module.Shortcut) bool { return s.When == nil || s.When() }

// navShortcuts is the "go to" shortcut of each sidebar link: "nav.<path>",
// Ctrl+1…9 for the primary section and Alt+1…9 for the library's, by
// position, unless the item names its own keys (v0.2.0's compose.ts).
func navShortcuts(items [3][]module.NavItem, push func(string)) []module.Shortcut {
	var out []module.Shortcut
	for section, list := range items {
		mod := ""
		switch module.NavSection(section) {
		case module.NavPrimary:
			mod = "Ctrl"
		case module.NavLibrary:
			mod = "Alt"
		}
		for i, item := range list {
			keys := item.Keys
			if keys == nil && mod != "" && i < 9 {
				keys = []string{mod + "+" + string(rune('1'+i))}
			}
			s := module.Shortcut{
				ID:    "nav." + navID(item.To),
				Label: item.Label,
				Group: "pages",
				Keys:  keys,
				Scope: "global",
				Icon:  item.Icon,
				Run:   func() { push(item.To) },
			}
			if module.NavSection(section) == module.NavLibrary {
				label := item.Label
				s.Label = func() string { return i18n.T("Library · {page}", "page", label()) }
				s.Keywords = func() string { return i18n.T("library") }
			}
			out = append(out, s)
		}
	}
	return out
}

// navID is a path as an ID: "/library/albums" is "library.albums", "/" is
// "home".
func navID(to string) string {
	id := strings.Join(strings.FieldsFunc(to, func(r rune) bool { return r == '/' }), ".")
	if id == "" {
		return "home"
	}
	return id
}
