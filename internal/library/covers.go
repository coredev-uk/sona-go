package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// coverBatch bounds the playlists looked up by one statement.
const coverBatch = 500

// CoverKind is what a library playlist shows as its cover.
type CoverKind string

const (
	// CoverAuto: Apple's artwork, or a mosaic of the playlist's tracks'
	// albums when Apple has none.
	CoverAuto CoverKind = "auto"
	// CoverNone: the plain placeholder.
	CoverNone CoverKind = "none"
	// CoverPreset: a generated cover (artwork.Presets); Template is its
	// template.
	CoverPreset CoverKind = "preset"
	// CoverUpload: an image the user chose; Template is its template
	// (artwork.SaveCover).
	CoverUpload CoverKind = "upload"
)

// CoverChoice is the cover the user chose for a library playlist. The
// zero value is CoverAuto with no image kept.
type CoverChoice struct {
	Kind CoverKind
	// Template is the artwork template of a preset or an upload.
	Template string
	// Upload is the template of the last image the user chose for the
	// playlist, kept while another cover shows ("" when none).
	Upload string
}

// PlaylistCover returns the cover chosen for library playlist id (the zero
// CoverChoice when none was).
func (l *Library) PlaylistCover(ctx context.Context, id string) (CoverChoice, error) {
	r, err := l.db.RQ.GetPlaylistCover(ctx, db.NullString(id))
	if errors.Is(err, sql.ErrNoRows) {
		return CoverChoice{Kind: CoverAuto}, nil
	}
	if err != nil {
		return CoverChoice{}, fmt.Errorf("library: playlist cover: %w", err)
	}
	return CoverChoice{Kind: CoverKind(r.Kind), Template: r.Template, Upload: r.Upload}, nil
}

// SetPlaylistCover stores the cover chosen for library playlist id, or
// forgets it when c is the zero choice (automatic, no image kept). It
// fails with ErrNotFound for a playlist the library does not hold.
func (l *Library) SetPlaylistCover(ctx context.Context, id string, c CoverChoice) error {
	if c.Kind == "" {
		c.Kind = CoverAuto
	}
	return l.db.Tx(ctx, func(q *dbq.Queries) error {
		if c.Kind == CoverAuto && c.Upload == "" {
			return q.DeletePlaylistCover(ctx, db.NullString(id))
		}
		n, err := q.SetPlaylistCover(ctx, dbq.SetPlaylistCoverParams{
			Kind: string(c.Kind), Template: c.Template, Upload: c.Upload,
			UpdatedAt: db.Millis(time.Now()), LibraryID: db.NullString(id),
		})
		if err != nil {
			return fmt.Errorf("library: set playlist cover: %w", err)
		}
		if n == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// CoverUploads returns the templates of the images the user chose for any
// playlist, the ones to keep when the covers directory is tidied.
func (l *Library) CoverUploads(ctx context.Context) ([]string, error) {
	refs, err := l.db.RQ.ListPlaylistCoverUploads(ctx)
	if err != nil {
		return nil, fmt.Errorf("library: cover uploads: %w", err)
	}
	return refs, nil
}

// AutoCover returns the automatic cover of library playlist id whatever the
// user chose: Apple's artwork, or the mosaic of its tracks' albums when
// Apple has none ("" when it has neither).
func (l *Library) AutoCover(ctx context.Context, id string) (string, error) {
	pls, err := query(ctx, l, `SELECT `+playlistColumns+` FROM playlists p
		JOIN library_items li ON li.kind = 'playlist' AND li.ref = p.id
		WHERE p.library_id = ?`, scanPlaylist, id)
	if err != nil {
		return "", err
	}
	if len(pls) == 0 {
		return "", ErrNotFound
	}
	if err := l.fillMosaics(ctx, pls, nil); err != nil {
		return "", err
	}
	return pls[0].ArtworkTemplate, nil
}

// FillCovers sets the cover of every library playlist of pls into
// ArtworkTemplate, where the artwork cache reads it like any other. A cover
// the user chose wins ("" for none, the plain placeholder). Otherwise a
// playlist that has no artwork of its own (Apple renders none for many user
// playlists) gets the cover artwork.Mosaic picks from its tracks: the
// artworks of its first distinct albums, in track order. A playlist without
// tracks, or whose tracks have no artwork, keeps an empty template. The
// tracks are read from the synced library, so the cover follows the
// playlist as soon as its tracks change.
func (l *Library) FillCovers(ctx context.Context, pls []Playlist) error {
	var ids []int
	for i, p := range pls {
		if p.LibraryID != "" {
			ids = append(ids, i)
		}
	}
	chosen := map[int]bool{}
	for rest := ids; len(rest) > 0; {
		n := min(len(rest), coverBatch)
		batch := rest[:n]
		rest = rest[n:]
		args := make([]any, len(batch))
		at := make(map[string]int, len(batch))
		for j, i := range batch {
			args[j] = pls[i].LibraryID
			at[pls[i].LibraryID] = i
		}
		rows, err := query(ctx, l, `SELECT p.library_id, pc.kind, pc.template
			FROM playlist_covers pc JOIN playlists p ON p.id = pc.playlist_id
			WHERE p.library_id IN (`+placeholders(len(args))+`) AND pc.kind <> 'auto'`,
			func(sc interface{ Scan(...any) error }) ([3]string, error) {
				var r [3]string
				err := sc.Scan(&r[0], &r[1], &r[2])
				return r, err
			}, args...)
		if err != nil {
			return fmt.Errorf("library: playlist covers: %w", err)
		}
		for _, r := range rows {
			i := at[r[0]]
			pls[i].ArtworkTemplate = ""
			if CoverKind(r[1]) != CoverNone {
				pls[i].ArtworkTemplate = r[2]
			}
			chosen[i] = true
		}
	}
	return l.fillMosaics(ctx, pls, chosen)
}

// fillMosaics gives every library playlist of pls that has no artwork, and
// is not in skip, its mosaic.
func (l *Library) fillMosaics(ctx context.Context, pls []Playlist, skip map[int]bool) error {
	var need []int
	for i, p := range pls {
		if p.ArtworkTemplate == "" && p.LibraryID != "" && !skip[i] {
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
