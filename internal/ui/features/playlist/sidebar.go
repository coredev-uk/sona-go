package playlist

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The sidebar's playlists, under the library links (PlaylistSidebar.tsx):
// Pinned, Made for you (Apple's personal mixes), Apple Music (playlists
// added from Apple Music or another curator, which cannot be edited) and the
// user's own Playlists with their folders (a SidebarSection slot, stacked in
// the shell's scrolling middle). A pinned playlist shows under Pinned only.
// The sidebar is not a routed page, so its links are rows that push the path
// through env.Nav.

const (
	// springDelay is how long a drag rests on a closed folder before it
	// opens.
	springDelay = 600 * time.Millisecond
	// folderIndent is the indent of each folder level.
	folderIndent = 14
)

// sidebar is the sidebar's UI state: one menu per row that has one, the item
// menus, the folder a drag is resting on and the lists derived from the
// tree. Main thread only.
type sidebar struct {
	menus   map[string]*kit.Menu
	actions *kit.ActionMenus
	spring  struct {
		id    string
		since time.Time
	}
	derived derived
}

// derived is what the sidebar makes of the tree and the pins. Building it
// sorts every playlist by name, so it is kept until the tree or the pins
// change (a refetch brings new slices).
type derived struct {
	valid     bool
	ready     bool
	folders   *services.PlaylistFolder
	nFolders  int
	playlists *services.TreePlaylist
	nLists    int
	pins      *services.Pin
	nPins     int

	lists    sidebarLists
	editable map[string]bool
	anyEdit  bool
}

func firstOf[T any](s []T) *T {
	if len(s) == 0 {
		return nil
	}
	return &s[0]
}

// derive is the sections for tree, with what the pins hide (ready: the pins
// are known; until then nothing is listed, so a pinned playlist does not
// show in its section for a moment first), and the editable playlists by ID.
func (s *sidebar) derive(tree services.PlaylistTree, pins []services.Pin, ready bool) *derived {
	d := &s.derived
	if d.valid && d.ready == ready && d.folders == firstOf(tree.Folders) && d.nFolders == len(tree.Folders) &&
		d.playlists == firstOf(tree.Playlists) && d.nLists == len(tree.Playlists) && d.pins == firstOf(pins) && d.nPins == len(pins) {
		return d
	}
	*d = derived{
		valid: true, ready: ready,
		folders: firstOf(tree.Folders), nFolders: len(tree.Folders),
		playlists: firstOf(tree.Playlists), nLists: len(tree.Playlists),
		pins: firstOf(pins), nPins: len(pins),
		editable: map[string]bool{},
	}
	for _, it := range tree.Playlists {
		if it.Playlist.CanEdit {
			d.editable[it.Playlist.ID] = true
			d.anyEdit = true
		}
	}
	if ready {
		pinned := make(map[string]bool, len(pins))
		for _, pin := range pins {
			pinned[pin.ID] = true
		}
		d.lists = sidebarPlaylists(tree, pinned)
	}
	return d
}

// menu is the state of the menu keyed id.
func (s *sidebar) menu(id string) *kit.Menu {
	if s.menus == nil {
		s.menus = map[string]*kit.Menu{}
	}
	m := s.menus[id]
	if m == nil {
		m = &kit.Menu{}
		s.menus[id] = m
	}
	return m
}

// sidebarActions are the item actions' menus of the playlist rows (nil when the
// Env has no registry).
func (p *playlists) sidebarActions() *kit.ActionMenus {
	if p.side.actions == nil && p.env.Actions != nil {
		p.side.actions = kit.NewActionMenus(p.env.Actions, nil)
	}
	return p.side.actions
}

// sidebarSection builds the sections of the sidebar's playlists.
func (p *playlists) sidebarSection(c *ui.Context) {
	tree, pins := p.tree(), p.pins()
	treePending := !tree.HasData && tree.Err == nil
	pinsPending := !pins.HasData && pins.Err == nil
	var d *derived
	if tree.HasData {
		d = p.side.derive(tree.Data, pins.Data, !pinsPending)
	} else {
		d = &derived{}
	}
	p.pinsSection(c, pins.Data, d.editable)
	p.readOnlySection(c, "made-for-you", i18n.T("Made for you"), d.lists.MadeForYou)
	p.readOnlySection(c, "apple-music", i18n.T("Apple Music"), d.lists.Apple)
	p.ownSection(c, d.lists.Own, treePending || pinsPending, d.anyEdit)
	if am := p.sidebarActions(); am != nil {
		am.Popup(c)
	}
}

// sideText is a Text in one of the theme's type styles.
func sideText(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// sideHeading is a section's heading (12 muted, 24 above and 6 below, 20
// high) with an action at its end.
func sideHeading(c *ui.Context, title string, action func()) {
	ui.Row(c).FillWidth().Height(20).AlignItems(ui.Center).Padding(0, 4, 0, 10).Margin(24, 0, 6, 0).Children(func() {
		sideText(c, title, "meta").TextColor(theme.C("ink-faint")).SingleLine().Grow(1).MinWidth(0).
			Role(ui.RoleHeading).Level(2)
		if action != nil {
			action()
		}
	})
}

// rowFace styles a sidebar row as navLink does: 32 high, 10 across, radius
// md, the page's row on --active in ink at weight 500, a hovered one on
// --hover, and a row a drag is over inverted so where the drop lands is
// unmistakable. It returns the ink for the content.
func rowFace(row ui.Element, hot, active, over bool) ui.Color {
	row.Height(theme.Current().Metric["nav-row"]).Gap(10).Justify(ui.Start).AlignItems(ui.Center).
		Radius(theme.Current().Radius["md"]).Cursor(ui.CursorPointer)
	fg := theme.C("ink-muted")
	switch {
	case over:
		row.Background(theme.C("ink"))
		fg = theme.C("ground")
	case active:
		row.Background(theme.C("active"))
		fg = theme.C("ink")
	case hot:
		row.Background(theme.C("hover"))
		fg = theme.C("ink")
	}
	return fg
}

// rowContent is a row's picture (20, round for a person) and name.
func (p *playlists) rowContent(c *ui.Context, art string, round bool, name string, fg ui.Color, bold bool) {
	kit.Artwork(c, p.env.Art, art, 20, kit.ArtOpts{Round: round})
	t := sideText(c, name, "body").TextColor(fg).SingleLine().Shrink(1).MinWidth(0)
	if bold {
		t.FontWeight(500)
	}
}

// moreMenu is a row's "more" trigger, at its end: invisible until the row is
// hovered or has the focus within, and shown while its menu is open, and the
// menu of items (kit.Menu, 192 wide).
func (p *playlists) moreMenu(c *ui.Context, key, label string, hot bool, items func() []kit.MenuItem) {
	m := p.side.menu(key)
	ui.Box(c.Key("more:" + key)).Absolute().Top(4).Right(4).Children(func() {
		btn := kit.Button(c.Key("btn"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeXS, Icon: "ellipsis", A11yLabel: label})
		btn.Role(ui.RoleMenuButton).Expanded(m.IsOpen())
		if !hot && !m.IsOpen() && !btn.FocusWithin() {
			btn.Opacity(0)
		}
		if btn.Clicked() {
			if m.IsOpen() {
				m.Close()
			} else {
				m.OpenFrom(btn.FocusVisible())
			}
		}
		var rows []kit.MenuItem
		if m.IsOpen() {
			rows = items()
		}
		m.Show(c, kit.MenuOpts{ID: key, Label: label, Width: 192}, btn, rows)
	})
}

// Pinned.

// pinPath is where a pin opens: its library page (a song opens its album);
// "" when there is none or the page is not ported.
func (p *playlists) pinPath(pin services.Pin) string {
	switch pin.Kind {
	case "playlist":
		return "/playlist/" + pin.ID
	case "album":
		return p.routeOr("/album/" + pin.ID)
	case "artist":
		return p.routeOr("/artist/" + pin.ID)
	}
	if pin.AlbumID != "" {
		return p.routeOr("/album/" + pin.AlbumID)
	}
	return ""
}

func (p *playlists) routeOr(path string) string {
	if p.hasRoute(path) {
		return path
	}
	return ""
}

// pinsSection is Pinned: the pins, and while library items that can be
// pinned are dragged a dashed "Pin here" zone, even when nothing is pinned
// yet. The whole section takes the drop.
func (p *playlists) pinsSection(c *ui.Context, pins []services.Pin, editable map[string]bool) {
	offered := kit.DragActive(func(d *kit.DragPayload) bool { return len(pinnable(d, pins)) > 0 })
	if len(pins) == 0 && !offered {
		return
	}
	sec := ui.Column(c.Key("section:pinned")).FillWidth()
	over := p.dropOnPins(sec, pins)
	sec.Children(func() {
		sideHeading(c, i18n.T("Pinned"), nil)
		ui.Column(c.Key("pins")).FillWidth().Gap(2).Role(ui.RoleList).Label(i18n.T("Pinned")).Children(func() {
			for i, pin := range pins {
				// Moving up one place puts the pin after the one two above it.
				upAfter, canUp := "", i > 0
				if i >= 2 {
					upAfter = pins[i-2].ID
				}
				p.pinRow(c, pin, upAfter, canUp, editable[pin.ID] && pin.Kind == "playlist")
			}
			if offered {
				p.dropZone(c, "pin-here", "pin", i18n.T("Pin here"), over, nil)
			}
		})
	})
}

// dropZone is a dashed row shown only while a drag it takes is in progress;
// it inverts while the drag is over it (over, or what on, which makes the row
// a drop target, reports).
func (p *playlists) dropZone(c *ui.Context, key, icon, label string, over bool, on func(ui.Element) bool) {
	z := ui.Row(c.Key(key)).FillWidth().Height(theme.Current().Metric["nav-row"]).PaddingX(10).Gap(10).AlignItems(ui.Center).
		Radius(theme.Current().Radius["md"]).Role(ui.RoleListItem)
	fg := theme.C("ink-muted")
	if on != nil {
		over = on(z)
	}
	z.Border(1, theme.C("control-line")).BorderStyle(ui.BorderDashed)
	if over {
		z.Border(1, theme.C("ink")).Background(theme.C("ink"))
		fg = theme.C("ground")
	}
	z.Children(func() {
		ui.Icon(c, theme.Icon(icon)).FontSize(16).TextColor(fg)
		sideText(c, label, "body").TextColor(fg).SingleLine().Shrink(1).MinWidth(0)
	})
}

// pinRow is one pin: a link (it opens) or a button (it plays or shuffles)
// with a "More for <name>" menu. A pinned playlist the user can edit takes
// drops like its row would.
func (p *playlists) pinRow(c *ui.Context, pin services.Pin, upAfter string, canUp, editable bool) {
	path := p.pinPath(pin)
	opens := path != "" && (pin.Action == "drillIn" || pin.Kind == "artist")
	wrap := ui.Row(c.Key("pin:" + pin.ID)).FillWidth().Role(ui.RoleListItem)
	over := p.dropOnPlaylist(wrap, "pin:"+pin.ID, pin.ID, pin.Name, editable)
	hot := wrap.Hovered() || wrap.FocusWithin()
	wrap.Children(func() {
		var row ui.Element
		active := false
		if opens {
			row = ui.ButtonBase(c.Key("open")).Role(ui.RoleLink).Label(pin.Name)
			active = p.env.Nav != nil && p.env.Nav.Path() == path
		} else {
			// The words a screen reader adds: what choosing it does.
			suffix := i18n.T(", plays")
			if pin.Action == "shuffle" {
				suffix = i18n.T(", shuffles")
			}
			row = ui.ButtonBase(c.Key("open")).Label(pin.Name + suffix)
		}
		row.Grow(1).MinWidth(0).Padding(0, 32, 0, 10)
		fg := rowFace(row, hot, active, over)
		if active {
			row.Description(i18n.T("Current page"))
		}
		switch {
		case !row.Clicked():
		case opens:
			p.env.Nav.Push(path)
		default:
			p.playPin(pin.ID, pin.Name, pin.Action == "shuffle")
		}
		row.Children(func() { p.rowContent(c, pin.Art.Small, pin.Kind == "artist", pin.Name, fg, active) })

		p.moreMenu(c, "pin:"+pin.ID, i18n.T("More for {pinName}", "pinName", pin.Name), hot, func() []kit.MenuItem {
			return p.pinMenu(pin, path, opens, upAfter, canUp)
		})
	})
}

// pinMenu is a pin's menu: what choosing it does (a radio group; Open is
// off for a pin whose page is not ported, which plays instead), Open or Go
// to album for a pin that does not open on a click, Move up (not for the
// first) and Unpin.
func (p *playlists) pinMenu(pin services.Pin, path string, opens bool, upAfter string, canUp bool) []kit.MenuItem {
	choose := func(action string) func() {
		return func() {
			if pin.Action == action {
				return
			}
			p.run(i18n.T("Couldn't change the pin"),
				func(ctx context.Context) error { return p.api.SetPinAction(ctx, pin.ID, action) }, nil)
		}
	}
	items := []kit.MenuItem{
		{Key: "when", Kind: kit.MenuLabel, Label: i18n.T("When chosen")},
		{Key: "play", Kind: kit.MenuRadio, Label: i18n.T("Play"), Checked: pin.Action == "play" || pin.Action == "drillIn" && path == "", Run: choose("play")},
		{Key: "shuffle", Kind: kit.MenuRadio, Label: i18n.T("Shuffle"), Checked: pin.Action == "shuffle", Run: choose("shuffle")},
		{Key: "drillIn", Kind: kit.MenuRadio, Label: i18n.T("Open"), Checked: pin.Action == "drillIn" && path != "", Disabled: path == "", Run: choose("drillIn")},
		{Key: "sep", Kind: kit.MenuSeparator},
	}
	if path != "" && !opens {
		label := i18n.T("Open")
		if pin.Kind == "song" {
			label = i18n.T("Go to album")
		}
		items = append(items, kit.MenuItem{Key: "open", Label: label, Run: func() { p.env.Nav.Push(path) }})
	}
	if canUp {
		items = append(items, kit.MenuItem{Key: "up", Label: i18n.T("Move up"), Run: func() {
			p.run(i18n.T("Couldn't move the pin"),
				func(ctx context.Context) error { return p.api.MovePin(ctx, pin.ID, upAfter) }, nil)
		}})
	}
	return append(items, kit.MenuItem{Key: "unpin", Label: i18n.T("Unpin"), Run: func() { p.unpin(pin.ID, pin.Name) }})
}

// Playlist rows.

// playlistRow is a library playlist: a link to its page (indented by its
// folder depth) with its art, the item actions' "more" menu, and a drop
// target when the user can edit it.
func (p *playlists) playlistRow(c *ui.Context, item services.TreePlaylist, depth int) {
	pl := item.Playlist
	path := "/playlist/" + pl.ID
	wrap := ui.Row(c.Key("playlist:" + pl.ID)).FillWidth().Role(ui.RoleListItem)
	over := p.dropOnPlaylist(wrap, "playlist:"+pl.ID, pl.ID, pl.Name, pl.CanEdit)
	hot := wrap.Hovered() || wrap.FocusWithin()
	wrap.Children(func() {
		active := p.env.Nav != nil && p.env.Nav.Path() == path
		row := ui.ButtonBase(c.Key("open")).Role(ui.RoleLink).Label(pl.Name).Grow(1).MinWidth(0).
			Padding(0, 32, 0, 10+float32(depth)*folderIndent)
		fg := rowFace(row, hot, active, over)
		if active {
			row.Description(i18n.T("Current page"))
		}
		if row.Clicked() {
			p.env.Nav.Push(path)
		}
		row.Children(func() { p.rowContent(c, pl.Art.Small, false, pl.Name, fg, active) })

		if am := p.sidebarActions(); am != nil {
			ui.Box(c.Key("more")).Absolute().Top(4).Right(4).Children(func() {
				am.Trigger(c, kit.ActionTarget{Item: kit.PlaylistItem(pl), Name: pl.Name, Key: pl.ID}, kit.TriggerOpts{Visible: hot})
			})
		}
	})
}

// readOnlySection is a flat list of library playlists the user cannot edit:
// no folders, no drops, no "New playlist". Its heading is a button that
// folds the list shut (kept across restarts; every section starts open);
// the folded list is not built. A section with no playlists is omitted.
func (p *playlists) readOnlySection(c *ui.Context, id, title string, items []services.TreePlaylist) {
	if len(items) == 0 {
		return
	}
	folded := p.prefs.Collapsed(id)
	ui.Column(c.Key("section:" + id)).FillWidth().Children(func() {
		// The heading and the button that folds the list are siblings: MyGo
		// reads a heading as one piece, so a button inside it would be out
		// of reach of a screen reader. The title is drawn over the button
		// (the pointer goes through it) and is the section's heading.
		ui.Row(c.Key("head")).FillWidth().Height(20).Margin(24, 0, 6, 0).Children(func() {
			fold := ui.ButtonBase(c.Key("fold")).Role(ui.RoleDisclosure).Label(title).Expanded(!folded).FillWidth().Height(20).
				Padding(0, 10).AlignItems(ui.Center).Radius(theme.Current().Radius["md"]).Cursor(ui.CursorPointer)
			fg := theme.C("ink-faint")
			if fold.Hovered() {
				fg = theme.C("ink")
			}
			if fold.Clicked() {
				folded = !folded
				p.prefs.SetCollapsed(id, folded)
			}
			ui.Row(c.Key("title")).Absolute().Left(10).Right(34).Top(0).Bottom(0).AlignItems(ui.Center).IgnoreInput(true).Children(func() {
				sideText(c, title, "meta").TextColor(fg).SingleLine().MinWidth(0).Role(ui.RoleHeading).Level(2)
			})
			fold.Children(func() {
				ui.Box(c).Grow(1)
				chevron := "chevron-down"
				if folded {
					chevron = "chevron-right"
				}
				ui.Icon(c, theme.Icon(chevron)).FontSize(14).TextColor(fg)
			})
		})
		if folded {
			return
		}
		ui.Column(c.Key("list")).FillWidth().Gap(2).Role(ui.RoleList).Label(title).Children(func() {
			for _, it := range items {
				p.playlistRow(c, it, 0)
			}
		})
	})
}

// Playlists.

// ownSection is Playlists: the user's own playlists in their folders under
// a heading with the "New playlist or folder" menu, and, while addable items
// are dragged, a dashed "New playlist" drop row. pending: the tree or the
// pins are not known yet (no empty message then); anyEditable: the user has
// playlists they can edit (they are all pinned when none is listed).
func (p *playlists) ownSection(c *ui.Context, nodes []*treeNode, pending, anyEditable bool) {
	ui.Column(c.Key("section:playlists")).FillWidth().Children(func() {
		sideHeading(c, i18n.T("Playlists"), p.newMenu(c))
		if kit.DragActive(addable) {
			p.dropZone(c, "new-playlist", "list-plus", i18n.T("New playlist"), false, p.dropOnNewPlaylist)
		}
		switch {
		case len(nodes) > 0:
			p.treeLevel(c, nodes, 0)
		case !pending:
			msg := i18n.T("Your playlists appear here.")
			if anyEditable {
				msg = i18n.T("All your playlists are pinned above.")
			}
			sideText(c, msg, "meta").TextColor(theme.C("ink-faint")).Padding(0, 10)
		}
	})
}

// newMenu is the heading's "New playlist or folder" (+) button and its menu.
func (p *playlists) newMenu(c *ui.Context) func() {
	return func() {
		m := p.side.menu("new")
		label := i18n.T("New playlist or folder")
		btn := kit.Button(c.Key("new"), "", kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeXS, Icon: "plus", A11yLabel: label})
		btn.Role(ui.RoleMenuButton).Expanded(m.IsOpen())
		if btn.Clicked() {
			if m.IsOpen() {
				m.Close()
			} else {
				m.OpenFrom(btn.FocusVisible())
			}
		}
		var rows []kit.MenuItem
		if m.IsOpen() {
			rows = []kit.MenuItem{
				{Key: "playlist", Label: i18n.T("New playlist"), Run: func() { p.dialogs.show(dialogState{kind: dialogCreate}) }},
				{Key: "folder", Label: i18n.T("New folder…"), Run: func() { p.dialogs.show(dialogState{kind: dialogFolder}) }},
			}
		}
		m.Show(c, kit.MenuOpts{ID: "new", Label: label, Width: 176}, btn, rows)
	}
}

// treeLevel builds a level of the folder tree.
func (p *playlists) treeLevel(c *ui.Context, nodes []*treeNode, depth int) {
	list := ui.Column(c).FillWidth().Gap(2).Role(ui.RoleList)
	if depth == 0 {
		list.Label(i18n.T("Playlists"))
	}
	list.Children(func() {
		for _, n := range nodes {
			if n.Folder != nil {
				p.folderRow(c, n, depth)
			} else {
				p.playlistRow(c, *n.Item, depth)
			}
		}
	})
}

// springOpen reports whether a drag has rested on the closed folder id for
// springDelay, so its playlists can take the drop; leaving cancels.
func (p *playlists) springOpen(c *ui.Context, row ui.Element, id string, open bool) bool {
	s := &p.side.spring
	if !open && kit.DragActive(nil) {
		if _, over := ui.DragOver[*kit.DragPayload](row); over {
			now := c.Now()
			if s.id != id {
				s.id, s.since = id, now
			}
			if wait := springDelay - now.Sub(s.since); wait > 0 {
				c.After(wait)
				return false
			}
			s.id = ""
			return true
		}
	}
	if s.id == id {
		s.id = ""
	}
	return false
}

// folderRow is a folder: a button that opens and closes it (open: the open
// folder icon; kept across restarts), its "More for the folder" menu, and
// below it, when open, its contents or why there are none. Folders are not
// drop targets themselves; a drag resting on a closed one opens it.
func (p *playlists) folderRow(c *ui.Context, n *treeNode, depth int) {
	f := *n.Folder
	open := p.prefs.FolderOpen(f.ID)
	pad := float32(depth) * folderIndent
	ui.Column(c.Key("folder:" + f.ID)).FillWidth().Gap(2).Role(ui.RoleListItem).Children(func() {
		wrap := ui.Row(c.Key("row")).FillWidth()
		hot := wrap.Hovered() || wrap.FocusWithin()
		wrap.Children(func() {
			row := ui.ButtonBase(c.Key("toggle")).Role(ui.RoleDisclosure).Label(f.Name).Expanded(open).Grow(1).MinWidth(0).Padding(0, 32, 0, 10+pad)
			fg := rowFace(row, hot, false, false)
			if row.Clicked() || p.springOpen(c, row, f.ID, open) {
				open = !open
				p.prefs.SetFolderOpen(f.ID, open)
			}
			row.Children(func() {
				icon := "folder-closed"
				if open {
					icon = "folder-open"
				}
				ui.Row(c).Size(20, 20).Shrink(0).Center().Children(func() {
					ui.Icon(c, theme.Icon(icon)).FontSize(16).TextColor(fg)
				})
				sideText(c, f.Name, "body").TextColor(fg).SingleLine().Shrink(1).MinWidth(0)
				chevron := "chevron-right"
				if open {
					chevron = "chevron-down"
				}
				ui.Box(c).Grow(1)
				ui.Icon(c, theme.Icon(chevron)).FontSize(14).TextColor(theme.C("ink-faint"))
			})
			p.moreMenu(c, "folder:"+f.ID, i18n.T("More for the folder {folderName}", "folderName", f.Name), hot, func() []kit.MenuItem {
				return p.folderMenu(f, len(n.Children)+n.Hidden > 0)
			})
		})
		if !open {
			return
		}
		if len(n.Children) > 0 {
			p.treeLevel(c, n.Children, depth+1)
			return
		}
		msg := i18n.T("Empty folder")
		if n.Hidden > 0 {
			msg = i18n.T("{hidden, plural, one {Its playlist is listed in another section} other {Its # playlists are listed in other sections}}", "hidden", n.Hidden)
		}
		sideText(c, msg, "meta").TextColor(theme.C("ink-faint")).Padding(6, 10, 6, 36+pad)
	})
}

// folderMenu is a folder's menu: New playlist here, Rename…, and Delete…,
// which is disabled while the folder holds playlists (its own or ones listed
// in another section).
func (p *playlists) folderMenu(f services.PlaylistFolder, holds bool) []kit.MenuItem {
	del := kit.MenuItem{Key: "delete", Label: i18n.T("Delete…"), Run: func() { p.deleteFolder(f.ID, f.Name) }}
	if holds {
		del.Label, del.Disabled = i18n.T("Delete (move its playlists out first)"), true
	}
	return []kit.MenuItem{
		{Key: "new", Label: i18n.T("New playlist here"), Run: func() {
			p.dialogs.show(dialogState{kind: dialogCreate, folderID: f.ID})
		}},
		{Key: "rename", Label: i18n.T("Rename…"), Run: func() {
			p.dialogs.show(dialogState{kind: dialogFolder, folder: &f})
		}},
		del,
	}
}
