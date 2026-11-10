package playlist

import (
	"slices"
	"strings"

	"golang.org/x/text/collate"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// folded is a song's searchable text, folded once for a pass over the list.
type folded struct{ name, artist, album string }

// viewOrder is the songs the page shows and the order they show in: the
// positions in tracks of those that match query, sorted by key (a kit.Sort…
// key; "" is the playlist's own order). The query is the words of
// i18n.Fold(query), and a song matches when each of them is somewhere in its
// name, artist or album, so "mia salt" finds Mia's song on Salt Flats. The
// sort folds case and accents and reads digits as numbers (collate.Numeric,
// "Track 2" before "Track 10"); songs that compare equal keep the order they
// have in the playlist, descending or not, so the result is stable. The
// answers are original indices: positions in the playlist, which is what
// RemoveTracks and MoveTrack are given.
func viewOrder(tracks []services.Song, key string, desc bool, query string) []int {
	words := strings.Fields(i18n.Fold(query))
	needText := len(words) > 0 || key == kit.SortTitle || key == kit.SortArtist || key == kit.SortAlbum
	var text []folded
	if needText {
		text = make([]folded, len(tracks))
		for i, s := range tracks {
			text[i] = folded{i18n.Fold(s.Name), i18n.Fold(s.Artist), i18n.Fold(s.Album)}
		}
	}

	order := make([]int, 0, len(tracks))
	for i := range tracks {
		if matchesAll(text, i, words) {
			order = append(order, i)
		}
	}

	var cmp func(a, b int) int
	switch key {
	case kit.SortTitle:
		cmp = textCompare(func(i int) string { return text[i].name })
	case kit.SortArtist:
		cmp = textCompare(func(i int) string { return text[i].artist })
	case kit.SortAlbum:
		cmp = textCompare(func(i int) string { return text[i].album })
	case kit.SortDuration:
		cmp = func(a, b int) int {
			switch x, y := tracks[a].DurationMs, tracks[b].DurationMs; {
			case x < y:
				return -1
			case x > y:
				return 1
			}
			return 0
		}
	default:
		return order
	}
	if desc {
		asc := cmp
		cmp = func(a, b int) int { return asc(b, a) }
	}
	slices.SortStableFunc(order, cmp)
	return order
}

// matchesAll reports whether every word is in song i's folded text (no
// words: every song matches).
func matchesAll(text []folded, i int, words []string) bool {
	for _, w := range words {
		t := text[i]
		if !strings.Contains(t.name, w) && !strings.Contains(t.artist, w) && !strings.Contains(t.album, w) {
			return false
		}
	}
	return true
}

// textCompare orders positions by the text of at, by the shown language's
// rules with digits read as numbers.
func textCompare(at func(i int) string) func(a, b int) int {
	col := i18n.Collator(collate.Numeric)
	return func(a, b int) int { return col.CompareString(at(a), at(b)) }
}

// shown is what the page lists: the songs in the order they are listed, and
// where each is in the playlist (nil when that is the playlist itself).
type shown struct {
	songs []services.Song
	order []int
}

// place is the position in the playlist of listed song i.
func (v shown) place(i int) int {
	if v.order == nil {
		return i
	}
	return v.order[i]
}

// viewCache is shown's last answer and what it came from (the page asks every
// frame, and the answer changes only with these).
type viewCache struct {
	tracks *services.Song
	n      int
	key    string
	desc   bool
	query  string
	valid  bool
	shown
}

// view is the playlist's songs as the sort and the filter list them,
// worked out again only when one of them, or the tracks, changed.
func (pg *page) view(tracks []services.Song) shown {
	v := &pg.shownCache
	var first *services.Song
	if len(tracks) > 0 {
		first = &tracks[0]
	}
	key, desc, query := pg.sort.Key, pg.sort.Desc, pg.query
	if v.valid && v.tracks == first && v.n == len(tracks) && v.key == key && v.desc == desc && v.query == query {
		return v.shown
	}
	*v = viewCache{tracks: first, n: len(tracks), key: key, desc: desc, query: query, valid: true}
	if key == "" && strings.TrimSpace(query) == "" {
		v.songs = tracks
		return v.shown
	}
	v.order = viewOrder(tracks, key, desc, query)
	v.songs = make([]services.Song, len(v.order))
	for i, at := range v.order {
		v.songs[i] = tracks[at]
	}
	return v.shown
}

// filtering reports whether the page narrows the playlist to a query.
func (pg *page) filtering() bool { return strings.TrimSpace(pg.query) != "" }

// clearView puts the songs back in the playlist's order, unfiltered.
func (pg *page) clearView() {
	pg.sort.Key, pg.sort.Desc, pg.query = "", false, ""
}
