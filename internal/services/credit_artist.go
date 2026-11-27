package services

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/namefold"
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
	want := namefold.Fold(name)
	if want == "" {
		return Artist{}, false
	}
	for _, a := range own {
		if a.ID != "" && namefold.Fold(a.Name) == want {
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
	want := namefold.Fold(name)
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
		if node.Attrs(&at) != nil || namefold.Fold(at.Name) != want {
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
