package home

import (
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Geometry of the item rows (v0.2.0's ItemRows), in DIPs.
const (
	rowMin   = 264 // the smallest column
	rowH     = 56  // h-14
	rowArt   = 40
	rowPadX  = 8
	rowGap   = 12
	rowGapX  = kit.TileGapX
	rowMarkA = 0.7 // the play mark's ground wash
)

// rowsState is the state of one ItemRows list: its selection and menu, its
// keyboard navigation and what the last frame showed.
type rowsState struct {
	sel   kit.Selection
	nav   kit.GridNav
	menus *kit.ActionMenus
	width widthTracker
	cols  int

	// The rows shown, for the selection and the drags: an ID per row, and
	// the item's action target where it has one (keys is "" for a row that
	// cannot be selected).
	first   *services.Item
	count   int
	ids     []string
	keys    kit.RowKeys
	targets []kit.Item
}

// rowsOpts are the options of itemRows.
type rowsOpts struct {
	// Label names the grid for assistive technology.
	Label string
	// Rows shown until Open.
	Rows int
	// Open shows every item.
	Open bool
	// Min is the smallest column width (rowMin).
	Min float32
	// OnColumns reports the columns that fit, when they change (to offer
	// "Show all").
	OnColumns func(cols int)
	// OnBackdrop: the rows sit on a cover's backdrop, where a hover tints
	// with ink, not the opaque --hover fill.
	OnBackdrop bool
}

// sync brings the rows' IDs, keys and targets up to date with the items
// shown: they are rebuilt only when the items are another slice.
func (st *rowsState) sync(items []services.Item, p *kit.ItemPlayers) {
	if st.ids != nil && len(items) == st.count && (len(items) == 0 || &items[0] == st.first) {
		return
	}
	st.count, st.first = len(items), nil
	if len(items) > 0 {
		st.first = &items[0]
	}
	st.ids = make([]string, len(items))
	st.keys = make(kit.RowKeys, len(items))
	st.targets = make([]kit.Item, len(items))
	for i := range items {
		it := &items[i]
		id := it.ID
		if id == "" {
			id = it.URL
		}
		st.ids[i] = strconv.Itoa(i) + ":" + it.Kind + ":" + id
		if t, ok := kit.ActionItemOf(*it, p); ok {
			st.targets[i], st.keys[i] = t, st.ids[i]
		}
	}
}

func (st *rowsState) itemAt(i int) (kit.Item, bool) {
	if i < 0 || i >= len(st.keys) || st.keys[i] == "" {
		return kit.Item{}, false
	}
	return st.targets[i], true
}

// itemRows is albums, playlists and stations as compact rows (40 DIP art,
// title, Apple's reason and the artist or curator) in as many columns as fit,
// reading across (v0.2.0's ItemRows). One Tab stop: the arrow keys, Home and
// End move between rows, Enter opens, Space plays, Shift+F10 or the Menu key
// opens the row's menu. Rows select, open their menu on a right-click and
// drag like cover tiles.
func (h *home) itemRows(c *ui.Context, st *rowsState, items []services.Item, ctx kit.ItemsCtx, o rowsOpts) {
	if st.menus == nil {
		st.menus = kit.NewActionMenus(h.env.Actions, &st.sel)
	}
	if o.Min == 0 {
		o.Min = rowMin
	}
	hint := i18n.T("Enter opens, Space plays, Shift+F10 shows more actions.")
	root := ui.Grid(c.Key("rows." + o.Label)).GapX(rowGapX).Role(ui.RoleList).Label(o.Label).Description(hint)
	width := st.width.track(c, root)
	cols := 1
	if width > 0 {
		cols, _ = kit.CoverColumns(width, o.Min)
		root.Columns(cols)
	}
	root.Children(func() {
		if width <= 0 {
			ui.Box(c).Height(1) // the first frame measures; the next builds the rows
			return
		}
		if cols != st.cols {
			st.cols = cols
			if o.OnColumns != nil {
				o.OnColumns(cols)
			}
		}
		shown := items
		if !o.Open {
			shown = items[:min(len(items), cols*o.Rows)]
		}
		st.sync(shown, ctx.Players)
		st.sel.Hold = func() bool { return st.menus.IsOpen() || kit.ConfirmPending() }
		st.sel.Rows(st.keys, st.itemAt)
		st.nav.Count, st.nav.Columns, st.nav.Page, st.nav.Ends, st.nav.ScrollTo = len(shown), cols, 0, true, nil
		if f := st.nav.Focus(); f >= len(shown) {
			st.nav.SetFocus(max(0, len(shown)-1)) // the roving stop stays on a row
		}
		for i := range shown {
			h.itemRow(c, st, ctx, o, shown[i], i)
		}
		st.nav.Tick(c)
		st.sel.OutsidePress(root)
		st.menus.Popup(c)
	})
}

// itemRow is one row.
func (h *home) itemRow(c *ui.Context, st *rowsState, ctx kit.ItemsCtx, o rowsOpts, it services.Item, i int) {
	tk := theme.Current()
	key := st.keys[i]
	selectable := key != ""
	selected := selectable && st.sel.Selected(key)
	withheld := ctx.Restricted && it.Explicit
	link := kit.ItemLink(it)
	canOpen := h.opens(link)
	playable := ctx.Players.CanPlay(it) && !withheld
	// A station with no page plays when it is opened: the name is that
	// button, and the cover's play mark is not a second control.
	direct := playable && kit.PlaysWhenOpened(it, ctx.Players)
	round := kit.ItemRound(it)
	subtitle := kit.ItemSubtitle(it)
	name := it.Name
	art := it.Art.Small
	if art == "" {
		art = it.Art.Medium
	}
	target := kit.ActionTarget{Item: st.targets[i], Name: name, Key: key}
	menu := selectable && h.hasMenu(st.targets[i])
	hoverBg := theme.C("hover")
	if o.OnBackdrop {
		hoverBg = theme.C("ink").Alpha(0.06)
	}
	radius := tk.Radius["sm"]
	if round {
		radius = tk.Radius["full"]
	}

	_, src := kit.DragItem(c, kit.DragOpts{
		Key: st.ids[i], Scope: st, Index: i, Art: h.env.Art, Row: true,
		Payload: func() *kit.DragPayload {
			if !selectable || withheld {
				return nil
			}
			items, _, group := st.sel.GroupWith(key)
			label := name
			if group {
				label = kit.DescribeItems(items)
			} else {
				items = []kit.Item{target.Item}
			}
			return &kit.DragPayload{Items: items, Label: label, Art: it.Art.Small}
		},
		Outer: func(w ui.Element) {
			w.Height(rowH).Shrink(0).MinWidth(0).Role(ui.RoleListItem)
			if selectable {
				w.Checked(selected)
			}
			if withheld {
				w.Opacity(0.5).Tooltip(i18n.T("Explicit content is restricted on this account"))
			}
			bg := hoverBg.Alpha(0)
			switch {
			case selected:
				bg = theme.C("ink").Alpha(0.12)
			case w.Hovered():
				bg = hoverBg
			}
			if !selected && !theme.ReducedMotion() {
				m := tk.Motion["press"]
				bg = w.AnimateColor("home.bg", bg, ui.Animation{Duration: m.Duration, Ease: m.Ease})
			}
			radius := tk.Radius["md"]
			w.Draw(func(p *ui.Painter, r ui.Rect) {
				if bg.A > 0 {
					p.Fill(r, bg, radius)
				}
			})
		},
		Source: func(e ui.Element) {
			e.PaddingX(rowPadX).Gap(rowGap).Radius(tk.Radius["md"]).Label(name)
			if subtitle != "" {
				// A button or a link is read as one piece: the line under
				// the name is its description.
				e.Description(subtitle)
			}
			switch {
			case direct:
				e.Role(ui.RoleButton).Label(name + " " + i18n.T("Play"))
			case link != "" && canOpen:
				e.Role(ui.RoleLink)
			case link != "":
				e.Role(ui.RoleGroup) // its page is not served: nothing to press
			default:
				e.Role(ui.RoleButton)
			}
			if i == st.nav.Focus() {
				e.Focusable()
			}
		},
		After: func(w, src ui.Element) {
			// The play control and the more button are elements of their own,
			// beside the opener: a button or a link is a leaf for assistive
			// technology, so inside it they would not be on its tree. Both
			// are built at rest, hidden: the pointer and Tab ignore them (the
			// row is the list's one stop; Space plays, Shift+F10 opens the
			// menu), assistive technology finds and presses them.
			hover := w.Hovered()
			if playable && !direct {
				play := ui.ButtonBase(c.Key("play")).Absolute().Left(rowPadX).Top((rowH-rowArt)/2).Size(rowArt, rowArt).Center().Radius(radius).
					Background(theme.C("ground").Alpha(rowMarkA)).Label(i18n.T("Play {name}", "name", name)).IgnoreInput(!hover)
				kit.Fade(play, hover)
				play.Children(func() {
					theme.Glyph(c, "play", 16, theme.GlyphOpts{Color: theme.C("ink"), Filled: true}).Margin(0, 0, 0, 1)
				})
				if play.Clicked() {
					ctx.Players.PlayItem(it)
				}
			}
			if menu {
				st.menus.Trigger(c.Key("more"), target, kit.TriggerOpts{Visible: hover || src.FocusWithin()}).
					Absolute().Right(rowPadX).Top((rowH - 24) / 2)
			}
		},
	}, func(w, src ui.Element) {
		hot := w.Hovered() || src.FocusWithin()
		ui.Box(c.Key("art")).Size(rowArt, rowArt).Shrink(0).Children(func() {
			kit.Artwork(c, h.env.Art, art, rowArt, kit.ArtOpts{Round: round})
			if direct {
				// A station with no page plays when it is opened: the mark is
				// the name's button, not a second control.
				mark := ui.Row(c.Key("mark")).Absolute().Left(0).Top(0).Size(rowArt, rowArt).Center().Radius(radius).
					Background(theme.C("ground").Alpha(rowMarkA)).IgnoreInput(true)
				kit.Fade(mark, hot)
				mark.Children(func() {
					theme.Glyph(c, "play", 16, theme.GlyphOpts{Color: theme.C("ink"), Filled: true}).Margin(0, 0, 0, 1)
				})
			}
		})
		ui.Column(c.Key("text")).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c.Key("name")).Gap(4).AlignItems(ui.Center).MinWidth(0).Children(func() {
				label := tk.Type["label"]
				t := ui.Text(c, name).FontSize(tk.Type["ui"].Size).FixedLineHeight(tk.Type["ui"].LineHeight).FontWeight(int(label.Weight)).
					TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0).TextAlign(ui.Start)
				if w.Hovered() && canOpen {
					t.Underline()
				}
				if it.Explicit {
					kit.ExplicitMark(c.Key("explicit"))
				}
			})
			if subtitle != "" {
				theme.Text(c, subtitle, "sub").TextColor(theme.C("ink-muted")).SingleLine().TextAlign(ui.Start)
			}
		})
		if menu {
			ui.Box(c.Key("more-room")).Size(24, 24).Shrink(0).IgnoreInput(true) // where the more button sits
		}
	})

	st.nav.Claim(src, i)
	if src.Clicked() {
		mods := kit.ModsOf(src.ClickModifiers())
		st.nav.SetFocus(i)
		if !src.Focused() {
			st.nav.Want(i) // the clicked row takes the keyboard focus
		}
		switch {
		case mods.Range || mods.Toggle:
			if selectable {
				st.sel.Click(i, mods)
			}
		default:
			h.open(it, ctx)
		}
	}
	if src.Focused() {
		switch {
		case st.nav.Keys(src, i, &st.sel):
		case st.cols == 1 && h.rowSideways(src, st, i):
		case st.sel.Keys(src, i):
		case src.Shortcut(0, ui.KeySpace):
			if playable {
				ctx.Players.PlayItem(it)
			}
		case src.Shortcut(0, ui.KeyEnter):
			h.open(it, ctx)
		}
	}
	if selectable {
		rightClickMenu(&st.sel, st.menus, src, target, i)
		keyMenu(&st.sel, st.menus, src, target, i)
	}
}

// hasMenu says whether the item has a more menu (a kind with actions, or one
// the bulk actions take): the more button is built, and its room kept, only
// then.
func (h *home) hasMenu(it kit.Item) bool {
	return h.env.Actions.Has(it.Kind) || len(h.env.Actions.BulkFor([]kit.Item{it})) > 0
}

// rowSideways moves the focus with Left and Right in a list of one column,
// where v0.2.0's rows moved by one with them as with Up and Down (the kit's
// grid navigation leaves them to the columns).
func (h *home) rowSideways(src ui.Element, st *rowsState, i int) bool {
	for _, shift := range [...]bool{false, true} {
		mods := ui.Modifiers(0)
		if shift {
			mods = ui.Shift
		}
		d := 0
		switch {
		case src.Shortcut(mods, ui.KeyLeft):
			d = -1
		case src.Shortcut(mods, ui.KeyRight):
			d = 1
		default:
			continue
		}
		if to := i + d; to >= 0 && to < st.nav.Count {
			st.nav.Want(to)
			st.sel.ArrowTo(to, shift)
		}
		return true
	}
	return false
}

// rightClickMenu opens an item's menu at the pointer on a right-click, first
// selecting the item alone when it is outside the selection (a menu acts on
// the whole selection only when the item is in it).
func rightClickMenu(sel *kit.Selection, menus *kit.ActionMenus, e ui.Element, t kit.ActionTarget, index int) {
	if e.RightClicked() {
		sel.MenuOn(index)
		x, y, _ := e.PointerPosition()
		wx, wy := e.LocalToWindow(x, y)
		menus.OpenAt(t, wx, wy, false)
	}
}

// keyMenu opens an item's menu below e on Shift+F10 or the Menu key while e
// has the focus.
func keyMenu(sel *kit.Selection, menus *kit.ActionMenus, e ui.Element, t kit.ActionTarget, index int) {
	if e.Shortcut(ui.Shift, ui.KeyF10) || e.Shortcut(0, ui.KeyContextMenu) {
		sel.MenuOn(index)
		b := e.Bounds()
		menus.OpenAt(t, b.X+16, b.Y+b.H, true)
	}
}
