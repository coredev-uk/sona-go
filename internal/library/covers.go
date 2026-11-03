package library

import (
	"context"
	"fmt"

	"github.com/coredev-uk/sona-go/internal/artwork"
)

// coverBatch bounds the playlists looked up by one statement.
const coverBatch = 500

// FillCovers gives every playlist of pls that has no artwork of its own
// (Apple renders none for many user playlists) the cover artwork.Mosaic
// picks from its tracks: the artworks of its first distinct albums, in
// track order. ArtworkTemplate then holds that template for the artwork
// cache, like any other. A playlist without tracks, or whose tracks have
// no artwork, keeps an empty template. The tracks are read from the synced
// library, so the cover follows the playlist as soon as its tracks change.
func (l *Library) FillCovers(ctx context.Context, pls []Playlist) error {
	var need []int
	for i, p := range pls {
		if p.ArtworkTemplate == "" && p.LibraryID != "" {
			need = append(need, i)
		}
	}
	for len(need) > 0 {
		n := min(len(need), coverBatch)
		batch := need[:n]
		need = need[n:]
		args := make([]any, len(batch))
		for j, i := range batch {
			args[j] = pls[i].LibraryID
		}
		// Per playlist: each artwork at its first position, the first
		// MosaicTiles of those in order (Mosaic picks the cover from them).
		covers, err := query(ctx, l, `WITH firsts AS (
				SELECT p.library_id AS lib, t.artwork_template AS tpl, MIN(pt.position) AS pos
				FROM playlists p
				JOIN playlist_tracks pt ON pt.playlist_id = p.id
				JOIN tracks t ON t.id = pt.track_id
				WHERE p.library_id IN (`+placeholders(len(args))+`) AND t.artwork_template <> ''
				GROUP BY p.library_id, t.artwork_template
			), ranked AS (
				SELECT lib, tpl, ROW_NUMBER() OVER (PARTITION BY lib ORDER BY pos) AS rn FROM firsts
			)
			SELECT lib, tpl FROM ranked WHERE rn <= ? ORDER BY lib, rn`,
			func(sc interface{ Scan(...any) error }) ([2]string, error) {
				var r [2]string
				err := sc.Scan(&r[0], &r[1])
				return r, err
			}, append(args, artwork.MosaicTiles)...)
		if err != nil {
			return fmt.Errorf("library: playlist covers: %w", err)
		}
		tracks := map[string][]string{}
		for _, r := range covers {
			tracks[r[0]] = append(tracks[r[0]], r[1])
		}
		for _, i := range batch {
			pls[i].ArtworkTemplate = artwork.Mosaic(tracks[pls[i].LibraryID])
		}
	}
	return nil
}
