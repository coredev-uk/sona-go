package kit

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// MenuItemKind is what a menu row is.
type MenuItemKind int

const (
	// MenuAction runs Run when chosen.
	MenuAction MenuItemKind = iota
	// MenuCheckbox shows a check at its start while Checked.
	MenuCheckbox
	// MenuRadio shows a filled dot at its start while Checked.
	MenuRadio
	// MenuLabel is a muted heading; it cannot be chosen.
	MenuLabel
	// MenuSeparator is a 1 px line between groups.
	MenuSeparator
	// MenuSub opens Sub beside it: on hover or Right, closing on Left.
	MenuSub
)

// MenuItem is one row of a Menu (shadcn's DropdownMenuItem and its
// siblings: checkbox, radio, label, separator, sub-trigger, shortcut).
type MenuItem struct {
	// Key is the row's stable ID (keys, tests).
	Key   string
	Kind  MenuItemKind
	Label string
	// Icon is a Lucide name; Filled draws it filled.
	Icon   string
	Filled bool
	// Checked is the state of a MenuCheckbox or MenuRadio.
	Checked  bool
	Disabled bool
	// Reason greys the row out and prints this faint line under its label.
	Reason string
	// Description is read to assistive technology as the row's description,
	// never shown ("Opens in your browser" on a seller's row).
	Description string
	// Destructive colours the row --danger (a MenuLabel: its Icon).
	Destructive bool
	// Note styles a MenuLabel: the default is the faint heading line;
	// LabelHeader is a medium ink line (the account menu's "Apple Music"),
	// LabelNote a muted one that may lead with Icon.
	Note MenuLabelNote
	// Hint is a muted right-aligned shortcut ("Ctrl+K").
	Hint string
	// Inset indents the row to line up with rows that have an icon or mark.
	Inset bool
	// Sub holds a MenuSub's rows.
	Sub []MenuItem
	// Run runs after the menu has closed and given the focus back, or at
	// once while the menu stays open when KeepOpen is set.
	Run func()
	// KeepOpen keeps the menu open when the row is chosen (a checkbox whose
	// tick is the confirmation: shadcn's onSelect with preventDefault).
	KeepOpen bool
}

// MenuLabelNote is how a MenuLabel reads.
type MenuLabelNote uint8

const (
	// LabelFaint is a faint meta heading above a group.
	LabelFaint MenuLabelNote = iota
	// LabelHeader is a 14 medium ink line heading the menu.
	LabelHeader
	// LabelNote is a 12 muted line of explanation, with a leading Icon when
	// set.
	LabelNote
)

func (it *MenuItem) choosable() bool {
	if it.Disabled || it.Reason != "" {
		return false
	}
	switch it.Kind {
	case MenuAction, MenuCheckbox, MenuRadio, MenuSub:
		return true
	}
	return false
}

// MenuOpts places and names a menu.
type MenuOpts struct {
	// ID keys the menu's elements; give each menu on a page its own.
	ID string
	// Label names the menu for assistive technology.
	Label string
	// Width is the panel's width in DIPs; 0 sizes it to its rows (at least
	// 8 rem).
	Width float32
	// Start lines the panel's start up with its trigger's, as under a text;
	// the default lines the ends up, as under a button at a row's end.
	Start bool
}

// Menu is the state of a dropdown or context menu and draws it (shadcn's
// dropdown-menu): 4 DIP offset, min width 8 rem, capped to the room there
// is, scrolling, rounded popover with border and shadow, fading in. It is
// opened at a point (right-click, Shift+F10) or from its trigger and holds
// the keyboard focus while it shows: Up and Down, Home and End and
// type-ahead move among the rows past those disabled, Enter and Space
// choose, Right opens a sub-menu and Left closes it, Escape closes (the
// sub-menu on top first) and so does Tab or a press outside; closing gives
// the focus back to the element that had it. A menu opened from the
// keyboard starts on its first row. The chosen row's Run runs once the menu
// is gone and the focus is back, so a dialog it opens returns the focus to
// the page.
//
//	var m kit.Menu                       // field of the view's state
//	more := kit.Button(c, "", …)
//	if more.Clicked() { m.OpenFrom(more.ClickModifiers() == 0 && !pointer) }
//	m.Show(c, kit.MenuOpts{ID: "more"}, more, items)
type Menu struct {
	open     bool
	anchored bool
	x, y     float32
	keyboard bool
	levels   []menuLevel
	focus    int
	age      int
	typed    string
	typedAt  time.Time
	pending  func()
	settle   int
	// moved: the pointer moved since the last frame. Only a real move lets
	// a row under the pointer take the highlight; a menu appearing or
	// scrolling under a resting pointer must not take it from the keys.
	moved bool
	// aimHold: a row under a pointer that heads for an open sub-menu kept
	// its highlight this frame (aim), so the move is still to be taken when
	// the grace ends.
	aimHold bool
	// subs binds each level's panel, for the box of a sub-menu between builds.
	subs []*ui.Handle
	now  time.Time
	// dismissed is set the frame the user closed the menu (Escape, a
	// press outside, Tab): Dismissed reports it once.
	dismissed bool
}

type menuLevel struct {
	// sub is the key of the item whose sub-menu this level is ("" for the
	// menu).
	sub string
	// hi is the highlighted row's key ("" none); byKey: the keys put it
	// there, so it draws the ring.
	hi     string
	byKey  bool
	reveal bool
	// last is where the pointer was over the panel as it last built, when
	// lastOK; aimUntil, nonzero, is when the pointer heading for the
	// sub-menu (aim) stops keeping it open.
	last     [2]float32
	lastOK   bool
	aimUntil time.Time
}

// menuAimGrace is how long a pointer heading for an open sub-menu across the
// rows of the menu that holds it keeps the sub-menu open after its last move
// toward it: the menu-aim rule of macOS's menus and Radix's.
const menuAimGrace = 300 * time.Millisecond

// IsOpen reports whether the menu shows.
func (m *Menu) IsOpen() bool { return m.open }

// openMenus counts the menus that are open, whoever owns them (main thread
// only, like every view).
var openMenus int

// MenusOpen reports whether any menu is open anywhere: a page's, the
// sidebar's, the account's. What rotates by itself waits while one is.
func MenusOpen() bool { return openMenus > 0 }

// setOpen opens or closes the menu, keeping the count of open ones.
func (m *Menu) setOpen(on bool) {
	if m.open == on {
		return
	}
	m.open = on
	if on {
		openMenus++
	} else {
		openMenus--
	}
}

// OpenAt opens the menu at a point in window DIPs (a right-click, the
// focused row's corner for Shift+F10). keyboard starts it on its first row.
func (m *Menu) OpenAt(x, y float32, keyboard bool) {
	m.start(false, keyboard)
	m.x, m.y = x, y
}

// OpenOn wires row to open the menu, for a row with a "More" button of its
// own (not an ActionMenus item): a right-click opens it at the pointer, and
// Shift+F10 or the Menu key while the row has the focus within, below the row
// (starting on its first entry). Call it after building the row.
func (m *Menu) OpenOn(row ui.Element) {
	if row.RightClicked() {
		x, y, _ := row.PointerPosition()
		wx, wy := row.LocalToWindow(x, y)
		m.OpenAt(wx, wy, false)
	}
	if row.Shortcut(ui.Shift, ui.KeyF10) || row.Shortcut(0, ui.KeyContextMenu) {
		b := row.Bounds()
		m.OpenAt(b.X+16, b.Y+b.H, true)
	}
}

// OpenFrom opens the menu below its trigger (MenuOpts.Anchor); keyboard
// (Enter, Space, a click with no pointer) starts it on its first row.
func (m *Menu) OpenFrom(keyboard bool) { m.start(true, keyboard) }

func (m *Menu) start(anchored, keyboard bool) {
	m.setOpen(true)
	m.anchored, m.keyboard = anchored, keyboard
	m.levels = []menuLevel{{}}
	m.focus, m.age, m.typed, m.dismissed = 0, 0, "", false
	m.moved, m.aimHold = false, false
}

// Close closes the menu (a trigger pressed again).
func (m *Menu) Close() { m.setOpen(false); m.levels = nil }

// Dismissed reports, once, that the user closed the menu without choosing.
func (m *Menu) Dismissed() bool { d := m.dismissed; m.dismissed = false; return d }

// Show draws the menu while it is open; call it every frame after the
// trigger, whose element is anchor (the panel goes below it, above where
// there is no room, its end lined up with the trigger's; ignored for a menu
// opened at a point). It also runs what the last choice left to run.
func (m *Menu) Show(c *ui.Context, o MenuOpts, anchor ui.Element, items []MenuItem) {
	if !m.open {
		if p := m.pending; p != nil {
			// Two frames after the menu is gone, so the focus has gone back
			// before what the row does (a dialog notes where focus is): a
			// row chosen with the pointer is pressed as its menu goes, and
			// MyGo drops the menu's elements, and gives the focus back, a
			// frame after it ends that press.
			if m.settle > 0 {
				m.settle--
				c.AnimationFrame()
				return
			}
			m.pending = nil
			p()
		}
		return
	}
	if o.ID == "" {
		o.ID = "menu"
	}
	m.age++
	m.now = c.Now()
	tk := theme.Current()
	var focusEl ui.Element
	focusLv := -1
	ui.Overlay(c, func() {
		if !m.anchored {
			anchor = ui.Box(c.Key(o.ID+":at")).Absolute().Left(m.x).Top(m.y).Size(0, 0)
		}
		rows := items
		for lv := 0; lv < len(m.levels) && m.open; lv++ {
			panel, byKey := m.panel(c, o, tk, lv, rows, anchor)
			if m.open && m.focus == lv {
				focusEl, focusLv = panel, lv
			}
			anchor = panel
			if lv+1 >= len(m.levels) {
				break
			}
			sub := m.levels[lv+1].sub
			i := slices.IndexFunc(rows, func(it MenuItem) bool { return it.Key == sub })
			if i < 0 || rows[i].Kind != MenuSub || !rows[i].choosable() {
				m.levels = m.levels[:lv+1]
				break
			}
			anchor = byKey[sub]
			rows = rows[i].Sub
		}
	})
	m.moved, m.aimHold = m.aimHold, false
	if !m.open {
		m.levels = nil
		return
	}
	// After the overlay noted the focus it came with, so closing gives that back.
	if focusLv >= 0 && m.focus == focusLv {
		focusEl.Focus()
		m.focus = -1
	}
}

// panel draws the menu (lv 0) or the sub-menu at level lv; it returns the
// panel and the rows by key (a sub-menu goes beside its row).
func (m *Menu) panel(c *ui.Context, o MenuOpts, tk *theme.Tokens, lv int, items []MenuItem, anchor ui.Element) (ui.Element, map[string]ui.Element) {
	id := o.ID + ":" + string(rune('0'+lv))
	level := func() *menuLevel { return &m.levels[lv] }
	panel := ui.Scroll(c.Key(id)).Role(ui.RoleMenu).Focusable().FocusRing(false).Label(o.Label)
	sh := tk.Shadow["float"]
	panel.MinWidth(128).Padding(4).Radius(tk.Radius["lg"]).Background(theme.C("popover")).Border(1, theme.C("border")).
		Shadow(sh.X, sh.Y, sh.Blur, sh.Spread, sh.Color)
	if o.Width > 0 {
		panel.Width(o.Width)
	}
	switch {
	case lv > 0:
		panel.AttachTo(anchor, ui.AnchorTopEnd, ui.AnchorTopStart).Margin(-5, 0, 0, 0)
	case m.anchored && o.Start:
		panel.AttachTo(anchor, ui.AnchorBottomStart, ui.AnchorTopStart).Margin(4, 0, 0, 0)
	case m.anchored:
		panel.AttachTo(anchor, ui.AnchorBottomEnd, ui.AnchorTopEnd).Margin(4, 0, 0, 0)
	default:
		panel.AttachTo(anchor, ui.AnchorTopStart, ui.AnchorTopStart).Margin(2, 0, 0, 0)
	}
	if !theme.ReducedMotion() {
		mo := tk.Motion["float"]
		panel.Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 4}, Duration: mo.Duration, Ease: mo.Ease})
	}
	panel.HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerMove:
			// Taken, so a move builds a frame: the row under the pointer
			// takes the highlight there, not on the next key.
			m.moved = true
			return true
		case ui.InputKeyDown:
			// Type-ahead reads the keys: GTK and AppKit send no text to an
			// element that takes none.
			r, ok := overlaysTypedRune(ev.Mods, ev.Key)
			if !ok {
				return false
			}
			if ev.Time.Sub(m.typedAt) > time.Second {
				m.typed = ""
			}
			if r == ' ' && m.typed == "" {
				return false // Space chooses
			}
			return m.typeAhead(lv, items, string(r), ev.Time)
		}
		return false
	})

	marks := slices.ContainsFunc(items, func(it MenuItem) bool { return it.Kind == MenuCheckbox || it.Kind == MenuRadio })
	var choices []string
	for i := range items {
		if items[i].choosable() {
			choices = append(choices, items[i].Key)
		}
	}
	if m.keyboard && lv == 0 && len(choices) > 0 {
		level().hi, level().byKey = choices[0], true
	}
	if lv == 0 {
		m.keyboard = false
	}
	rowEls := map[string]ui.Element{}
	hovered := ""
	var chosen, opener *MenuItem
	panel.Children(func() {
		for i := range items {
			it := &items[i]
			switch it.Kind {
			case MenuSeparator:
				ui.Box(c.Key(id+":sep:"+strconv.Itoa(i))).Height(1).Margin(4, 0).Background(theme.C("line")).Shrink(0)
				continue
			case MenuLabel:
				menuLabel(c.Key(id+":lbl:"+strconv.Itoa(i)), *it)
				continue
			}
			row := m.row(c.Key(id+":"+it.Key), it, marks, level().hi == it.Key, level().byKey, lv)
			rowEls[it.Key] = row
			if !it.choosable() {
				continue
			}
			if row.Hovered() {
				hovered = it.Key
			}
			if row.Clicked() {
				if it.Kind == MenuSub {
					opener = it
				} else {
					chosen = it
				}
			}
		}
	})
	if hi, ok := rowEls[level().hi]; ok {
		panel.ActiveDescendant(hi)
		if level().reveal {
			hi.ScrollIntoView()
		}
	}
	level().reveal = false

	for len(m.subs) <= lv {
		m.subs = append(m.subs, new(ui.Handle))
	}
	panel.Bind(m.subs[lv])
	// The pointer moving onto a row highlights it, opening its sub-menu and
	// closing the others', unless it heads for an open sub-menu (aiming); the
	// keys go on from there.
	aim := m.aiming(c, panel, lv, hovered)
	if m.moved && hovered != "" && !aim {
		changed := level().hi != hovered
		level().hi, level().byKey = hovered, false
		isSub := slices.ContainsFunc(items, func(it MenuItem) bool { return it.Key == hovered && it.Kind == MenuSub })
		open := len(m.levels) > lv+1 && m.levels[lv+1].sub == hovered
		switch {
		case isSub && !open:
			m.levels = append(m.levels[:lv+1], menuLevel{sub: hovered})
		case !isSub && changed:
			m.levels = m.levels[:lv+1]
		}
		if changed {
			m.focus = lv
		}
	}
	if opener != nil {
		level().hi, level().byKey = opener.Key, false
		m.levels = append(m.levels[:lv+1], menuLevel{sub: opener.Key})
		m.focus = lv
	}
	if chosen != nil {
		m.choose(chosen)
		return panel, rowEls
	}
	m.keys(panel, lv, items, choices)
	if m.open && lv == 0 && m.age > 1 && panel.PressedOutside() {
		m.dismiss()
	}
	return panel, rowEls
}

// aiming reports whether the pointer, over the row hovered of the panel at
// level lv whose sub-menu is open from another row, heads for that sub-menu:
// it moved toward it since the panel last built, into the triangle between
// where it was and the sub-menu's near edge. The sub-menu then stays open with
// the highlight where it is, for menuAimGrace after the last such move; once
// the pointer rests or goes elsewhere the row under it takes the highlight,
// as the frame the grace ends in (Context.After) lets it. Where the pointer is
// is noted whenever it is over the panel, so the move right after a sub-menu
// opens has a start; the sub-menu's box is its last laid-out one.
func (m *Menu) aiming(c *ui.Context, panel ui.Element, lv int, hovered string) bool {
	l := &m.levels[lv]
	x, y, over := panel.PointerPosition()
	if !over {
		l.lastOK, l.aimUntil = false, time.Time{}
		return false
	}
	wx, wy := panel.LocalToWindow(x, y)
	at := [2]float32{wx, wy}
	moved := l.lastOK && at != l.last
	last := l.last
	l.last, l.lastOK = at, true
	if lv+1 >= len(m.levels) || lv+1 >= len(m.subs) {
		l.aimUntil = time.Time{}
		return false
	}
	if moved {
		l.aimUntil = time.Time{}
		if menuAimToward(m.subs[lv+1].Bounds(c), last, at) {
			l.aimUntil = m.now.Add(menuAimGrace)
		}
	}
	if hovered == "" || hovered == m.levels[lv+1].sub || l.aimUntil.IsZero() {
		return false
	}
	if !m.now.Before(l.aimUntil) {
		l.aimUntil = time.Time{}
		return false
	}
	c.After(l.aimUntil.Sub(m.now))
	if m.moved {
		m.aimHold = true
	}
	return true
}

// dropAim ends the pointer's claim on the highlight: a key acts on the menu,
// so a pointer move held back while it aimed at a sub-menu is not taken once
// its grace ends.
func (m *Menu) dropAim() {
	m.moved, m.aimHold = false, false
	for i := range m.levels {
		m.levels[i].aimUntil = time.Time{}
	}
}

// menuAimToward reports whether a pointer moving from one window point to
// another heads for the sub-menu panel sub: it lies in the triangle from
// where it was to the corners of the panel's edge facing it: its left edge
// where the pointer is to the left of the panel, its right edge where it is to
// the right (a sub-menu that opened before its menu, flipped at the window's
// edge or in a right-to-left layout).
func menuAimToward(sub ui.Rect, from, to [2]float32) bool {
	if sub.W <= 0 || sub.H <= 0 {
		return false
	}
	var edge float32
	switch {
	case from[0] < sub.X:
		edge = sub.X
	case from[0] > sub.X+sub.W:
		edge = sub.X + sub.W
	default:
		return false // over it, or beside it, which no triangle leads to
	}
	if (to[0]-from[0])*(edge-from[0]) <= 0 {
		return false // not toward it
	}
	top, bottom := sub.Y, sub.Y+sub.H
	side := func(ax, ay, bx, by float32) float32 {
		return (bx-ax)*(to[1]-ay) - (by-ay)*(to[0]-ax)
	}
	d1 := side(from[0], from[1], edge, top)
	d2 := side(edge, top, edge, bottom)
	d3 := side(edge, bottom, from[0], from[1])
	neg := d1 < 0 || d2 < 0 || d3 < 0
	pos := d1 > 0 || d2 > 0 || d3 > 0
	return !neg || !pos
}

// row draws one choosable row.
func (m *Menu) row(c *ui.Context, it *MenuItem, marks, hi, byKey bool, lv int) ui.Element {
	tk := theme.Current()
	row := ui.Row(c).AlignItems(ui.Center).Gap(8).Radius(tk.Radius["sm"]).Shrink(0).Padding(6, 8).Label(it.Label)
	switch it.Kind {
	case MenuCheckbox:
		row.Role(ui.RoleMenuItemCheckBox).Checked(it.Checked)
	case MenuRadio:
		row.Role(ui.RoleMenuItemRadio).Checked(it.Checked)
	default:
		row.Role(ui.RoleMenuItem)
	}
	if it.Kind == MenuSub {
		row.Expanded(len(m.levels) > lv+1 && m.levels[lv+1].sub == it.Key)
	}
	fg := theme.C("ink")
	if it.Destructive {
		fg = theme.C("danger")
	}
	switch {
	case it.Disabled || it.Reason != "":
		row.Disabled(true).Opacity(0.5)
	case hi:
		bg := theme.C("accent")
		if it.Destructive {
			bg = theme.C("danger-wash")
		}
		row.Background(bg)
	}
	// The ring is always laid out (transparent) so a highlight never moves the text.
	ring := ui.Transparent
	if hi && byKey {
		ring = theme.C("ring")
	}
	row.Border(2, ring)
	switch {
	case it.Reason != "":
		// Menu items are leaves for assistive technology: say why it is dimmed.
		row.Description(it.Reason)
	case it.Description != "":
		row.Description(it.Description)
	case it.Hint != "":
		row.Description(it.Hint)
	}
	row.TextColor(fg)
	row.Children(func() {
		switch {
		case it.Kind == MenuCheckbox || it.Kind == MenuRadio:
			slot := ui.Row(c.Key("mark")).Size(16, 16).Shrink(0).Center()
			slot.Children(func() {
				if !it.Checked {
					return
				}
				if it.Kind == MenuRadio {
					ui.Icon(c.Key("dot"), theme.IconFilled("circle")).FontSize(8).TextColor(fg)
				} else {
					ui.Icon(c.Key("check"), theme.Icon("check")).FontSize(16).TextColor(fg)
				}
			})
		case it.Icon != "":
			col := fg
			if !it.Destructive {
				col = theme.C("ink-muted")
			}
			theme.Glyph(c.Key("ic"), it.Icon, 16, theme.GlyphOpts{Color: col, Filled: it.Filled})
		case marks || it.Inset:
			ui.Box(c.Key("pad")).Size(16, 16).Shrink(0)
		}
		if it.Reason != "" {
			ui.Column(c.Key("txt")).Grow(1).MinWidth(0).Children(func() {
				theme.Text(c.Key("l"), it.Label, "body").SingleLine()
				theme.Text(c.Key("r"), it.Reason, "meta").TextColor(theme.C("ink-faint")).FontWeight(400)
			})
		} else {
			theme.Text(c.Key("l"), it.Label, "body").SingleLine().Grow(1).MinWidth(0)
		}
		if it.Hint != "" {
			theme.Text(c.Key("h"), it.Hint, "meta").SingleLine().TextColor(theme.C("ink-faint")).MarginStart(16)
		}
		if it.Kind == MenuSub {
			theme.Glyph(c.Key("sub"), "chevron-right", 16, theme.GlyphOpts{Color: theme.C("ink-muted")})
		}
	})
	return row
}

// keys moves the highlight, opens and closes sub-menus and chooses for the
// keys pressed while the panel at level lv has the focus.
func (m *Menu) keys(panel ui.Element, lv int, items []MenuItem, choices []string) {
	level := func() *menuLevel { return &m.levels[lv] }
	n := len(choices)
	at := slices.Index(choices, level().hi)
	// Space while typing ahead is part of the word, not a choice (Radix).
	typing := m.typed != "" && m.now.Sub(m.typedAt) <= time.Second
	move := func(i int) {
		m.dropAim()
		if n > 0 {
			level().hi, level().byKey, level().reveal = choices[(i%n+n)%n], true, true
			m.levels = m.levels[:lv+1]
		}
	}
	find := func(k string) *MenuItem {
		if i := slices.IndexFunc(items, func(it MenuItem) bool { return it.Key == k }); i >= 0 {
			return &items[i]
		}
		return nil
	}
	enter := func() {
		m.dropAim()
		it := find(level().hi)
		switch {
		case it == nil || !it.choosable():
		case it.Kind == MenuSub:
			m.levels = append(m.levels[:lv+1], menuLevel{sub: it.Key})
			m.levels[lv+1].hi = firstChoice(it.Sub)
			m.levels[lv+1].byKey = true
			m.focus = lv + 1
		default:
			m.choose(it)
		}
	}
	switch {
	case panel.Shortcut(0, ui.KeyDown):
		move(at + 1)
	case panel.Shortcut(0, ui.KeyUp):
		if at < 0 {
			at = 0
		}
		move(at - 1)
	case panel.Shortcut(0, ui.KeyHome):
		move(0)
	case panel.Shortcut(0, ui.KeyEnd):
		move(n - 1)
	case panel.Shortcut(0, ui.KeyRight):
		if it := find(level().hi); it != nil && it.Kind == MenuSub {
			enter()
		}
	case panel.Shortcut(0, ui.KeyLeft):
		if lv > 0 {
			m.dropAim()
			m.levels = m.levels[:lv]
			m.focus = lv - 1
		}
	case panel.Shortcut(0, ui.KeyEnter), !typing && panel.Shortcut(0, ui.KeySpace):
		enter()
	case panel.Shortcut(0, ui.KeyTab), panel.Shortcut(ui.Shift, ui.KeyTab):
		m.dismiss()
		return
	}
	// The focus is in the panel, which takes Escape first: MyGo builds the
	// tips after the view, over every overlay, so a tip showing elsewhere
	// (the pointer resting on a toolbar button) would take an overlay's
	// Escape and leave the menu open. The overlay's is for a menu the focus
	// has left.
	if panel.Shortcut(0, ui.KeyEscape) || panel.OverlayShortcut(0, ui.KeyEscape) {
		if lv == 0 {
			m.dismiss()
		} else {
			m.dropAim()
			m.levels = m.levels[:lv]
			m.focus = lv - 1
		}
	}
}

// typeAhead moves to the next choosable row whose label starts with what
// was typed (the same letter again goes on to the next).
func (m *Menu) typeAhead(lv int, items []MenuItem, text string, now time.Time) bool {
	if lv >= len(m.levels) {
		return false
	}
	if now.Sub(m.typedAt) > time.Second {
		m.typed = ""
	}
	m.typedAt = now
	m.typed += strings.ToLower(text)
	var choices []*MenuItem
	for i := range items {
		if items[i].choosable() {
			choices = append(choices, &items[i])
		}
	}
	if len(choices) == 0 {
		return true
	}
	level := &m.levels[lv]
	at := slices.IndexFunc(choices, func(it *MenuItem) bool { return it.Key == level.hi })
	// The same letter again goes on to the next row starting with it.
	q := m.typed
	if r := []rune(q); len(r) > 1 && strings.Count(q, string(r[0])) == len(r) {
		q = string(r[0])
	}
	start := at
	if len([]rune(q)) > 1 && at >= 0 {
		start = at - 1
	}
	for k := 1; k <= len(choices); k++ {
		j := ((start+k)%len(choices) + len(choices)) % len(choices)
		if strings.HasPrefix(strings.ToLower(choices[j].Label), q) {
			level.hi, level.byKey, level.reveal = choices[j].Key, true, true
			m.levels = m.levels[:lv+1]
			m.dropAim()
			break
		}
	}
	return true
}

func firstChoice(items []MenuItem) string {
	for i := range items {
		if items[i].choosable() {
			return items[i].Key
		}
	}
	return ""
}

func (m *Menu) choose(it *MenuItem) {
	if it.KeepOpen {
		if it.Run != nil {
			it.Run()
		}
		return
	}
	m.pending, m.settle = it.Run, 2
	m.setOpen(false)
	m.levels = nil
}

func (m *Menu) dismiss() {
	m.setOpen(false)
	m.levels = nil
	m.dismissed = true
}

// overlaysTypedRune is the character a key types for type-ahead: letters and
// digits as the layout types them, and Space; none with Ctrl, Alt or Super.
func overlaysTypedRune(mods ui.Modifiers, key ui.Key) (rune, bool) {
	if mods&^ui.Shift != 0 {
		return 0, false
	}
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return 'a' + rune(key-ui.KeyA), true
	case key >= ui.Key0 && key <= ui.Key9:
		return '0' + rune(key-ui.Key0), true
	case key == ui.KeySpace:
		return ' ', true
	}
	return 0, false
}

// menuLabel builds a MenuLabel row by its Note.
func menuLabel(c *ui.Context, it MenuItem) {
	switch it.Note {
	case LabelHeader:
		theme.Text(c, it.Label, "label").TextColor(theme.C("ink")).Padding(6, 8).Shrink(0)
	case LabelNote:
		ui.Row(c).AlignItems(ui.Start).Gap(6).Padding(2, 8).Shrink(0).Children(func() {
			if it.Icon != "" {
				fg := theme.C("ink-muted")
				if it.Destructive {
					fg = theme.C("danger")
				}
				theme.Glyph(c.Key("icon"), it.Icon, 14, theme.GlyphOpts{Color: fg}).Shrink(0).Margin(1, 0, 0, 0)
			}
			theme.Text(c.Key("text"), it.Label, "meta").TextColor(theme.C("ink-muted"))
		})
	default:
		theme.Text(c, it.Label, "meta").TextColor(theme.C("ink-faint")).Padding(6, 8).Shrink(0)
	}
}
