package services

import (
	"context"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// creditSearchLimit is how many artists a credited name's search asks for:
// enough to see that a name is shared (Apple lists three "Philip Selway"s),
// few enough that the answer is small.
const creditSearchLimit = 10

// creditArtistsType is the resource type a credited name is searched in.
const creditArtistsType = "artists"

// OwnArtist is the artist of own (the artists a song page holds, with their
// IDs) that a credited name is, ignoring case, accents and width, false when
// it is none of them. It needs no request, so the song page opens such a
// credit at once.
func OwnArtist(name string, own []Artist) (Artist, bool) {
	want := foldName(name)
	if want == "" {
		return Artist{}, false
	}
	for _, a := range own {
		if a.ID != "" && foldName(a.Name) == want {
			return a, true
		}
	}
	return Artist{}, false
}

// CreditedArtist is the catalogue artist a credited person is, by ID, or ""
// when no single artist is known to be them. Apple's credit (a
// credit-artists resource, docs/apple-music-reference.md, "Song") carries a
// name, roles and a picture and no link to an artist, so the person is found
// by name:
//
//   - A name that is one of own, the artists the song page already holds
//     with their IDs (OwnArtist), is that artist: no request.
//   - Any other name is searched in the catalogue's artists
//     (creditSearchLimit of them), and it is that artist only when exactly
//     one result is named the same. Two artists of one name, or none, are
//     not guessed between: the answer is "", and the UI opens a search for
//     the name for the user to pick from.
//
// A failed search returns its error; the UI treats it as "".
func (s *CatalogService) CreditedArtist(ctx context.Context, name string, own []Artist) (string, error) {
	want := foldName(name)
	if want == "" {
		return "", nil
	}
	if a, ok := OwnArtist(name, own); ok {
		return a.ID, nil
	}
	if s.c == nil {
		return "", nil
	}
	res, err := s.c.Search(ctx, name, applemusic.SearchOptions{Limit: creditSearchLimit, Types: []string{creditArtistsType}})
	if err != nil {
		return "", s.fail(err)
	}
	var id string
	var n int
	for _, ref := range res.Groups[applemusic.GroupArtist].Data {
		node, ok := res.Resources.Get(ref)
		if !ok {
			continue
		}
		var at applemusic.ArtistAttributes
		if node.Attrs(&at) != nil || foldName(at.Name) != want {
			continue
		}
		if n++; n == 1 {
			id = node.ID
		}
	}
	if n != 1 {
		return "", nil
	}
	return id, nil
}

// foldName is name as CreditedArtist compares it: Unicode-folded (widths and
// compatibility forms, accents dropped, case folded), its words
// single-spaced and typographic apostrophes straight ("Ed O’Brien" is
// "Ed O'Brien"). A transform chain keeps state, so each call builds its own.
func foldName(name string) string {
	fold := transform.Chain(norm.NFKD, runes.Remove(runes.In(unicode.Mn)), norm.NFC, cases.Fold())
	folded, _, err := transform.String(fold, name)
	if err != nil {
		folded = strings.ToLower(name)
	}
	folded = strings.NewReplacer("’", "'", "ʼ", "'", "‘", "'").Replace(folded)
	return strings.Join(strings.Fields(folded), " ")
}
