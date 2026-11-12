package playlist

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// startOffsets is when each song starts if the playlist plays through from
// its first song: 0:00 for the first, then the running total of the lengths
// before it. nil when no song has a length (nothing to count).
func startOffsets(tracks []services.Song) []time.Duration {
	at := make([]time.Duration, len(tracks))
	var run time.Duration
	for i, s := range tracks {
		at[i] = run
		run += time.Duration(max(0, s.DurationMs)) * time.Millisecond
	}
	if run == 0 {
		return nil
	}
	return at
}

// startsCache is the labels of startOffsets for the tracks they were made
// from. A playlist edit makes a new slice (withMove, withRemoved), so the
// first song's address and the count say when the labels are stale.
type startsCache struct {
	first *services.Song
	n     int
	valid bool
	// at is the label of each position; byRow the same by the song's
	// address in the list that shows the playlist's own order.
	at    []string
	byRow map[*services.Song]string
}

// starts are the "starts at" labels of tracks, worked out again only when
// the tracks changed. nil when no song has a length.
func (pg *page) starts(tracks []services.Song) *startsCache {
	sc := &pg.startsCache
	var first *services.Song
	if len(tracks) > 0 {
		first = &tracks[0]
	}
	if sc.valid && sc.first == first && sc.n == len(tracks) {
		return sc
	}
	*sc = startsCache{first: first, n: len(tracks), valid: true}
	offsets := startOffsets(tracks)
	if offsets == nil {
		return sc
	}
	sc.at = make([]string, len(offsets))
	sc.byRow = make(map[*services.Song]string, len(offsets))
	for i, o := range offsets {
		sc.at[i] = i18n.FormatDuration(o)
		sc.byRow[&tracks[i]] = sc.at[i]
	}
	return sc
}
