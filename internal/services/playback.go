package services

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// trackMeta is what a queue slot needs beyond coordinator.Item.
type trackMeta struct {
	source                   Source
	id, catalogID, libraryID string
	albumID, artistID        string
	// artists are every credited artist where the song came from the
	// catalogue with them named (a fetched song, a station's); a stored row
	// keeps only the first artist's ID.
	artists []Artist
}

// PlaybackService controls the play queue (internal/playback/coordinator).
type PlaybackService struct {
	o *Env
	m mapper

	mu   sync.Mutex
	meta map[string]trackMeta // by coordinator.Item.ID
	// replMu guards replCancel, which ends the queue-replacing command (PlayItems,
	// PlayLibrary) still resolving its songs when a later one starts.
	replMu     sync.Mutex
	replCancel context.CancelCauseFunc

	volMu    sync.Mutex
	volTimer *time.Timer

	started bool
	quit    chan struct{} // closed by close
	done    chan struct{} // closed when the event forwarder ends
}

// newPlaybackService starts forwarding coordinator snapshots as events;
// sign-out stops playback and empties the queue (it names the user's
// songs). The account's explicit-content restriction (Apple's itre flag,
// auth Status.Restricted) makes the coordinator skip explicit items.
// AutoPlay is fetched ahead only once the network opens after the first
// paint, while signed in (its station needs the Music User Token) and
// reaching Apple.
func newPlaybackService(e *Env) *PlaybackService {
	s := &PlaybackService{o: e, m: e.m, meta: map[string]trackMeta{}, quit: make(chan struct{}), done: make(chan struct{})}
	e.Coord.SetStationSource(stationFeed{s})
	e.Coord.SetAutoPlay(e.Settings.Get().AutoPlay)
	online := func(st auth.Status) {
		e.Coord.SetOnline(e.Gate.IsOpen() && st.State == auth.StateSignedIn && !st.Offline)
	}
	unwatch := e.Auth.OnChange(func(st auth.Status) {
		e.Coord.SetRestricted(st.Restricted)
		online(st)
	})
	e.Coord.SetRestricted(e.Auth.Status().Restricted)
	e.OnNetwork(func() { online(e.Auth.Status()) })
	s.run()
	e.OnSignOut("playback", e.Coord.Stop)
	e.OnClose(func() {
		unwatch()
		s.close()
	})
	return s
}

// run forwards coordinator snapshots as events until close. A volume
// change from any path (the player bar, MPRIS, `sona volume`) is saved
// into the settings a second after it settles.
func (s *PlaybackService) run() {
	snaps, cancel := s.o.Coord.Subscribe()
	s.started = true
	volume := s.o.Coord.Snapshot().Volume
	s.o.Go("playback events", func() {
		defer close(s.done)
		defer cancel()
		var (
			lastQueue uint64
			sent      bool
			auth      bool
		)
		for {
			var snap coordinator.Snapshot
			select {
			case <-s.quit:
				return
			case sn, ok := <-snaps:
				if !ok {
					return
				}
				snap = sn
			}
			s.o.Emit.Emit(EventPlaybackState, s.state(snap))
			if !sent || snap.QueueVersion != lastQueue {
				sent, lastQueue = true, snap.QueueVersion
				s.o.Emit.Emit(EventPlaybackQueue, s.queue(snap))
			}
			if snap.AuthRequired && !auth {
				s.o.Report(applemusic.ErrUnauthorized)
			}
			auth = snap.AuthRequired
			if snap.Volume != volume {
				volume = snap.Volume
				s.saveVolumeSoon()
			}
		}
	})
}

func (s *PlaybackService) close() {
	if s.started {
		close(s.quit)
		<-s.done
	}
	s.volMu.Lock()
	if s.volTimer != nil {
		s.volTimer.Stop()
	}
	s.volMu.Unlock()
	s.flushVolume()
}

// State returns the now-playing state.
func (s *PlaybackService) State() PlaybackState { return s.state(s.o.Coord.Snapshot()) }

// Queue returns the queue.
func (s *PlaybackService) Queue() QueueState { return s.queue(s.o.Coord.Snapshot()) }

func (s *PlaybackService) state(snap coordinator.Snapshot) PlaybackState {
	out := PlaybackState{
		State:          string(snap.State),
		Index:          snap.Index,
		PositionMs:     snap.Position.Milliseconds(),
		DurationMs:     snap.Duration.Milliseconds(),
		At:             unixMs(snap.At),
		Volume:         snap.Volume,
		Shuffle:        snap.Shuffle,
		Repeat:         string(snap.Repeat),
		Context:        snap.Context,
		QueueLength:    len(snap.Queue),
		QueueVersion:   snap.QueueVersion,
		Controls:       PlaybackControls(snap.Controls),
		Error:          playbackError(snap),
		AuthRequired:   snap.AuthRequired,
		NoCDM:          snap.NoCDM,
		Preview:        snap.Preview,
		OnAir:          s.onAir(snap.OnAir),
		Broadcaster:    broadcaster(snap.Info),
		AutoPlay:       snap.AutoPlay,
		Station:        queueStation(snap.Station),
		Sleep:          sleepTimer(snap.Sleep),
		CanUndoReplace: snap.CanUndoReplace,
	}
	if out.State == "" {
		out.State = string(playback.StateIdle)
	}
	if snap.Current != nil {
		qi := s.item(snap.Current.UID, snap.Current.Item)
		qi.Backdrop = s.backdrop(snap.Current.Item.Artwork)
		out.Current = &qi
		if c := snap.Current.Item.Container; c.ID != "" {
			out.Container = &QueueContainer{Kind: string(c.Kind), ID: c.ID}
		}
	}
	return out
}

// playbackError classifies the snapshot's failure: an engine's own failure
// text, with no error behind it, is unknown.
func playbackError(s coordinator.Snapshot) ErrorCode {
	switch {
	case s.Error == "":
		return ""
	case s.Cause == nil:
		return CodeUnknown
	}
	return CodeOf(s.Cause)
}

// onAir maps what a broadcaster has on air; nil when it names nothing.
func (s *PlaybackService) onAir(o playback.OnAir) *OnAir {
	if o.Title == "" && o.Artist == "" {
		return nil
	}
	return &OnAir{Title: o.Title, Artist: o.Artist, Art: s.m.artOf(o.Art)}
}

// broadcaster maps what TuneIn says about a broadcaster; nil when nothing.
func broadcaster(i playback.StationInfo) *BroadcasterInfo {
	if i == (playback.StationInfo{}) {
		return nil
	}
	return &BroadcasterInfo{Description: i.Description, Genre: i.Genre, Location: i.Location, Website: i.Website}
}

// sleepTimer maps the coordinator's sleep timer for the UI.
func sleepTimer(sl *coordinator.Sleep) *SleepTimer {
	if sl == nil {
		return nil
	}
	out := &SleepTimer{Mode: string(sl.Mode)}
	if !sl.At.IsZero() {
		out.EndsAt = unixMs(sl.At)
	}
	return out
}

func (s *PlaybackService) backdrop(tmpl string) string {
	if tmpl == "" || s.o.Art == nil {
		return ""
	}
	return s.o.Art.BackdropURL(tmpl)
}

func (s *PlaybackService) queue(snap coordinator.Snapshot) QueueState {
	out := QueueState{Version: snap.QueueVersion, Index: snap.Index, Items: make([]QueueItem, len(snap.Queue)), History: make([]QueueItem, len(snap.History)), Station: queueStation(snap.Station)}
	keep := make(map[string]bool, len(snap.Queue)+len(snap.History))
	for i, e := range snap.Queue {
		out.Items[i] = s.item(e.UID, e.Item)
		out.Items[i].Auto, out.Items[i].Added = e.Auto, e.Added
		keep[e.Item.ID] = true
	}
	for i, it := range snap.History {
		out.History[i] = s.item(0, it)
		keep[it.ID] = true
	}
	// Forget metadata of items no longer queued or in the history.
	s.mu.Lock()
	for id := range s.meta {
		if !keep[id] {
			delete(s.meta, id)
		}
	}
	s.mu.Unlock()
	return out
}

// item builds a QueueItem, resolving metadata of items queued in an
// earlier run (restored queue) from SQLite.
func (s *PlaybackService) item(uid uint64, it coordinator.Item) QueueItem {
	s.mu.Lock()
	m, ok := s.meta[it.ID]
	s.mu.Unlock()
	if !ok {
		m = s.lookupMeta(it.ID)
		s.mu.Lock()
		s.meta[it.ID] = m
		s.mu.Unlock()
	}
	return QueueItem{
		UID: uid, ID: m.id, Source: m.source, CatalogID: m.catalogID, LibraryID: m.libraryID,
		Name: it.Title, Artist: it.Artist, Album: it.Album, AlbumID: m.albumID, ArtistID: m.artistID, Artists: m.artists,
		DurationMs: it.Duration.Milliseconds(), Art: s.m.artOf(it.Artwork),
		Live:      it.Live(),
		Broadcast: it.Broadcast,
		Explicit:  it.Explicit,
		Video:     playback.IsVideo(it.ID),
	}
}

// lookupMeta resolves a playback ID (catalog, or library for tracks
// without a catalog ID) against the local store, preferring the library
// copy.
func (s *PlaybackService) lookupMeta(id string) trackMeta {
	ctx := context.Background()
	if c := playback.VideoCatalogID(id); c != "" {
		return trackMeta{source: SourceCatalog, id: c, catalogID: c}
	}
	if applemusic.IsLibrarySongID(id) {
		if t, err := s.o.DB.RQ.GetTrackByLibraryID(ctx, db.NullString(id)); err == nil {
			return metaOf(t, SourceLibrary)
		}
		return trackMeta{source: SourceLibrary, id: id, libraryID: id}
	}
	if t, err := s.o.DB.RQ.GetTrackByCatalogID(ctx, db.NullString(id)); err == nil {
		src := SourceCatalog
		if t.LibraryID.Valid {
			src = SourceLibrary
		}
		return metaOf(t, src)
	}
	return trackMeta{source: SourceCatalog, id: id, catalogID: id}
}

func metaOf(t dbq.Track, src Source) trackMeta {
	m := trackMeta{source: src, catalogID: t.CatalogID.String, libraryID: t.LibraryID.String}
	if src == SourceLibrary {
		m.id, m.albumID, m.artistID = t.LibraryID.String, t.AlbumLibraryID.String, t.ArtistLibraryID.String
	} else {
		m.id, m.albumID, m.artistID = t.CatalogID.String, t.AlbumCatalogID.String, t.ArtistCatalogID.String
	}
	return m
}

// resolved is one ref turned into a queue slot, or why it can't be.
type resolved struct {
	item coordinator.Item
	meta trackMeta
	err  error
}

// errNotStored is resolveStored's answer for a catalog song with no row in
// SQLite yet: resolve fetches those together.
var errNotStored = errors.New("song not stored")

// resolve turns refs into coordinator items from SQLite. The catalog songs
// not stored yet (a chart, search results or a room: their songs are only
// stored once fetched on their own) are fetched together, one request per
// hundred, never one per song: the start song waits on that and on nothing
// else, so playback begins about one round trip after the click, whatever the
// queue's length. Unplayable songs and songs not out yet are dropped; start
// is remapped to the same song, and fails with not_playable or not_released
// when that song itself is one.
func (s *PlaybackService) resolve(ctx context.Context, refs []TrackRef, start int) ([]coordinator.Item, int, error) {
	if start < 0 || (len(refs) > 0 && start >= len(refs)) {
		return nil, 0, fmt.Errorf("%w: start %d of %d", ErrInvalidArgument, start, len(refs))
	}
	slots := make([]resolved, len(refs))
	var fetch []int
	for i, r := range refs {
		slots[i].item, slots[i].meta, slots[i].err = s.resolveStored(ctx, r)
		if errors.Is(slots[i].err, errNotStored) {
			fetch = append(fetch, i)
		}
	}
	if len(fetch) > 0 {
		if err := s.fetchSongs(ctx, refs, slots, fetch); err != nil {
			return nil, 0, err
		}
	}
	items := make([]coordinator.Item, 0, len(refs))
	metas := make([]trackMeta, 0, len(refs))
	newStart := -1
	for i, r := range refs {
		if err := slots[i].err; err != nil {
			if i == start {
				return nil, 0, err
			}
			if errors.Is(err, applemusic.ErrNotPlayable) || errors.Is(err, ErrNotReleased) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, applemusic.ErrNotFound) {
				s.o.Logger.Debug("playback: skipping item", "id", r.ID, "err", err)
				continue
			}
			return nil, 0, err
		}
		if i == start {
			newStart = len(items)
		}
		items = append(items, slots[i].item)
		metas = append(metas, slots[i].meta)
	}
	s.mu.Lock()
	for i, it := range items {
		s.meta[it.ID] = metas[i]
	}
	s.mu.Unlock()
	return items, max(newStart, 0), nil
}

// fetchSongs fills slots[i] for every i in fetch (catalog songs resolveStored
// didn't find) from one batched catalog lookup. A song Apple doesn't serve in
// the storefront fails its slot with not_found, which drops it like any
// unplayable one; a failed lookup fails them all.
func (s *PlaybackService) fetchSongs(ctx context.Context, refs []TrackRef, slots []resolved, fetch []int) error {
	ids := make([]string, len(fetch))
	for k, i := range fetch {
		ids[k] = refs[i].ID
	}
	songs, err := s.o.Catalog.Songs(ctx, ids)
	if err != nil {
		s.o.Report(err)
		return err
	}
	for _, i := range fetch {
		r := refs[i]
		so, ok := songs[r.ID]
		if !ok {
			slots[i] = resolved{err: fmt.Errorf("song %s: %w", r.ID, applemusic.ErrNotFound)}
			continue
		}
		if err := s.released(so.Attributes.ReleaseDate, so.Attributes.PlayParams != nil, func() string { return songAlbumDate(so) }); err != nil {
			slots[i] = resolved{err: fmt.Errorf("track %s/%s: %w", r.Source, r.ID, err)}
			continue
		}
		dto := s.m.amSong(so)
		slots[i] = resolved{item: songItem(so), meta: trackMeta{source: SourceCatalog, id: r.ID, catalogID: r.ID, albumID: dto.AlbumID, artistID: dto.ArtistID, artists: dto.Artists}}
	}
	return nil
}

// resolveStored resolves r from SQLite alone: errNotStored for a catalog song
// that isn't there. (A music video is looked up in the catalog, one request
// each: videos are queued one at a time.)
func (s *PlaybackService) resolveStored(ctx context.Context, r TrackRef) (coordinator.Item, trackMeta, error) {
	switch r.Kind {
	case "":
	case TrackKindVideo:
		return s.resolveVideo(ctx, r)
	default:
		return coordinator.Item{}, trackMeta{}, fmt.Errorf("%w: kind %q", ErrInvalidArgument, r.Kind)
	}
	var (
		t   dbq.Track
		err error
	)
	switch r.Source {
	case SourceLibrary:
		t, err = s.o.DB.RQ.GetTrackByLibraryID(ctx, db.NullString(r.ID))
	case SourceCatalog:
		t, err = s.o.DB.RQ.GetTrackByCatalogID(ctx, db.NullString(r.ID))
		if errors.Is(err, sql.ErrNoRows) {
			return coordinator.Item{}, trackMeta{}, errNotStored
		}
	default:
		return coordinator.Item{}, trackMeta{}, fmt.Errorf("%w: source %q", ErrInvalidArgument, r.Source)
	}
	if err != nil {
		return coordinator.Item{}, trackMeta{}, fmt.Errorf("track %s/%s: %w", r.Source, r.ID, err)
	}
	if err := s.storedReleased(ctx, t); err != nil {
		return coordinator.Item{}, trackMeta{}, fmt.Errorf("track %s/%s: %w", r.Source, r.ID, err)
	}
	return trackItem(t)
}

// resolveVideo turns a music video ref into a coordinator item, played by
// its catalog ID (a library copy names its catalog video).
func (s *PlaybackService) resolveVideo(ctx context.Context, r TrackRef) (coordinator.Item, trackMeta, error) {
	id := r.ID
	switch r.Source {
	case SourceCatalog:
	case SourceLibrary:
		lv, err := s.o.API.LibraryMusicVideo(ctx, r.ID)
		if err != nil {
			s.o.Report(err)
			return coordinator.Item{}, trackMeta{}, err
		}
		if id = lv.CatalogID(); id == "" {
			return coordinator.Item{}, trackMeta{}, fmt.Errorf("library music video %s has no catalog copy: %w", r.ID, applemusic.ErrNotPlayable)
		}
	default:
		return coordinator.Item{}, trackMeta{}, fmt.Errorf("%w: source %q", ErrInvalidArgument, r.Source)
	}
	v, err := s.o.Catalog.MusicVideo(ctx, id)
	if err != nil {
		s.o.Report(err)
		return coordinator.Item{}, trackMeta{}, err
	}
	a := v.Attributes
	m := trackMeta{source: SourceCatalog, id: id, catalogID: id}
	if ar := v.Relationships.Artists.Data; len(ar) > 0 {
		m.artistID = ar[0].ID
		m.artists = s.m.amArtists(ar)
	}
	return itemOf(playback.VideoID(id), a.Name, a.ArtistName, "", a.DurationInMillis, a.Artwork.Template(), a.ContentRating == "explicit"), m, nil
}

// released fails with ErrNotReleased for a song released on date that
// isn't out yet. A song without a date is out when Apple gave it playParams
// (some singles of a pre-release album carry none: Flower In A Hurricane),
// and otherwise goes by its album's date (album, asked only then).
func (s *PlaybackService) released(date string, hasPlayParams bool, album func() string) error {
	if date == "" {
		if hasPlayParams {
			return nil
		}
		date = album()
	}
	if upcoming(date, false, s.m.clock()) {
		return ErrNotReleased
	}
	return nil
}

// storedReleased is released for a stored song. A library song is out by the
// library's rule (library.Unreleased); a catalog one without a date of its
// own and without playParams goes by its catalog album's date.
func (s *PlaybackService) storedReleased(ctx context.Context, t dbq.Track) error {
	if t.LibraryID.Valid {
		un, err := s.o.Library.Unreleased(ctx, t.LibraryID.String)
		switch {
		case err != nil:
			return err
		case un:
			return ErrNotReleased
		}
		return nil
	}
	if upcoming(t.ReleaseDate, false, s.m.clock()) {
		return ErrNotReleased
	}
	if t.ReleaseDate != "" || t.NoPlayParams == 0 {
		return nil
	}
	return s.released("", false, func() string { return s.catalogAlbumDate(ctx, t) })
}

// catalogAlbumDate is the release date of a stored song's catalog album
// ("" when the album isn't stored).
func (s *PlaybackService) catalogAlbumDate(ctx context.Context, t dbq.Track) string {
	if t.AlbumCatalogID.Valid {
		if a, err := s.o.DB.RQ.GetCatalogAlbum(ctx, t.AlbumCatalogID); err == nil {
			return a.ReleaseDate
		}
	}
	return ""
}

// songAlbumDate is the release date of a fetched song's album ("" when
// Apple didn't include it).
func songAlbumDate(so applemusic.Song) string {
	if d := so.Relationships.Albums.Data; len(d) > 0 {
		return d[0].Attributes.ReleaseDate
	}
	return ""
}

// trackItem maps a stored track to a coordinator item. The playback ID is
// the catalog ID, or for library tracks without one (uploads, which the
// engine plays as plain files) the library ID.
func trackItem(t dbq.Track) (coordinator.Item, trackMeta, error) {
	id := t.CatalogID.String
	if id == "" {
		id = t.LibraryID.String
	}
	if id == "" {
		return coordinator.Item{}, trackMeta{}, fmt.Errorf("track without an ID: %w", applemusic.ErrNotPlayable)
	}
	src := SourceCatalog
	if t.LibraryID.Valid {
		src = SourceLibrary
	}
	return itemOf(id, t.Name, t.ArtistName, t.AlbumName, t.DurationMs, t.ArtworkTemplate, t.ContentRating == "explicit"), metaOf(t, src), nil
}

func songItem(so applemusic.Song) coordinator.Item {
	a := so.Attributes
	return itemOf(so.CatalogID(), a.Name, a.ArtistName, a.AlbumName, a.DurationInMillis, a.Artwork.Template(), a.ContentRating == "explicit")
}

// itemOf is the coordinator item of a song or video.
func itemOf(id, title, artist, album string, durationMs int64, artwork string, explicit bool) coordinator.Item {
	return coordinator.Item{
		ID:       id,
		Title:    title,
		Artist:   artist,
		Album:    album,
		Duration: time.Duration(durationMs) * time.Millisecond,
		Artwork:  artwork,
		Explicit: explicit,
	}
}

func (s *PlaybackService) fail(err error) error {
	if err != nil {
		s.o.Report(err)
	}
	return err
}

// errSuperseded is why a queue-replacing command that was still resolving
// its songs ended: a later one took over.
var errSuperseded = errors.New("playback: superseded by a later play command")

// beginReplace starts a command that replaces the queue (PlayItems,
// PlayLibrary) and ends the one still resolving its songs, so that a slow
// lookup never lands after, and replaces, what the user chose last. Resolve
// with the returned context; superseded says whether a later command took
// over (it did: do nothing and report no failure, as the coordinator does for
// its own superseded commands); end releases the context.
func (s *PlaybackService) beginReplace(ctx context.Context) (rctx context.Context, superseded func() bool, end func()) {
	rctx, cancel := context.WithCancelCause(ctx)
	s.replMu.Lock()
	if s.replCancel != nil {
		s.replCancel(errSuperseded)
	}
	s.replCancel = cancel
	s.replMu.Unlock()
	return rctx, func() bool { return errors.Is(context.Cause(rctx), errSuperseded) }, func() { cancel(context.Canceled) }
}

// PlayItems replaces the queue with refs and plays refs[start]. from
// names where they came from: its ID comes back as PlaybackState.Context
// and, for an album, playlist or artist, is reported to Apple with the
// plays. A replaced queue can be brought back with UndoReplace. A later
// PlayItems or PlayLibrary that arrives while this one still resolves its
// songs ends it without a failure.
func (s *PlaybackService) PlayItems(ctx context.Context, refs []TrackRef, start int, from PlayFrom) error {
	s.logCmd("play-items")
	rctx, superseded, end := s.beginReplace(ctx)
	defer end()
	items, i, err := s.resolve(rctx, refs, start)
	if superseded() {
		return nil
	}
	if err != nil {
		return err
	}
	if c := from.container(); c != (coordinator.Container{}) {
		for j := range items {
			items[j].Container = c
		}
	}
	return s.fail(s.o.Coord.SetQueue(ctx, items, i, from.ID))
}

// container is from as a play-activity container (zero for sources that
// are not Apple collections, such as search or charts).
func (f PlayFrom) container() coordinator.Container {
	k := coordinator.ContainerKind(f.Kind)
	if !k.Valid() || k == coordinator.ContainerStation || f.ID == "" {
		return coordinator.Container{}
	}
	return coordinator.Container{Kind: k, ID: f.ID}
}

// UndoReplace brings back the queue the last PlayItems, PlayLibrary or
// station replaced, at the same item and position, playing if it was. It
// reports whether there was one.
func (s *PlaybackService) UndoReplace(ctx context.Context) (bool, error) {
	s.logCmd("undo-replace")
	ok, err := s.o.Coord.UndoReplace(ctx)
	return ok, s.fail(err)
}

// SetSleep sets the sleep timer: mode "after" pauses playback after
// minutes, "track" when the current song ends, "queue" when the last song
// the user queued ends; "" turns it off. The volume fades out over the
// last 10 seconds and is restored once playback stops.
func (s *PlaybackService) SetSleep(mode string, minutes int) error {
	if err := s.o.Coord.SetSleep(coordinator.SleepMode(mode), time.Duration(minutes)*time.Minute); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	return nil
}

// PlayLibrary queues the whole library songs list in the given order
// (as LibraryService.Songs, which leaves out the songs not out yet) and plays
// the song at position start.
func (s *PlaybackService) PlayLibrary(ctx context.Context, sort string, desc bool, start int) error {
	rctx, superseded, end := s.beginReplace(ctx)
	defer end()
	songs, err := s.o.Library.AllSongs(rctx, library.ListOptions{Sort: library.Sort(sort), Desc: desc})
	if superseded() {
		return nil
	}
	if err != nil {
		return err
	}
	if start < 0 || start >= len(songs) {
		return fmt.Errorf("%w: start %d of %d", ErrInvalidArgument, start, len(songs))
	}
	items := make([]coordinator.Item, 0, len(songs))
	idx := 0
	s.mu.Lock()
	for i, so := range songs {
		// Uploads have no catalog ID and play by their library ID.
		id := cmp.Or(so.CatalogID, so.LibraryID)
		if i == start {
			idx = len(items)
		}
		items = append(items, itemOf(id, so.Name, so.ArtistName, so.AlbumName, so.DurationMS, so.ArtworkTemplate, so.Explicit))
		s.meta[id] = trackMeta{
			source: SourceLibrary, id: so.LibraryID, catalogID: so.CatalogID, libraryID: so.LibraryID,
			albumID: so.AlbumLibraryID, artistID: so.ArtistLibraryID,
		}
	}
	s.mu.Unlock()
	return s.fail(s.o.Coord.SetQueue(ctx, items, idx, "library:songs"))
}

// queueItems resolves refs for PlayNext, Append and Insert, which like
// them fail while a station plays: the queue is the station.
func (s *PlaybackService) queueItems(ctx context.Context, refs []TrackRef) ([]coordinator.Item, error) {
	if err := s.o.Coord.Snapshot().Refusal(func(c coordinator.Controls) bool { return c.Queue }); err != nil {
		return nil, err
	}
	items, _, err := s.resolve(ctx, refs, 0)
	return items, err
}

// PlayNext inserts refs after the current item. Like Append and Insert it
// fails with coordinator.ErrLiveStation or ErrStation while a station
// plays: the queue is the station.
func (s *PlaybackService) PlayNext(ctx context.Context, refs []TrackRef) error {
	if len(refs) == 0 {
		return nil
	}
	items, err := s.queueItems(ctx, refs)
	if err != nil {
		return err
	}
	s.o.Coord.PlayNext(items)
	return nil
}

// Append adds refs to the end of the queue.
func (s *PlaybackService) Append(ctx context.Context, refs []TrackRef) error {
	if len(refs) == 0 {
		return nil
	}
	items, err := s.queueItems(ctx, refs)
	if err != nil {
		return err
	}
	s.o.Coord.Append(items)
	return nil
}

// Insert adds refs at queue position at, kept within the upcoming songs
// the user queued (after the current one, before station songs).
func (s *PlaybackService) Insert(ctx context.Context, at int, refs []TrackRef) error {
	if len(refs) == 0 {
		return nil
	}
	items, err := s.queueItems(ctx, refs)
	if err != nil {
		return err
	}
	s.o.Coord.Insert(at, items)
	return nil
}

// Remove deletes queue[index].
func (s *PlaybackService) Remove(ctx context.Context, index int) error {
	return s.fail(s.o.Coord.Remove(ctx, index))
}

// Move moves queue[from] to position to.
func (s *PlaybackService) Move(from, to int) error { return s.o.Coord.Move(from, to) }

// MoveNext moves queue[from] to play right after the current song.
func (s *PlaybackService) MoveNext(from int) error { return s.o.Coord.MoveNext(from) }

// MoveToEnd moves queue[from] to the end of the user's songs, before
// AutoPlay's.
func (s *PlaybackService) MoveToEnd(from int) error { return s.o.Coord.MoveToEnd(from) }

// Clear removes every queued item except the current one.
func (s *PlaybackService) Clear() { s.o.Coord.Clear() }

// Stop stops playback and empties the queue.
func (s *PlaybackService) Stop(ctx context.Context) error { return s.fail(s.o.Coord.Stop(ctx)) }

// Jump plays queue[index].
func (s *PlaybackService) Jump(ctx context.Context, index int) error {
	s.logCmd("jump")
	return s.fail(s.o.Coord.Jump(ctx, index))
}

// Pause pauses playback.
func (s *PlaybackService) Pause(ctx context.Context) error { return s.fail(s.o.Coord.Pause(ctx)) }

// Resume resumes (or starts a restored queue).
func (s *PlaybackService) Resume(ctx context.Context) error {
	s.logCmd("resume")
	return s.fail(s.o.Coord.Resume(ctx))
}

// Toggle pauses when playing and resumes otherwise.
func (s *PlaybackService) Toggle(ctx context.Context) error {
	s.logCmd("toggle")
	return s.fail(s.o.Coord.Toggle(ctx))
}

// logCmd records commands that start or change what plays, so an
// unexpected start or a slow skip can be traced to the UI (MPRIS logs its
// own).
func (s *PlaybackService) logCmd(name string) {
	s.o.Logger.Info("playback: command", "cmd", name, "from", "ui")
}

// Seek moves the current item to positionMs.
func (s *PlaybackService) Seek(ctx context.Context, positionMs int64) error {
	return s.fail(s.o.Coord.Seek(ctx, time.Duration(max(positionMs, 0))*time.Millisecond))
}

// Next plays the following item.
func (s *PlaybackService) Next(ctx context.Context) error {
	s.logCmd("next")
	return s.fail(s.o.Coord.Next(ctx))
}

// Prev restarts the current item, or plays the previous one within the
// first seconds.
func (s *PlaybackService) Prev(ctx context.Context) error {
	s.logCmd("previous")
	return s.fail(s.o.Coord.Previous(ctx))
}

// SetVolume sets the volume (0..1). The settings remember it (run).
func (s *PlaybackService) SetVolume(ctx context.Context, volume float64) error {
	return s.o.Coord.SetVolume(ctx, volume)
}

// saveVolumeSoon saves the volume a second after the last change, so a
// dragged slider writes the settings file once.
func (s *PlaybackService) saveVolumeSoon() {
	s.volMu.Lock()
	defer s.volMu.Unlock()
	if s.volTimer == nil {
		s.volTimer = time.AfterFunc(time.Second, s.flushVolume)
	} else {
		s.volTimer.Reset(time.Second)
	}
}

// flushVolume saves the coordinator volume into the settings.
func (s *PlaybackService) flushVolume() {
	v := s.o.Coord.Snapshot().Volume
	if s.o.Settings.Get().Volume == v {
		return
	}
	if _, err := s.o.Settings.Update(func(st *config.Settings) { st.Volume = v }); err != nil {
		s.o.Logger.Warn("playback: save volume", "err", err)
	}
}

// SetShuffle turns shuffle on or off.
func (s *PlaybackService) SetShuffle(on bool) { s.o.Coord.SetShuffle(on) }

// SetRepeat sets the repeat mode: "off", "all" or "one".
func (s *PlaybackService) SetRepeat(mode string) error {
	if err := s.o.Coord.SetRepeat(coordinator.Repeat(mode)); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	return nil
}
