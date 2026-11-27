package services

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
)

// Artist tracker timing and sizes.
const (
	// trackerEvery is how often a complete check runs; the last one's time is
	// kept, so a restart doesn't check again within it.
	trackerEvery = 24 * time.Hour
	// trackerStartDelay is how long after the first paint the loop waits
	// before its first look, so a check never competes with the page that is
	// loading.
	trackerStartDelay = 30 * time.Second
	// trackerRetryMin and trackerRetryMax bound the wait before a failed
	// check is tried again; it doubles after each failure.
	trackerRetryMin = 15 * time.Minute
	trackerRetryMax = 6 * time.Hour
	// trackerNoAccount is how long the loop waits before looking for an
	// account again while signed out (no request is made).
	trackerNoAccount = 15 * time.Minute
	// trackerBatch is how many artists one catalogue request asks about, and
	// trackerPause the wait between two requests of a check, so a long list
	// of favourites never bursts.
	trackerBatch = 25
	trackerPause = 2 * time.Second
	// trackerFresh is how far back a release found for the first time still
	// counts as new: an older one (an album Apple filed late, a back catalogue
	// that grew) is recorded as seen and never shown. trackerKeep is how long
	// a record is kept.
	trackerFresh = 90 * 24 * time.Hour
	trackerKeep  = 180 * 24 * time.Hour
	// trackerFullAlbums and trackerSingles are how many of an artist's newest
	// albums and singles and EPs a check reads next to its latest release.
	trackerFullAlbums = 3
	trackerSingles    = 5
)

// Go's own text in the tracker's desktop notifications, in the user's
// language (Deps.Language).
const (
	msgTrackAlbum   l10n.Message = "New album by %s"                 // a desktop notification: the body; %s is the artist
	msgTrackEP      l10n.Message = "New EP by %s"                    // a desktop notification: the body; %s is the artist
	msgTrackSingle  l10n.Message = "New single by %s"                // a desktop notification: the body; %s is the artist
	msgTrackSeveral l10n.Message = "New releases"                    // a desktop notification: the title when several releases arrived at once
	msgTrackCount   l10n.Message = "From your favourite artists: %d" // a desktop notification: the body when several arrived at once; %d is 2 or more
	msgTrackOpen    l10n.Message = "Show new releases"               // a desktop notification: its click, which opens the list
)

// ArtistTrackerService is the opt-in artist tracker (config.ArtistTracker,
// off until Settings → General turns it on): Sona watches the artists the
// user has favourited and lists, on For You (a shelf, and the page behind its
// "See all"), the releases that come out afterwards. It uses the release
// reminders' pieces: an upcoming release carries the album "Remind me"
// action, and the optional desktop notification is shown by the same
// notifier, a click opening the release (or
// the list, for a summary).
//
// There is no Apple call that lists favourite artists (/v1/me/ratings/artists
// answers only for IDs it is given), so a check takes the artists of the
// synced library that have a catalogue ID and the artists favourited in Sona
// as candidates and asks Apple which of them are rated 1. It then reads their
// releases from the catalogue, batched (one request per trackerBatch artists
// with trackerPause between): the latest release, the newest albums and the
// newest singles and EPs, newest first. Nothing is read from the client's
// play reporting, so a check is the same in private listening.
//
// An artist's first check records what is out as seen and announces nothing,
// so turning the tracker on, or favouriting an artist later, never floods the
// list with the back catalogue; releases still to come are listed (marked
// read) so Remind me is there. Every later release is found once, however
// many checks or restarts follow (table artist_releases). The loop starts
// after the first paint (Env.OnNetwork), waits trackerStartDelay, checks
// about once a day and when the setting is turned on, retries a failed check
// with growing waits, and makes no request while the setting is off or the
// user is signed out. Its state is user data, cleared on sign-out.
type ArtistTrackerService struct {
	e       *Env
	notices clickNotices
	wake    chan struct{}
	quit    chan struct{}
	done    chan struct{}
	ctx     context.Context
	cancel  context.CancelFunc

	// startDelay and pause are trackerStartDelay and trackerPause; tests
	// shorten them.
	startDelay, pause time.Duration

	mu      sync.Mutex
	started bool

	// reseed is set when the setting is turned on: the next check forgets
	// which artists were seeded, so what came out while it was off is seen,
	// not announced.
	reseed atomic.Bool
	// epoch changes at sign-out, under writeMu, so a check that began for the
	// account that left writes nothing.
	epoch   atomic.Int64
	writeMu sync.Mutex
	// failures is how many checks in a row failed (the loop's goroutine
	// only).
	failures int
}

func newArtistTrackerService(e *Env) *ArtistTrackerService {
	ctx, cancel := context.WithCancel(context.Background())
	s := &ArtistTrackerService{
		e: e, notices: clickNotices{e: e}, ctx: ctx, cancel: cancel,
		wake: make(chan struct{}, 1), quit: make(chan struct{}), done: make(chan struct{}),
		startDelay: trackerStartDelay, pause: trackerPause,
	}
	e.Settings.OnChange(func(prev, next config.Settings) {
		if prev.ArtistTracker == next.ArtistTracker {
			return
		}
		if next.ArtistTracker.Enabled && !prev.ArtistTracker.Enabled {
			s.reseed.Store(true)
		}
		s.poke()
	})
	e.OnNetwork(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		select {
		case <-s.quit:
			return
		default:
		}
		s.started = true
		e.Go("artist tracker", s.run)
	})
	e.OnSignOut("artist tracker", func(ctx context.Context) error {
		s.writeMu.Lock()
		s.epoch.Add(1)
		err := s.clear(ctx)
		s.writeMu.Unlock()
		if err != nil {
			return err
		}
		s.changed(ctx)
		return nil
	})
	e.OnClose(s.close)
	return s
}

// clear drops the tracker's whole state.
func (s *ArtistTrackerService) clear(ctx context.Context) error {
	return s.e.DB.Tx(ctx, func(q *dbq.Queries) error {
		for _, f := range []func(context.Context) error{q.ClearArtistReleases, q.ClearTrackedArtists, q.ClearArtistTrackerState} {
			if err := f(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

// Releases returns the tracker's list.
func (s *ArtistTrackerService) Releases(ctx context.Context) (NewReleases, error) {
	rows, err := s.e.DB.RQ.ListArtistReleases(ctx)
	if err != nil {
		return NewReleases{}, err
	}
	out := NewReleases{Items: make([]NewRelease, len(rows))}
	for i, r := range rows {
		out.Items[i] = s.release(r)
		if !out.Items[i].Read {
			out.Unread++
		}
	}
	if at, err := s.e.DB.RQ.GetArtistTrackerChecked(ctx); err == nil {
		out.CheckedAt = at
	} else if !errors.Is(err, sql.ErrNoRows) {
		return NewReleases{}, err
	}
	return out, nil
}

// MarkRead marks the release of catalogue album id as read (nothing when it
// is read or unknown).
func (s *ArtistTrackerService) MarkRead(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: release %q", ErrInvalidArgument, id)
	}
	if err := s.e.DB.Q.MarkArtistReleaseRead(ctx, dbq.MarkArtistReleaseReadParams{ReadAt: s.now(), AlbumID: id}); err != nil {
		return err
	}
	s.changed(ctx)
	return nil
}

// MarkAllRead marks every release in the list as read.
func (s *ArtistTrackerService) MarkAllRead(ctx context.Context) error {
	if err := s.e.DB.Q.MarkArtistReleasesRead(ctx, s.now()); err != nil {
		return err
	}
	s.changed(ctx)
	return nil
}

// Dismiss takes the release of catalogue album id out of the list. It is
// remembered, so a later check doesn't find it again.
func (s *ArtistTrackerService) Dismiss(ctx context.Context, id string) error {
	if id == "" {
		return fmt.Errorf("%w: release %q", ErrInvalidArgument, id)
	}
	if err := s.e.DB.Q.DismissArtistRelease(ctx, dbq.DismissArtistReleaseParams{DismissedAt: s.now(), AlbumID: id}); err != nil {
		return err
	}
	s.changed(ctx)
	return nil
}

// Clear takes every release out of the list (they stay remembered).
func (s *ArtistTrackerService) Clear(ctx context.Context) error {
	if err := s.e.DB.Q.DismissArtistReleases(ctx, s.now()); err != nil {
		return err
	}
	s.changed(ctx)
	return nil
}

func (s *ArtistTrackerService) now() sql.NullInt64 {
	return sql.NullInt64{Int64: s.e.Now().UnixMilli(), Valid: true}
}

// release is a stored row as its DTO. The album is made as the catalogue's
// are, so its Prerelease follows the clock.
func (s *ArtistTrackerService) release(r dbq.ArtistRelease) NewRelease {
	al := Album{
		ID: r.AlbumID, Source: SourceCatalog, CatalogID: r.AlbumID, Name: r.Name, Artist: r.ArtistName, ArtistID: r.ArtistID,
		ReleaseDate: r.ReleaseDate, TrackCount: int(r.TrackCount), Explicit: r.Explicit != 0, Single: r.Kind != ReleaseAlbum,
		Art: s.e.m.artOf(r.ArtworkTemplate),
	}
	s.e.m.release(&al, r.Prerelease != 0)
	return NewRelease{Album: al, Kind: r.Kind, FoundAt: r.FoundAt, Read: r.ReadAt.Valid}
}

// changed emits the list as "artisttracker:changed".
func (s *ArtistTrackerService) changed(ctx context.Context) {
	if s.e.Emit == nil {
		return
	}
	list, err := s.Releases(ctx)
	if err != nil {
		s.e.Logger.Warn("artist tracker: list", "err", err)
		return
	}
	s.e.Emit.Emit(EventArtistReleasesChanged, list)
}

// poke makes the loop look again.
func (s *ArtistTrackerService) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// run checks until close.
func (s *ArtistTrackerService) run() {
	defer close(s.done)
	if s.startDelay > 0 {
		t := time.NewTimer(s.startDelay)
		select {
		case <-s.quit:
			t.Stop()
			return
		case <-t.C:
		}
	}
	for {
		t := time.NewTimer(s.pass())
		select {
		case <-s.quit:
			t.Stop()
			return
		case <-s.wake:
		case <-t.C:
		}
		t.Stop()
	}
}

// pass does what is due and returns how long to wait before looking again.
// Nothing is asked of Apple while the tracker is off or nobody is signed in.
func (s *ArtistTrackerService) pass() time.Duration {
	cfg := s.e.Settings.Get().ArtistTracker
	if !cfg.Enabled {
		return trackerEvery
	}
	if !s.e.hasMUT() {
		return trackerNoAccount
	}
	ctx := s.ctx
	if s.reseed.Swap(false) {
		if err := s.e.DB.Tx(ctx, func(q *dbq.Queries) error {
			if err := q.ClearTrackedArtists(ctx); err != nil {
				return err
			}
			return q.ClearArtistTrackerState(ctx)
		}); err != nil {
			s.reseed.Store(true)
			s.e.Logger.Warn("artist tracker: reseed", "err", err)
			return trackerRetryMin
		}
	}
	now := s.e.Now()
	if at, err := s.e.DB.RQ.GetArtistTrackerChecked(ctx); err == nil {
		if due := time.UnixMilli(at).Add(trackerEvery); now.Before(due) {
			return due.Sub(now)
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		s.e.Logger.Warn("artist tracker: state", "err", err)
		return trackerRetryMin
	}
	found, err := s.check(ctx, cfg, now)
	s.announce(ctx, found, cfg)
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, errTrackerAccount) {
			return trackerEvery
		}
		s.failures++
		wait := min(trackerRetryMin<<min(s.failures-1, 8), trackerRetryMax)
		s.e.Logger.Info("artist tracker: check failed; trying again later", "err", err, "in", wait)
		return wait
	}
	s.failures = 0
	if err := s.e.DB.Q.PutArtistTrackerChecked(ctx, now.UnixMilli()); err != nil {
		s.e.Logger.Warn("artist tracker: state", "err", err)
	}
	s.changed(ctx)
	return trackerEvery
}

// trackerFind is a release a check found: its DTO, and the artwork template
// a notification shows (the DTO's Art is a Go-served URL).
type trackerFind struct {
	NewRelease
	art string
}

// errTrackerAccount ends a check whose account left while it ran.
var errTrackerAccount = errors.New("the account changed during a check")

// check looks at the favourite artists' releases and records them. It
// returns the releases found new (to announce) even when it fails part-way:
// what was recorded is not found again.
func (s *ArtistTrackerService) check(ctx context.Context, cfg config.ArtistTracker, now time.Time) ([]trackerFind, error) {
	epoch := s.epoch.Load()
	favs, err := s.favourites(ctx)
	if err != nil {
		return nil, s.e.Fail(err)
	}
	tracked, err := s.e.DB.RQ.ListTrackedArtists(ctx)
	if err != nil {
		return nil, err
	}
	seen, err := s.e.DB.RQ.ListSeenReleaseIDs(ctx)
	if err != nil {
		return nil, err
	}
	trackedSet := make(map[string]bool, len(tracked))
	for _, id := range tracked {
		trackedSet[id] = true
	}
	seenSet := make(map[string]bool, len(seen))
	for _, id := range seen {
		seenSet[id] = true
	}
	favSet := make(map[string]bool, len(favs))
	for _, id := range favs {
		favSet[id] = true
	}
	// An artist that is no longer a favourite leaves: its records go, and a
	// later favourite is seeded again.
	for _, id := range tracked {
		if favSet[id] {
			continue
		}
		if err := s.write(epoch, func(q *dbq.Queries) error {
			if err := q.DeleteTrackedArtist(ctx, id); err != nil {
				return err
			}
			return q.DeleteArtistReleasesOf(ctx, id)
		}); err != nil {
			return nil, err
		}
	}
	sf := s.e.Storefront()
	if sf == "" {
		return nil, errors.New("no storefront")
	}
	var found []trackerFind
	for i, batch := range slices.Collect(slices.Chunk(favs, trackerBatch)) {
		if i > 0 {
			if err := sleep(ctx, s.pause); err != nil {
				return found, err
			}
		}
		artists, err := applemusic.AMPResources[applemusic.Artist](ctx, s.e.API, sf, applemusic.TypeArtists, batch, trackerQuery())
		if err != nil {
			return found, s.e.Fail(err)
		}
		for _, a := range artists {
			news, err := s.record(ctx, epoch, a, trackedSet[a.ID], seenSet, now)
			found = append(found, news...)
			if err != nil {
				return found, err
			}
		}
	}
	if err := s.write(epoch, func(q *dbq.Queries) error {
		return q.PruneArtistReleases(ctx, now.Add(-trackerKeep).UnixMilli())
	}); err != nil {
		s.e.Logger.Warn("artist tracker: prune", "err", err)
	}
	return found, nil
}

// trackerQuery is the request for a batch of artists' releases: the latest
// release and the newest albums and singles (views come newest first), with
// only the album attributes the tracker keeps.
func trackerQuery() applemusic.AMPQuery {
	return applemusic.AMPQuery{
		Views: []string{applemusic.ViewLatestRelease, applemusic.ViewFullAlbums, applemusic.ViewSingles},
		Fields: map[string][]string{
			"artists": {"name"},
			"albums": {
				"name", "artistName", "releaseDate", "isSingle", "isCompilation", "isPrerelease", "artwork", "url",
				"contentRating", "trackCount",
			},
		},
		Extra: url.Values{
			"limit[full-albums]": {fmt.Sprint(trackerFullAlbums)},
			"limit[singles]":     {fmt.Sprint(trackerSingles)},
		},
	}
}

// favourites returns the catalogue IDs of the favourite artists: those of
// the candidates (the synced library's artists with a catalogue ID and the
// artists favourited in Sona) that Apple says are rated 1.
func (s *ArtistTrackerService) favourites(ctx context.Context) ([]string, error) {
	lib, err := s.e.DB.RQ.ListLibraryArtistCatalogIDs(ctx)
	if err != nil {
		return nil, err
	}
	local, err := s.e.DB.RQ.ListFavouriteArtistIDs(ctx)
	if err != nil {
		return nil, err
	}
	var candidates []string
	for _, id := range lib {
		if id.Valid {
			candidates = append(candidates, id.String)
		}
	}
	candidates = append(candidates, local...)
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	if len(candidates) == 0 {
		return nil, nil
	}
	ratings, err := s.e.API.Ratings(ctx, applemusic.TypeArtists, candidates)
	if err != nil {
		return nil, err
	}
	var favs []string
	for _, id := range candidates {
		if ratings[id] == applemusic.RatingFavorite {
			favs = append(favs, id)
		}
	}
	return favs, nil
}

// write runs fn in a write transaction unless the account left since the
// check began (errTrackerAccount): sign-out and these writes never overlap.
func (s *ArtistTrackerService) write(epoch int64, fn func(q *dbq.Queries) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.epoch.Load() != epoch {
		return errTrackerAccount
	}
	return s.e.DB.Tx(s.ctx, fn)
}

// record stores what a check read of one artist and returns the releases
// that are new to announce. seeding artists (not tracked yet) announce
// nothing: what is out is recorded as seen, releases still to come are
// listed as read.
func (s *ArtistTrackerService) record(ctx context.Context, epoch int64, a applemusic.Artist, tracked bool, seen map[string]bool, now time.Time) ([]trackerFind, error) {
	var albums []applemusic.Album
	for _, v := range [][]applemusic.Album{a.Views.LatestRelease.Data, a.Views.FullAlbums.Data, a.Views.Singles.Data} {
		for _, al := range v {
			if !slices.ContainsFunc(albums, func(x applemusic.Album) bool { return x.ID == al.ID }) {
				albums = append(albums, al)
			}
		}
	}
	var news []trackerFind
	at := now.UnixMilli()
	err := s.write(epoch, func(q *dbq.Queries) error {
		for _, al := range albums {
			row := s.row(al, a, at)
			switch {
			case seen[al.ID]:
				// Known: only what Apple may change moves.
			case !tracked:
				// Seeding: out already is seen, never shown; still to come is
				// listed, read.
				row.Listed = b2i(row.Prerelease != 0)
				if row.Listed != 0 {
					row.ReadAt = sql.NullInt64{Int64: at, Valid: true}
				}
			default:
				old := row.Prerelease == 0 && row.ReleaseAt != 0 && now.Sub(time.UnixMilli(row.ReleaseAt)) > trackerFresh
				row.Listed = b2i(!old)
			}
			if err := q.PutArtistRelease(ctx, row); err != nil {
				return err
			}
			if !seen[al.ID] && tracked && row.Listed != 0 {
				news = append(news, trackerFind{art: row.ArtworkTemplate, NewRelease: s.release(dbq.ArtistRelease{
					AlbumID: row.AlbumID, ArtistID: row.ArtistID, Name: row.Name, ArtistName: row.ArtistName, ArtworkTemplate: row.ArtworkTemplate,
					Kind: row.Kind, ReleaseDate: row.ReleaseDate, ReleaseAt: row.ReleaseAt, Prerelease: row.Prerelease, TrackCount: row.TrackCount,
					Explicit: row.Explicit, Listed: row.Listed, FoundAt: row.FoundAt,
				})})
			}
		}
		if !tracked {
			return q.PutTrackedArtist(ctx, dbq.PutTrackedArtistParams{ArtistID: a.ID, Name: a.Attributes.Name, SeededAt: at})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, al := range albums {
		seen[al.ID] = true
	}
	return news, nil
}

// row is catalogue album al of artist a as a row to store, listed, with the
// time it was found.
func (s *ArtistTrackerService) row(al applemusic.Album, a applemusic.Artist, found int64) dbq.PutArtistReleaseParams {
	at := al.Attributes
	date := at.ReleaseDate
	return dbq.PutArtistReleaseParams{
		AlbumID: al.ID, ArtistID: a.ID, Name: at.Name, ArtistName: cmp.Or(at.ArtistName, a.Attributes.Name),
		ArtworkTemplate: at.Artwork.Template(), Kind: releaseKind(at.Name, at.IsSingle),
		ReleaseDate: date, ReleaseAt: releaseAt(date), Prerelease: b2i(upcoming(date, at.IsPrerelease, s.e.m.clock())),
		TrackCount: int64(at.TrackCount), Explicit: b2i(at.ContentRating == "explicit"),
		Listed: 1, FoundAt: found,
	}
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// releaseKind is what a catalogue album is: Apple names an EP "… - EP" and a
// single "… - Single", and flags both isSingle.
func releaseKind(name string, single bool) string {
	switch {
	case strings.HasSuffix(name, " - EP"):
		return ReleaseEP
	case single || strings.HasSuffix(name, " - Single"):
		return ReleaseSingle
	}
	return ReleaseAlbum
}

// announce emits the list, posts the new releases of a check to the
// notification centre (NoticeNewReleases: the release, or how many) and,
// where the setting and the platform allow, shows the same as a desktop
// notification. Releases still to come get none: their reminder is the
// album's "Remind me".
func (s *ArtistTrackerService) announce(ctx context.Context, found []trackerFind, cfg config.ArtistTracker) {
	if len(found) == 0 {
		return
	}
	s.changed(ctx)
	out := found[:0:0]
	for _, r := range found {
		if !r.Album.Prerelease {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return
	}
	n := notice{kind: NoticeNewReleases, count: len(out), art: out[0].art, target: OpenTarget{Kind: OpenNewReleases}, account: true}
	if len(out) == 1 {
		r := out[0]
		n.key = NoticeNewReleases + ":" + r.Album.CatalogID
		n.subject, n.artist, n.variant = r.Album.Name, r.Album.Artist, r.Kind
		n.target = OpenTarget{Kind: string(app.KindAlbum), ID: r.Album.CatalogID}
	} else {
		n.key = NoticeNewReleases + ":" + strconv.FormatInt(s.e.Now().UnixMilli(), 10)
	}
	if err := s.e.postNotice(ctx, n); err != nil {
		s.e.Logger.Warn("artist tracker: notice not saved", "err", err)
	}
	if !cfg.Desktop || s.e.Platform.Notifications == nil {
		return
	}
	p := s.e.Language.Printer()
	var err error
	if len(out) == 1 {
		r := out[0]
		err = s.notices.show(ctx, r.Album.Name, p.Sprintf(trackKindMsg[r.Kind], r.Album.Artist), r.art, r.Album.Name, n.target)
	} else {
		err = s.notices.show(ctx, p.T(msgTrackSeveral), p.Sprintf(msgTrackCount, len(out)), out[0].art, p.T(msgTrackOpen), n.target)
	}
	if err != nil {
		s.e.Logger.Info("artist tracker: notification not shown", "err", err)
	}
}

// trackKindMsg is a release kind's notification body.
var trackKindMsg = map[string]l10n.Message{ReleaseAlbum: msgTrackAlbum, ReleaseEP: msgTrackEP, ReleaseSingle: msgTrackSingle}

// close stops the loop (and a check in progress). Shown notifications stay
// on screen.
func (s *ArtistTrackerService) close() {
	s.mu.Lock()
	close(s.quit)
	s.cancel()
	started := s.started
	s.mu.Unlock()
	if started {
		<-s.done
	}
	s.notices.close()
}

// sleep waits d or until ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
