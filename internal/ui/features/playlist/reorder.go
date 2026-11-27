package playlist

import (
	"context"
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Moving the user's own playlists in the sidebar: drag a row between two
// rows to place it there (it joins their folder), onto a folder to move it
// in, or onto the list's empty space to move it to the level that lists it;
// the row's menu (Move up, Move down, Move to folder…) and Alt+↑ / Alt+↓ do
// the same without a pointer. Moving into a folder is Apple's call
// (PUT …/playlists/{id}/parent). The order among siblings is Sona's own,
// kept in SQLite: Apple's web API has no call that sets it
// (docs/apple-music-reference.md §5.8).

// sidebarList is the DragOrigin.List of a drag that starts on a sidebar
// playlist row: only these drags move playlists; any other drag of a playlist
// (a tile) adds its songs.
const sidebarList = "sidebar:playlists"

// dropZone is where on a row a drag of a sidebar playlist is: above the
// row's middle band the playlist lands before the row, below it after, and
// in the middle band (when the row takes songs) the drop adds the
// playlist's songs to the row's playlist instead of moving it.
type dropZone int

const (
	zoneOnto dropZone = iota
	zoneBefore
	zoneAfter
)

// bandEdge is the share of a row's height, at its top and at its bottom,
// that places a dragged playlist before or after the row when the row also
// takes songs.
const bandEdge = 0.28

// zoneAt is the zone of a row h high for a pointer y down it. A row that
// takes no songs (canAdd false) is only a place: its upper half is before,
// its lower half after.
func zoneAt(y, h float32, canAdd bool) dropZone {
	if h <= 0 {
		return zoneOnto
	}
	switch {
	case canAdd && y < h*bandEdge, !canAdd && y < h/2:
		return zoneBefore
	case canAdd && y > h*(1-bandEdge), !canAdd:
		return zoneAfter
	}
	return zoneOnto
}

// dropHint is the zone the pointer of a drag is in over the row target.
type dropHint struct {
	target string
	zone   dropZone
}

// draggedPlaylist is the library playlist a drag that started on a sidebar
// row carries.
func draggedPlaylist(d *kit.DragPayload) (services.Playlist, bool) {
	if d.Origin == nil || d.Origin.List != sidebarList || len(d.Items) != 1 || d.Items[0].Kind != kit.KindPlaylist {
		return services.Playlist{}, false
	}
	return d.Items[0].Playlist(), true
}

// sidebarPayload is what a sidebar playlist row drags: the library playlist,
// as a tile's drag carries it, marked as the sidebar's own.
func (p *playlists) sidebarPayload(pl services.Playlist, index int) *kit.DragPayload {
	return &kit.DragPayload{
		Items:  []kit.Item{kit.PlaylistItem(pl)},
		Origin: &kit.DragOrigin{List: sidebarList, Index: index},
		Label:  pl.Name,
		Art:    pl.Art.Small,
	}
}

// itemsOf are the playlists among nodes, in order.
func itemsOf(nodes []*treeNode) []*services.TreePlaylist {
	var items []*services.TreePlaylist
	for _, n := range nodes {
		if n.Item != nil {
			items = append(items, n.Item)
		}
	}
	return items
}

// ownLevel finds the level of the sidebar's own tree that lists playlist id:
// the folder holding it ("" at the top), the playlists listed there in order
// and the playlist's place among them. ok is false when it is not listed
// there (pinned, Apple's, or the tree has not loaded).
func (s *sidebar) ownLevel(id string) (folderID string, list []*services.TreePlaylist, at int, ok bool) {
	var find func(nodes []*treeNode, folder string) bool
	find = func(nodes []*treeNode, folder string) bool {
		items := itemsOf(nodes)
		if i := slices.IndexFunc(items, func(it *services.TreePlaylist) bool { return it.Playlist.ID == id }); i >= 0 {
			folderID, list, at = folder, items, i
			return true
		}
		for _, n := range nodes {
			if n.Folder != nil && find(n.Children, n.Folder.ID) {
				return true
			}
		}
		return false
	}
	ok = find(s.derived.lists.Own, "")
	return folderID, list, at, ok
}

// folderNode is folder id of the sidebar's own tree (nil when it is not
// listed).
func (s *sidebar) folderNode(id string) *treeNode {
	var find func(nodes []*treeNode) *treeNode
	find = func(nodes []*treeNode) *treeNode {
		for _, n := range nodes {
			if n.Folder == nil {
				continue
			}
			if n.Folder.ID == id {
				return n
			}
			if got := find(n.Children); got != nil {
				return got
			}
		}
		return nil
	}
	return find(s.derived.lists.Own)
}

// levelItems are the playlists the sidebar lists in folder folderID ("" for
// the top level), in order.
func (s *sidebar) levelItems(folderID string) []*services.TreePlaylist {
	if folderID == "" {
		return itemsOf(s.derived.lists.Own)
	}
	if f := s.folderNode(folderID); f != nil {
		return itemsOf(f.Children)
	}
	return nil
}

// folderName is the name of folder id ("" for the top level, or a folder
// that is not listed).
func (s *sidebar) folderName(id string) string {
	if f := s.folderNode(id); f != nil {
		return f.Folder.Name
	}
	return ""
}

// dropOnTreeRow makes e (the row of the user's own playlist item, in the
// sidebar's tree; next is the playlist listed after it at its level, "" when
// it is the last) a target. A drag that started on another sidebar row places
// that playlist before or after this one, by where the pointer is on the row,
// or, in the middle band of a row that takes songs, adds its songs, as a
// drop of a playlist from anywhere else does. It reports whether a drag it
// takes is over it and the zone the pointer is in.
func (p *playlists) dropOnTreeRow(c *ui.Context, e ui.Element, item services.TreePlaylist, next string) (over bool, zone dropZone) {
	pl := item.Playlist
	id := "playlist:" + pl.ID
	h := e.Bounds().H
	if h == 0 {
		h = theme.Current().Metric["nav-row"]
	}
	canAdd := func(d *kit.DragPayload) bool {
		return pl.CanEdit && addable(d) && !slices.ContainsFunc(d.Items, func(it kit.Item) bool { return isPlaylist(it, pl.ID) })
	}
	over = kit.DropOn(e, id, kit.DropTarget{
		Accepts: func(d *kit.DragPayload) bool {
			if dragged, ok := draggedPlaylist(d); ok {
				return dragged.ID != pl.ID
			}
			return canAdd(d)
		},
		Describe: func(d *kit.DragPayload) string {
			if _, ok := draggedPlaylist(d); ok && p.side.hint.target == id {
				switch p.side.hint.zone {
				case zoneBefore:
					return i18n.T("Move before “{name}”", "name", pl.Name)
				case zoneAfter:
					return i18n.T("Move after “{name}”", "name", pl.Name)
				}
			}
			return i18n.T("Add to “{name}”", "name", pl.Name)
		},
		Hover: func(d *kit.DragPayload, at *kit.DragPoint) {
			hint := dropHint{}
			if at != nil {
				hint = dropHint{id, zoneOnto}
				if _, ok := draggedPlaylist(d); ok {
					hint.zone = zoneAt(at.LocalY, h, canAdd(d))
				}
			}
			if at == nil && p.side.hint.target != id {
				return
			}
			if hint != p.side.hint {
				p.side.hint = hint
				c.Invalidate()
			}
		},
		Drop: func(d *kit.DragPayload, at kit.DragPoint) {
			dragged, ok := draggedPlaylist(d)
			if !ok {
				p.addToPlaylistNow(pl.ID, pl.Name, playlistItems(d.Items), kit.DescribeItems(d.Items))
				return
			}
			switch zoneAt(at.LocalY, h, canAdd(d)) {
			case zoneBefore:
				p.place(dragged, item.FolderID, pl.ID)
			case zoneAfter:
				p.place(dragged, item.FolderID, next)
			default:
				if canAdd(d) {
					p.addToPlaylistNow(pl.ID, pl.Name, playlistItems(d.Items), kit.DescribeItems(d.Items))
				}
			}
		},
	})
	if over && p.side.hint.target == id {
		zone = p.side.hint.zone
	}
	return over, zone
}

// dropOnFolder makes e (the row of folder f) a target for a playlist dragged
// from the sidebar: dropping it moves it into the folder.
func (p *playlists) dropOnFolder(e ui.Element, f services.PlaylistFolder) bool {
	return kit.DropOn(e, "folder:"+f.ID, kit.DropTarget{
		Accepts: func(d *kit.DragPayload) bool { _, ok := draggedPlaylist(d); return ok },
		Describe: func(d *kit.DragPayload) string {
			if dragged, ok := draggedPlaylist(d); ok && p.holder(dragged.ID) == f.ID {
				return i18n.T("Already in “{folder}”", "folder", f.Name)
			}
			return i18n.T("Move to “{folder}”", "folder", f.Name)
		},
		Drop: func(d *kit.DragPayload, _ kit.DragPoint) {
			if dragged, ok := draggedPlaylist(d); ok && p.holder(dragged.ID) != f.ID {
				p.moveTo(dragged.ID, dragged.Name, f.ID, f.Name)
			}
		},
	})
}

// dropOnLevel makes e (the space of a level of the tree: the Playlists
// section for the top level, a folder's contents) a target for a playlist
// dragged from the sidebar that no row under the pointer takes: dropping it
// moves it to the end of that level.
func (p *playlists) dropOnLevel(e ui.Element, targetID, folderID string) {
	kit.DropOn(e, targetID, kit.DropTarget{
		Accepts: func(d *kit.DragPayload) bool { _, ok := draggedPlaylist(d); return ok },
		Describe: func(d *kit.DragPayload) string {
			dragged, ok := draggedPlaylist(d)
			switch {
			case ok && p.holder(dragged.ID) == folderID:
				return i18n.T("Stays where it is")
			case folderID == "":
				return i18n.T("Move to the top level")
			}
			return i18n.T("Move to “{folder}”", "folder", p.side.folderName(folderID))
		},
		Drop: func(d *kit.DragPayload, _ kit.DragPoint) {
			if dragged, ok := draggedPlaylist(d); ok && p.holder(dragged.ID) != folderID {
				p.moveTo(dragged.ID, dragged.Name, folderID, p.side.folderName(folderID))
			}
		},
	})
}

// holder is the folder the sidebar lists playlist id in ("" at the top, and
// when it is not listed).
func (p *playlists) holder(id string) string {
	folder, _, _, _ := p.side.ownLevel(id)
	return folder
}

// moveTo moves library playlist id (called name) into folder folderID
// (called folder; "" is the top level), with a toast either way.
func (p *playlists) moveTo(id, name, folderID, folder string) {
	p.run(i18n.T("Couldn't move “{name}”", "name", name),
		func(ctx context.Context) error { return p.api.MoveToFolder(ctx, id, folderID) },
		func(ok bool) {
			switch {
			case !ok:
			case folderID == "":
				kit.ShowToast(kit.ToastSpec{Title: i18n.T("Moved “{name}” to the top level", "name", name)})
			default:
				kit.ShowToast(kit.ToastSpec{Title: i18n.T("Moved “{name}” to “{folder}”", "name", name, "folder", folder)})
			}
		})
}

// place puts the playlist pl just before playlist before of folder folderID
// ("" for last there): a move into the folder first when it is listed in
// another, then the order. The result is announced.
func (p *playlists) place(pl services.Playlist, folderID, before string) {
	from, _, was, ok := p.side.ownLevel(pl.ID)
	if !ok {
		return
	}
	// Where it lands among the playlists listed there once it is out of its
	// place: before the playlist before, else last.
	var others []string
	for _, it := range p.side.levelItems(folderID) {
		if it.Playlist.ID != pl.ID {
			others = append(others, it.Playlist.ID)
		}
	}
	at := len(others)
	if i := slices.Index(others, before); before != "" && i >= 0 {
		at = i
	}
	if before == pl.ID || from == folderID && at == was {
		return // it stays where it is
	}
	count := len(others) + 1
	var said string
	switch {
	case from == folderID:
		said = i18n.T("Moved “{name}” to position {position} of {count}", "name", pl.Name, "position", at+1, "count", count)
	case folderID == "":
		said = i18n.T("Moved “{name}” to the top level, position {position} of {count}", "name", pl.Name, "position", at+1, "count", count)
	default:
		said = i18n.T("Moved “{name}” to “{folder}”, position {position} of {count}", "name", pl.Name, "folder", p.side.folderName(folderID), "position", at+1, "count", count)
	}
	p.run(i18n.T("Couldn't move “{name}”", "name", pl.Name),
		func(ctx context.Context) error {
			if from != folderID {
				if err := p.api.MoveToFolder(ctx, pl.ID, folderID); err != nil {
					return err
				}
			}
			return p.api.ReorderPlaylist(ctx, pl.ID, before)
		},
		func(ok bool) {
			if ok {
				p.side.said = said
			}
		})
}

// nudge moves playlist id one place up (dir -1) or down (+1) among the
// playlists listed at its level; it reports whether there was a place to go.
func (p *playlists) nudge(id string, dir int) bool {
	folder, list, at, ok := p.side.ownLevel(id)
	to := at + dir
	if !ok || to < 0 || to >= len(list) {
		return false
	}
	before := ""
	switch {
	case dir < 0:
		before = list[to].Playlist.ID
	case to+1 < len(list):
		before = list[to+1].Playlist.ID
	}
	p.place(list[at].Playlist, folder, before)
	return true
}

// canNudge reports whether playlist id has a place to go in direction dir.
func (p *playlists) canNudge(id string, dir int) bool {
	_, list, at, ok := p.side.ownLevel(id)
	return ok && at+dir >= 0 && at+dir < len(list)
}
