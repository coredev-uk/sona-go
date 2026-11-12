package album

import (
	"regexp"
	"sort"
	"strings"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is v0.2.0's features/album/album.ts, album/layout.ts and
// album/search-params.ts: the page's pure logic.

// disc is one disc of a multi-disc album: where its songs start in the
// list, how many, how long.
type disc struct {
	number     int
	first      int // index of its first song in the album's list
	songs      int
	durationMs int64
}

// discsOf is the album's discs in list order, or none when the songs sit on
// one disc (a single disc needs no index).
func discsOf(songs []services.Song) []disc {
	var discs []disc
	for i, s := range songs {
		if n := len(discs); n > 0 && discs[n-1].number == s.DiscNumber {
			discs[n-1].songs++
			discs[n-1].durationMs += s.DurationMs
		} else {
			discs = append(discs, disc{number: s.DiscNumber, first: i, songs: 1, durationMs: s.DurationMs})
		}
	}
	if len(discs) > 1 {
		return discs
	}
	return nil
}

// isCompilation: whether the songs are credited to many artists (a
// soundtrack, a compilation), which gives the list an Artist column. An
// album with a featured artist on a few songs names them under those titles
// instead. A song with no credit yet counts for neither.
func isCompilation(songs []services.Song, albumArtist string) bool {
	credited, others := 0, 0
	for _, s := range songs {
		if s.Artist == "" {
			continue
		}
		credited++
		if s.Artist != albumArtist {
			others++
		}
	}
	return others*2 > credited
}

var writerSplit = regexp.MustCompile(`\s*(?:,|&)\s*`)

// writersOf are the people in Apple's composer credits ("Taylor Swift, Jack
// Antonoff & Zoë Kravitz"), each once, most credited first (ties in album
// order).
func writersOf(songs []services.Song) []string {
	count := map[string]int{}
	var order []string
	for _, s := range songs {
		for _, name := range writerSplit.Split(s.Composer, -1) {
			n := strings.TrimSpace(name)
			if n == "" {
				continue
			}
			if _, seen := count[n]; !seen {
				order = append(order, n)
			}
			count[n]++
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return count[order[i]] > count[order[j]] })
	return order
}

// hasWorks: a classical album, where Apple names the work each track
// belongs to. Its rows are grouped under their works and read by movement.
func hasWorks(songs []services.Song) bool {
	for _, s := range songs {
		if s.Work != "" {
			return true
		}
	}
	return false
}

// workHeading is the heading a work's first track carries on a classical
// album: the work, led by its composer when the album has more than one
// ("Johann Sebastian Bach · Goldberg Variations"). A track outside any work,
// or continuing the previous track's work on the same disc, gets none.
func workHeading(song services.Song, prev *services.Song, composers int) string {
	if song.Work == "" {
		return ""
	}
	if prev != nil && prev.Work == song.Work && prev.DiscNumber == song.DiscNumber {
		return ""
	}
	if composers > 1 && song.Composer != "" {
		return i18n.Meta(song.Composer, song.Work)
	}
	return song.Work
}

// movementTitle is a track's title under its work's heading: the movement,
// when Apple names one.
func movementTitle(song services.Song) string {
	if song.Work != "" && song.Movement != "" {
		return song.Movement
	}
	return song.Name
}

var fourDigits = regexp.MustCompile(`^\d{4}$`)

// firstReleased is the year the album's oldest recording first came out,
// when that is before the album's own year (a reissue, a compilation); ""
// otherwise.
func firstReleased(songs []services.Song, albumDate string) string {
	album := prefix(albumDate, 4)
	first := ""
	for _, s := range songs {
		y := prefix(s.ReleaseDate, 4)
		if fourDigits.MatchString(y) && (first == "" || y < first) {
			first = y
		}
	}
	if first != "" && album != "" && first < album {
		return first
	}
	return ""
}

// prefix is the first n bytes of s (JavaScript's slice(0, n) on the ASCII
// dates Apple sends).
func prefix(s string, n int) string {
	if len(s) < n {
		return s
	}
	return s[:n]
}

// withoutPlaceholderCredits: an album not out yet lists songs Apple hasn't
// published as placeholders ("Track 3", no duration) credited to an artist it
// doesn't know yet. Their credit is dropped so the rows don't repeat Apple's
// stand-in name. The songs are copied; the input is never changed.
func withoutPlaceholderCredits(songs []services.Song, upcoming bool) []services.Song {
	if !upcoming {
		return songs
	}
	out := make([]services.Song, len(songs))
	copy(out, songs)
	for i := range out {
		if !out[i].Playable && out[i].DurationMs == 0 {
			out[i].Artist = ""
		}
	}
	return out
}

// share is how much of a record a library album holds.
type share struct{ have, total int }

// libraryShare: a library album holding only some of its songs: how many it
// has of the catalogue album's total. Nil when it holds them all, or when the
// catalogue album isn't known (total 0).
func libraryShare(have, catalogue int) *share {
	if catalogue > have {
		return &share{have, catalogue}
	}
	return nil
}

// layout is the album page's layout for the content's width
// (docs/design.md, Album page): the sleeve beside the listing from 860, the
// cover larger from 1280; below 860 the sleeve heads the page with the title
// beside the cover. Covers come in fixed sizes so Go serves them at the
// exact box.
type layout struct {
	wide  bool
	cover float32
}

func albumLayout(width float32) layout {
	switch {
	case width >= 1280:
		return layout{true, 320}
	case width >= 860:
		return layout{true, 256}
	}
	return layout{false, 168}
}

// isLibraryID: library album IDs carry Apple's "l." prefix; everything else
// is catalogue (shared/lib/queries.ts).
func isLibraryID(id string) bool { return strings.HasPrefix(id, "l.") }
