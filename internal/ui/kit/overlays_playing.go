package kit

import "github.com/coredev-uk/sona-go/internal/services"

// PlayingTarget is the action-menu target of the playing queue entry, for
// the player bar, Now Playing and the mini player: a song as a song row's,
// a music video as a video's, a live station as a station's. It reports
// false when nothing plays, so there is no menu. The entry has no
// container: it is not in a list the menu could remove it from.
func PlayingTarget(cur *services.QueueItem) (ActionTarget, bool) {
	if cur == nil {
		return ActionTarget{}, false
	}
	var it Item
	switch {
	case cur.Live:
		it = StationItem(services.Station{ID: cur.ID, Name: cur.Name, Kind: services.StationLive, Playable: true})
	case cur.Video:
		id := cur.CatalogID
		if id == "" {
			id = cur.ID
		}
		it = MusicVideoItem(services.MusicVideo{
			ID: cur.ID, Source: cur.Source, CatalogID: id, LibraryID: cur.LibraryID, Name: cur.Name,
			Artist: cur.Artist, ArtistID: cur.ArtistID, Album: cur.Album, DurationMs: cur.DurationMs,
			Explicit: cur.Explicit, Art: cur.Art,
		})
	default:
		it = SongItem(services.Song{
			ID: cur.ID, Source: cur.Source, CatalogID: cur.CatalogID, LibraryID: cur.LibraryID, Name: cur.Name,
			Artist: cur.Artist, Album: cur.Album, AlbumID: cur.AlbumID, ArtistID: cur.ArtistID,
			DurationMs: cur.DurationMs, Explicit: cur.Explicit, Playable: true, Art: cur.Art,
		})
	}
	return ActionTarget{Item: it, Name: cur.Name, Key: "playing"}, true
}
