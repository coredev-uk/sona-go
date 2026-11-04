package player

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

// nowPlayingState is what the Now Playing view (the feat/now-playing PR,
// nowplaying_*.go) keeps and hooks into the bar and the queue. This file is
// the seam this PR builds alone against: while no Now Playing view exists
// there is no lyrics surface, no Up next pane and no rating for it. The
// Now Playing PR replaces the whole file (delete it when nowplaying_state.go
// lands).
type nowPlayingState struct {
	// queueView builds the Up next pane (the queue list, its summary and
	// tools); leave closes the view as a link in it is followed.
	queueView func(c *ui.Context, leave func())
	// rating builds the Favourite and Suggest less buttons for the song.
	rating func(c *ui.Context, cur *services.QueueItem)
}

// lyricsSurface reports whether the surface the bar's Lyrics button opens
// exists; toggleLyrics opens or closes it.
func (p *player) lyricsSurface() bool { return false }
func (p *player) toggleLyrics()       {}
