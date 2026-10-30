package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
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
}

// PlaybackService controls the play queue (internal/playback/coordinator).
type PlaybackService struct {
	o *Env
	m mapper

	mu   sync.Mutex
	meta map[string]trackMeta // by coordinator.Item.ID

	volMu    sync.Mutex
	volTimer *time.Timer

	started bool
	quit    chan struct{} // closed by close
	done    chan struct{} // closed when the event forwarder ends
}

// newPlaybackService starts forwarding coordinator snapshots as events;
// sign-out stops playback and empties the queue (it names the user's
// songs).
func newPlaybackService(e *Env) *PlaybackService {
	s := &PlaybackService{o: e, m: e.m, meta: map[string]trackMeta{}, quit: make(chan struct{}), done: make(chan struct{})}
	e.Coord.SetStationSource(stationFeed{s})
	e.Coord.SetAutoPlay(e.Settings.Get().AutoPlay)
	s.run()
	e.OnSignOut("playback", e.Coord.Stop)
	e.OnClose(s.close)
	return s
}

// run forwards coordinator snapshots as events until close.
func (s *PlaybackService) run() {
	snaps, cancel := s.o.Coord.Subscribe()
	s.started = true
	go func() {
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
		}
	}()
}

func (s *PlaybackService) close() {
	if s.started {
		close(s.quit)
		<-s.done
	}
	s.flushVolume()
}

// State returns the now-playing state.
func (s *PlaybackService) State() PlaybackState { return s.state(s.o.Coord.Snapshot()) }

// Queue returns the queue.
func (s *PlaybackService) Queue() QueueState { return s.queue(s.o.Coord.Snapshot()) }

func (s *PlaybackService) state(snap coordinator.Snapshot) PlaybackState {
	out := PlaybackState{
		State:        string(snap.State),
		Index:        snap.Index,
		PositionMs:   snap.Position.Milliseconds(),
		DurationMs:   snap.Duration.Milliseconds(),
		At:           unixMs(snap.At),
		Volume:       snap.Volume,
		Shuffle:      snap.Shuffle,
		Repeat:       string(snap.Repeat),
		Context:      snap.Context,
		QueueLength:  len(snap.Queue),
		QueueVersion: snap.QueueVersion,
		CanNext:      canNext(snap),
		CanPrevious:  snap.Current != nil,
		Error:        snap.Error,
		AuthRequired: snap.AuthRequired,
		AutoPlay:     snap.AutoPlay,
		Station:      queueStation(snap.Station),
	}
	if out.State == "" {
		out.State = string(playback.StateIdle)
	}
	if snap.Current != nil {
		qi := s.item(snap.Current.UID, snap.Current.Item)
		qi.Backdrop = s.backdrop(snap.Current.Item.Artwork)
		out.Current = &qi
	}
	return out
}

func canNext(snap coordinator.Snapshot) bool {
	if snap.Current == nil || len(snap.Queue) == 0 {
		return false
	}
	// A station (or AutoPlay, which only runs with repeat off) continues
	// past the end of the queue.
	return snap.Index < len(snap.Queue)-1 || snap.Repeat == coordinator.RepeatAll ||
		snap.Station != nil || (snap.AutoPlay && snap.Repeat == coordinator.RepeatOff)
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
		out.Items[i].Auto = e.Auto
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
		Name: it.Title, Artist: it.Artist, Album: it.Album, AlbumID: m.albumID, ArtistID: m.artistID,
		DurationMs: it.Duration.Milliseconds(), Art: s.m.artOf(it.Artwork),
		Live: isStationID(it.ID) && it.Duration <= 0,
	}
}

// lookupMeta resolves a playback ID (catalog, or library for tracks
// without a catalog ID) against the local store, preferring the library
// copy.
func (s *PlaybackService) lookupMeta(id string) trackMeta {
	ctx := context.Background()
	if isLibraryID(id) {
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

func isLibraryID(id string) bool { return strings.HasPrefix(id, "i.") }

func metaOf(t dbq.Track, src Source) trackMeta {
	m := trackMeta{source: src, catalogID: t.CatalogID.String, libraryID: t.LibraryID.String}
	if src == SourceLibrary {
		m.id, m.albumID, m.artistID = t.LibraryID.String, t.AlbumLibraryID.String, t.ArtistLibraryID.String
	} else {
		m.id, m.albumID, m.artistID = t.CatalogID.String, t.AlbumCatalogID.String, t.ArtistCatalogID.String
	}
	return m
}

// resolve turns refs into coordinator items from SQLite (falling back to
// the catalog for catalog songs not stored yet). Unplayable songs
// (uploads) are dropped; start is remapped to the same song, and fails
// with not_playable when that song itself is unplayable.
func (s *PlaybackService) resolve(ctx context.Context, refs []TrackRef, start int) ([]coordinator.Item, int, error) {
	if start < 0 || (len(refs) > 0 && start >= len(refs)) {
		return nil, 0, fmt.Errorf("%w: start %d of %d", ErrInvalidArgument, start, len(refs))
	}
	items := make([]coordinator.Item, 0, len(refs))
	metas := make([]trackMeta, 0, len(refs))
	newStart := -1
	for i, r := range refs {
		it, m, err := s.resolveOne(ctx, r)
		if err != nil {
			if i == start {
				return nil, 0, err
			}
			if errors.Is(err, applemusic.ErrNotPlayable) || errors.Is(err, sql.ErrNoRows) || errors.Is(err, applemusic.ErrNotFound) {
				s.o.Logger.Debug("playback: skipping item", "id", r.ID, "err", err)
				continue
			}
			return nil, 0, err
		}
		if i == start {
			newStart = len(items)
		}
		items = append(items, it)
		metas = append(metas, m)
	}
	s.mu.Lock()
	for i, it := range items {
		s.meta[it.ID] = metas[i]
	}
	s.mu.Unlock()
	return items, max(newStart, 0), nil
}

func (s *PlaybackService) resolveOne(ctx context.Context, r TrackRef) (coordinator.Item, trackMeta, error) {
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
			// Not cached yet: the catalog stores the song as it fetches it.
			var so applemusic.Song
			if so, err = s.o.Catalog.Song(ctx, r.ID); err != nil {
				s.o.Report(err)
				return coordinator.Item{}, trackMeta{}, err
			}
			dto := s.m.amSong(so)
			return songItem(so), trackMeta{source: SourceCatalog, id: r.ID, catalogID: r.ID, albumID: dto.AlbumID, artistID: dto.ArtistID}, nil
		}
	default:
		return coordinator.Item{}, trackMeta{}, fmt.Errorf("%w: source %q", ErrInvalidArgument, r.Source)
	}
	if err != nil {
		return coordinator.Item{}, trackMeta{}, fmt.Errorf("track %s/%s: %w", r.Source, r.ID, err)
	}
	return trackItem(t)
}

// trackItem maps a stored track to a coordinator item. The playback ID is
// the catalog ID; library tracks without one (uploads) cannot play.
func trackItem(t dbq.Track) (coordinator.Item, trackMeta, error) {
	if !t.CatalogID.Valid || t.CatalogID.String == "" {
		return coordinator.Item{}, trackMeta{}, fmt.Errorf("track %s: %w", t.LibraryID.String, applemusic.ErrNotPlayable)
	}
	src := SourceCatalog
	if t.LibraryID.Valid {
		src = SourceLibrary
	}
	return coordinator.Item{
		ID:       t.CatalogID.String,
		Title:    t.Name,
		Artist:   t.ArtistName,
		Album:    t.AlbumName,
		Duration: time.Duration(t.DurationMs) * time.Millisecond,
		Artwork:  t.ArtworkTemplate,
	}, metaOf(t, src), nil
}

func songItem(so applemusic.Song) coordinator.Item {
	a := so.Attributes
	return coordinator.Item{
		ID:       so.CatalogID(),
		Title:    a.Name,
		Artist:   a.ArtistName,
		Album:    a.AlbumName,
		Duration: time.Duration(a.DurationInMillis) * time.Millisecond,
		Artwork:  a.Artwork.Template(),
	}
}

func (s *PlaybackService) fail(err error) error {
	if err != nil {
		s.o.Report(err)
	}
	return err
}

// PlayItems replaces the queue with refs and plays refs[start]. label
// names the source (album or playlist ID) and comes back as
// PlaybackState.Context.
func (s *PlaybackService) PlayItems(ctx context.Context, refs []TrackRef, start int, label string) error {
	s.logCmd("play-items")
	items, i, err := s.resolve(ctx, refs, start)
	if err != nil {
		return err
	}
	return s.fail(s.o.Coord.SetQueue(ctx, items, i, label))
}

// PlayLibrary queues the whole library songs list in the given order
// (as LibraryService.Songs) and plays the song at position start.
func (s *PlaybackService) PlayLibrary(ctx context.Context, sort string, desc bool, start int) error {
	var songs []library.Song
	for offset := 0; ; {
		page, err := s.o.Library.Songs(ctx, library.ListOptions{Sort: library.Sort(sort), Desc: desc, Offset: offset, Limit: 1000})
		if err != nil {
			return err
		}
		songs = append(songs, page...)
		if len(page) < 1000 {
			break
		}
		offset += len(page)
	}
	if start < 0 || start >= len(songs) {
		return fmt.Errorf("%w: start %d of %d", ErrInvalidArgument, start, len(songs))
	}
	if songs[start].CatalogID == "" {
		return fmt.Errorf("song %s: %w", songs[start].LibraryID, applemusic.ErrNotPlayable)
	}
	items := make([]coordinator.Item, 0, len(songs))
	idx := 0
	s.mu.Lock()
	for i, so := range songs {
		if so.CatalogID == "" {
			continue
		}
		if i == start {
			idx = len(items)
		}
		items = append(items, coordinator.Item{
			ID: so.CatalogID, Title: so.Name, Artist: so.ArtistName, Album: so.AlbumName,
			Duration: time.Duration(so.DurationMS) * time.Millisecond, Artwork: so.ArtworkTemplate,
		})
		s.meta[so.CatalogID] = trackMeta{
			source: SourceLibrary, id: so.LibraryID, catalogID: so.CatalogID, libraryID: so.LibraryID,
			albumID: so.AlbumLibraryID, artistID: so.ArtistLibraryID,
		}
	}
	s.mu.Unlock()
	return s.fail(s.o.Coord.SetQueue(ctx, items, idx, "library:songs"))
}

// PlayNext inserts refs after the current item.
func (s *PlaybackService) PlayNext(ctx context.Context, refs []TrackRef) error {
	if len(refs) == 0 {
		return nil
	}
	items, _, err := s.resolve(ctx, refs, 0)
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
	items, _, err := s.resolve(ctx, refs, 0)
	if err != nil {
		return err
	}
	s.o.Coord.Append(items)
	return nil
}

// Remove deletes queue[index].
func (s *PlaybackService) Remove(ctx context.Context, index int) error {
	return s.fail(s.o.Coord.Remove(ctx, index))
}

// Move moves queue[from] to position to.
func (s *PlaybackService) Move(from, to int) error { return s.o.Coord.Move(from, to) }

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

// logCmd records commands that start playback, so an unexpected start can
// be traced to the UI (MPRIS logs its own).
func (s *PlaybackService) logCmd(name string) {
	s.o.Logger.Info("playback: command", "cmd", name, "from", "ui")
}

// Seek moves the current item to positionMs.
func (s *PlaybackService) Seek(ctx context.Context, positionMs int64) error {
	return s.fail(s.o.Coord.Seek(ctx, time.Duration(max(positionMs, 0))*time.Millisecond))
}

// Next plays the following item.
func (s *PlaybackService) Next(ctx context.Context) error { return s.fail(s.o.Coord.Next(ctx)) }

// Prev restarts the current item, or plays the previous one within the
// first seconds.
func (s *PlaybackService) Prev(ctx context.Context) error { return s.fail(s.o.Coord.Previous(ctx)) }

// SetVolume sets the volume (0..1); it is remembered in the settings.
func (s *PlaybackService) SetVolume(ctx context.Context, volume float64) error {
	if err := s.o.Coord.SetVolume(ctx, volume); err != nil {
		return err
	}
	s.volMu.Lock()
	defer s.volMu.Unlock()
	if s.volTimer == nil {
		s.volTimer = time.AfterFunc(time.Second, s.flushVolume)
	} else {
		s.volTimer.Reset(time.Second)
	}
	return nil
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
