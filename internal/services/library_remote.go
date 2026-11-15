package services

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/library"
)

// Shallow checks for changes made on other devices.
//
// Apple has no cheap "something changed" signal for the library (see
// docs/apple-music-reference.md, "Change signals"): the one stamp it gives,
// a playlist's lastModifiedDate, comes with the playlist list, which is one
// request per 100 playlists. So a check is the incremental run of the
// playlists domains (library.Syncer.syncPlaylists and syncPlaylistTracks):
// it reads that list, rewrites only the playlists whose stamp moved and
// refetches the tracks of those alone. Albums, songs, artists and ratings
// have no stamp and wait for the full runs.
const (
	// remoteCheckEvery is the period of the check while the window is
	// focused: long enough to cost one small request a few times an hour,
	// short enough that a change made on the phone shows within minutes.
	remoteCheckEvery = 5 * time.Minute
	// remoteCheckGap is the least time between two checks, whatever starts
	// them (the period, the window gaining focus): the request budget Apple
	// allows is shared with the sync, playback and the pages.
	remoteCheckGap = 2 * time.Minute
	// remoteFocusDelay is how long the window must keep the focus before a
	// check starts, so switching windows with the keyboard asks nothing.
	remoteFocusDelay = 2 * time.Second
)

// remoteChecks schedules the checks: a periodic one while the window is
// focused and one when it gains the focus. Both start only after the first
// paint (LibraryService starts them from OnNetwork) and not before the
// library has synced once.
type remoteChecks struct {
	// ready reports whether a check may talk to Apple: signed in and online.
	ready func() bool
	// focused reports whether the main window is shown and focused.
	focused func() bool
	// begin starts the quiet sync a check is.
	begin func() error
	// onFocusChange registers the window's focus events (nil: none, only
	// the periodic check runs).
	onFocusChange func(func(on bool))

	ctx  context.Context
	stop context.CancelFunc

	mu      sync.Mutex
	started bool
	last    time.Time   // when the last check started
	pending *time.Timer // the focus check waiting out remoteFocusDelay
}

func (r *remoteChecks) init(s *LibraryService) {
	e := s.o
	r.ctx, r.stop = context.WithCancel(context.Background())
	r.onFocusChange = e.OnFocusChange
	r.focused = func() bool { return e.Focused != nil && e.Focused() }
	r.ready = e.reachable
	r.begin = func() error { return s.start(syncRun{mode: library.Incremental, domains: remoteDomains, quiet: true}) }
}

// reachable reports whether Apple can be asked: signed in and online.
func (e *Env) reachable() bool {
	st := e.Auth.Status()
	return st.State == auth.StateSignedIn && !st.Offline
}

// close stops the schedule.
func (r *remoteChecks) close() {
	r.stop()
	r.mu.Lock()
	if r.pending != nil {
		r.pending.Stop()
	}
	r.mu.Unlock()
}

// startRemoteChecks begins the schedule (once the network opens; the first
// check is a gap away, so it never competes with the startup sync).
func (s *LibraryService) startRemoteChecks() {
	r := &s.remote
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started, r.last = true, s.o.Now()
	r.mu.Unlock()
	if r.onFocusChange != nil {
		r.onFocusChange(s.windowFocus)
	}
	s.o.Go("remote checks", func() {
		t := time.NewTicker(remoteCheckEvery)
		defer t.Stop()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-t.C:
				if r.focused() {
					s.remoteCheck("periodic")
				}
			}
		}
	})
}

// windowFocus follows the main window's focus: gaining it starts a check
// after remoteFocusDelay, if the window still has it and the last check is
// a gap old; losing it drops the one waiting.
func (s *LibraryService) windowFocus(on bool) {
	r := &s.remote
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending != nil {
		r.pending.Stop()
		r.pending = nil
	}
	if !on || r.ctx.Err() != nil {
		return
	}
	r.pending = time.AfterFunc(remoteFocusDelay, func() {
		s.o.Go("remote check", func() {
			if r.focused() {
				s.remoteCheck("focus")
			}
		})
	})
}

// remoteCheck runs one shallow check unless one ran within remoteCheckGap,
// a sync is running, the library has not synced yet, or Apple is out of
// reach (signed out, offline). reason is for the log.
func (s *LibraryService) remoteCheck(reason string) {
	r := &s.remote
	if r.ctx.Err() != nil || !r.ready() {
		return
	}
	if st, err := s.SyncStatus(r.ctx); err != nil || st.LastSuccess == 0 {
		return // the startup sync has yet to finish the first run
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := s.o.Now()
	if now.Sub(r.last) < remoteCheckGap {
		s.o.Logger.Debug("library: remote check skipped, too soon", "reason", reason, "last", r.last)
		return
	}
	err := r.begin()
	switch {
	case err == nil:
		r.last = now
		s.o.Logger.Debug("library: remote check started", "reason", reason)
	case errors.Is(err, library.ErrSyncRunning):
		s.o.Logger.Debug("library: remote check skipped, a sync is running", "reason", reason)
	case !errors.Is(err, auth.ErrNotSignedIn) && !errors.Is(err, context.Canceled):
		s.o.Logger.Warn("library: remote check", "reason", reason, "err", err)
	}
}

// remoteDomains are what a check syncs: the playlists and the tracks of
// those whose stamp moved.
var remoteDomains = []library.Domain{library.DomainPlaylists, library.DomainPlaylistTracks}

// playlistStamp is what a check compares: where a playlist sits and how it
// looks in the tree (look), and the two modification stamps (the playlist's
// and the one its stored tracks were fetched at).
type playlistStamp struct {
	look                     playlistLook
	modified, tracksModified string
}

type playlistLook struct {
	name, folder, artwork string
	position, tracks      int64
	public                bool
}

// playlistSnapshot is the stored library playlists by library ID; nil when
// SQLite could not be read.
type playlistSnapshot map[string]playlistStamp

func (s *LibraryService) playlistSnapshot(ctx context.Context) playlistSnapshot {
	tree, err := s.o.DB.RQ.LibraryPlaylistTree(ctx)
	if err != nil {
		s.o.Logger.Warn("library: read playlists", "err", err)
		return nil
	}
	tracks, err := s.o.DB.RQ.LibraryPlaylistsForTrackSync(ctx)
	if err != nil {
		s.o.Logger.Warn("library: read playlists", "err", err)
		return nil
	}
	out := make(playlistSnapshot, len(tree))
	for _, r := range tree {
		out[r.LibraryID.String] = playlistStamp{
			look: playlistLook{
				name: r.Name, folder: r.FolderID, artwork: r.ArtworkTemplate,
				position: r.FolderPosition, tracks: r.TrackCount, public: r.IsPublic != 0,
			},
			modified: r.LastModified,
		}
	}
	for _, r := range tracks {
		if st, ok := out[r.LibraryID.String]; ok {
			st.tracksModified = r.TracksModified
			out[r.LibraryID.String] = st
		}
	}
	return out
}

// diff is what changed from p to next: the playlists whose stamps moved or
// that left (their pages refresh), and whether the tree changed (one was
// added or removed, or one shows differently).
func (p playlistSnapshot) diff(next playlistSnapshot) PlaylistsChanged {
	ev := PlaylistsChanged{IDs: []string{}}
	for id, was := range p {
		now, ok := next[id]
		switch {
		case !ok:
			ev.IDs = append(ev.IDs, id)
			ev.Tree = true
		case now != was:
			ev.IDs = append(ev.IDs, id)
			ev.Tree = ev.Tree || now.look != was.look
		}
	}
	for id := range next {
		if _, ok := p[id]; !ok {
			ev.Tree = true
		}
	}
	slices.Sort(ev.IDs)
	return ev
}

// announceRemote ends a check: it compares the playlists with what they
// were before and, if anything changed, emits "playlists:changed" for just
// those, which refreshes those pages and the tree only.
func (s *LibraryService) announceRemote(before playlistSnapshot, start time.Time, err error) {
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.o.Logger.Warn("library: remote check failed", "err", err)
		}
		return
	}
	if before == nil {
		return
	}
	ev := before.diff(s.playlistSnapshot(context.Background()))
	s.o.Logger.Debug("library: remote check done", "took", s.o.Now().Sub(start).Round(time.Millisecond),
		"playlists", len(before), "changed", len(ev.IDs), "tree", ev.Tree)
	if len(ev.IDs) == 0 && !ev.Tree {
		return
	}
	if ev.Tree {
		pruneCoverFiles(context.Background(), s.o) // a playlist removed elsewhere
	}
	s.o.Emit.Emit(EventPlaylistsChanged, ev)
}
