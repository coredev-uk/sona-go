package library

import (
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/collate"
	"golang.org/x/text/unicode/norm"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// indexedArtist is a library artist ready to list and filter (v0.2.0's
// IndexedArtist): its folded name and the letter it files under.
type indexedArtist struct {
	entry services.ArtistEntry
	// key is the name folded (lower case, no accents), what the filter matches.
	key string
	// loose is key without spaces and punctuation, so "acdc" finds "AC/DC".
	loose string
	// letter is "A"–"Z", or "#" for digits, symbols and other scripts.
	letter string
}

// artistsOthers is the heading of everything that is not A–Z.
const artistsOthers = "#"

// artistsSplit are the letters NFD does not split into a base and a mark.
var artistsSplit = strings.NewReplacer(
	"ß", "ss", "æ", "ae", "œ", "oe", "ø", "o", "đ", "d", "ð", "d", "ł", "l", "þ", "th", "ı", "i",
)

// fold is text in lower case without accents: "Édith Piaf" → "edith piaf".
func fold(text string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(text)) {
		if !unicode.Is(unicode.M, r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(artistsSplit.Replace(b.String()))
}

// loosen is key without its spaces and punctuation.
func loosen(key string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return r
		}
		return -1
	}, key)
}

func letterOf(key string) string {
	if key != "" && key[0] >= 'a' && key[0] <= 'z' {
		return strings.ToUpper(key[:1])
	}
	return artistsOthers
}

// letterRank sorts the letters A–Z, then "#" for everything else, as music
// libraries do.
func letterRank(letter string) int {
	if letter == artistsOthers {
		return 26
	}
	return int(letter[0] - 'A')
}

// indexArtists sorts a library's artists by name, accents and case aside,
// and files each under its letter. The names compare as numbers where they
// hold digits ("Band 9" before "Band 10").
func indexArtists(entries []services.ArtistEntry) []indexedArtist {
	out := make([]indexedArtist, len(entries))
	for i, e := range entries {
		key := fold(e.Name)
		out[i] = indexedArtist{entry: e, key: key, loose: loosen(key), letter: letterOf(key)}
	}
	col := i18n.Collator(collate.Numeric)
	slices.SortStableFunc(out, func(a, b indexedArtist) int {
		if d := letterRank(a.letter) - letterRank(b.letter); d != 0 {
			return d
		}
		if d := col.CompareString(a.key, b.key); d != 0 {
			return d
		}
		if d := col.CompareString(a.entry.Name, b.entry.Name); d != 0 {
			return d
		}
		return col.CompareString(a.entry.ID, b.entry.ID)
	})
	return out
}

// filterArtists are the artists matching query, best first: a name that
// starts with it, then one with a word that does, then one that contains it,
// then one that contains it ignoring spaces and punctuation. Within each,
// A to Z. An empty query keeps every artist.
func filterArtists(artists []indexedArtist, query string) []indexedArtist {
	q := fold(query)
	if q == "" {
		return slices.Clone(artists)
	}
	loose := loosen(q)
	type hit struct {
		artist indexedArtist
		score  int
	}
	var hits []hit
	for _, a := range artists {
		score := -1
		switch {
		case strings.HasPrefix(a.key, q):
			score = 0
		case strings.Contains(a.key, " "+q):
			score = 1
		case strings.Contains(a.key, q):
			score = 2
		case loose != "" && strings.Contains(a.loose, loose):
			score = 3
		}
		if score >= 0 {
			hits = append(hits, hit{a, score})
		}
	}
	// A stable sort keeps each score A to Z.
	slices.SortStableFunc(hits, func(a, b hit) int { return a.score - b.score })
	out := make([]indexedArtist, len(hits))
	for i, h := range hits {
		out[i] = h.artist
	}
	return out
}

// artistRow is one row of the virtual list: a letter heading (letter set,
// artist nil) or an artist at position in the results.
type artistRow struct {
	letter   string
	artist   *indexedArtist
	position int
}

// Row heights, so the list never measures (docs/design.md, Lists).
const (
	letterRowHeight = 32
	artistRowHeight = 44
)

// listRows are the artists as list rows, with a heading before each letter
// when grouped.
func listRows(artists []indexedArtist, grouped bool) []artistRow {
	rows := make([]artistRow, 0, len(artists))
	letter := ""
	for i := range artists {
		a := &artists[i]
		if grouped && a.letter != letter {
			letter = a.letter
			rows = append(rows, artistRow{letter: letter})
		}
		rows = append(rows, artistRow{artist: a, position: i})
	}
	return rows
}

// rowTops are the offsets of the rows in the list, n+1 of them: the last is
// the list's height.
func rowTops(rows []artistRow) []float32 {
	tops := make([]float32, len(rows)+1)
	for i, r := range rows {
		h := float32(artistRowHeight)
		if r.artist == nil {
			h = letterRowHeight
		}
		tops[i+1] = tops[i] + h
	}
	return tops
}

// libraryLine is what the library holds of an artist: "12 albums, 148
// songs"; either part alone when the other is 0.
func libraryLine(albums, songs int) string {
	var a, s string
	if albums > 0 {
		a = i18n.T("{albums, plural, one {# album} other {# albums}}", "albums", albums)
	}
	if songs > 0 {
		s = i18n.T("{songs, plural, one {# song} other {# songs}}", "songs", songs)
	}
	switch {
	case a != "" && s != "":
		return i18n.T("{a}, {s}", "a", a, "s", s)
	case a != "":
		return a
	}
	return s
}
