package playlist

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// pinnedData is the pinned library IDs, read when a menu opens.
func (p *playlists) pinnedData() kit.MenuData {
	return kit.MenuData{
		ID: "playlists.pinned",
		Load: func(ctx context.Context, _ []kit.Item) any {
			set := map[string]bool{}
			pins, err := p.api.Pins(ctx)
			if err != nil {
				return set
			}
			for _, pin := range pins {
				set[pin.ID] = true
			}
			return set
		},
	}
}

// treeData is the library playlists by ID, with their folder and
// permissions, read when a menu opens.
func (p *playlists) treeData() kit.MenuData {
	return kit.MenuData{
		ID: "playlists.tree",
		Load: func(ctx context.Context, _ []kit.Item) any {
			byID := map[string]services.TreePlaylist{}
			tree, err := p.api.Tree(ctx)
			if err != nil {
				return byID
			}
			for _, tp := range tree.Playlists {
				byID[tp.Playlist.ID] = tp
			}
			return byID
		},
	}
}

func (p *playlists) addTo(kind kit.ItemKind) kit.ActionDef {
	return kit.ActionDef{
		ID:     "playlists.add-to." + string(kind),
		Target: kind,
		Label:  func(*kit.ActionContext) string { return i18n.T("Add to playlist") },
		Icon:   "list-plus",
		Group:  kit.GroupItem,
		Order:  40,
		Run: func(a *kit.ActionContext) {
			it := a.Item()
			p.addToPlaylist(playlistItems([]kit.Item{it}), kit.DescribeItems([]kit.Item{it}))
		},
	}
}

// pinLabels are the per-kind pin and unpin labels.
func pinLabel(kind kit.ItemKind, pinned bool) string {
	switch kind {
	case kit.KindSong:
		if pinned {
			return i18n.T("Unpin song")
		}
		return i18n.T("Pin song")
	case kit.KindAlbum:
		if pinned {
			return i18n.T("Unpin album")
		}
		return i18n.T("Pin album")
	case kit.KindArtist:
		if pinned {
			return i18n.T("Unpin artist")
		}
		return i18n.T("Pin artist")
	}
	if pinned {
		return i18n.T("Unpin playlist")
	}
	return i18n.T("Pin playlist")
}

func (p *playlists) pinAction(kind kit.ItemKind) kit.ActionDef {
	data := p.pinnedData()
	isPinned := func(a *kit.ActionContext) bool {
		set, _ := kit.DataOf[map[string]bool](a, data)
		return set[libraryIDOf(a.Item())]
	}
	return kit.ActionDef{
		ID:        "playlists.pin." + string(kind),
		Target:    kind,
		Label:     func(a *kit.ActionContext) string { return pinLabel(kind, isPinned(a)) },
		Icon:      "pin",
		Group:     kit.GroupItem,
		Order:     60,
		Available: func(a *kit.ActionContext) bool { return libraryIDOf(a.Item()) != "" },
		Data:      []kit.MenuData{data},
		Run: func(a *kit.ActionContext) {
			it := a.Item()
			id := libraryIDOf(it)
			if isPinned(a) {
				p.unpin(id, it.Name())
			} else {
				p.pin(id, it.Name(), nil)
			}
		},
	}
}

func isLibraryPlaylist(pl services.Playlist) bool { return pl.Source == services.SourceLibrary }

func inEditablePlaylist(a *kit.ActionContext) bool {
	return a.Container != nil && a.Container.Kind == "playlist" && a.Container.Editable
}

// actions are the item actions: add songs, albums and playlists to a
// playlist; pin library items to the sidebar; edit, change the cover of,
// move and delete library playlists. A selection can be added to a playlist, or removed from the
// playlist it is listed in, at once.
func (p *playlists) actions() []kit.ActionDef {
	tree := p.treeData()
	return []kit.ActionDef{
		p.addTo(kit.KindSong),
		{
			ID:        "playlists.remove-from",
			Target:    kit.KindSong,
			Label:     func(*kit.ActionContext) string { return i18n.T("Remove from playlist") },
			Icon:      "list-minus",
			Group:     kit.GroupItem,
			Order:     45,
			Available: inEditablePlaylist,
			Run: func(a *kit.ActionContext) {
				p.removeTracks(a.Container.ID, []int{a.Container.Index}, []services.Song{a.Item().Song()})
			},
		},
		p.addTo(kit.KindAlbum),
		p.addTo(kit.KindPlaylist),
		{
			ID:     "playlists.add-to.many",
			Target: kit.KindSelection,
			Kinds:  []kit.ItemKind{kit.KindSong, kit.KindAlbum, kit.KindPlaylist},
			Label:  func(*kit.ActionContext) string { return i18n.T("Add to playlist") },
			Icon:   "list-plus",
			Group:  kit.GroupItem,
			Order:  40,
			Run: func(a *kit.ActionContext) {
				p.addToPlaylist(playlistItems(a.Items), kit.DescribeItems(a.Items))
			},
		},
		{
			ID:        "playlists.remove-from.many",
			Target:    kit.KindSelection,
			Kinds:     []kit.ItemKind{kit.KindSong},
			Label:     func(*kit.ActionContext) string { return i18n.T("Remove from playlist") },
			Icon:      "list-minus",
			Group:     kit.GroupItem,
			Order:     45,
			Available: inEditablePlaylist,
			Run: func(a *kit.ActionContext) {
				songs := make([]services.Song, 0, len(a.Items))
				for _, it := range a.Items {
					songs = append(songs, it.Song())
				}
				p.removeTracks(a.Container.ID, a.Container.Indices, songs)
			},
		},
		p.pinAction(kit.KindSong),
		p.pinAction(kit.KindAlbum),
		p.pinAction(kit.KindArtist),
		p.pinAction(kit.KindPlaylist),
		{
			ID:     "playlists.edit",
			Target: kit.KindPlaylist,
			Label:  func(*kit.ActionContext) string { return i18n.T("Edit details…") },
			Icon:   "pencil",
			Group:  kit.GroupItem,
			Order:  70,
			Available: func(a *kit.ActionContext) bool {
				pl := a.Item().Playlist()
				return isLibraryPlaylist(pl) && pl.CanEdit
			},
			Run: func(a *kit.ActionContext) { p.editDetails(a.Item().Playlist()) },
		},
		{
			ID:     "playlists.cover",
			Target: kit.KindPlaylist,
			Label:  func(*kit.ActionContext) string { return i18n.T("Change cover…") },
			Icon:   "image",
			Group:  kit.GroupItem,
			Order:  72,
			// The cover is Sona's own, not Apple's: any playlist of the library
			// has one to choose, even one Apple keeps up to date.
			Available: func(a *kit.ActionContext) bool { return isLibraryPlaylist(a.Item().Playlist()) },
			Run: func(a *kit.ActionContext) {
				pl := a.Item().Playlist()
				p.changeCover(pl.ID, pl.Name)
			},
		},
		{
			ID:     "playlists.move",
			Target: kit.KindPlaylist,
			Label:  func(*kit.ActionContext) string { return i18n.T("Move to folder…") },
			Icon:   "folder-input",
			Group:  kit.GroupItem,
			Order:  75,
			// Folders hold the user's own playlists; the others sit in their
			// sidebar section.
			Available: func(a *kit.ActionContext) bool {
				pl := a.Item().Playlist()
				return isLibraryPlaylist(pl) && pl.CanEdit
			},
			Data: []kit.MenuData{tree},
			Run: func(a *kit.ActionContext) {
				pl := a.Item().Playlist()
				byID, _ := kit.DataOf[map[string]services.TreePlaylist](a, tree)
				p.dialogs.show(dialogState{kind: dialogMove, id: pl.ID, name: pl.Name, folderID: byID[pl.ID].FolderID})
			},
		},
		{
			ID:     "playlists.delete",
			Target: kit.KindPlaylist,
			Label:  func(*kit.ActionContext) string { return i18n.T("Delete from library") },
			Icon:   "trash",
			Group:  kit.GroupItem,
			Order:  90,
			// Apple keeps the personal mixes it makes; only playlists the user
			// added can go.
			Available: func(a *kit.ActionContext) bool {
				pl := a.Item().Playlist()
				return isLibraryPlaylist(pl) && playlistSection(pl) != sectionMadeForYou
			},
			Enabled: func(a *kit.ActionContext) bool {
				byID, _ := kit.DataOf[map[string]services.TreePlaylist](a, tree)
				return byID[a.Item().Playlist().ID].CanDelete
			},
			Data: []kit.MenuData{tree},
			Run: func(a *kit.ActionContext) {
				pl := a.Item().Playlist()
				p.deletePlaylist(pl.ID, pl.Name, func(ok bool) {
					if ok && p.env.Nav.Path() == "/playlist/"+pl.ID {
						// Leave the deleted playlist's page: to the library's
						// Playlists when that page is ported, else back.
						if p.env.Routed(playlistsPath) {
							a.Navigate(playlistsPath)
						} else {
							p.env.Nav.Back()
						}
					}
				})
			},
		},
	}
}

// editDetails opens the details dialog with the playlist's current name,
// description and visibility (the row's own data when the read fails).
func (p *playlists) editDetails(pl services.Playlist) {
	go func() {
		d, err := p.api.Playlist(context.Background(), pl.ID)
		p.env.Post(func() {
			if err != nil {
				p.dialogs.show(dialogState{kind: dialogDetails, id: pl.ID, name: pl.Name, description: pl.Description})
				return
			}
			p.dialogs.show(dialogState{kind: dialogDetails, id: pl.ID, name: d.Playlist.Name, description: d.Playlist.Description, isPublic: d.IsPublic})
		})
	}()
}
