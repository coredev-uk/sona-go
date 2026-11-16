package services

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
)

// remoteRig is a LibraryService whose checks only count: the schedule is
// what is under test, on the fake clock of a synctest bubble.
type remoteRig struct {
	s       *LibraryService
	begins  atomic.Int32
	ready   atomic.Bool
	focused atomic.Bool
	refuse  atomic.Bool // begin answers ErrSyncRunning
	focus   func(on bool)
}

// newRemoteRig builds the rig inside a bubble (its timers and context live
// there) over d, a database made outside it.
func newRemoteRig(t *testing.T, d *db.DB) *remoteRig {
	t.Helper()
	e := &Env{Deps: Deps{DB: d, Library: library.New(d, nil), Syncer: library.NewSyncer(d, nil, nil, 1), Logger: discard(), Now: time.Now}}
	r := &remoteRig{s: &LibraryService{o: e}}
	r.ready.Store(true)
	r.focused.Store(true)
	r.s.remote.init(r.s)
	rc := &r.s.remote
	rc.ready, rc.focused = r.ready.Load, r.focused.Load
	rc.onFocusChange = func(fn func(bool)) { r.focus = fn }
	rc.begin = func() error {
		if r.refuse.Load() {
			return library.ErrSyncRunning
		}
		r.begins.Add(1)
		return nil
	}
	t.Cleanup(rc.close)
	return r
}

// syncedDB is a database whose library finished a first sync.
func syncedDB(t *testing.T, synced bool) *db.DB {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if synced {
		for _, dom := range library.AllDomains {
			if _, err := d.W.ExecContext(t.Context(), `INSERT INTO sync_state (domain, last_success) VALUES (?, ?)`, string(dom), time.Now().UnixMilli()); err != nil {
				t.Fatal(err)
			}
		}
	}
	return d
}

func (r *remoteRig) want(t *testing.T, n int32, what string) {
	t.Helper()
	synctest.Wait()
	if got := r.begins.Load(); got != n {
		t.Fatalf("%s: %d checks, want %d", what, got, n)
	}
}

// The period runs a check only while the window is focused and Apple is in
// reach: hidden, signed out and offline are all silent.
func TestRemoteCheckPeriodicOnlyWhileFocusedAndReachable(t *testing.T) {
	d := syncedDB(t, true)
	synctest.Test(t, func(t *testing.T) {
		r := newRemoteRig(t, d)
		r.s.startRemoteChecks()
		step := remoteCheckEvery + time.Second

		r.focused.Store(false)
		time.Sleep(step)
		r.want(t, 0, "hidden")

		r.focused.Store(true)
		r.ready.Store(false) // signed out, or offline
		time.Sleep(step)
		r.want(t, 0, "not reachable")

		r.ready.Store(true)
		time.Sleep(step)
		r.want(t, 1, "focused and reachable")
		time.Sleep(step)
		r.want(t, 2, "the next period")
	})
}

// Nothing runs before the first paint opens the network (startRemoteChecks
// is OnNetwork's), and nothing before the library has synced once.
func TestRemoteCheckWaitsForTheNetworkAndTheFirstSync(t *testing.T) {
	d := syncedDB(t, true)
	synctest.Test(t, func(t *testing.T) {
		r := newRemoteRig(t, d)
		time.Sleep(3 * remoteCheckEvery)
		r.want(t, 0, "before the network opens")
		r.s.startRemoteChecks()
		time.Sleep(remoteCheckEvery + time.Second)
		r.want(t, 1, "after")
	})
	fresh := syncedDB(t, false)
	synctest.Test(t, func(t *testing.T) {
		r := newRemoteRig(t, fresh)
		r.s.startRemoteChecks()
		time.Sleep(3 * remoteCheckEvery)
		r.want(t, 0, "before the first sync")
	})
}

// Gaining the focus starts a check after a short delay, debounced and rate
// limited, and the period does not double it up.
func TestRemoteCheckOnFocusIsDebouncedAndRateLimited(t *testing.T) {
	d := syncedDB(t, true)
	synctest.Test(t, func(t *testing.T) {
		r := newRemoteRig(t, d)
		r.s.startRemoteChecks()
		if r.focus == nil {
			t.Fatal("the schedule did not follow the window's focus")
		}

		// Back from another device within a gap of start-up: too soon.
		r.focus(true)
		time.Sleep(remoteFocusDelay + time.Second)
		r.want(t, 0, "focus within a gap of the start")

		time.Sleep(remoteCheckGap)
		// Flicker: focus and blur inside the delay asks nothing.
		r.focus(true)
		time.Sleep(remoteFocusDelay / 2)
		r.focus(false)
		time.Sleep(remoteFocusDelay)
		r.want(t, 0, "focus lost before the delay")

		// Focus gained and kept: one check, however often it is re-sent.
		r.focus(true)
		r.focus(true)
		time.Sleep(remoteFocusDelay + time.Second)
		r.want(t, 1, "focus kept")

		// Away and back within a gap: rate limited.
		r.focus(false)
		time.Sleep(10 * time.Second)
		r.focus(true)
		time.Sleep(remoteFocusDelay + time.Second)
		r.want(t, 1, "focus again within a gap")

		// The window is hidden by the time the delay ends.
		time.Sleep(remoteCheckGap)
		r.focus(true)
		r.focused.Store(false)
		time.Sleep(remoteFocusDelay + time.Second)
		r.want(t, 1, "hidden before the delay ended")

		// A gap after the last check, focus checks again.
		r.focused.Store(true)
		r.focus(true)
		time.Sleep(remoteFocusDelay + time.Second)
		r.want(t, 2, "focus a gap later")
	})
}

// The period respects the gap too: a check made on focus 1m58s before the
// tick is not repeated by it.
func TestRemoteCheckPeriodRespectsTheGap(t *testing.T) {
	d := syncedDB(t, true)
	synctest.Test(t, func(t *testing.T) {
		r := newRemoteRig(t, d)
		r.s.startRemoteChecks()
		time.Sleep(3 * time.Minute)
		r.focus(true)
		time.Sleep(remoteFocusDelay)
		r.want(t, 1, "focus at 3m")
		time.Sleep(remoteCheckEvery - 3*time.Minute - remoteFocusDelay + time.Second) // past the 5m tick
		r.want(t, 1, "the tick under two minutes after")
		time.Sleep(remoteCheckEvery)
		r.want(t, 2, "the next tick")
	})
}

// A sync that is running holds a check back without using up the gap, so
// the next trigger can run.
func TestRemoteCheckSkipsWhileASyncRuns(t *testing.T) {
	d := syncedDB(t, true)
	synctest.Test(t, func(t *testing.T) {
		r := newRemoteRig(t, d)
		r.s.startRemoteChecks()
		r.refuse.Store(true)
		time.Sleep(remoteCheckEvery + time.Second)
		r.want(t, 0, "sync running")
		r.refuse.Store(false)
		time.Sleep(remoteCheckEvery)
		r.want(t, 1, "sync over")
	})
}

// Closing stops the schedule.
func TestRemoteCheckStopsOnClose(t *testing.T) {
	d := syncedDB(t, true)
	synctest.Test(t, func(t *testing.T) {
		r := newRemoteRig(t, d)
		r.s.startRemoteChecks()
		r.s.remote.close()
		time.Sleep(3 * remoteCheckEvery)
		r.want(t, 0, "after close")
		r.focus(true)
		time.Sleep(remoteCheckGap)
		r.want(t, 0, "focus after close")
	})
}

func TestPlaylistSnapshotDiff(t *testing.T) {
	look := func(name string) playlistLook { return playlistLook{name: name, tracks: 3} }
	was := playlistSnapshot{
		"p.same":    {look: look("Same"), modified: "1", tracksModified: "1"},
		"p.order":   {look: look("Order"), modified: "1", tracksModified: "1"},
		"p.rename":  {look: look("Old"), modified: "1", tracksModified: "1"},
		"p.removed": {look: look("Gone"), modified: "1", tracksModified: "1"},
	}
	next := playlistSnapshot{
		"p.same":   was["p.same"],
		"p.order":  {look: look("Order"), modified: "2", tracksModified: "2"},
		"p.rename": {look: look("New"), modified: "2", tracksModified: "2"},
		"p.added":  {look: look("Added")},
	}
	got := was.diff(next)
	if want := []string{"p.order", "p.removed", "p.rename"}; !slices.Equal(got.IDs, want) || !got.Tree {
		t.Errorf("diff = %+v, want ids %v and the tree", got, want)
	}
	// A reorder alone leaves the tree as it was.
	was = playlistSnapshot{"p.order": {look: look("Order"), modified: "1", tracksModified: "1"}}
	next = playlistSnapshot{"p.order": {look: look("Order"), modified: "2", tracksModified: "2"}}
	if got := was.diff(next); !slices.Equal(got.IDs, []string{"p.order"}) || got.Tree {
		t.Errorf("reorder diff = %+v", got)
	}
	if got := was.diff(was); len(got.IDs) != 0 || got.Tree {
		t.Errorf("no change diff = %+v", got)
	}
}

// remoteApple is the playlist endpoints of a library that changes on
// another device.
type remoteApple struct {
	mu       sync.Mutex
	stamp    string
	tracks   []string // song IDs in order
	name     string
	requests []string
}

func (f *remoteApple) song(id string) map[string]any {
	return map[string]any{"id": id, "type": "library-songs", "attributes": map[string]any{
		"name": "Song " + id, "durationInMillis": 1000, "playParams": map[string]any{"id": id, "catalogId": "c" + id},
	}}
}

func (f *remoteApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.Path)
	pl := map[string]any{"id": "p.r", "type": "library-playlists", "attributes": map[string]any{
		"name": f.name, "lastModifiedDate": f.stamp, "canEdit": true, "canDelete": true, "dateAdded": "2024-01-01T00:00:00Z",
	}}
	var data []any
	switch {
	case r.URL.Path == "/v1/me/library/playlists":
		data = []any{pl}
	case r.URL.Path == "/v1/me/library/playlists/p.r":
		data = []any{pl}
	case r.URL.Path == "/v1/me/library/playlists/p.r/tracks":
		for _, id := range f.tracks {
			data = append(data, f.song(id))
		}
	case strings.HasSuffix(r.URL.Path, "/children"):
		data = []any{map[string]any{"id": "p.r", "type": "library-playlists", "attributes": map[string]any{"name": f.name, "canDelete": true}}}
	default:
		http.NotFound(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": data, "meta": map[string]any{"total": len(data)}})
}

func (f *remoteApple) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.requests
	f.requests = nil
	return out
}

func (f *remoteApple) set(stamp string, tracks ...string) {
	f.mu.Lock()
	f.stamp, f.tracks = stamp, tracks
	f.mu.Unlock()
}

type remoteEnv struct {
	s    *LibraryService
	pl   *PlaylistsService
	api  *remoteApple
	rec  *recorder
	db   *db.DB
	lib  *library.Library
	sync *library.Syncer
}

func newRemoteEnv(t *testing.T) *remoteEnv {
	t.Helper()
	d := syncedDB(t, false)
	fake := &remoteApple{stamp: "2026-11-01T10:00:00Z", name: "Road", tracks: []string{"i.1", "i.2", "i.3"}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return "mut" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	rec := &recorder{}
	re := &remoteEnv{api: fake, rec: rec, db: d, lib: library.New(d, api), sync: library.NewSyncer(d, api, nil, 2)}
	e := &Env{Deps: Deps{DB: d, API: api, Library: re.lib, Syncer: re.sync, Emit: rec, Logger: discard(), Now: time.Now, Storefront: func() string { return "gb" }}, members: newMembership(d)}
	re.s = &LibraryService{o: e, m: e.m, signedIn: func() bool { return true }}
	re.s.remote.init(re.s)
	t.Cleanup(re.s.close)
	re.pl = &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}, checked: map[string]time.Time{}, reachable: func() bool { return true }}
	// The last sync: the playlist as it was.
	if err := re.sync.Sync(t.Context(), library.Full, nil, library.DomainPlaylists, library.DomainPlaylistTracks); err != nil {
		t.Fatal(err)
	}
	fake.take()
	return re
}

func (re *remoteEnv) storedOrder(t *testing.T) []string {
	t.Helper()
	_, songs, err := re.lib.AllPlaylist(t.Context(), "p.r")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range songs {
		out = append(out, s.LibraryID)
	}
	return out
}

// until waits for cond (events come from the sync's goroutine).
func until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

// A check is a quiet incremental sync of the playlists: unchanged, it reads
// the list and says nothing; after a reorder elsewhere it refetches that
// playlist's tracks, announces just it, and shows no sync progress.
func TestRemoteCheckRefreshesAChangedPlaylistQuietly(t *testing.T) {
	re := newRemoteEnv(t)
	run := func() {
		t.Helper()
		if err := re.s.start(syncRun{mode: library.Incremental, domains: remoteDomains, quiet: true}); err != nil {
			t.Fatal(err)
		}
		until(t, "the check to end", func() bool {
			re.s.mu.Lock()
			defer re.s.mu.Unlock()
			return re.s.syncDone == nil
		})
	}

	run()
	if got := re.api.take(); !slices.Equal(got, []string{"/v1/me/library/playlists"}) {
		t.Errorf("an unchanged library cost %v, want the playlist list alone", got)
	}
	re.rec.mu.Lock()
	if n := len(re.rec.events); n != 0 {
		t.Errorf("an unchanged check emitted %v", re.rec.events)
	}
	re.rec.mu.Unlock()

	re.api.set("2026-11-02T10:00:00Z", "i.3", "i.2", "i.1") // reordered on the phone
	run()
	if got := re.api.take(); !slices.Contains(got, "/v1/me/library/playlists/p.r/tracks") || len(got) > 6 {
		t.Errorf("a reorder cost %v", got)
	}
	if got := re.storedOrder(t); !slices.Equal(got, []string{"i.3", "i.2", "i.1"}) {
		t.Errorf("stored order %v", got)
	}
	until(t, "playlists:changed", func() bool { return re.rec.count(EventPlaylistsChanged) == 1 })
	re.rec.mu.Lock()
	defer re.rec.mu.Unlock()
	if !slices.Equal(re.rec.events, []string{EventPlaylistsChanged}) {
		t.Fatalf("events %v: a check must not announce sync progress or completion", re.rec.events)
	}
	if ev := re.rec.data[0].(PlaylistsChanged); !slices.Equal(ev.IDs, []string{"p.r"}) || ev.Tree {
		t.Errorf("announced %+v, want p.r alone and the tree untouched", ev)
	}
}

// Opening a playlist: Apple's stamp moved, so the tracks are refetched and
// the page told; unchanged, one request and nothing else.
func TestCheckRemotePlaylist(t *testing.T) {
	re := newRemoteEnv(t)
	ctx := t.Context()
	changed, err := re.pl.CheckRemote(ctx, "p.r")
	if err != nil || changed {
		t.Fatalf("unchanged: %v, %v", changed, err)
	}
	if got := re.api.take(); !slices.Equal(got, []string{"/v1/me/library/playlists/p.r"}) {
		t.Errorf("an unchanged playlist cost %v", got)
	}
	if re.rec.count(EventPlaylistsChanged) != 0 {
		t.Error("an unchanged playlist was announced")
	}

	re.api.set("2026-11-02T10:00:00Z", "i.2", "i.3", "i.1")
	re.pl.checked = map[string]time.Time{} // past the per-playlist gap
	changed, err = re.pl.CheckRemote(ctx, "p.r")
	if err != nil || !changed {
		t.Fatalf("changed: %v, %v", changed, err)
	}
	if got := re.storedOrder(t); !slices.Equal(got, []string{"i.2", "i.3", "i.1"}) {
		t.Errorf("stored order %v", got)
	}
	if re.rec.count(EventPlaylistsChanged) != 1 {
		t.Errorf("events %v", re.rec.events)
	}
	re.api.take()

	// Stored now: asking again after the gap finds nothing to do but ask.
	re.pl.checked = map[string]time.Time{}
	if changed, err := re.pl.CheckRemote(ctx, "p.r"); err != nil || changed {
		t.Fatalf("again: %v, %v", changed, err)
	}
	if got := re.api.take(); len(got) != 1 {
		t.Errorf("a stored playlist cost %v", got)
	}
}

// One ask per playlist per remotePlaylistGap; nothing signed out or offline;
// only library playlists.
func TestCheckRemoteRateLimitAndReach(t *testing.T) {
	re := newRemoteEnv(t)
	ctx := t.Context()
	if _, err := re.pl.CheckRemote(ctx, "p.r"); err != nil {
		t.Fatal(err)
	}
	re.api.take()
	re.api.set("2026-11-02T10:00:00Z", "i.3", "i.2", "i.1")
	if changed, err := re.pl.CheckRemote(ctx, "p.r"); err != nil || changed {
		t.Fatalf("within the gap: %v, %v", changed, err)
	}
	if got := re.api.take(); len(got) != 0 {
		t.Errorf("within the gap Apple was asked %v", got)
	}

	re.pl.checked = map[string]time.Time{}
	re.pl.reachable = func() bool { return false }
	if changed, err := re.pl.CheckRemote(ctx, "p.r"); err != nil || changed {
		t.Fatalf("offline: %v, %v", changed, err)
	}
	if got := re.api.take(); len(got) != 0 {
		t.Errorf("offline Apple was asked %v", got)
	}
	if _, err := re.pl.CheckRemote(ctx, "pl.catalog"); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("a catalogue playlist: %v", err)
	}
}

// An edit made here stores Apple's stamp, so a check after it finds the
// playlist as it is and does not overwrite it with a refetch.
func TestCheckRemoteLeavesALocalEditAlone(t *testing.T) {
	re := newRemoteEnv(t)
	ctx := t.Context()
	// The edit in flight holds the playlist; the check waits behind it.
	unlock := re.pl.lock("p.r")
	done := make(chan bool, 1)
	go func() {
		changed, _ := re.pl.CheckRemote(ctx, "p.r")
		done <- changed
	}()
	select {
	case <-done:
		t.Fatal("the check ran while an edit held the playlist")
	case <-time.After(100 * time.Millisecond):
	}
	// The edit lands: Apple's list, our copy and the stamp agree.
	re.api.set("2026-11-03T10:00:00Z", "i.2", "i.1", "i.3")
	if err := re.lib.StorePlaylistTracks(ctx, "p.r", "2026-11-03T10:00:00Z", songsOf(re.api, "i.2", "i.1", "i.3")); err != nil {
		t.Fatal(err)
	}
	re.api.take()
	unlock()
	if changed := <-done; changed {
		t.Error("the check overwrote an edit that Apple's stamp already agreed with")
	}
	if got := re.api.take(); slices.Contains(got, "/v1/me/library/playlists/p.r/tracks") {
		t.Errorf("the check refetched the tracks: %v", got)
	}
	if got := re.storedOrder(t); !slices.Equal(got, []string{"i.2", "i.1", "i.3"}) {
		t.Errorf("stored order %v", got)
	}
}

func songsOf(f *remoteApple, ids ...string) []applemusic.Song {
	var out []applemusic.Song
	for _, id := range ids {
		b, _ := json.Marshal(f.song(id))
		var s applemusic.Song
		_ = json.Unmarshal(b, &s)
		out = append(out, s)
	}
	return out
}
