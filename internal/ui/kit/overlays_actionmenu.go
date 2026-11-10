package kit

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// SelectionScope is the enclosing list's selection (collections' Selection
// implements it): when the item with this key is one of several selected,
// the menu acts on all of them (the `targets[]` form).
type SelectionScope interface {
	// GroupWith returns the selected items in list order and their
	// positions when key is one of two or more selected rows; ok is false
	// when the row is alone or outside the selection.
	GroupWith(key string) (items []Item, indices []int, ok bool)
}

// ActionTarget is the row or tile a menu belongs to.
type ActionTarget struct {
	Item Item
	// Name is the item's name for the trigger's accessible name: "More for
	// Airbag".
	Name string
	// Container is the collection the item is listed in (a playlist's
	// tracks); Index is filled in by the menu for bulk runs.
	Container *ActionContainer
	// Key is the item's key in the enclosing selection ("" for none).
	Key string
}

// TriggerOpts shapes Trigger.
type TriggerOpts struct {
	// Header makes the trigger an always-visible outline icon button (a
	// collection header's action row); otherwise it is a quiet ghost button
	// that shows while Visible.
	Header bool
	// Visible: the row is hovered or has the focus within. The trigger is
	// shown while its menu is open as well.
	Visible bool
}

// ActionMenus is the "more" menu of a list's rows and tiles (v0.2.0's
// ActionMenu, ItemActionMenu and ContextActionMenu): every registered
// action for the target, grouped and ordered by the registry, with the
// menu data (ratings) read when the menu opens, not per row. A row in a
// multi-selection lists the bulk actions for the whole selection instead,
// under a muted "<what> selected" label.
//
// One ActionMenus serves a whole list or page; only one menu is open at a
// time. Per row call Trigger (the button; it draws its own menu) and
// Row (right-click, Shift+F10 or the Menu key open a menu at a point), and
// call Popup once after the rows to draw point menus. Everything runs on
// the main thread.
type ActionMenus struct {
	reg  *ActionRegistry
	sel  SelectionScope
	menu Menu
	cur  openMenu
	// Navigate, when set, opens the pages of the menu's actions instead of the
	// registry's navigator (a surface that covers the page closes first).
	Navigate func(path string)
	// Before, when set, runs just before a chosen action does (a surface that
	// is not the main window brings it back for an action that needs it).
	Before func(d *ActionDef)
	epoch  int // frames, counted by Popup
}

type overlaysLoaded struct {
	id string
	v  any
}

// openMenu is what the menu captured when it opened: the selection is
// taken then, not as it changes under an open menu.
type openMenu struct {
	target  ActionTarget
	items   []Item
	bulk    bool
	indices []int
	loaded  map[string]any
	// results carries loaded menu data from the goroutines; the main thread
	// alone writes loaded, from it (the Poster only asks for a frame, so a
	// synchronous test Poster cannot race the map).
	results chan overlaysLoaded
	cancel  context.CancelFunc
	// anchored: opened from its trigger rather than at a point.
	anchored bool
	// seen is the Popup epoch the menu was last drawn in: a menu whose row
	// stopped being built (it scrolled out of a virtualised list) closes.
	seen int
}

// NewActionMenus returns the menus of a list. sel may be nil (no bulk
// menus).
func NewActionMenus(reg *ActionRegistry, sel SelectionScope) *ActionMenus {
	return &ActionMenus{reg: reg, sel: sel}
}

// IsOpen reports whether a menu shows.
func (a *ActionMenus) IsOpen() bool { return a.menu.IsOpen() }

// OpenKey is the selection key of the row whose menu is open ("" when none).
func (a *ActionMenus) OpenKey() string {
	if !a.menu.IsOpen() {
		return ""
	}
	return a.cur.target.Key
}

// Close closes the open menu.
func (a *ActionMenus) Close() { a.menu.Close(); a.release() }

func (a *ActionMenus) release() {
	if a.cur.cancel != nil {
		a.cur.cancel()
		a.cur.cancel = nil
	}
}

// open captures the selection and starts loading the menu data.
func (a *ActionMenus) open(t ActionTarget, anchored bool) {
	a.release()
	cur := openMenu{target: t, items: []Item{t.Item}, anchored: anchored, seen: a.epoch, loaded: map[string]any{}}
	if a.sel != nil && t.Key != "" {
		if items, idx, ok := a.sel.GroupWith(t.Key); ok && len(items) >= 2 {
			cur.items, cur.indices, cur.bulk = items, idx, true
		}
	}
	a.cur = cur
	a.loadData()
}

// actions are the registered actions the open menu lists, unfiltered by
// availability.
func (a *ActionMenus) actions() []*ActionDef {
	if a.cur.bulk {
		return a.reg.BulkFor(a.cur.items)
	}
	return a.reg.For(a.cur.target.Item.Kind)
}

// context is the ActionContext of the open menu with the data loaded so far.
func (a *ActionMenus) context() *ActionContext {
	var ct *ActionContainer
	if c := a.cur.target.Container; c != nil {
		cp := *c
		if a.cur.bulk {
			cp.Indices, cp.Index = make([]int, len(a.cur.indices)), 0
			for i, row := range a.cur.indices {
				cp.Indices[i] = c.place(row)
			}
		}
		ct = &cp
	}
	for drained := false; !drained && a.cur.results != nil; {
		select {
		case r := <-a.cur.results:
			a.cur.loaded[r.id] = r.v
		default:
			drained = true
		}
	}
	ctx := a.reg.Context(a.cur.items, ct, a.cur.loaded)
	if a.Navigate != nil {
		ctx.Navigate = a.Navigate
	}
	return ctx
}

// loadData loads every distinct menu data the open menu's actions read,
// once, off the main thread; the results come back through the registry's
// Poster. Actions sharing a MenuData (by ID) share one load.
func (a *ActionMenus) loadData() {
	seen := map[string]bool{}
	var datas []MenuData
	for _, d := range a.actions() {
		for _, md := range d.Data {
			if !seen[md.ID] {
				seen[md.ID] = true
				datas = append(datas, md)
			}
		}
	}
	if len(datas) == 0 {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cur.cancel = cancel
	results := make(chan overlaysLoaded, len(datas))
	a.cur.results = results
	items := a.cur.items
	post := a.reg.poster()
	for _, md := range datas {
		go func() {
			v := md.Load(ctx, items)
			if ctx.Err() != nil {
				return
			}
			results <- overlaysLoaded{md.ID, v}
			post(func() {}) // a frame, which reads the result
		}()
	}
}

// menuItems are the rows of the open menu: the actions available for the
// target in group order, a separator between neighbouring non-empty groups.
func (a *ActionMenus) menuItems() []MenuItem {
	ctx := a.context()
	var out []MenuItem
	if a.cur.bulk {
		out = append(out, MenuItem{
			Key: "selected", Kind: MenuLabel,
			Label: i18n.T("{what} selected", "what", DescribeItems(a.cur.items)),
		})
	}
	var avail []*ActionDef
	for _, d := range a.actions() {
		if d.Available == nil || d.Available(ctx) {
			avail = append(avail, d)
		}
	}
	for i, sec := range Sections(avail) {
		if i > 0 {
			out = append(out, MenuItem{Key: "sep:" + string(rune('0'+i)), Kind: MenuSeparator})
		}
		for _, d := range sec.Actions {
			it := MenuItem{Key: d.ID, Label: d.Label(ctx), Icon: d.Icon, Run: func() {
				if a.Before != nil {
					a.Before(d)
				}
				d.Run(ctx)
			}}
			if d.Filled != nil {
				it.Filled = d.Filled(ctx)
			}
			if d.Enabled != nil {
				it.Disabled = !d.Enabled(ctx)
			}
			if d.DisabledReason != nil {
				it.Reason = d.DisabledReason(ctx)
			}
			out = append(out, it)
		}
	}
	return out
}

func (a *ActionMenus) show(c *ui.Context, anchor ui.Element) {
	if !a.menu.IsOpen() {
		a.menu.Show(c, MenuOpts{}, anchor, nil) // runs the row a choice left to run
		a.release()
		return
	}
	a.cur.seen = a.epoch
	a.menu.Show(c, MenuOpts{ID: "actions", Label: i18n.T("More for {name}", "name", a.cur.target.Name), Width: 224}, anchor, a.menuItems())
	if !a.menu.IsOpen() {
		a.release()
	}
}

// Trigger draws the row's "More for <name>" button and, while its menu is
// open, the menu below it (aligned to its end, 224 DIPs wide). It draws
// nothing for a kind with no registered actions. The row variant is a
// quiet 24 DIP ghost button, visible while the row is hovered, has the
// focus within or its menu is open; it is not a tab stop (a list is one
// stop: Shift+F10 or the Menu key on the row open the same menu). The
// header variant is an always-visible outline icon button.
func (a *ActionMenus) Trigger(c *ui.Context, t ActionTarget, o TriggerOpts) ui.Element {
	if !a.reg.Has(t.Item.Kind) && a.reg.BulkFor([]Item{t.Item}) == nil {
		return ui.Element{}
	}
	mine := a.menu.IsOpen() && a.cur.target.Key == t.Key && a.cur.target.Item.Kind == t.Item.Kind &&
		a.cur.target.Item.ID() == t.Item.ID() && a.cur.anchored
	label := i18n.T("More for {name}", "name", t.Name)
	id := "more:" + t.Key + string(t.Item.Kind) + t.Item.ID()
	var btn ui.Element
	clicked := false
	keyboard := false
	if o.Header {
		btn = Button(c.Key(id), "", ButtonOpts{Variant: Outline, Size: SizeSM, Icon: "ellipsis", A11yLabel: label})
		clicked, keyboard = btn.Clicked(), btn.FocusVisible()
		btn.Role(ui.RoleMenuButton).Expanded(mine)
	} else {
		btn = ui.Row(c.Key(id)).Size(24, 24).Shrink(0).Center().Radius(theme.Current().Radius["md"]).Role(ui.RoleMenuButton).
			Label(label).Cursor(ui.CursorPointer).Expanded(mine)
		fg := theme.C("ink-muted")
		if btn.Hovered() || mine {
			fg = theme.C("ink")
			btn.Background(theme.C("accent-soft"))
		}
		if !o.Visible && !mine && !btn.FocusWithin() {
			btn.Opacity(0)
		}
		clicked = btn.Clicked()
		btn.TextColor(fg).Children(func() { theme.Glyph(c, "ellipsis", 16, theme.GlyphOpts{Color: fg, Inherit: true}) })
	}
	if clicked {
		if mine {
			a.Close()
		} else {
			a.open(t, true)
			a.menu.OpenFrom(keyboard)
			mine = true
		}
	}
	if mine {
		a.show(c, btn)
	}
	return btn
}

// Row wires a row or tile to open its menu at a point: right-click (where
// the pointer is) and Shift+F10 or the Menu key while the row has the
// focus within (below the row's start). Call it after building the row
// element; draw the menu with Popup.
func (a *ActionMenus) Row(row ui.Element, t ActionTarget) {
	if row.RightClicked() {
		x, y, _ := row.PointerPosition()
		wx, wy := row.LocalToWindow(x, y)
		a.OpenAt(t, wx, wy, false)
	}
	if row.Shortcut(ui.Shift, ui.KeyF10) || row.Shortcut(0, ui.KeyContextMenu) {
		b := row.Bounds()
		a.OpenAt(t, b.X+16, b.Y+b.H, true)
	}
}

// OpenAt opens the target's menu at a point in window DIPs; keyboard starts
// it on its first row. Draw it with Popup.
func (a *ActionMenus) OpenAt(t ActionTarget, x, y float32, keyboard bool) {
	if !a.reg.Has(t.Item.Kind) {
		return
	}
	a.open(t, false)
	a.menu.OpenAt(x, y, keyboard)
}

// Popup draws a menu opened at a point and runs what a chosen row left to
// run once the menu closed. Call it once per list, after the rows, every
// frame.
func (a *ActionMenus) Popup(c *ui.Context) {
	a.epoch++
	if a.menu.IsOpen() && a.epoch-a.cur.seen > 1 {
		a.Close() // its row is no longer built
	}
	if a.menu.IsOpen() && a.cur.anchored {
		return // its Trigger draws it
	}
	a.show(c, ui.Element{})
}
