package applemusic

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"
)

// CreditCategory is one group of a song's credits ("role-categories"):
// Kind is "performer", "composer-and-lyrics" or
// "production-and-engineering".
type CreditCategory struct {
	Resource[struct {
		Kind  string `json:"kind"`
		Title string `json:"title"`
	}]
	Relationships struct {
		CreditArtists Relationship[CreditArtist] `json:"credit-artists"`
	} `json:"relationships"`
}

// CreditArtist is one credited person with their roles on the song.
// Artwork is the person's picture where Apple has one: a person without a
// picture has no artwork, or a generated initials image (Generated).
type CreditArtist = Resource[struct {
	Name      string   `json:"name"`
	RoleNames []string `json:"roleNames"`
	Artwork   *Artwork `json:"artwork"`
}]

// SongPage returns catalog song id from the web player's API with its
// albums, artists, credits and music videos and the lyrics excerpt.
func SongPage(ctx context.Context, c *Client, sf, id string) (Song, error) {
	return AMPResource[Song](ctx, c, sf, TypeSongs, id, AMPQuery{
		Include: []string{"albums", "artists", "credits", "music-videos"},
		Extend:  []string{"lyricsExcerpt"},
		Fields: map[string][]string{
			"albums":  {"artistName", "artwork", "name", "url", "releaseDate", "trackCount", "contentRating", "isSingle", "genreNames", "playParams"},
			"artists": {"name", "url", "artwork", "genreNames"},
		},
		Extra: url.Values{"include[music-videos]": {"artists"}},
	})
}

// songArtists asks for the artists' names with the relationship: without it
// Apple sends the artists as IDs alone.
var songArtists = url.Values{"include": {"artists"}}

// Song returns catalog song id (with its album relationship and its artists,
// named).
func (c *Client) Song(ctx context.Context, sf, id string) (Song, error) {
	var body Page[Song]
	if err := c.Get(ctx, CatalogPath(sf, "songs", id), songArtists, &body); err != nil {
		return Song{}, err
	}
	return first(body.Data, "song "+id)
}

// songsBatch is how many IDs one Songs request carries.
const songsBatch = 100

// Songs returns the catalog songs with the given IDs, as Song does (with
// their album relationship and their artists, named): one request per 100 IDs, four
// requests at a time, so a whole chart or playlist costs one round trip, not
// one per song. A song Apple doesn't serve in the storefront is omitted; the
// order follows Apple's reply, not ids.
func (c *Client) Songs(ctx context.Context, sf string, ids []string) ([]Song, error) {
	chunks := slices.Collect(slices.Chunk(ids, songsBatch))
	results := make([][]Song, len(chunks))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(defaultParallel)
	for i, batch := range chunks {
		g.Go(func() error {
			var body Page[Song]
			if err := c.Get(gctx, CatalogPath(sf, "songs"), url.Values{"ids": {strings.Join(batch, ",")}, "include": {"artists"}}, &body); err != nil {
				return err
			}
			results[i] = body.Data
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return slices.Concat(results...), nil
}

// LyricsOptions select the lyrics variant.
type LyricsOptions struct {
	// Syllable asks for word-timed lyrics ("syllable-lyrics", the karaoke
	// variant) first, falling back to line-timed lyrics.
	Syllable bool
	// Language (e.g. "en-GB") asks for the translation into that language
	// and the transliteration into its script (l[lyrics], l[script],
	// extend=ttmlLocalizations). Apple only localizes syllable lyrics.
	Language string
}

// lyricsQuery is the localization query for language tag lang: "en-GB" →
// l[lyrics]=en-gb, l[script]=en-Latn.
func lyricsQuery(lang string) url.Values {
	if lang == "" {
		return nil
	}
	base, _, _ := strings.Cut(lang, "-")
	return url.Values{
		"l[lyrics]": {strings.ToLower(lang)},
		"l[script]": {strings.ToLower(base) + "-Latn"},
		"extend":    {"ttmlLocalizations"},
	}
}

// Lyrics fetches the TTML lyrics of catalog song id and parses them (see
// LyricsOptions). Songs without lyrics return an error matching
// ErrNotFound.
func (c *Client) Lyrics(ctx context.Context, sf, id string, opt LyricsOptions) (Lyrics, error) {
	kinds := []string{"lyrics"}
	if opt.Syllable {
		kinds = []string{"syllable-lyrics", "lyrics"}
	}
	var lastErr error
	for _, kind := range kinds {
		var body Page[Resource[struct {
			TTML              string `json:"ttml"`
			TTMLLocalizations string `json:"ttmlLocalizations"`
		}]]
		var q url.Values
		if kind == "syllable-lyrics" {
			q = lyricsQuery(opt.Language)
		}
		err := c.Get(ctx, c.ampBase()+CatalogPath(sf, "songs", id, kind), q, &body)
		ttml := ""
		if err == nil && len(body.Data) > 0 {
			ttml = body.Data[0].Attributes.TTMLLocalizations
			if ttml == "" {
				ttml = body.Data[0].Attributes.TTML
			}
		}
		if err == nil && ttml == "" {
			err = fmt.Errorf("%w: lyrics for song %s", ErrNotFound, id)
		}
		if err != nil {
			lastErr = err
			continue
		}
		return ParseTTML(ttml)
	}
	return Lyrics{}, lastErr
}
