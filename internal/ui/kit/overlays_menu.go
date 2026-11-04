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
	// Destructive colours the row --danger.
	Destructive bool
	// Hint is a muted right-aligned shortcut ("Ctrl+K").
	Hint string
	// Inset indents the row to line up with rows that have an icon or mark.
	Inset bool
	// Sub holds a MenuSub's rows.
	Sub []MenuItem
	// Run runs after the menu has closed and given the focus back.
	Run func()
}

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
	now   time.Time
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
}

// IsOpen reports whether the menu shows.
func (m *Menu) IsOpen() bool { return m.open }

// OpenAt opens the menu at a point in window DIPs (a right-click, the
// focused row's corner for Shift+F10). keyboard starts it on its first row.
func (m *Menu) OpenAt(x, y float32, keyboard bool) {
	m.start(false, keyboard)
	m.x, m.y = x, y
}

// OpenFrom opens the menu below its trigger (MenuOpts.Anchor); keyboard
// (Enter, Space, a click with no pointer) starts it on its first row.
func (m *Menu) OpenFrom(keyboard bool) { m.start(true, keyboard) }

func (m *Menu) start(anchored, keyboard bool) {
	m.open, m.anchored, m.keyboard = true, anchored, keyboard
	m.levels = []menuLevel{{}}
	m.focus, m.age, m.typed, m.dismissed = 0, 0, "", false
}

// Close closes the menu (a trigger pressed again).
func (m *Menu) Close() { m.open = false; m.levels = nil }

// Dismissed reports, once, that the user closed the menu without choosing.
func (m *Menu) Dismissed() bool { d := m.dismissed; m.dismissed = false; return d }

// Show draws the menu while it is open; call it every frame after the
// trigger, whose element is anchor (the panel goes below it, above where
// there is no room, its end lined up with the trigger's; ignored for a menu
// opened at a point). It also runs what the last choice left to run.
func (m *Menu) Show(c *ui.Context, o MenuOpts, anchor ui.Element, items []MenuItem) {
	if !m.open {
		if p := m.pending; p != nil {
			// One frame after the menu is gone, so the focus has gone back
			// before what the row does (a dialog notes where focus is).
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
	m.moved = false
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
				overlaysText(c.Key(id+":lbl:"+strconv.Itoa(i)), it.Label, "meta").TextColor(theme.C("ink-faint")).Padding(6, 8).Shrink(0)
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

	// The pointer moving onto a row highlights it, opening its sub-menu and
	// closing the others'; the keys go on from there.
	if m.moved && hovered != "" {
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
			ic := theme.Icon(it.Icon)
			if it.Filled {
				ic = theme.IconFilled(it.Icon)
			}
			col := fg
			if !it.Destructive {
				col = theme.C("ink-muted")
			}
			ui.Icon(c.Key("ic"), ic).FontSize(16).TextColor(col)
		case marks || it.Inset:
			ui.Box(c.Key("pad")).Size(16, 16).Shrink(0)
		}
		if it.Reason != "" {
			ui.Column(c.Key("txt")).Grow(1).MinWidth(0).Children(func() {
				overlaysText(c.Key("l"), it.Label, "body").SingleLine()
				overlaysText(c.Key("r"), it.Reason, "meta").TextColor(theme.C("ink-faint")).FontWeight(400)
			})
		} else {
			overlaysText(c.Key("l"), it.Label, "body").SingleLine().Grow(1).MinWidth(0)
		}
		if it.Hint != "" {
			overlaysText(c.Key("h"), it.Hint, "meta").SingleLine().TextColor(theme.C("ink-faint")).MarginStart(16)
		}
		if it.Kind == MenuSub {
			ui.Icon(c.Key("sub"), theme.Icon("chevron-right")).FontSize(16).TextColor(theme.C("ink-muted"))
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
			m.levels = m.levels[:lv]
			m.focus = lv - 1
		}
	case panel.Shortcut(0, ui.KeyEnter), !typing && panel.Shortcut(0, ui.KeySpace):
		enter()
	case panel.Shortcut(0, ui.KeyTab), panel.Shortcut(ui.Shift, ui.KeyTab):
		m.dismiss()
		return
	}
	if panel.OverlayShortcut(0, ui.KeyEscape) {
		if lv == 0 {
			m.dismiss()
		} else {
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
	m.pending, m.settle = it.Run, 1
	m.open, m.levels = false, nil
}

func (m *Menu) dismiss() {
	m.open, m.levels = false, nil
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

// overlaysText is a Text in one of the theme's type styles.
func overlaysText(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}
