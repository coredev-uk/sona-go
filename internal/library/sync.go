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
	"maps"
	"slices"
	"strings"
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

// Mode selects how much of the library Sync fetches.
type Mode int

const (
	// Incremental fetches what changed since the last run: songs and albums
	// newest-added first up to the first one already stored, the artist
	// count, the playlist list (tracks and folders only when a playlist
	// changed) and the ratings of items stored since the last ratings run.
	// A domain whose local count then differs from Apple's (something was
	// removed) runs fully at once. Sync runs fully instead when a requested
	// domain never completed a full run, has an interrupted run to resume,
	// or last ran fully more than FullSyncEvery ago.
	Incremental Mode = iota
	// Full refetches every requested domain and removes whatever Apple no
	// longer lists, ratings included.
	Full
)

// FullSyncEvery is how long Incremental syncs go between full runs. Full
// runs catch what incremental ones cannot see: a removal offset by an add,
// metadata edits, and ratings changed on other devices.
const FullSyncEvery = 72 * time.Hour

// incrementalMax bounds the new items an incremental run takes per domain;
// more than this runs the domain fully, which pages in parallel.
const incrementalMax = 1000

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
// order, calling progress (if non-nil; never concurrently) as items are
// stored. mode chooses an incremental or a full run (see Mode). A domain
// interrupted earlier (cancelled, crashed, failed) resumes its full run
// from its checkpoint; a full run, when it completes, removes items that
// are no longer in the library. The search index is rebuilt at the end if
// the library changed.
//
// Failing domains do not stop later ones, except on cancellation or an
// authorization error (errors.Is(err, applemusic.ErrUnauthorized)); the
// returned error joins every domain error.
func (s *Syncer) Sync(ctx context.Context, mode Mode, progress func(Progress), domains ...Domain) error {
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
	if mode == Incremental {
		due, err := s.fullDue(ctx, domains)
		if err != nil {
			return err
		}
		if due {
			mode = Full
		}
	}

	// Progress calls are serialised: domains of a stage run concurrently.
	var pmu sync.Mutex
	report := func(p Progress) {
		pmu.Lock()
		defer pmu.Unlock()
		progress(p)
	}
	var (
		mu      sync.Mutex
		errs    []error
		changed bool
		stop    bool
	)
	run := func(d Domain) {
		start := s.now()
		total, dchanged, err := s.syncDomain(ctx, d, mode, report)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			s.log.Warn("library sync failed", "domain", d, "err", err)
			if ferr := s.db.Q.FailSync(context.WithoutCancel(ctx), dbq.FailSyncParams{LastError: err.Error(), Domain: string(d)}); ferr != nil {
				s.log.Warn("library sync: record failure", "domain", d, "err", ferr)
			}
			report(Progress{Domain: d, Finished: true, Err: err})
			errs = append(errs, fmt.Errorf("%s: %w", d, err))
			stop = stop || ctx.Err() != nil || errors.Is(err, applemusic.ErrUnauthorized)
			return
		}
		changed = changed || dchanged
		s.log.Info("library synced", "domain", d, "full", mode == Full, "items", total, "changed", dchanged, "took", s.now().Sub(start).Round(time.Millisecond))
		report(Progress{Domain: d, Done: total, Total: total, Finished: true})
	}
	for _, stage := range stages(mode) {
		var wg sync.WaitGroup
		for _, d := range stage {
			if want[d] {
				wg.Go(func() { run(d) })
			}
		}
		wg.Wait()
		if stop {
			break
		}
	}
	if changed {
		if err := RebuildSearch(context.WithoutCancel(ctx), s.db); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// stages groups AllDomains into the steps Sync runs one after another; the
// domains of a step run concurrently. A full run takes one domain at a
// time (each pages in parallel already); an incremental run makes a few
// small requests per domain, so the independent ones overlap.
func stages(mode Mode) [][]Domain {
	if mode == Incremental {
		return [][]Domain{{DomainArtists, DomainAlbums, DomainSongs, DomainPlaylists}, {DomainPlaylistTracks}, {DomainRatings}}
	}
	out := make([][]Domain, len(AllDomains))
	for i, d := range AllDomains {
		out[i] = []Domain{d}
	}
	return out
}

// fullDue reports whether an incremental sync of domains must run fully: a
// domain never completed a full run, has an interrupted run, or last ran
// fully more than FullSyncEvery ago.
func (s *Syncer) fullDue(ctx context.Context, domains []Domain) (bool, error) {
	now := db.Millis(s.now())
	for _, d := range domains {
		st, err := s.db.Q.GetSyncState(ctx, string(d))
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return true, nil
		case err != nil:
			return false, err
		case st.InProgress != 0 || !st.LastFull.Valid || now-st.LastFull.Int64 > FullSyncEvery.Milliseconds():
			return true, nil
		}
	}
	return false, nil
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
// drops rows nothing references any more and marks d done (and fully
// synced when full).
func (s *Syncer) finish(ctx context.Context, d Domain, kind string, gen int64, total int, full bool) error {
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
		return s.finishSync(ctx, q, d, total, full)
	})
}

func (s *Syncer) finishSync(ctx context.Context, q *dbq.Queries, d Domain, total int, full bool) error {
	now := db.Millis(s.now())
	return q.FinishSync(ctx, dbq.FinishSyncParams{
		Total: int64(total), LastSuccess: sql.NullInt64{Int64: now, Valid: true},
		LastFull: sql.NullInt64{Int64: now, Valid: full}, Domain: string(d),
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

// syncDomain runs domain d and reports how many items it holds and
// whether the library's searchable items changed.
func (s *Syncer) syncDomain(ctx context.Context, d Domain, mode Mode, progress func(Progress)) (total int, changed bool, err error) {
	switch d {
	case DomainPlaylistTracks:
		total, err = s.syncPlaylistTracks(ctx, progress)
		return total, false, err
	case DomainRatings:
		total, err = s.syncRatings(ctx, mode, progress)
		return total, false, err
	case DomainArtists, DomainAlbums, DomainSongs:
		if mode == Incremental {
			total, changed, ok, err := s.syncNewest(ctx, d, progress)
			if err != nil || ok {
				return total, changed, err
			}
			s.log.Info("library sync: local count differs from Apple's, running fully", "domain", d)
		}
	case DomainPlaylists:
		if mode == Incremental {
			return s.syncPlaylists(ctx)
		}
	}
	return s.syncAll(ctx, d, mode, progress)
}

// syncNewest is the incremental run of artists, albums or songs: it stores
// the items added since the last run (newest first, up to the first item
// already stored; for artists, which Apple cannot list by date, none) and
// reports ok when the library then holds as many items as Apple counts.
// Otherwise something was removed or missed and the domain needs a full
// run.
func (s *Syncer) syncNewest(ctx context.Context, d Domain, progress func(Progress)) (total int, changed, ok bool, err error) {
	st, err := s.db.Q.GetSyncState(ctx, string(d))
	if err != nil {
		return 0, false, false, err
	}
	if err := s.db.Q.ResumeSync(ctx, dbq.ResumeSyncParams{LastStarted: sql.NullInt64{Int64: db.Millis(s.now()), Valid: true}, Domain: string(d)}); err != nil {
		return 0, false, false, err
	}
	var kind string
	var write func(q *dbq.Queries, now int64) error
	n := 0
	switch d {
	case DomainArtists:
		kind = "artist"
		p, err := s.api.LibraryArtistsPage(ctx, 1, 0)
		if err != nil {
			return 0, false, false, err
		}
		total = p.Meta.Total
	case DomainAlbums:
		kind = "album"
		var albums []applemusic.Album
		albums, total, ok, err = newest(ctx, s.api.LibraryAlbumsPage, func(a applemusic.Album) (bool, error) {
			return s.db.RQ.LibraryAlbumKnown(ctx, db.NullString(a.ID))
		})
		if err != nil || !ok {
			return 0, false, false, err
		}
		fresh, moved, err := s.refreshUpcomingAlbums(ctx, albums)
		if err != nil {
			return 0, false, false, err
		}
		n = len(albums) + moved
		write = func(q *dbq.Queries, now int64) error {
			for _, it := range albums {
				if err := storeLibraryAlbum(ctx, q, it, st.Generation, now); err != nil {
					return err
				}
			}
			for _, it := range fresh {
				if _, err := q.UpsertLibraryAlbum(ctx, libraryAlbumParams(it, now)); err != nil {
					return fmt.Errorf("album %s: %w", it.ID, err)
				}
			}
			return nil
		}
	case DomainSongs:
		kind = "song"
		var songs []applemusic.Song
		songs, total, ok, err = newest(ctx, s.api.LibrarySongsPage, func(t applemusic.Song) (bool, error) {
			return s.db.RQ.LibrarySongKnown(ctx, db.NullString(t.ID))
		})
		if err != nil || !ok {
			return 0, false, false, err
		}
		fresh, moved, err := s.refreshUpcomingSongs(ctx, songs)
		if err != nil {
			return 0, false, false, err
		}
		n = len(songs) + moved
		write = func(q *dbq.Queries, now int64) error {
			// Newer than every stored song: ranks below the lowest one.
			low, err := q.MinSongAddedRank(ctx)
			if err != nil {
				return err
			}
			for i, it := range songs {
				if err := storeLibrarySong(ctx, q, it, low-int64(len(songs)-i), st.Generation, now); err != nil {
					return err
				}
			}
			for _, it := range fresh {
				if _, err := q.UpsertLibraryTrack(ctx, libraryTrackParams(it, now)); err != nil {
					return fmt.Errorf("song %s: %w", it.ID, err)
				}
			}
			return nil
		}
	}
	err = s.db.Tx(ctx, func(q *dbq.Queries) error {
		if write != nil {
			if err := write(q, db.Millis(s.now())); err != nil {
				return err
			}
		}
		have, err := q.CountLibraryKind(ctx, kind)
		if err != nil {
			return err
		}
		if ok = int(have) == total; !ok {
			return nil // the full run follows
		}
		return s.finishSync(ctx, q, d, total, false)
	})
	if err != nil {
		return 0, false, false, err
	}
	if n > 0 {
		progress(Progress{Domain: d, Done: n, Total: total})
	}
	return total, n > 0, ok, nil
}

// refreshUpcomingAlbums asks Apple again for the stored albums still
// ahead of their release (see UpcomingLibraryAlbums), except the ones just
// fetched in skip: the newest-first walk stops at the first album it
// has, so without this a pre-added album would keep its release state for
// good, and the flag would never clear once the album is out. It returns
// them for storing and how many of them differ from what is stored.
func (s *Syncer) refreshUpcomingAlbums(ctx context.Context, skip []applemusic.Album) (fresh []applemusic.Album, changed int, err error) {
	rows, err := s.db.RQ.UpcomingLibraryAlbums(ctx, s.now().Format(time.DateOnly))
	if err != nil {
		return nil, 0, err
	}
	stored := make(map[string]dbq.UpcomingLibraryAlbumsRow, len(rows))
	for _, r := range rows {
		stored[r.LibraryID.String] = r
	}
	for _, a := range skip {
		delete(stored, a.ID)
	}
	ids := slices.Sorted(maps.Keys(stored))
	for batch := range slices.Chunk(ids, 100) {
		albums, err := s.api.LibraryAlbumsByID(ctx, batch)
		if err != nil {
			return nil, 0, fmt.Errorf("library sync: refresh upcoming albums: %w", err)
		}
		for _, a := range albums {
			p := libraryAlbumParams(a, 0)
			if r := stored[a.ID]; r.IsPrerelease != p.IsPrerelease || r.ReleaseDate != p.ReleaseDate || r.CatalogReleaseDate != p.CatalogReleaseDate {
				changed++
			}
		}
		fresh = append(fresh, albums...)
	}
	return fresh, changed, nil
}

// refreshUpcomingSongs is refreshUpcomingAlbums for the songs of those
// albums that Apple gave no playParams yet: a song comes out before its
// album, and the song walk stops at the first song it has too. It returns
// the songs for storing and how many gained playParams.
func (s *Syncer) refreshUpcomingSongs(ctx context.Context, skip []applemusic.Song) (fresh []applemusic.Song, changed int, err error) {
	rows, err := s.db.RQ.UnplayableUpcomingLibrarySongs(ctx, s.now().Format(time.DateOnly))
	if err != nil {
		return nil, 0, err
	}
	skipped := make(map[string]bool, len(skip))
	for _, t := range skip {
		skipped[t.ID] = true
	}
	var ids []string
	for _, r := range rows {
		if !skipped[r.String] {
			ids = append(ids, r.String)
		}
	}
	for batch := range slices.Chunk(ids, 100) {
		songs, err := s.api.LibrarySongsByID(ctx, batch)
		if err != nil {
			return nil, 0, fmt.Errorf("library sync: refresh upcoming songs: %w", err)
		}
		for _, t := range songs {
			if t.Attributes.PlayParams != nil {
				changed++
			}
		}
		fresh = append(fresh, songs...)
	}
	return fresh, changed, nil
}

// newest walks a newest-first library collection from the top until an
// item known reports stored, returning the items before it (newest first)
// and Apple's total. The first page is small: usually nothing is new. ok
// is false when more than incrementalMax items are new.
func newest[T any](ctx context.Context, page func(ctx context.Context, limit, offset int) (applemusic.Page[T], error), known func(T) (bool, error)) (items []T, total int, ok bool, err error) {
	limit := 10
	for off := 0; ; off += limit {
		if off > 0 {
			limit = 100
		}
		p, err := page(ctx, limit, off)
		if err != nil {
			return nil, 0, false, err
		}
		total = p.Meta.Total
		for _, it := range p.Data {
			k, err := known(it)
			if err != nil {
				return nil, 0, false, err
			}
			if k {
				return items, total, true, nil
			}
			items = append(items, it)
		}
		if len(items) > incrementalMax {
			return nil, total, false, nil
		}
		if len(p.Data) < limit || off+limit >= total {
			return items, total, true, nil
		}
	}
}

func storeLibrarySong(ctx context.Context, q *dbq.Queries, it applemusic.Song, rank, gen, now int64) error {
	id, err := q.UpsertLibraryTrack(ctx, libraryTrackParams(it, now))
	if err != nil {
		return fmt.Errorf("song %s: %w", it.ID, err)
	}
	return q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "song", Ref: id, AddedRank: sql.NullInt64{Int64: rank, Valid: true}, Generation: gen})
}

// storeLibraryPlaylist stores playlist it as a library item of run gen.
// id is its unchanged stored row, if any: then only the membership is
// refreshed.
func storeLibraryPlaylist(ctx context.Context, q *dbq.Queries, it applemusic.Playlist, id, gen, now int64) error {
	a := it.Attributes
	if id == 0 {
		var err error
		id, err = q.UpsertLibraryPlaylist(ctx, dbq.UpsertLibraryPlaylistParams{
			LibraryID: db.NullString(it.ID), CatalogID: db.NullString(it.CatalogID()),
			Name: a.Name, Description: a.Description.Text(), CuratorName: a.CuratorName,
			ArtworkTemplate: a.Artwork.Template(), CanEdit: db.Bool(a.CanEdit), IsPublic: db.Bool(a.IsPublic),
			LastModified: a.LastModifiedDate, UpdatedAt: now,
		})
		if err != nil {
			return fmt.Errorf("playlist %s: %w", it.ID, err)
		}
	}
	return q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "playlist", Ref: id, DateAdded: parseDate(a.DateAdded), Generation: gen})
}

// IsSingleRelease reports whether library album it is a single or an EP
// (Apple's "Singles & EPs"). A library album has no isSingle of its own, so
// the rule reads, in order: the isSingle of its catalog album (fetched with
// the library walk), then the name Apple gives singles and EPs, which ends
// in " - Single" or " - EP". The suffix is English only, so for other
// storefront languages the catalog flag decides; an album with no catalog
// album and no suffix is a standard album. The track count is not used: a
// library album's trackCount is the number of its tracks in the library.
func IsSingleRelease(it applemusic.Album) bool {
	if it.Attributes.IsSingle {
		return true
	}
	if c := it.Relationships.Catalog.Data; len(c) > 0 && c[0].Attributes.IsSingle {
		return true
	}
	n := it.Attributes.Name
	return strings.HasSuffix(n, " - Single") || strings.HasSuffix(n, " - EP")
}

func libraryAlbumParams(it applemusic.Album, now int64) dbq.UpsertLibraryAlbumParams {
	a := it.Attributes
	// A library album has no isPrerelease and, pre-added, no releaseDate:
	// the catalog album, included in the walk, has both.
	var cat applemusic.AlbumAttributes
	if c := it.Relationships.Catalog.Data; len(c) > 0 {
		cat = c[0].Attributes
	}
	return dbq.UpsertLibraryAlbumParams{
		LibraryID: db.NullString(it.ID), CatalogID: db.NullString(it.CatalogID()),
		Name: a.Name, ArtistName: a.ArtistName,
		ArtistLibraryID: db.NullString(firstID(it.Relationships.Artists.Data, func(r applemusic.Artist) string { return r.ID })),
		ArtworkTemplate: a.Artwork.Template(), ReleaseDate: a.ReleaseDate, Genre: firstString(a.GenreNames),
		TrackCount: int64(a.TrackCount), ContentRating: a.ContentRating, IsSingle: db.Bool(IsSingleRelease(it)),
		IsPrerelease: db.Bool(cat.IsPrerelease), CatalogReleaseDate: cat.ReleaseDate, UpdatedAt: now,
	}
}

func storeLibraryAlbum(ctx context.Context, q *dbq.Queries, it applemusic.Album, gen, now int64) error {
	id, err := q.UpsertLibraryAlbum(ctx, libraryAlbumParams(it, now))
	if err != nil {
		return fmt.Errorf("album %s: %w", it.ID, err)
	}
	return q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: "album", Ref: id, DateAdded: parseDate(it.Attributes.DateAdded), Generation: gen})
}

// syncAll walks every item of domain d in parallel pages with a resumable
// checkpoint and removes what the walk did not see.
func (s *Syncer) syncAll(ctx context.Context, d Domain, mode Mode, progress func(Progress)) (int, bool, error) {
	gen, start, err := s.begin(ctx, d)
	if err != nil {
		return 0, false, err
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
					if err := storeLibrarySong(ctx, q, it, int64(offset+i), gen, now); err != nil {
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
					if err := storeLibraryAlbum(ctx, q, it, gen, now); err != nil {
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
					if err := storeLibraryPlaylist(ctx, q, it, 0, gen, now); err != nil {
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
		return 0, false, fmt.Errorf("library: unknown domain %q", d)
	}
	if err != nil {
		return 0, false, err
	}
	return total, true, s.finish(ctx, d, kind, gen, total, mode == Full)
}

// syncPlaylists is the incremental run of the playlists domain. Apple
// cannot list playlists by modification date, but users have few, so the
// whole list is read (100 per request) and only playlists whose
// lastModifiedDate moved are rewritten. Removals are seen at once; the
// folder tree is refreshed only when something changed.
func (s *Syncer) syncPlaylists(ctx context.Context) (int, bool, error) {
	d := DomainPlaylists
	rows, err := s.db.RQ.LibraryPlaylistsForTrackSync(ctx)
	if err != nil {
		return 0, false, err
	}
	stored := make(map[string]dbq.LibraryPlaylistsForTrackSyncRow, len(rows))
	for _, r := range rows {
		stored[r.LibraryID.String] = r
	}
	var items []applemusic.Playlist
	total := 0
	for off := 0; off == 0 || off < total; off += 100 {
		p, err := s.api.LibraryPlaylistsPage(ctx, 100, off)
		if err != nil {
			return 0, false, err
		}
		total = p.Meta.Total
		items = append(items, p.Data...)
		if len(p.Data) == 0 {
			break
		}
	}
	gen, _, err := s.begin(ctx, d)
	if err != nil {
		return 0, false, err
	}
	seen, changed := 0, false
	err = s.db.Tx(ctx, func(q *dbq.Queries) error {
		now := db.Millis(s.now())
		for _, it := range items {
			if it.Deleted() {
				continue
			}
			var id int64
			if old, ok := stored[it.ID]; ok {
				seen++
				if old.LastModified != "" && old.LastModified == it.Attributes.LastModifiedDate {
					id = old.ID
				}
			}
			if id == 0 {
				changed = true
			}
			if err := storeLibraryPlaylist(ctx, q, it, id, gen, now); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, false, err
	}
	if seen < len(stored) {
		changed = true // some were removed
	}
	if changed {
		if err := s.syncPlaylistTree(ctx, gen); err != nil {
			return 0, false, err
		}
	}
	return total, changed, s.finish(ctx, d, "playlist", gen, total, false)
}

// syncPlaylistTracks refreshes the tracks of every library playlist whose
// lastModifiedDate changed since its tracks were last stored. Playlists are
// fetched in parallel; each playlist's tracks are replaced atomically, which
// makes the per-playlist modification stamp the resume checkpoint. Every
// run is complete, so it counts as full.
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
	return total, s.finish(ctx, d, "", 0, total, true)
}

// ratingKinds are the library resource types whose ratings are mirrored,
// with the stored IDs updated after a time.
var ratingKinds = []struct {
	typ string
	ids func(*dbq.Queries, context.Context, int64) ([]sql.NullString, error)
}{
	{applemusic.TypeLibrarySongs, (*dbq.Queries).LibrarySongIDsSince},
	{applemusic.TypeLibraryAlbums, (*dbq.Queries).LibraryAlbumIDsSince},
	{applemusic.TypeLibraryPlaylists, (*dbq.Queries).LibraryPlaylistIDsSince},
}

// syncRatings mirrors the ratings of library songs, albums and playlists
// into favorites. A full run checks every item and deletes ratings Apple no
// longer reports; an incremental run checks only items stored since the
// previous ratings run started (ratings made in Sona are mirrored when they
// are made).
func (s *Syncer) syncRatings(ctx context.Context, mode Mode, progress func(Progress)) (int, error) {
	d := DomainRatings
	var since int64
	if mode == Incremental {
		prev, err := s.db.Q.GetSyncState(ctx, string(d))
		if err != nil {
			return 0, err
		}
		since = prev.LastStarted.Int64
	}
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
		rows, err := k.ids(s.db.RQ, ctx, since)
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
		if len(k.ids) == 0 && mode == Incremental {
			continue
		}
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
			if mode == Incremental {
				return nil
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
	s.log.Debug("library ratings", "checked", total, "rated", rated)
	return total, s.finish(ctx, d, "", 0, total, mode == Full)
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
		Isrc: a.ISRC, Composer: a.ComposerName, NoPlayParams: db.Bool(a.PlayParams == nil), UpdatedAt: now,
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
