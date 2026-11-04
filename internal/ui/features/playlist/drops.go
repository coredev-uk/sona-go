package playlist

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The sidebar's drop targets (drops.ts, over kit.DropOn): a playlist row
// takes songs, albums and playlists dragged onto it; the Pinned section pins
// library items; the "New playlist" row starts a playlist holding them. A
// target takes any payload whose Items hold what it wants: a queue row's
// payload carries its song in Items beside an Origin, which a target here
// ignores.

// addable reports whether the payload holds anything a playlist can take.
func addable(d *kit.DragPayload) bool { return len(playlistItems(d.Items)) > 0 }

// isPlaylist reports whether it is the library playlist id (by either of its
// IDs): a playlist cannot be added to itself.
func isPlaylist(it kit.Item, id string) bool {
	if it.Kind != kit.KindPlaylist {
		return false
	}
	pl := it.Playlist()
	return pl.ID == id || pl.LibraryID == id
}

// dropOnPlaylist makes e (a playlist's row, `targetID` its id: "playlist:<id>"
// or "pin:<id>") a target for adding to the library playlist id; it reports
// whether a drag that would be taken is over it. A playlist the user cannot
// edit declines everything.
func (p *playlists) dropOnPlaylist(e ui.Element, targetID, id, name string, canEdit bool) bool {
	return kit.DropOn(e, targetID, kit.DropTarget{
		Accepts: func(d *kit.DragPayload) bool {
			return canEdit && addable(d) && !slices.ContainsFunc(d.Items, func(it kit.Item) bool { return isPlaylist(it, id) })
		},
		Describe: func(*kit.DragPayload) string { return i18n.T("Add to “{name}”", "name", name) },
		Drop: func(d *kit.DragPayload, _ kit.DragPoint) {
			p.addToPlaylistNow(id, name, playlistItems(d.Items), kit.DescribeItems(d.Items))
		},
	})
}

// pinChoice is a library item to pin.
type pinChoice struct{ id, name string }

// pinnable are the library items of a payload that can be pinned and are not
// yet: none unless every item is a library item (never a station or a music
// video), and none when they would make more than maxPins.
func pinnable(d *kit.DragPayload, pins []services.Pin) []pinChoice {
	pinned := make(map[string]bool, len(pins))
	for _, pin := range pins {
		pinned[pin.ID] = true
	}
	var out []pinChoice
	for _, it := range d.Items {
		if it.Kind == kit.KindStation || it.Kind == kit.KindMusicVideo {
			return nil
		}
		id := libraryIDOf(it)
		if id == "" {
			return nil
		}
		if !pinned[id] {
			out = append(out, pinChoice{id, it.Name()})
		}
	}
	if len(out) == 0 || len(out)+len(pins) > maxPins {
		return nil
	}
	return out
}

// dropOnPins makes the Pinned section a target: a drop pins the dragged
// library items, in order. It reports whether a drag it takes is over it.
func (p *playlists) dropOnPins(e ui.Element, pins []services.Pin) bool {
	return kit.DropOn(e, "pins", kit.DropTarget{
		Accepts:  func(d *kit.DragPayload) bool { return len(pinnable(d, pins)) > 0 },
		Describe: func(*kit.DragPayload) string { return i18n.T("Pin to sidebar") },
		Drop:     func(d *kit.DragPayload, _ kit.DragPoint) { p.pinInOrder(pinnable(d, pins)) },
	})
}

// pinInOrder pins one item after the other (each with its own toast, as
// v0.2.0's loop did: one that fails does not stop the rest).
func (p *playlists) pinInOrder(list []pinChoice) {
	if len(list) == 0 {
		return
	}
	p.pin(list[0].id, list[0].name, func(bool) { p.pinInOrder(list[1:]) })
}

// dropOnNewPlaylist makes the "New playlist" row a target: a drop opens the
// new-playlist dialog holding the items. It reports whether a drag it takes
// is over it.
func (p *playlists) dropOnNewPlaylist(e ui.Element) bool {
	return kit.DropOn(e, "playlist:new", kit.DropTarget{
		Accepts:  addable,
		Describe: func(*kit.DragPayload) string { return i18n.T("New playlist") },
		Drop: func(d *kit.DragPayload, _ kit.DragPoint) {
			p.dialogs.show(dialogState{
				kind:  dialogCreate,
				items: playlistItems(d.Items),
				label: kit.DescribeItems(d.Items),
			})
		},
	})
}
