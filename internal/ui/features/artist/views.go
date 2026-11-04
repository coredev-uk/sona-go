package artist

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is v0.2.0's features/artist/views.ts: the artist views Sona
// shows (Go's View* names) and their titles, in Apple's wording (sentence
// case), each naming the artist.

// viewTitle is a shelf's or see-all page's title, naming the artist; ok is
// false for a view Sona does not show.
func viewTitle(view, artist string) (title string, ok bool) {
	switch view {
	case services.ViewTopSongs:
		return i18n.T("Top songs by {artist}", "artist", artist), true
	case services.ViewFeaturedAlbums:
		return i18n.T("Essential albums by {artist}", "artist", artist), true
	case services.ViewFullAlbums:
		return i18n.T("Albums by {artist}", "artist", artist), true
	case services.ViewSingles:
		return i18n.T("Singles and EPs featuring {artist}", "artist", artist), true
	case services.ViewLiveAlbums:
		return i18n.T("Live albums by {artist}", "artist", artist), true
	case services.ViewCompilations:
		return i18n.T("Compilations featuring {artist}", "artist", artist), true
	case services.ViewAppearsOn:
		return i18n.T("Albums featuring {artist}", "artist", artist), true
	case services.ViewPlaylists:
		return i18n.T("Playlists featuring {artist}", "artist", artist), true
	case services.ViewMusicVideos, services.ViewTopMusicVideos:
		return i18n.T("Music videos by {artist}", "artist", artist), true
	case services.ViewSimilarArtists:
		return i18n.T("Similar artists to {artist}", "artist", artist), true
	}
	return "", false
}

// isArtistView reports whether Sona shows the view.
func isArtistView(view string) bool {
	_, ok := viewTitle(view, "")
	return ok
}
