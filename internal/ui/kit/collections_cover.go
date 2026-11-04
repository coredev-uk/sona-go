package kit

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Geometry of cover tiles (v0.2.0's CoverGrid and useCoverColumns), in DIPs.
const (
	// TileGapX is the gap between tiles of a row, TileGapY the one between rows.
	TileGapX = 24
	TileGapY = 28
	// TileMin is the default smallest tile width; tiles grow to fill the row.
	TileMin = 160
	// tileText is the cover-to-title gap plus the 14/20 title and 13/18
	// subtitle lines.
	tileText = 8 + 20 + 18
	// VirtualFrom: grids longer than this are built by row, as they show.
	VirtualFrom = 200
	// coverOverscan is how many rows beyond the ones showing are built.
	coverOverscan = 3
)

// CoverColumns is how many tiles of at least min DIPs fit a row of the given
// width with TileGapX between, and the exact tile width that fills it. Artwork
// is requested at that width. A width of 0 (before the first layout) gives
// one column of min.
func CoverColumns(width, min float32) (cols int, tile float32) {
	if min <= 0 {
		min = TileMin
	}
	if width <= 0 {
		return 1, min
	}
	cols = max(1, int((width+TileGapX)/(min+TileGapX)))
	return cols, (width - TileGapX*float32(cols-1)) / float32(cols)
}

// coverArtHeight is the height of a tile's cover: square, or 16:9.
func coverArtHeight(tile float32, video bool) float32 {
	if video {
		return tile * 9 / 16
	}
	return tile
}

// CoverRowHeight is the height one row of a CoverGrid takes at a tile width,
// the gap below it included.
func CoverRowHeight(tile float32, video bool) float32 {
	return coverArtHeight(tile, video) + tileText + TileGapY
}

// CoverPlay is a tile's play control: its label ("Airbag" in "Play Airbag")
// and what it does.
type CoverPlay struct {
	Label string
	Run   func()
	// RunFrom, when set, runs instead of Run with the handle of the play
	// button (bound by the tile): what opens a view over the page passes it
	// on (store.VideoView.OpenFrom), so closing the view puts the focus back
	// on this button.
	RunFrom func(opener *ui.Handle)
}

// CoverItem is one cover tile (v0.2.0's CoverItem).
type CoverItem struct {
	// Key identifies the tile; unique in its grid.
	Key      string
	Title    string
	Subtitle string
	// Art is the artwork's URL (a DTO's "/art/<key>/…").
	Art string
	// Round draws the cover as a circle and centres the texts (artists).
	Round bool
	// Video makes the cover 16:9 (music videos); else it is square.
	Video bool
	// Open runs when the tile is opened (a click or Enter on it): the
	// navigation to the item's page, or what a page-less item does. Nil:
	// the tile is not a link.
	Open func()
	// Play is a play control on the cover, for items that play directly.
	Play *CoverPlay
	// PlaysOnOpen: the tile plays when opened (a station with no page); its
	// opener is named "{title} Play" and the cover shows a play mark on
	// hover, which is not a second control.
	PlaysOnOpen bool
	// Badge is a short note on the cover's corner where the play control
	// would be: an upcoming album's release date. It is read after the title.
	Badge string
	// Item is what the tile is, for its menu, selection and drag. Without
	// one (music videos, web-only items) the tile has no menu and cannot
	// be selected.
	Item *Item
	// Explicit marks the title; a restricted account sees the tile dimmed
	// and cannot play it.
	Explicit bool
	// Motion is Go's square motion artwork URL, played on hover or focus.
	Motion string
}

// CoverOpts are what a cover collection (grid, row, shelf) needs from its
// feature.
type CoverOpts struct {
	// Label names the grid for assistive technology.
	Label string
	// Min is the smallest tile width (default 160); tiles grow to fill the row.
	Min float32
	// Video makes the tiles' covers 16:9 unless an item says otherwise.
	Video bool
	// Art decodes the covers at the exact tile size.
	Art *ArtLoader
	// Motion decides whether motion covers play; nil: none.
	Motion *MotionPolicy
	// Menus is the item menu of the list (right-click, Shift+F10, the more
	// button); nil: no menu.
	Menus *ActionMenus
	// NoPopup: the page draws the menu's popup itself (Menus.Popup, once),
	// as several lists share one ActionMenus.
	NoPopup bool
	// Container is the collection the tiles are listed in, for menu actions.
	Container *ActionContainer
	// Restricted: the account may not play explicit items: they are dimmed
	// and cannot be played.
	Restricted bool
	// Hold, when it reports true, keeps a press outside the list from
	// clearing the selection (a feature's own dialog); open menus and a
	// pending confirmation always hold it.
	Hold func() bool
}

// collectionsHold is the Selection.Hold of a list: its menu open, a question
// waiting for an answer, or the feature's own hold.
func collectionsHold(menus *ActionMenus, extra func() bool) func() bool {
	return func() bool {
		return (menus != nil && menus.IsOpen()) || ConfirmPending() || (extra != nil && extra())
	}
}

// CoverState is the state of one cover collection: its selection, its
// keyboard navigation and the geometry of its last frame. A feature keeps one
// per grid or row, as a field of its own state.
type CoverState struct {
	// Sel is the selection of the tiles (Ctrl+click, Shift+click, Shift+arrows, Ctrl+A).
	Sel Selection
	nav GridNav

	rows coverRows

	// Geometry of the last frame.
	width float32
	cols  int
	rowH  float32
	top   float32 // the grid's offset in its pane's content
	// drawTop is that offset as the last paint saw it: the position and the
	// scroll offset of one frame, never of two. drawn: there was a paint.
	drawTop float32
	drawn   bool
	// menus is the item menu over this list's own selection, made by
	// menusOver for lists that share one page's ActionMenus.
	menus *ActionMenus
	// reported is the tile range OnRange was last told.
	reported [2]int
	// openers are the play buttons' handles, by tile key.
	openers Openers
}

// menusOver is the item menu of a list that shares a page's ActionMenus with
// others: same actions, but bulk runs follow this list's own selection (the
// page's menu has one scope, fixed when it was made). nil for no menus.
func (st *CoverState) menusOver(base *ActionMenus) *ActionMenus {
	if base == nil {
		return nil
	}
	if st.menus == nil || st.menus.reg != base.reg {
		st.menus = NewActionMenus(base.reg, &st.Sel)
	}
	return st.menus
}

// Nav is the keyboard navigation, for a feature that wants to move the focus
// (Want) or know it (Focus).
func (st *CoverState) Nav() *GridNav { return &st.nav }

// Columns is the tile columns of the last frame.
func (st *CoverState) Columns() int { return st.cols }

// coverRows is Rows over a grid's items, without building a key slice.
type coverRows struct {
	n  int
	at func(i int) *CoverItem
}

func (r *coverRows) Len() int { return r.n }

func (r *coverRows) Key(i int) string {
	if i < 0 || i >= r.n {
		return ""
	}
	if it := r.at(i); it != nil && it.Item != nil {
		return it.Key
	}
	return ""
}

func (r *coverRows) item(i int) (Item, bool) {
	if it := r.at(i); it != nil && it.Item != nil {
		return *it.Item, true
	}
	return Item{}, false
}

// setRows points the selection and the navigation at this frame's items.
func (st *CoverState) setRows(count int, at func(i int) *CoverItem, cols int, menus *ActionMenus, hold func() bool) {
	st.rows = coverRows{n: count, at: at}
	st.Sel.Hold = collectionsHold(menus, hold)
	st.Sel.Rows(&st.rows, st.rows.item)
	st.nav.Count, st.nav.Columns = count, cols
}

// tile builds one cover tile at index i.
func (st *CoverState) tile(c *ui.Context, o *CoverOpts, it *CoverItem, i int, size float32) {
	tk := theme.Current()
	if it == nil {
		st.loadingTile(c, i, size, o.Video)
		return
	}
	video := it.Video || o.Video
	artH := coverArtHeight(size, video)
	withheld := o.Restricted && it.Explicit
	selectable := it.Item != nil
	selected := selectable && st.Sel.Selected(it.Key)

	opener := it.Title
	if it.Badge != "" {
		opener += ", " + it.Badge
	}
	if it.PlaysOnOpen {
		opener += " " + i18n.T("Play")
	}

	_, s := DragItem(c, DragOpts{
		Key:    it.Key,
		Scope:  st,
		Index:  i,
		Art:    o.Art,
		Button: true,
		Payload: func() *DragPayload {
			if !selectable || withheld {
				return nil
			}
			items, _, group := st.Sel.GroupWith(it.Key)
			label := it.Title
			if group {
				label = DescribeItems(items)
			} else {
				items = []Item{*it.Item}
			}
			return &DragPayload{Items: items, Label: label, Art: it.Art}
		},
		Outer: func(w ui.Element) {
			w.Width(size).Height(artH + tileText).Shrink(0).Role(ui.RoleCell)
			if selectable {
				w.Checked(selected)
			}
			if selected {
				plate := theme.C("ink").Alpha(0.12)
				w.Draw(func(p *ui.Painter, r ui.Rect) {
					p.Fill(ui.Rect{X: r.X - 8, Y: r.Y - 8, W: r.W + 16, H: r.H + 16}, plate, tk.Radius["lg"])
				})
			}
			if withheld {
				w.Opacity(0.5).Tooltip(i18n.T("Explicit content is restricted on this account"))
			}
		},
		After: func(w, s ui.Element) {
			// The play control and the more button are elements of their own,
			// beside the opener: a button or a link is a leaf for assistive
			// technology, so inside it they would not be on its tree.
			hot := w.Hovered() || w.FocusWithin()
			st.playControl(c, it, artH, hot, withheld)
			if it.Item != nil && o.Menus != nil {
				o.Menus.Trigger(c.Key("more"), ActionTarget{Item: *it.Item, Name: it.Title, Container: o.Container, Key: it.Key},
					TriggerOpts{Visible: hot}).Absolute().Right(0).Top(artH + 6)
			}
		},
		Source: func(s ui.Element) {
			s.Column().Label(opener).Radius(tk.Radius["sm"])
			if it.Subtitle != "" {
				// A button or link is a leaf on the accessibility tree, so the
				// subtitle drawn inside it is read as its description.
				s.Description(it.Subtitle)
			}
			switch {
			case it.PlaysOnOpen:
				s.Role(ui.RoleButton)
			case it.Open == nil:
				s.Role(ui.RoleGroup) // nothing to open: not a control that does nothing
			default:
				s.Role(ui.RoleLink)
			}
		},
	}, func(w, s ui.Element) {
		hot := w.Hovered() || s.FocusWithin()
		st.cover(c, o, it, size, artH, hot, !w.Hovered() && s.FocusWithin(), withheld)
		st.titleRow(c, o, it, w.Hovered(), hot, withheld)
		if it.Subtitle == "" {
			ui.Box(c).Height(18)
			return
		}
		sub := ui.Text(c, it.Subtitle).FontSize(tk.Type["secondary"].Size).FixedLineHeight(tk.Type["secondary"].LineHeight).
			TextColor(theme.C("ink-muted")).SingleLine().TextAlign(ui.Start)
		if it.Round {
			sub.TextAlign(ui.Center)
		}
	})

	st.nav.Claim(s, i)
	if s.Clicked() {
		mods := ModsOf(s.ClickModifiers())
		switch {
		case mods.Range || mods.Toggle:
			if selectable {
				st.Sel.Click(i, mods)
			}
		case it.Open != nil:
			it.Open()
		}
	}
	if !st.nav.Keys(s, i, &st.Sel) {
		st.Sel.Keys(s, i)
	}
	if selectable && o.Menus != nil {
		collectionsMenu(&st.Sel, o.Menus, s, ActionTarget{Item: *it.Item, Name: it.Title, Container: o.Container, Key: it.Key}, i)
	}
}

// cover builds the art box: the cover, the motion over it, and its corner
// control (play, play mark or release badge).
func (st *CoverState) cover(c *ui.Context, o *CoverOpts, it *CoverItem, size, artH float32, hot, focus, withheld bool) {
	tk := theme.Current()
	ui.Box(c.Key("cover")).Size(size, artH).Children(func() {
		aspect := float32(0)
		if artH != size {
			aspect = size / artH
		}
		Artwork(c, o.Art, it.Art, size, ArtOpts{Edge: true, Round: it.Round, Aspect: aspect})
		if it.Motion != "" && !withheld {
			TileMotion(c, o.Motion, it.Motion, size, TileMotionOpts{Round: it.Round, Hot: hot, Focus: focus}).Absolute().Left(0).Top(0)
		}
		switch {
		case it.Play != nil && !withheld:
			// The play control is built beside the tile (playControl).
		case it.PlaysOnOpen && !withheld:
			mark := ui.Row(c.Key("mark")).Absolute().Left(8).Bottom(8).Size(36, 36).Center().
				Radius(tk.Radius["full"]).Background(theme.C("play")).IgnoreInput(true)
			collectionsFade(mark, hot)
			mark.Children(func() {
				ui.Icon(c, theme.Icon("play")).FontSize(16).TextColor(theme.C("on-play"))
			})
		case it.Badge != "":
			ui.Row(c.Key("badge")).Absolute().Left(8).Bottom(8).PaddingX(6).PaddingY(2).Radius(tk.Radius["md"]).
				Background(theme.C("raised")).Border(1, theme.C("ink").Alpha(0.1)).IgnoreInput(true).Children(func() {
				ui.Text(c, it.Badge).FontSize(tk.Type["meta"].Size).FixedLineHeight(tk.Type["meta"].LineHeight).
					FontWeight(500).TextColor(theme.C("ink")).FontFeatures("tnum").SingleLine()
			})
		}
	})
}

// titleRow builds the title (underlined while the tile is hovered), the
// explicit mark and the item's more button.
func (st *CoverState) titleRow(c *ui.Context, o *CoverOpts, it *CoverItem, hovered, hot, withheld bool) {
	tk := theme.Current()
	label := tk.Type["label"]
	ui.Row(c.Key("title")).Height(20).Margin(8, 0, 0, 0).Gap(4).AlignItems(ui.Center).MinWidth(0).Children(func() {
		t := ui.Text(c, it.Title).FontSize(label.Size).FixedLineHeight(label.LineHeight).FontWeight(int(label.Weight)).
			TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
		if !it.Explicit {
			t.Grow(1)
		}
		if it.Round {
			t.TextAlign(ui.Center)
		} else {
			t.TextAlign(ui.Start)
		}
		if hovered {
			t.Underline()
		}
		if it.Explicit {
			ExplicitMark(c.Key("explicit"))
			ui.Box(c).Grow(1)
		}
		if it.Item != nil && o.Menus != nil {
			ui.Box(c.Key("more-room")).Size(24, 24).Shrink(0).IgnoreInput(true) // where the more button sits (tile)
		}
	})
}

// playControl is a tile's play button over the cover's bottom left, shown
// while the tile is hovered or has the focus within, and always on the
// accessibility tree and a stop of Tab. It sits beside the opener, not inside
// it.
func (st *CoverState) playControl(c *ui.Context, it *CoverItem, artH float32, hot, withheld bool) {
	if it.Play == nil || withheld {
		return
	}
	const size = 36
	play := PlayButton(c.Key("play"), i18n.T("Play {playLabel}", "playLabel", it.Play.Label), size).Absolute().Left(8).Top(artH - 8 - size)
	collectionsFade(play, hot || play.Focused())
	opener := st.openers.For(it.Key)
	play.Bind(opener)
	if play.Clicked() {
		if it.Play.RunFrom != nil {
			it.Play.RunFrom(opener)
		} else {
			it.Play.Run()
		}
	}
}

// loadingTile is a tile whose page is still loading: flat bars at the real
// sizes, for assistive technology nothing.
func (st *CoverState) loadingTile(c *ui.Context, i int, size float32, video bool) {
	tk := theme.Current()
	hover := theme.C("hover")
	ui.Column(c.Key("loading-" + strconv.Itoa(i))).Width(size).Height(coverArtHeight(size, video) + tileText).Shrink(0).
		Role(ui.RoleNone).Children(func() {
		ui.Box(c).Size(size, coverArtHeight(size, video)).Radius(tk.Radius["sm"]).Background(hover)
		ui.Box(c).Width(size*3/4).Height(14).Margin(8+3, 0, 0, 0).Radius(tk.Radius["sm"]).Background(hover)
		ui.Box(c).Width(size/2).Height(12).Margin(6+3, 0, 0, 0).Radius(tk.Radius["sm"]).Background(hover)
	})
}

// collectionsFade shows or hides e over the theme's feedback duration (100
// ms in v0.2.0), at once while motion is reduced.
func collectionsFade(e ui.Element, show bool) {
	to := float32(0)
	if show {
		to = 1
	}
	if theme.ReducedMotion() {
		e.Opacity(to)
		return
	}
	e.Opacity(e.Animate("kit.fade", to, theme.Current().Motion["press"].Duration))
}

// collectionsMenu opens an item's menu on a right-click, or on Shift+F10 or
// the Menu key while it has the focus, as ActionMenus.Row does, but first
// selects the item alone when it is outside the selection (a menu acts on the
// whole selection only when the item is in it).
func collectionsMenu(sel *Selection, menus *ActionMenus, s ui.Element, t ActionTarget, index int) {
	if s.RightClicked() {
		sel.MenuOn(index)
		x, y, _ := s.PointerPosition()
		wx, wy := s.LocalToWindow(x, y)
		menus.OpenAt(t, wx, wy, false)
	}
	if s.Shortcut(ui.Shift, ui.KeyF10) || s.Shortcut(0, ui.KeyContextMenu) {
		sel.MenuOn(index)
		b := s.Bounds()
		menus.OpenAt(t, b.X+16, b.Y+b.H, true)
	}
}

// ReleaseBadge is the badge on an upcoming album's cover ("Out 19 Nov", with
// the year when it is not this one's); "" for an album that is out or has no
// full date (v0.2.0's releaseBadge).
func ReleaseBadge(a services.Album, now time.Time) string { return itemsReleaseBadge(a, now) }
