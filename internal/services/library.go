package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/library"
)

// SyncStaleAfter is how old the last complete library sync may be before
// startup (after first paint) syncs again.
const SyncStaleAfter = 6 * time.Hour

// LibraryService serves the user's library from SQLite and runs the sync.
type LibraryService struct {
	o *Env
	m mapper

	mu         sync.Mutex
	syncCancel context.CancelFunc
	syncDone   chan struct{}
	resync     []library.Domain // domains to sync again after the running sync
	closed     bool

	setClipboard func(string) bool // CopyLink; the Wails clipboard
	// removeRecheck is how long after a removal Remove checks that Apple
	// really dropped the item (see confirmRemoved).
	removeRecheck time.Duration
}

// newLibraryService also schedules the library sync: after first paint
// and on sign-in when stale, cancelled before the sign-out wipe.
func newLibraryService(e *Env) *LibraryService {
	s := &LibraryService{o: e, m: e.m, setClipboard: clipboardText, removeRecheck: 10 * time.Second}
	e.OnNetwork(func() {
		go s.syncIfStale(context.Background())
		if e.History != nil {
			go e.History.trim(context.Background())
		}
	})
	if e.History != nil && e.Coord != nil {
		e.Coord.ObserveActivity(e.History.observe)
	}
	prev := e.Auth.Status().State
	e.Auth.OnChange(func(st auth.Status) {
		was := prev
		prev = st.State
		if st.State == auth.StateSignedIn && was != auth.StateSignedIn {
			go e.OnNetwork(func() { s.syncIfStale(context.Background()) })
		}
	})
	e.OnSignOut("library sync", func(context.Context) error { s.stopSync(); return nil })
	e.OnClose(s.close)
	return s
}

func listOpts(sort string, desc bool, offset, limit int) library.ListOptions {
	return library.ListOptions{Sort: library.Sort(sort), Desc: desc, Offset: offset, Limit: limit}
}

// Songs lists library songs. sort is "title", "artist", "album" or
// "dateAdded"; limit 0 means 100 (max 1000).
func (s *LibraryService) Songs(ctx context.Context, sort string, desc bool, offset, limit int) (SongPage, error) {
	songs, err := s.o.Library.Songs(ctx, listOpts(sort, desc, offset, limit))
	if err != nil {
		return SongPage{}, err
	}
	c, err := s.o.Library.Counts(ctx)
	if err != nil {
		return SongPage{}, err
	}
	return SongPage{Items: s.m.libSongs(songs), Total: c.Songs}, nil
}

// Albums lists library albums. sort is "title", "artist", "dateAdded" or
// "releaseDate".
func (s *LibraryService) Albums(ctx context.Context, sort string, desc bool, offset, limit int) (AlbumPage, error) {
	albums, err := s.o.Library.Albums(ctx, listOpts(sort, desc, offset, limit))
	if err != nil {
		return AlbumPage{}, err
	}
	c, err := s.o.Library.Counts(ctx)
	if err != nil {
		return AlbumPage{}, err
	}
	return AlbumPage{Items: s.m.libAlbums(albums), Total: c.Albums}, nil
}

// Artists lists library artists by name (sort "title").
func (s *LibraryService) Artists(ctx context.Context, sort string, desc bool, offset, limit int) (ArtistPage, error) {
	artists, err := s.o.Library.Artists(ctx, listOpts(sort, desc, offset, limit))
	if err != nil {
		return ArtistPage{}, err
	}
	c, err := s.o.Library.Counts(ctx)
	if err != nil {
		return ArtistPage{}, err
	}
	out := ArtistPage{Items: make([]Artist, len(artists)), Total: c.Artists}
	for i, a := range artists {
		out.Items[i] = s.m.libArtist(a)
	}
	return out, nil
}

// Playlists lists library playlists. sort is "title", "dateAdded" or
// "lastModified".
func (s *LibraryService) Playlists(ctx context.Context, sort string, desc bool, offset, limit int) (PlaylistPage, error) {
	pls, err := s.o.Library.Playlists(ctx, listOpts(sort, desc, offset, limit))
	if err != nil {
		return PlaylistPage{}, err
	}
	c, err := s.o.Library.Counts(ctx)
	if err != nil {
		return PlaylistPage{}, err
	}
	out := PlaylistPage{Items: make([]Playlist, len(pls)), Total: c.Playlists}
	for i, p := range pls {
		out.Items[i] = s.m.libPlaylist(p)
	}
	return out, nil
}

// Album returns a library album and its tracks.
func (s *LibraryService) Album(ctx context.Context, id string) (AlbumDetail, error) {
	a, songs, err := s.o.Library.Album(ctx, id)
	if err != nil {
		return AlbumDetail{}, err
	}
	tracks := s.m.libSongs(songs)
	album := s.m.libAlbum(a)
	if album.Prerelease {
		markUnreleased(tracks, s.m.clock())
	}
	return AlbumDetail{
		Album:      album,
		Tracks:     tracks,
		DurationMs: totalMs(tracks),
		InLibrary:  true,
		Header:     s.m.header(ctx, a.ArtworkTemplate),
	}, nil
}

// Artist returns a library artist with its library albums and songs.
func (s *LibraryService) Artist(ctx context.Context, id string) (ArtistDetail, error) {
	a, albums, songs, err := s.o.Library.Artist(ctx, id)
	if err != nil {
		return ArtistDetail{}, err
	}
	tmpl := a.ArtworkTemplate
	if tmpl == "" && len(albums) > 0 {
		tmpl = albums[0].ArtworkTemplate
	}
	return ArtistDetail{
		Artist:   s.m.libArtist(a),
		TopSongs: []Song{},
		Albums:   s.m.libAlbums(albums),
		Songs:    s.m.libSongs(songs),
		Header:   s.m.header(ctx, tmpl),
	}, nil
}

// Counts returns the library list sizes.
func (s *LibraryService) Counts(ctx context.Context) (Counts, error) {
	c, err := s.o.Library.Counts(ctx)
	return Counts(c), err
}

// RecentlyAdded lists library albums and playlists by when they were
// added, newest first (from the synced library).
func (s *LibraryService) RecentlyAdded(ctx context.Context, offset, limit int) (LibraryEntryPage, error) {
	added, total, err := s.o.Library.RecentlyAdded(ctx, library.ListOptions{Offset: offset, Limit: limit})
	if err != nil {
		return LibraryEntryPage{}, err
	}
	out := LibraryEntryPage{Items: make([]LibraryEntry, len(added)), Total: total}
	for i, a := range added {
		switch {
		case a.Album != nil:
			al := s.m.libAlbum(*a.Album)
			out.Items[i] = LibraryEntry{Kind: kindAlbum, Album: &al}
		case a.Playlist != nil:
			pl := s.m.libPlaylist(*a.Playlist)
			out.Items[i] = LibraryEntry{Kind: kindPlaylist, Playlist: &pl}
		}
	}
	return out, nil
}

// FavoriteSongs lists the library songs rated Favourite, most recently
// favourited first (from the synced library and ratings).
func (s *LibraryService) FavoriteSongs(ctx context.Context, offset, limit int) (SongPage, error) {
	songs, total, err := s.o.Library.FavoriteSongs(ctx, library.ListOptions{Offset: offset, Limit: limit})
	if err != nil {
		return SongPage{}, err
	}
	return SongPage{Items: s.m.libSongs(songs), Total: total}, nil
}

// MadeForYou returns the library copies of the account's personal mixes
// from Apple (needs sign-in and the network).
func (s *LibraryService) MadeForYou(ctx context.Context) ([]Playlist, error) {
	pls, err := s.o.API.MadeForYou(ctx)
	if err != nil {
		return nil, s.o.Fail(err)
	}
	return s.m.amPlaylists(pls), nil
}

// MusicVideos lists the library's music videos from Apple (up to 100 per
// page; needs sign-in and the network).
func (s *LibraryService) MusicVideos(ctx context.Context, offset, limit int) (MusicVideoPage, error) {
	page, err := s.o.API.LibraryMusicVideos(ctx, limit, offset)
	if err != nil {
		return MusicVideoPage{}, s.o.Fail(err)
	}
	return MusicVideoPage{Items: s.m.amMusicVideos(page.Data), Total: page.Meta.Total}, nil
}

// RecentlyPlayed returns the account's recently played tracks from Apple
// (up to 30 per page; needs sign-in and the network).
func (s *LibraryService) RecentlyPlayed(ctx context.Context, limit, offset int) ([]Song, error) {
	r, err := s.o.API.RecentlyPlayed(ctx, limit, offset)
	if err != nil {
		s.o.Report(err)
		return nil, err
	}
	return s.m.amSongs(r.Data), nil
}

// recentPages is how many pages of Apple's recently played tracks History
// reads (Apple keeps about 180 songs, 30 per page).
const recentPages = 7

// History returns the listening history of the last days days (1–365;
// 0 means 30): the plays recorded on this computer (and the one in
// progress, once Apple reports it) merged with Apple's recently played
// songs from the account's other devices, grouped by local day, newest
// first (see library.MergeHistory). Apple's list is left out when signed
// out or unreachable (Apple is false then).
func (s *LibraryService) History(ctx context.Context, days int) (ListeningHistory, error) {
	if days <= 0 {
		days = 30
	}
	days = min(days, 365)
	now := s.o.Now()
	since := time.Date(now.Year(), now.Month(), now.Day()-days+1, 0, 0, 0, 0, now.Location())
	plays, last, more, err := s.o.Library.History(ctx, since)
	if err != nil {
		return ListeningHistory{}, err
	}
	var apple []applemusic.Song
	out := ListeningHistory{Since: since.UnixMilli(), More: more}
	if s.o.Auth.Status().State == auth.StateSignedIn {
		if apple, err = s.recentlyPlayedAll(ctx); err != nil {
			s.o.Report(err)
			s.o.Logger.Warn("library: history without Apple's recently played", "err", err)
		} else {
			out.Apple = true
		}
	}
	var started []library.Play
	if s.o.History != nil {
		started = s.o.History.started(ctx)
	}
	merged := library.MergeHistory(plays, started, last, apple, since, now.Location())
	out.Days = make([]HistoryDay, len(merged))
	for i, d := range merged {
		day := HistoryDay{Entries: make([]HistoryEntry, len(d.Entries))}
		if !d.Day.IsZero() {
			day.Day = d.Day.UnixMilli()
		}
		for j, e := range d.Entries {
			if e.Apple != nil {
				day.Entries[j] = HistoryEntry{Song: s.m.amSong(*e.Apple)}
				continue
			}
			p := e.Play
			song := Song{
				ID: p.CatalogID, Source: SourceCatalog, CatalogID: p.CatalogID, LibraryID: p.LibraryID,
				Name: p.Name, Artist: p.Artist, Album: p.Album, DurationMs: p.Duration.Milliseconds(),
				Playable: p.CatalogID != "" || p.LibraryID != "", Art: s.m.artOf(p.ArtworkTemplate),
			}
			if p.LibraryID != "" {
				song.ID, song.Source = p.LibraryID, SourceLibrary
			}
			day.Entries[j] = HistoryEntry{PlayedAt: p.PlayedAt.UnixMilli(), Song: song, PlayedMs: p.Played.Milliseconds(), SourceName: p.Context}
		}
		out.Days[i] = day
	}
	return out, nil
}

// recentlyPlayedAll reads Apple's whole recently played list, its pages
// in parallel.
func (s *LibraryService) recentlyPlayedAll(ctx context.Context) ([]applemusic.Song, error) {
	pages := make([][]applemusic.Song, recentPages)
	g, gctx := errgroup.WithContext(ctx)
	for i := range pages {
		g.Go(func() error {
			r, err := s.o.API.RecentlyPlayed(gctx, 30, i*30)
			pages[i] = r.Data
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	var out []applemusic.Song
	for _, p := range pages {
		if len(p) == 0 {
			break
		}
		out = append(out, p...)
	}
	return out, nil
}

// addDomains are the library domains an add changes, by catalog type
// (music videos are not stored locally).
var addDomains = map[string][]library.Domain{
	applemusic.TypeSongs:       {library.DomainAlbums, library.DomainSongs, library.DomainRatings},
	applemusic.TypeAlbums:      {library.DomainAlbums, library.DomainSongs, library.DomainRatings},
	applemusic.TypePlaylists:   {library.DomainPlaylists, library.DomainPlaylistTracks},
	applemusic.TypeMusicVideos: {},
}

// Add adds catalog items of type typ ("songs", "albums", "playlists" or
// "music-videos") to the library. Catalog details and States report them
// in the library (with the library IDs Apple gave them) at once; the
// affected library domains then sync in the background (after the running
// sync, if one is).
func (s *LibraryService) Add(ctx context.Context, typ string, ids []string) error {
	domains, ok := addDomains[typ]
	if !ok || len(ids) == 0 {
		return fmt.Errorf("%w: add %q × %d", ErrInvalidArgument, typ, len(ids))
	}
	libIDs, err := s.o.API.AddToLibrary(ctx, map[string][]string{typ: ids})
	if err != nil {
		return s.o.Fail(err)
	}
	s.o.members.markAdded(typ, ids)
	for _, id := range ids {
		k := applemusic.ItemKey{Type: typ, ID: id}
		st, _ := s.o.members.state(k)
		st.InLibrary, st.LibraryID = true, libIDs[k]
		s.o.members.hold(k, st)
	}
	if len(domains) > 0 {
		s.syncSoon(domains...)
	}
	return nil
}

// syncSoon syncs domains now, or after the running sync.
func (s *LibraryService) syncSoon(domains ...library.Domain) {
	switch err := s.startSync(library.Incremental, domains...); {
	case errors.Is(err, library.ErrSyncRunning):
		s.mu.Lock()
		s.resync = append(s.resync, domains...)
		s.mu.Unlock()
	case err != nil:
		s.o.Logger.Warn("library: sync after change", "domains", domains, "err", err)
	}
}

// Sync starts a full library sync in the background (the on-demand one;
// startup and changes made here sync incrementally, with a full run every
// library.FullSyncEvery); progress arrives as "sync:progress" events and
// the outcome as "sync:done". It fails with sync_running while a sync runs
// and not_signed_in when signed out.
func (s *LibraryService) Sync() error { return s.startSync(library.Full) }

// SyncStatus returns the stored sync state.
func (s *LibraryService) SyncStatus(ctx context.Context) (SyncStatus, error) {
	ds, err := s.o.Library.SyncStatus(ctx)
	if err != nil {
		return SyncStatus{}, err
	}
	return syncStatus(ds, s.o.Syncer.Running()), nil
}

func syncStatus(ds []library.DomainStatus, running bool) SyncStatus {
	out := SyncStatus{Running: running, Domains: make([]DomainSync, len(ds))}
	have := map[library.Domain]time.Time{}
	for i, d := range ds {
		out.Domains[i] = DomainSync{
			Domain: string(d.Domain), InProgress: d.InProgress, Done: d.Done, Total: d.Total,
			LastStarted: unixMs(d.LastStarted), LastSuccess: unixMs(d.LastSuccess), Failed: d.LastError != "",
		}
		if !d.LastSuccess.IsZero() {
			have[d.Domain] = d.LastSuccess
		}
	}
	var oldest time.Time
	for _, d := range library.AllDomains {
		t, ok := have[d]
		if !ok {
			return out
		}
		if oldest.IsZero() || t.Before(oldest) {
			oldest = t
		}
	}
	out.LastSuccess = unixMs(oldest)
	return out
}

// needsSync reports whether the stored state calls for a sync: a domain
// never completed, an interrupted run waits to resume, or the oldest
// success is older than SyncStaleAfter.
func needsSync(st SyncStatus, now time.Time) bool {
	if st.LastSuccess == 0 {
		return true
	}
	for _, d := range st.Domains {
		if d.InProgress {
			return true
		}
	}
	return now.Sub(time.UnixMilli(st.LastSuccess)) > SyncStaleAfter
}

// syncIfStale starts a sync when signed in and the library is stale
// (see needsSync). It is called after first paint and on sign-in.
func (s *LibraryService) syncIfStale(ctx context.Context) {
	if s.o.Auth.Status().State != auth.StateSignedIn || s.o.Syncer.Running() {
		return
	}
	st, err := s.SyncStatus(ctx)
	if err != nil {
		s.o.Logger.Warn("library: read sync state", "err", err)
		return
	}
	if !needsSync(st, s.o.Now()) {
		s.o.Logger.Info("library: sync not due", "last_success", time.UnixMilli(st.LastSuccess))
		return
	}
	if err := s.startSync(library.Incremental); err != nil && !errors.Is(err, library.ErrSyncRunning) {
		s.o.Logger.Warn("library: start sync", "err", err)
	}
}

func (s *LibraryService) startSync(mode library.Mode, domains ...library.Domain) error {
	if s.o.Auth.Status().State != auth.StateSignedIn {
		return auth.ErrNotSignedIn
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return context.Canceled
	}
	if s.syncDone != nil {
		s.mu.Unlock()
		return library.ErrSyncRunning
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.syncCancel, s.syncDone = cancel, done
	s.mu.Unlock()

	s.o.Go("library sync", func() {
		defer close(done)
		defer cancel()
		start := s.o.Now()
		s.o.Logger.Info("library: sync started", "full", mode == library.Full, "domains", domains)
		err := s.o.Syncer.Sync(ctx, mode, func(p library.Progress) {
			ev := SyncProgress{Domain: string(p.Domain), Done: p.Done, Total: p.Total, Finished: p.Finished}
			ev.ErrorCode = CodeOf(p.Err)
			s.o.Emit.Emit(EventSyncProgress, ev)
		}, domains...)
		s.mu.Lock()
		s.syncCancel, s.syncDone = nil, nil
		again := s.resync
		s.resync = nil
		s.mu.Unlock()
		if err != nil {
			s.o.Report(err)
		} else {
			s.o.members.synced(start)
		}
		c, cerr := s.o.Library.Counts(context.Background())
		if cerr != nil {
			s.o.Logger.Warn("library: counts", "err", cerr)
		}
		ev := SyncDone{OK: err == nil, Counts: Counts(c), FinishedAt: unixMs(s.o.Now())}
		ev.ErrorCode = CodeOf(err)
		s.o.Logger.Info("library: sync finished", "ok", err == nil, "took", s.o.Now().Sub(start).Round(time.Millisecond), "counts", c)
		s.o.Emit.Emit(EventSyncDone, ev)
		if len(again) > 0 && ctx.Err() == nil {
			if err := s.startSync(library.Incremental, again...); err != nil {
				s.o.Logger.Warn("library: sync after add", "err", err)
			}
		}
	})
	return nil
}

// stopSync cancels a running sync and waits for it to end (sign-out runs
// it before wiping the library, shutdown before closing the database).
func (s *LibraryService) stopSync() {
	s.mu.Lock()
	cancel, done := s.syncCancel, s.syncDone
	s.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

// close stops any sync and refuses new ones.
func (s *LibraryService) close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.stopSync()
}
