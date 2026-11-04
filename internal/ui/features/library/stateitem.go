package library

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// stateOfItem is the state kind and item of a menu's item for
// Env.ItemStates; ok is false for kinds with no library state (stations).
func stateOfItem(it kit.Item) (kind string, si kit.StateItem, ok bool) {
	switch it.Kind {
	case kit.KindSong:
		return kit.StateSongs, kit.StateOfSong(it.Song()), true
	case kit.KindAlbum:
		return kit.StateAlbums, kit.StateOfAlbum(it.Album()), true
	case kit.KindArtist:
		return kit.StateArtists, kit.StateOfArtist(it.Artist()), true
	case kit.KindPlaylist:
		return kit.StatePlaylists, kit.StateOfPlaylist(it.Playlist()), true
	case kit.KindMusicVideo:
		return kit.StateMusicVideos, kit.StateOfVideo(it.MusicVideo()), true
	}
	return "", kit.StateItem{}, false
}

// copyLink copies an item's music.apple.com link (Go writes the clipboard):
// kind is a state kind or "stations".
func (l *library) copyLink(kind string, it kit.StateItem) {
	l.run(i18n.T("Couldn't copy the link"), func(context.Context) error {
		_, err := l.lib.CopyLink(kit.ItemRefOf(kind, it))
		return err
	}, func(ok bool) {
		if ok {
			kit.ShowToast(kit.ToastSpec{Title: i18n.T("Link copied")})
		}
	})
}
