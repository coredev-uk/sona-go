// Package library mirrors the user's Apple Music library into SQLite and
// serves the UI's library views from it.
//
// Syncer pulls each library domain from Apple with checkpoints in
// sync_state, so an interrupted sync resumes where it stopped; Library
// answers list, detail, count and search queries from the local store only.
package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Domain is one independently checkpointed part of the library sync.
type Domain string

// Sync domains, in the order Sync runs them. Ratings and playlist tracks
// depend on the songs, albums and playlists domains having run.
const (
	DomainArtists        Domain = "artists"
	DomainAlbums         Domain = "albums"
	DomainSongs          Domain = "songs"
	DomainPlaylists      Domain = "playlists"
	DomainPlaylistTracks Domain = "playlist-tracks"
	DomainRatings        Domain = "ratings"
)

// AllDomains lists every domain in dependency order.
var AllDomains = []Domain{DomainArtists, DomainAlbums, DomainSongs, DomainPlaylists, DomainPlaylistTracks, DomainRatings}

// ErrSyncRunning is returned by Sync while another sync is in progress.
var ErrSyncRunning = errors.New("library: sync already running")

// Progress reports sync progress. Done and Total count items (playlists for
// DomainPlaylistTracks, rated IDs checked for DomainRatings). Finished is
// set once per domain, with Err when the domain failed.
type Progress struct {
	Domain   Domain `json:"domain"`
	Done     int    `json:"done"`
	Total    int    `json:"total"`
	Finished bool   `json:"finished"`
	Err      error  `json:"-"`
}

// Syncer mirrors the library into the database. It is safe for concurrent
// use; only one Sync runs at a time.
type Syncer struct {
	db       *db.DB
	api      *applemusic.Client
	log      *slog.Logger
	parallel int
	now      func() time.Time

	mu      sync.Mutex
	running bool
	moves   map[string]pendingMove // playlist library ID → a folder move made here (playlists.go)
}

// NewSyncer returns a Syncer writing to d through api's library endpoints.
// parallel bounds concurrent Apple requests per domain (0 means 4).
func NewSyncer(d *db.DB, api *applemusic.Client, log *slog.Logger, parallel int) *Syncer {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if parallel <= 0 {
		parallel = 4
	}
	return &Syncer{db: d, api: api, log: log, parallel: parallel, now: time.Now, moves: map[string]pendingMove{}}
}

// Running reports whether a sync is in progress.
func (s *Syncer) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Sync syncs the given domains (all when none are given) in dependency
// order, calling progress (if non-nil; never concurrently) after every
// stored page. A domain interrupted earlier (cancelled, crashed,
// failed) resumes from its checkpoint; a completed domain starts a fresh
// run and, when it completes, removes items that are no longer in the
// library. The search index is rebuilt at the end if anything changed.
//
// Failing domains do not stop later ones, except on cancellation or an
// authorization error (errors.Is(err, applemusic.ErrUnauthorized)); the
// returned error joins every domain error.
func (s *Syncer) Sync(ctx context.Context, progress func(Progress), domains ...Domain) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return ErrSyncRunning
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	if progress == nil {
		progress = func(Progress) {}
	}
	if len(domains) == 0 {
		domains = AllDomains
	}
	want := make(map[Domain]bool, len(domains))
	for _, d := range domains {
		want[d] = true
	}

	var errs []error
	synced := false
	for _, d := range AllDomains {
		if !want[d] {
			continue
		}
		start := s.now()
		total, err := s.syncDomain(ctx, d, progress)
		if err != nil {
			s.log.Warn("library sync failed", "domain", d, "err", err)
			if ferr := s.db.Q.FailSync(context.WithoutCancel(ctx), dbq.FailSyncParams{LastError: err.Error(), Domain: string(d)}); ferr != nil {
				s.log.Warn("library sync: record failure", "domain", d, "err", ferr)
			}
			progress(Progress{Domain: d, Finished: true, Err: err})
			errs = append(errs, fmt.Errorf("%s: %w", d, err))
			if ctx.Err() != nil || errors.Is(err, applemusic.ErrUnauthorized) {
				break
			}
			continue
		}
		synced = true
		s.log.Info("library synced", "domain", d, "items", total, "took", s.now().Sub(start).Round(time.Millisecond))
		progress(Progress{Domain: d, Done: total, Total: total, Finished: true})
	}
	if synced {
		if err := RebuildSearch(context.WithoutCancel(ctx), s.db); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// begin returns the generation and start offset for domain d: the
// checkpoint of an unfinished run, or a new run.
func (s *Syncer) begin(ctx context.Context, d Domain) (gen int64, offset int, err error) {
	now := db.Millis(s.now())
	st, err := s.db.Q.GetSyncState(ctx, string(d))
	switch {
	case err == nil && st.InProgress == 1 && st.Generation > 0:
		err = s.db.Q.ResumeSync(ctx, dbq.ResumeSyncParams{LastStarted: sql.NullInt64{Int64: now, Valid: true}, Domain: string(d)})
		return st.Generation, int(st.NextOffset), err
	case err != nil && !errors.Is(err, sql.ErrNoRows):
		return 0, 0, err
	}
	// Generations increase strictly even if the clock does not.
	gen = max(now, st.Generation+1)
	err = s.db.Q.StartSync(ctx, dbq.StartSyncParams{Domain: string(d), Generation: gen, LastStarted: sql.NullInt64{Int64: now, Valid: true}})
	return gen, 0, err
}

// finish removes items of kind that the completed run gen did not see,
// drops rows nothing references any more and marks d done.
func (s *Syncer) finish(ctx context.Context, d Domain, kind string, gen int64, total int) error {
	return s.db.Tx(ctx, func(q *dbq.Queries) error {
		if kind != "" {
			n, err := q.DeleteStaleLibraryItems(ctx, dbq.DeleteStaleLibraryItemsParams{Kind: kind, Generation: gen})
			if err != nil {
				return err
			}
			if n > 0 {
				s.log.Info("library sync removed items", "domain", d, "removed", n)
			}
		}
		if err := gc(ctx, q); err != nil {
			return err
		}
		return q.FinishSync(ctx, dbq.FinishSyncParams{Total: int64(total), LastSuccess: sql.NullInt64{Int64: db.Millis(s.now()), Valid: true}, Domain: string(d)})
	})
}

func gc(ctx context.Context, q *dbq.Queries) error {
	for _, f := range []func(context.Context) (int64, error){
		q.DeleteOrphanLibraryPlaylists, q.DeleteOrphanLibraryTracks, q.DeleteOrphanLibraryAlbums, q.DeleteOrphanLibraryArtists,
	} {
		if _, err := f(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Syncer) syncDomain(ctx context.Context, d Domain, progress func(Progress)) (int, error) {
	switch d {
	case DomainPlaylistTracks:
		return s.syncPlaylistTracks(ctx, progress)
	case DomainRatings:
		return s.syncRatings(ctx, progress)
	}
	gen, start, err := s.begin(ctx, d)
	if err != nil {
		return 0, err
	}
	total := 0
	opt := applemusic.PageOptions{Start: start, Parallel: s.parallel, OnTotal: func(n int) { total = n }}
	// store writes one page and its checkpoint atomically.
	store := func(offset, n int, write func(q *dbq.Queries, now int64) error) error {
		err := s.db.Tx(ctx, func(q *dbq.Queries) error {
			if err := write(q, db.Millis(s.now())); err != nil {
				return err
			}
			return q.CheckpointSync(ctx, dbq.CheckpointSyncParams{NextOffset: int64(offset + n), Total: int64(total), Domain: string(d)})
		})
		if err == nil {
			progress(Progress{Domain: d, Done: min(offset+n, total), Total: total})
		}
		return err
	}
	var kind string
	switch d {
	case DomainSongs:
		kind = "song"
		_, err = s.api.LibrarySongs(ctx, opt, func(offset int, items []applemusic.Song) error {
			return store(offset, len(items), func(q *dbq.Queries, now int64) error {
				for i, it := range items {
					id, err := q.UpsertLibraryTrack(ctx, libraryTrackParams(it, now))
					if err != nil {
						return fmt.Errorf("song %s: %w", it.ID, err)
					}
					rank := sql.NullInt64{Int64: int64(offset + i), Valid: true}
					if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: kind, Ref: id, AddedRank: rank, Generation: gen}); err != nil {
						return err
					}
				}
				return nil
			})
		})
	case DomainAlbums:
		kind = "album"
		_, err = s.api.LibraryAlbums(ctx, opt, func(offset int, items []applemusic.Album) error {
			return store(offset, len(items), func(q *dbq.Queries, now int64) error {
				for _, it := range items {
					a := it.Attributes
					id, err := q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
						LibraryID: db.NullString(it.ID), CatalogID: db.NullString(it.CatalogID()),
						Name: a.Name, ArtistName: a.ArtistName,
						ArtistLibraryID: db.NullString(firstID(it.Relationships.Artists.Data, func(r applemusic.Artist) string { return r.ID })),
						ArtworkTemplate: a.Artwork.Template(), ReleaseDate: a.ReleaseDate, Genre: firstString(a.GenreNames),
						TrackCount: int64(a.TrackCount), ContentRating: a.ContentRating, UpdatedAt: now,
					})
					if err != nil {
						return fmt.Errorf("album %s: %w", it.ID, err)
					}
					if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: kind, Ref: id, DateAdded: parseDate(a.DateAdded), Generation: gen}); err != nil {
						return err
					}
				}
				return nil
			})
		})
	case DomainArtists:
		kind = "artist"
		_, err = s.api.LibraryArtists(ctx, opt, func(offset int, items []applemusic.Artist) error {
			return store(offset, len(items), func(q *dbq.Queries, now int64) error {
				for _, it := range items {
					p := dbq.UpsertLibraryArtistParams{LibraryID: db.NullString(it.ID), CatalogID: db.NullString(it.CatalogID()), Name: it.Attributes.Name, UpdatedAt: now}
					if cat := it.Relationships.Catalog.Data; len(cat) > 0 {
						p.ArtworkTemplate = cat[0].Attributes.Artwork.Template()
						p.Url = cat[0].Attributes.URL
					}
					id, err := q.UpsertLibraryArtist(ctx, p)
					if err != nil {
						return fmt.Errorf("artist %s: %w", it.ID, err)
					}
					if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: kind, Ref: id, Generation: gen}); err != nil {
						return err
					}
				}
				return nil
			})
		})
	case DomainPlaylists:
		kind = "playlist"
		_, err = s.api.LibraryPlaylists(ctx, opt, func(offset int, items []applemusic.Playlist) error {
			return store(offset, len(items), func(q *dbq.Queries, now int64) error {
				for _, it := range items {
					if it.Deleted() {
						continue // a deleted playlist's tombstone: dropped at finish
					}
					a := it.Attributes
					id, err := q.UpsertLibraryPlaylist(ctx, dbq.UpsertLibraryPlaylistParams{
						LibraryID: db.NullString(it.ID), CatalogID: db.NullString(it.CatalogID()),
						Name: a.Name, Description: a.Description.Text(), CuratorName: a.CuratorName,
						ArtworkTemplate: a.Artwork.Template(), CanEdit: db.Bool(a.CanEdit), IsPublic: db.Bool(a.IsPublic),
						LastModified: a.LastModifiedDate, UpdatedAt: now,
					})
					if err != nil {
						return fmt.Errorf("playlist %s: %w", it.ID, err)
					}
					if err := q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: kind, Ref: id, DateAdded: parseDate(a.DateAdded), Generation: gen}); err != nil {
						return err
					}
				}
				return nil
			})
		})
		if err == nil {
			err = s.syncPlaylistTree(ctx, gen)
		}
	default:
		return 0, fmt.Errorf("library: unknown domain %q", d)
	}
	if err != nil {
		return 0, err
	}
	return total, s.finish(ctx, d, kind, gen, total)
}

// syncPlaylistTracks refreshes the tracks of every library playlist whose
// lastModifiedDate changed since its tracks were last stored. Playlists are
// fetched in parallel; each playlist's tracks are replaced atomically, which
// makes the per-playlist modification stamp the resume checkpoint.
func (s *Syncer) syncPlaylistTracks(ctx context.Context, progress func(Progress)) (int, error) {
	d := DomainPlaylistTracks
	if _, _, err := s.begin(ctx, d); err != nil {
		return 0, err
	}
	pls, err := s.db.Q.LibraryPlaylistsForTrackSync(ctx)
	if err != nil {
		return 0, err
	}
	total := len(pls)
	var mu sync.Mutex
	done := 0
	step := func() {
		mu.Lock()
		defer mu.Unlock()
		done++
		progress(Progress{Domain: d, Done: done, Total: total})
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(s.parallel)
	for _, pl := range pls {
		if pl.LastModified != "" && pl.LastModified == pl.TracksModified {
			step()
			continue
		}
		g.Go(func() error {
			tracks, err := s.api.LibraryPlaylistTracks(gctx, pl.LibraryID.String, applemusic.PageOptions{Parallel: 2})
			if err != nil {
				return fmt.Errorf("playlist %s: %w", pl.LibraryID.String, err)
			}
			err = s.db.Tx(gctx, func(q *dbq.Queries) error {
				_, err := storePlaylistTracks(gctx, q, pl.ID, pl.LibraryID.String, pl.LastModified, tracks, db.Millis(s.now()))
				return err
			})
			if err == nil {
				step()
			}
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return 0, err
	}
	return total, s.finish(ctx, d, "", 0, total)
}

// ratingKinds are the library resource types whose ratings are mirrored.
var ratingKinds = []struct {
	typ string
	ids func(*dbq.Queries, context.Context) ([]sql.NullString, error)
}{
	{applemusic.TypeLibrarySongs, (*dbq.Queries).LibrarySongIDs},
	{applemusic.TypeLibraryAlbums, (*dbq.Queries).LibraryAlbumIDs},
	{applemusic.TypeLibraryPlaylists, (*dbq.Queries).LibraryPlaylistIDs},
}

// syncRatings mirrors the ratings of every library song, album and
// playlist into favorites, deleting ratings Apple no longer reports.
func (s *Syncer) syncRatings(ctx context.Context, progress func(Progress)) (int, error) {
	d := DomainRatings
	if _, _, err := s.begin(ctx, d); err != nil {
		return 0, err
	}
	runStart := db.Millis(s.now())
	type kindIDs struct {
		typ string
		ids []string
	}
	var all []kindIDs
	total := 0
	for _, k := range ratingKinds {
		rows, err := k.ids(s.db.RQ, ctx)
		if err != nil {
			return 0, err
		}
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.String)
		}
		all = append(all, kindIDs{k.typ, ids})
		total += len(ids)
	}
	done, rated := 0, 0
	for _, k := range all {
		ratings, err := s.api.Ratings(ctx, k.typ, k.ids)
		if err != nil {
			return 0, err
		}
		err = s.db.Tx(ctx, func(q *dbq.Queries) error {
			now := max(db.Millis(s.now()), runStart)
			for id, v := range ratings {
				if err := q.UpsertFavorite(ctx, dbq.UpsertFavoriteParams{Kind: k.typ, AppleID: id, Value: int64(v), UpdatedAt: now}); err != nil {
					return err
				}
			}
			_, err := q.DeleteStaleFavorites(ctx, dbq.DeleteStaleFavoritesParams{Kind: k.typ, UpdatedAt: runStart})
			return err
		})
		if err != nil {
			return 0, err
		}
		rated += len(ratings)
		done += len(k.ids)
		progress(Progress{Domain: d, Done: done, Total: total})
	}
	s.log.Debug("library ratings", "rated", rated)
	return total, s.finish(ctx, d, "", 0, total)
}

// RebuildSearch rebuilds the local search index from the library.
func RebuildSearch(ctx context.Context, d *db.DB) error {
	err := d.Tx(ctx, func(q *dbq.Queries) error {
		for _, f := range []func(context.Context) error{
			q.ClearSearchIndex, q.RebuildSearchTracks, q.RebuildSearchAlbums, q.RebuildSearchArtists, q.RebuildSearchPlaylists,
		} {
			if err := f(ctx); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("library: rebuild search index: %w", err)
	}
	return nil
}

func libraryTrackParams(s applemusic.Song, now int64) dbq.UpsertLibraryTrackParams {
	a := s.Attributes
	return dbq.UpsertLibraryTrackParams{
		LibraryID: db.NullString(s.ID), CatalogID: db.NullString(s.CatalogID()),
		Name: a.Name, ArtistName: a.ArtistName, AlbumName: a.AlbumName,
		AlbumLibraryID:  db.NullString(firstID(s.Relationships.Albums.Data, func(r applemusic.Album) string { return r.ID })),
		ArtistLibraryID: db.NullString(firstID(s.Relationships.Artists.Data, func(r applemusic.Artist) string { return r.ID })),
		ArtworkTemplate: a.Artwork.Template(), DurationMs: a.DurationInMillis,
		TrackNumber: int64(a.TrackNumber), DiscNumber: int64(a.DiscNumber), ReleaseDate: a.ReleaseDate,
		Genre: firstString(a.GenreNames), ContentRating: a.ContentRating, HasLyrics: db.Bool(a.HasLyrics),
		Isrc: a.ISRC, Composer: a.ComposerName, UpdatedAt: now,
	}
}

func catalogTrackParams(s applemusic.Song, now int64) dbq.UpsertCatalogTrackParams {
	a := s.Attributes
	return dbq.UpsertCatalogTrackParams{
		CatalogID: db.NullString(s.ID), Name: a.Name, ArtistName: a.ArtistName, AlbumName: a.AlbumName,
		AlbumCatalogID:  db.NullString(firstID(s.Relationships.Albums.Data, func(r applemusic.Album) string { return r.ID })),
		ArtistCatalogID: db.NullString(firstID(s.Relationships.Artists.Data, func(r applemusic.Artist) string { return r.ID })),
		ArtworkTemplate: a.Artwork.Template(), DurationMs: a.DurationInMillis,
		TrackNumber: int64(a.TrackNumber), DiscNumber: int64(a.DiscNumber), ReleaseDate: a.ReleaseDate,
		Genre: firstString(a.GenreNames), ContentRating: a.ContentRating, HasLyrics: db.Bool(a.HasLyrics),
		Isrc: a.ISRC, Composer: a.ComposerName, UpdatedAt: now,
	}
}

func firstID[T any](data []T, id func(T) string) string {
	if len(data) == 0 {
		return ""
	}
	return id(data[0])
}

func firstString(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

func parseDate(s string) sql.NullInt64 {
	if s == "" {
		return sql.NullInt64{}
	}
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if t, err := time.Parse(layout, s); err == nil {
			return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
		}
	}
	return sql.NullInt64{}
}
