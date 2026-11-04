package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// ItemPlayers is how shared components play items (v0.2.0's
// shared/items/play.ts). Shared code cannot import features, so the features
// that own playback register their players here at start-up (discover: songs,
// albums, playlists, music videos; radio: stations) and the app hands the
// registry to the shared components in ItemsCtx. A nil *ItemPlayers plays
// nothing. Main thread only.
type ItemPlayers struct {
	// Capabilities is what this platform and build support (the app's
	// answer of PlatformService.Capabilities); nil, or a nil result before
	// Go has answered, keeps music videos from playing.
	Capabilities func() *services.Capabilities
	// Now is the clock a release is compared with (nil: time.Now).
	Now func() time.Time

	kinds map[string]func(services.Item)
	songs func(songs []services.Song, start int, label string)
	video func(services.MusicVideo, *ui.Handle)
}

// Register sets how items of kind play; a later registration replaces it.
func (p *ItemPlayers) Register(kind string, play func(services.Item)) {
	if p.kinds == nil {
		p.kinds = map[string]func(services.Item){}
	}
	p.kinds[kind] = play
}

// RegisterSongs sets how a list of songs plays as the queue (track shelves).
func (p *ItemPlayers) RegisterSongs(play func(songs []services.Song, start int, label string)) {
	p.songs = play
}

// RegisterVideo sets how a music video (a MusicVideo, not an Item) plays.
// opener is the control that asked (nil when none did): the video view opened
// from it gives the focus back to it when it closes.
func (p *ItemPlayers) RegisterVideo(play func(v services.MusicVideo, opener *ui.Handle)) { p.video = play }

// CanPlayVideo reports whether music videos have a play control: a video
// player is registered and this platform and build play them
// (Capabilities.Playback.Video; false until Go has answered).
func (p *ItemPlayers) CanPlayVideo() bool {
	if p == nil || p.video == nil || p.Capabilities == nil {
		return false
	}
	caps := p.Capabilities()
	return caps != nil && caps.Playback.Video
}

// PlayVideo plays a music video alone, as the queue.
func (p *ItemPlayers) PlayVideo(v services.MusicVideo) { p.PlayVideoFrom(v, nil) }

// PlayVideoFrom is PlayVideo asked for by the control bound to opener.
func (p *ItemPlayers) PlayVideoFrom(v services.MusicVideo, opener *ui.Handle) {
	if p != nil && p.video != nil {
		p.video(v, opener)
	}
}

// CanPlay reports whether item has a play control: a registered kind that
// Sona can play (an album not out yet has none; a music video only where
// videos play).
func (p *ItemPlayers) CanPlay(item services.Item) bool {
	if p == nil {
		return false
	}
	if item.Kind == services.ItemMusicVideo {
		return item.MusicVideo != nil && p.CanPlayVideo()
	}
	if p.kinds[item.Kind] == nil {
		return false
	}
	switch item.Kind {
	case services.ItemSong:
		return item.Song != nil && item.Song.Playable
	case services.ItemAlbum:
		return item.Album == nil || !p.upcoming(*item.Album)
	case services.ItemStation:
		return item.Station != nil && item.Station.Playable
	}
	return true
}

// PlayItem plays an item through its kind's player; a music video plays
// alone through the video player.
func (p *ItemPlayers) PlayItem(item services.Item) { p.PlayItemFrom(item, nil) }

// PlayItemFrom is PlayItem asked for by the control bound to opener, which a
// music video's view returns the focus to when it closes.
func (p *ItemPlayers) PlayItemFrom(item services.Item, opener *ui.Handle) {
	if p == nil {
		return
	}
	if item.Kind == services.ItemMusicVideo {
		if item.MusicVideo != nil {
			p.PlayVideoFrom(*item.MusicVideo, opener)
		}
		return
	}
	if play := p.kinds[item.Kind]; play != nil {
		play(item)
	}
}

// PlaySongList plays songs from start as the queue, labelled with where they
// came from.
func (p *ItemPlayers) PlaySongList(songs []services.Song, start int, label string) {
	if p != nil && p.songs != nil {
		p.songs(songs, start, label)
	}
}

func (p *ItemPlayers) now() time.Time {
	if p != nil && p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// upcoming is v0.2.0's isUpcoming: an album not out yet is prerelease with no
// full release date, or one still to come.
func (p *ItemPlayers) upcoming(a services.Album) bool {
	return IsUpcoming(a, p.now())
}

// StationUnavailableText says why Sona can't play a station
// (Station.Unavailable), in words; "" when it can.
func StationUnavailableText(reason services.StationUnavailable) string {
	switch reason {
	case services.StationNoWidevine:
		return i18n.T("This stream isn't offered with Widevine, the only protection Sona can play.")
	}
	return ""
}

// OnAirLine is the song a broadcaster has on air, as one line: "Artist –
// Title", or just the title with no artist.
func OnAirLine(on services.OnAir) string {
	if on.Artist != "" {
		return on.Artist + " – " + on.Title
	}
	return on.Title
}
