package discover

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// registerPlayers is v0.2.0's registerPlayers: how the shared cards play
// songs, albums, playlists and music videos (kit.ItemPlayers): a song or a
// video alone, a collection from its first track, a track shelf as the queue.
// Whether a video shows its play control is the platform's call
// (ItemPlayers.CanPlayVideo).
func (d *discover) registerPlayers() {
	p := d.env.Players
	if p == nil {
		return
	}
	p.RegisterSongs(func(songs []services.Song, start int, label string) {
		d.env.Queue.PlaySongs(songs, start, services.PlayFrom{ID: label})
	})
	// A video plays alone as the queue, then the video view opens, from the
	// control that asked, which gets the focus back when it closes.
	p.RegisterVideo(func(v services.MusicVideo, opener *ui.Handle) {
		d.env.Queue.PlayVideos([]services.MusicVideo{v}, 0, func() { d.env.Video.OpenFrom(opener) })
	})
	p.Register(services.ItemSong, func(it services.Item) {
		if it.Song != nil {
			d.env.Queue.PlaySongs([]services.Song{*it.Song}, 0, services.PlayFrom{ID: it.Name})
		}
	})
	p.Register(services.ItemAlbum, d.playCollection)
	p.Register(services.ItemPlaylist, d.playCollection)
}

// playCollection plays an album or a playlist from its first track, from the
// collection itself ({kind, id}, reported with the plays). Its songs load off
// the main thread; a failure is a toast naming what failed, with the reason.
func (d *discover) playCollection(it services.Item) {
	var what string
	switch {
	case it.Album != nil:
		what = i18n.T("Couldn't load the album")
	case it.Playlist != nil:
		what = i18n.T("Couldn't load the playlist")
	default:
		return
	}
	from := services.PlayFrom{Kind: it.Kind, ID: it.ID}
	go func() {
		songs, err := d.tracks.Tracks(context.Background(), it)
		d.env.Post(func() {
			if err != nil {
				kit.ToastFailure(what, err)
				return
			}
			d.env.Queue.PlaySongs(songs, 0, from)
		})
	}()
}
