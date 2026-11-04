package kit

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// ChordParts is a chord in v0.2.0's spelling ("Ctrl+ArrowLeft") as the
// keycaps show it: ["Ctrl", "←"].
func ChordParts(chord string) []string {
	parts := strings.Split(chord, "+")
	out := make([]string, len(parts))
	for i, p := range parts {
		switch p {
		case "ArrowUp":
			out[i] = "↑"
		case "ArrowDown":
			out[i] = "↓"
		case "ArrowLeft":
			out[i] = "←"
		case "ArrowRight":
			out[i] = "→"
		case "Plus":
			out[i] = "+"
		case "Escape":
			out[i] = i18n.TC("keyboard key", "Esc")
		default:
			out[i] = keyName(p, false)
		}
	}
	return out
}

// ChordText is a chord in words for assistive technology: "Ctrl+Left Arrow".
func ChordText(chord string) string {
	parts := strings.Split(chord, "+")
	for i, p := range parts {
		switch p {
		case "ArrowUp":
			parts[i] = i18n.TC("keyboard key", "Up Arrow")
		case "ArrowDown":
			parts[i] = i18n.TC("keyboard key", "Down Arrow")
		case "ArrowLeft":
			parts[i] = i18n.TC("keyboard key", "Left Arrow")
		case "ArrowRight":
			parts[i] = i18n.TC("keyboard key", "Right Arrow")
		case ",":
			parts[i] = i18n.TC("keyboard key", "Comma")
		case ".":
			parts[i] = i18n.TC("keyboard key", "Period")
		case "/":
			parts[i] = i18n.TC("keyboard key", "Slash")
		case "?":
			parts[i] = i18n.TC("keyboard key", "Question mark")
		default:
			parts[i] = keyName(p, true)
		}
	}
	return strings.Join(parts, "+")
}

// keyName is the translated name of a modifier or named key; other keys are
// themselves.
func keyName(p string, _ bool) string {
	switch p {
	case "Ctrl":
		return i18n.TC("keyboard key", "Ctrl")
	case "Alt":
		return i18n.TC("keyboard key", "Alt")
	case "Shift":
		return i18n.TC("keyboard key", "Shift")
	case "Meta":
		return i18n.TC("keyboard key", "Super")
	case "Space":
		return i18n.TC("keyboard key", "Space")
	case "Enter":
		return i18n.TC("keyboard key", "Enter")
	case "Escape":
		return i18n.TC("keyboard key", "Escape")
	case "Tab":
		return i18n.TC("keyboard key", "Tab")
	case "Backspace":
		return i18n.TC("keyboard key", "Backspace")
	case "Delete":
		return i18n.TC("keyboard key", "Delete")
	case "Insert":
		return i18n.TC("keyboard key", "Insert")
	case "Home":
		return i18n.TC("keyboard key", "Home")
	case "End":
		return i18n.TC("keyboard key", "End")
	case "PageUp":
		return i18n.TC("keyboard key", "Page Up")
	case "PageDown":
		return i18n.TC("keyboard key", "Page Down")
	case "Plus":
		return i18n.TC("keyboard key", "Plus")
	}
	return p
}

// Keycaps draws chords as keycaps side by side (a 20 DIP tall bordered cap
// per key, tabular figures). Assistive technology reads each chord once, as
// words ("Ctrl+Left Arrow"), with "or" between chords; the caps themselves
// are left out of the tree. The shortcut help and the palette share it.
func Keycaps(c *ui.Context, chords ...string) {
	spoken := make([]string, len(chords))
	for i, chord := range chords {
		spoken[i] = ChordText(chord)
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).Shrink(0).Role(ui.RoleGroup).Label(strings.Join(spoken, " "+i18n.T("or")+" ")).Children(func() {
		for _, chord := range chords {
			ui.Row(c.Key("chord:" + chord)).Gap(2).AlignItems(ui.Center).Role(ui.RoleNone).Children(func() {
				for j, part := range ChordParts(chord) {
					ui.Row(c.Key("cap:"+string(rune('0'+j)))).Height(20).MinWidth(20).PaddingX(4).Center().Role(ui.RoleNone).
						Radius(theme.Current().Radius["sm"]).Border(1, theme.C("control-line")).Children(func() {
						overlaysText(c, part, "meta").FontWeight(500).TextColor(theme.C("ink-muted")).FontFeatures("tnum").Role(ui.RoleNone)
					})
				}
			})
		}
	})
}

// PaletteEntry is one row of the palette.
type PaletteEntry struct {
	// Key is stable across queries (tests, the active row follows it).
	Key   string
	Label string
	// Detail is a muted second line (a library item's artist · album).
	Detail string
	// Icon is a Lucide name shown muted in a 16 DIP cell; "" for none.
	Icon string
	// Leading draws something in place of the icon (an item's cover).
	Leading func(c *ui.Context)
	// Keys are the chords shown at the end (at most two are drawn).
	Keys []string
	Run  func()
}

// PaletteSection is a titled group of entries ("Commands", "In your
// library").
type PaletteSection struct {
	Title   string
	Entries []PaletteEntry
}

// PaletteSource gives the palette its sections for a query. The commands
// feature supplies it: with nothing typed, every available command by group;
// with a query, the ranked commands, then library items, then a search of
// Apple Music for the text.
type PaletteSource func(query string) []PaletteSection

// Palette is the command palette's frame (v0.2.0's CommandPalette.tsx, UI
// only): a modal dialog 14 % from the top, 600 wide, with one search field
// that is an ARIA combobox over a grouped list. Up and Down move (and wrap),
// Enter runs the active row, Escape closes and the focus goes back where it
// was; the chosen entry runs once the dialog has closed and the focus is
// back, since it may move the focus itself. Moving the pointer onto a row makes it active.
type Palette struct {
	open    bool
	query   string
	last    string
	active  string
	pending func()
	settle  int
	// moved: the pointer moved since the last frame (only a real move may
	// make a row active; a pointer resting under a re-laid-out list must
	// not take the choice from the keys).
	moved bool
	// reveal scrolls the active row into view, once, after the keys moved it.
	reveal bool
}

// IsOpen reports whether the palette shows.
func (p *Palette) IsOpen() bool { return p.open }

// Open shows the palette with an empty query.
func (p *Palette) Open() {
	p.open, p.query, p.last, p.active, p.moved, p.reveal = true, "", "", "", false, false
}

// Close hides it.
func (p *Palette) Close() { p.open = false }

// Query is what is typed.
func (p *Palette) Query() string { return p.query }

// Show draws the palette while open (every frame) and runs a chosen entry
// once it has closed.
func (p *Palette) Show(c *ui.Context, src PaletteSource) {
	if !p.open {
		if fn := p.pending; fn != nil {
			// One frame after the dialog is gone, so MyGo has given the
			// focus back before the entry runs (as Menu.Show settles): an
			// entry that opens a dialog notes the page's focus, not the
			// palette's dying field.
			if p.settle > 0 {
				p.settle--
				c.AnimationFrame()
				return
			}
			p.pending = nil
			fn()
		}
		return
	}
	_, h := c.Size()
	w, _ := c.Size()
	Dialog(c.Key("palette"), &p.open, DialogOpts{
		Title: i18n.T("Command palette"), HideHeader: true, HideClose: true, Bare: true, Raised: true,
		MaxWidth: min(600, max(w-32, 0)), Top: h * 0.14,
	}, func() { p.body(c, src, h) })
}

func (p *Palette) body(c *ui.Context, src PaletteSource, windowH float32) {
	sections := src(p.query)
	var flat []PaletteEntry
	for _, s := range sections {
		flat = append(flat, s.Entries...)
	}
	if p.query != p.last {
		p.last, p.active = p.query, ""
	}
	at := -1
	for i := range flat {
		if flat[i].Key == p.active {
			at = i
		}
	}
	if at < 0 && len(flat) > 0 {
		at, p.active, p.reveal = 0, flat[0].Key, true
	}
	pick := func(e PaletteEntry) {
		p.pending, p.settle = e.Run, 1
		p.open = false
	}

	// The field.
	var field ui.Element
	ui.Row(c.Key("palette:field")).Height(48).FillWidth().Padding(0, 16).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Icon(c.Key("search"), theme.Icon("search")).FontSize(16).TextColor(theme.C("ink-faint")).Shrink(0)
		field = ui.TextInputBase(c.Key("palette:input"), &p.query).Role(ui.RoleComboBox).Grow(1).MinWidth(0).Height(48).
			Label(i18n.T("Find a page, action or music in your library")).
			Placeholder(i18n.T("Go to a page, run an action, find music in your library")).
			PlaceholderColor(theme.C("ink-faint")).TextColor(theme.C("ink")).FontSize(theme.Current().Type["body"].Size)
		n := len(flat)
		switch {
		case n > 0 && field.Shortcut(0, ui.KeyDown):
			p.active, p.reveal = flat[(at+1)%n].Key, true
		case n > 0 && field.Shortcut(0, ui.KeyUp):
			p.active, p.reveal = flat[(at-1+n)%n].Key, true
		case n > 0 && field.Submitted():
			pick(flat[at])
		}
	})
	ui.Box(c.Key("palette:sep")).Height(1).FillWidth().Background(theme.C("line"))

	// The results.
	maxH := min(windowH*0.56, 440)
	list := ui.Scroll(c.Key("palette:list")).Role(ui.RoleList).Label(i18n.T("Results")).FillWidth().MaxHeight(maxH).Padding(6)
	list.HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind == ui.InputPointerMove {
			p.moved = true
			return true // a frame follows, which reads the row under the pointer
		}
		return false
	})
	hovered := ""
	var activeRow ui.Element
	list.Children(func() {
		for _, s := range sections {
			ui.Column(c.Key("palette:g:" + s.Title)).Role(ui.RoleGroup).Label(s.Title).FillWidth().Children(func() {
				overlaysText(c.Key("h"), s.Title, "meta").TextColor(theme.C("ink-faint")).Padding(10, 10, 4).Role(ui.RoleHeading)
				for _, e := range s.Entries {
					row := p.option(c, e, e.Key == p.active)
					if row.Hovered() {
						hovered = e.Key
					}
					if row.Clicked() {
						pick(e)
					}
					if e.Key == p.active {
						activeRow = row
						if p.reveal {
							row.ScrollIntoView()
						}
					}
				}
			})
		}
	})
	p.reveal = false
	if p.moved && hovered != "" {
		p.active = hovered // any real move over a row, as onMouseMove
	}
	p.moved = false
	if flat != nil {
		field.ActiveDescendant(activeRow)
	}

	// The hints.
	ui.Box(c.Key("palette:sep2")).Height(1).FillWidth().Background(theme.C("line"))
	ui.Row(c.Key("palette:hints")).Height(36).FillWidth().Padding(0, 16).Gap(16).AlignItems(ui.Center).Children(func() {
		hint := func(key string, label string, chords ...string) {
			ui.Row(c.Key("hint:" + key)).Gap(6).AlignItems(ui.Center).Children(func() {
				Keycaps(c, chords...)
				overlaysText(c.Key("l"), label, "meta").TextColor(theme.C("ink-faint"))
			})
		}
		hint("move", i18n.T("Move"), "ArrowUp", "ArrowDown")
		hint("run", i18n.T("Run"), "Enter")
		hint("close", i18n.T("Close"), "Escape")
	})
}

// option draws one entry: min 36 tall, radius 6, the active one on the
// active tone with an inset ring (the field keeps the focus, so the row
// carries it).
func (p *Palette) option(c *ui.Context, e PaletteEntry, active bool) ui.Element {
	row := ui.Row(c.Key("opt:"+e.Key)).MinHeight(36).FillWidth().Padding(4, 10).Gap(12).AlignItems(ui.Center).
		Radius(theme.Current().Radius["md"]).Role(ui.RoleListItem).Label(e.Label).Selected(active).Cursor(ui.CursorPointer)
	ring := ui.Transparent
	if active {
		row.Background(theme.C("active"))
		ring = theme.C("ring")
	}
	row.Border(2, ring)
	row.Children(func() {
		switch {
		case e.Leading != nil:
			e.Leading(c)
		case e.Icon != "":
			ui.Row(c.Key("ic")).Size(16, 16).Shrink(0).Center().Children(func() {
				ui.Icon(c, theme.Icon(e.Icon)).FontSize(16).TextColor(theme.C("ink-muted"))
			})
		default:
			ui.Box(c.Key("ic")).Size(16, 16).Shrink(0)
		}
		if e.Detail != "" {
			ui.Column(c.Key("txt")).Grow(1).MinWidth(0).Children(func() {
				overlaysText(c.Key("l"), e.Label, "body").TextColor(theme.C("ink")).SingleLine()
				overlaysText(c.Key("d"), e.Detail, "secondary").TextColor(theme.C("ink-muted")).SingleLine()
			})
		} else {
			overlaysText(c.Key("l"), e.Label, "body").TextColor(theme.C("ink")).SingleLine().Grow(1).MinWidth(0)
		}
		if len(e.Keys) > 0 {
			Keycaps(c.Key("keys"), e.Keys[:min(len(e.Keys), 2)]...)
		}
	})
	return row
}
