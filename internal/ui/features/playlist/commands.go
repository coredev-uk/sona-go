package playlist

import (
	"context"
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// maxPins is how many pins Apple keeps.
const maxPins = 6

// playlistItems are the songs, albums and playlists among items, as a
// playlist takes them (commands.ts's playlistItems).
func playlistItems(items []kit.Item) []services.PlaylistItem {
	var out []services.PlaylistItem
	for _, it := range items {
		switch t := it.Target.(type) {
		case services.Song:
			out = append(out, services.PlaylistItem{Kind: "song", ID: t.ID, Source: t.Source})
		case services.Album:
			out = append(out, services.PlaylistItem{Kind: "album", ID: t.ID, Source: t.Source})
		case services.Playlist:
			out = append(out, services.PlaylistItem{Kind: "playlist", ID: t.ID, Source: t.Source})
		}
	}
	return out
}

// libraryIDOf is the library ID an item can be pinned by ("" when it is not
// in the library): its libraryId, or its own ID in a library source.
func libraryIDOf(it kit.Item) string {
	var libraryID, id string
	var src services.Source
	switch t := it.Target.(type) {
	case services.Song:
		libraryID, id, src = t.LibraryID, t.ID, t.Source
	case services.Album:
		libraryID, id, src = t.LibraryID, t.ID, t.Source
	case services.Artist:
		libraryID, id, src = t.LibraryID, t.ID, t.Source
	case services.Playlist:
		libraryID, id, src = t.LibraryID, t.ID, t.Source
	}
	if libraryID != "" {
		return libraryID
	}
	if src == services.SourceLibrary {
		return id
	}
	return ""
}

// fail shows what failed and why (the product's words for Go's code).
func (p *playlists) fail(what string, err error) {
	kit.ToastErrorf(what, failText(err))
}

// run calls fn off the main thread and tells the user when it fails; done
// (nil: none) gets the outcome on the main thread.
func (p *playlists) run(what string, fn func(ctx context.Context) error, done func(ok bool)) {
	go func() {
		err := fn(context.Background())
		p.env.Post(func() {
			if err != nil {
				p.fail(what, err)
			}
			if done != nil {
				done(err == nil)
			}
		})
	}()
}

// openPlaylist goes to a playlist's page.
func (p *playlists) openPlaylist(id string) { p.env.Nav.Push("/playlist/" + id) }

// addToPlaylist opens the picker for adding items (label names them:
// “Hundred”, “Parachutes”).
func (p *playlists) addToPlaylist(items []services.PlaylistItem, label string) {
	p.dialogs.show(dialogState{kind: dialogAdd, items: items, label: label})
}

// addToPlaylistNow adds items to a library playlist (the picker's choice,
// or a drop on the sidebar) with a toast either way; the success toast's
// Open goes to the playlist.
func (p *playlists) addToPlaylistNow(playlistID, name string, items []services.PlaylistItem, label string) {
	go func() {
		detail, err := p.api.Add(context.Background(), playlistID, items)
		p.env.Post(func() {
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't add to “{name}”", "name", name), failText(err))
				return
			}
			store.Set(p.env.Queries, keyDetail(detail.Playlist.ID), detail)
			p.env.Queries.Invalidate(keyRecents)
			kit.ShowToast(kit.ToastSpec{
				Title:    i18n.T("Added {label} to “{name}”", "label", label, "name", name),
				Action:   i18n.T("Open"),
				OnAction: func() { p.openPlaylist(playlistID) },
			})
		})
	}()
}

// deletePlaylist asks, then deletes a library playlist; done (nil: none)
// is told whether it is gone.
func (p *playlists) deletePlaylist(id, name string, done func(ok bool)) {
	kit.Confirm(kit.ConfirmOptions{
		Title:        i18n.T("Delete “{name}”?", "name", name),
		Body:         i18n.T("It's removed from your library on all your devices. The songs in it stay in your library."),
		ConfirmLabel: i18n.T("Delete playlist"),
		Destructive:  true,
	}, func(ok bool) {
		if !ok {
			if done != nil {
				done(false)
			}
			return
		}
		p.run(i18n.T("Couldn't delete “{name}”", "name", name),
			func(ctx context.Context) error { return p.api.Delete(ctx, id) },
			func(ok bool) {
				if ok {
					kit.ShowToast(kit.ToastSpec{Title: i18n.T("Deleted “{name}”", "name", name)})
				}
				if done != nil {
					done(ok)
				}
			})
	})
}

// deleteFolder asks, then deletes an empty folder.
func (p *playlists) deleteFolder(id, name string) {
	kit.Confirm(kit.ConfirmOptions{
		Title:        i18n.T("Delete the folder “{name}”?", "name", name),
		Body:         i18n.T("The empty folder is removed on all your devices."),
		ConfirmLabel: i18n.T("Delete folder"),
		Destructive:  true,
	}, func(ok bool) {
		if !ok {
			return
		}
		p.run(i18n.T("Couldn't delete the folder “{name}”", "name", name),
			func(ctx context.Context) error { return p.api.DeleteFolder(ctx, id) },
			func(ok bool) {
				if ok {
					kit.ShowToast(kit.ToastSpec{Title: i18n.T("Deleted the folder “{name}”", "name", name)})
				}
			})
	})
}

// pin pins a library item to the sidebar (Apple keeps six); done (nil:
// none) is told whether it worked.
func (p *playlists) pin(id, name string, done func(ok bool)) {
	go func() {
		err := p.api.Pin(context.Background(), id)
		p.env.Post(func() {
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't pin “{name}”", "name", name), failText(err))
			} else {
				kit.ShowToast(kit.ToastSpec{Title: i18n.T("Pinned “{name}”", "name", name)})
			}
			if done != nil {
				done(err == nil)
			}
		})
	}()
}

// unpin removes a pin.
func (p *playlists) unpin(id, name string) {
	p.run(i18n.T("Couldn't unpin “{name}”", "name", name),
		func(ctx context.Context) error { return p.api.Unpin(ctx, id) }, nil)
}

// playPin plays (or shuffles) what a pin holds, from the local library.
func (p *playlists) playPin(id, name string, shuffle bool) {
	go func() {
		ctx := context.Background()
		songs, err := p.api.PinTracks(ctx, id)
		if err != nil {
			p.env.Post(func() { kit.ToastErrorf(i18n.T("Couldn't play “{name}”", "name", name), failText(err)) })
			return
		}
		if len(songs) == 0 {
			p.env.Post(func() { kit.ShowToast(kit.ToastSpec{Title: i18n.T("“{name}” has no songs to play", "name", name)}) })
			return
		}
		// Pins are library playlists ("p.…") or albums ("l.…").
		from := services.PlayFrom{ID: id}
		switch {
		case strings.HasPrefix(id, "p."):
			from.Kind = "playlist"
		case strings.HasPrefix(id, "l."):
			from.Kind = "album"
		}
		p.env.Post(func() { p.playSongs(songs, 0, from, shuffle) })
	}()
}

// playSongs plays songs from start as the queue, or shuffled (the shared
// queue commands: a long queue the user built is asked about or offered an
// Undo). Main thread.
func (p *playlists) playSongs(songs []services.Song, start int, from services.PlayFrom, shuffle bool) {
	if shuffle {
		p.env.Queue.ShuffleSongs(songs, from)
		return
	}
	p.env.Queue.PlaySongs(songs, start, from)
}
