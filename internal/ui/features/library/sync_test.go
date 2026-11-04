package library

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The sync store's rules (v0.2.0's sync.test.ts).
func TestSyncStoreRules(t *testing.T) {
	prog := func(domain string, done, total int, finished bool) services.SyncProgress {
		return services.SyncProgress{Domain: domain, Done: done, Total: total, Finished: finished}
	}
	finished := services.SyncDone{OK: true, FinishedAt: 1}

	t.Run("stops after a first full sync, whatever domain it ended on", func(t *testing.T) {
		var s syncStore
		for _, p := range []services.SyncProgress{
			prog("artists", 200, 414, false), prog("artists", 414, 414, true), prog("albums", 653, 653, true),
			prog("songs", 1075, 1075, false), prog("songs", 1075, 1075, true), prog("playlists", 25, 25, true),
			prog("playlist-tracks", 25, 25, true), prog("ratings", 1753, 1753, true),
		} {
			s.progress(p)
		}
		if running, _, _, _ := s.state(); !running {
			t.Fatal("not running mid-sync")
		}
		s.done(finished)
		running, cur, last, _ := s.state()
		if running || cur != nil || last == nil || *last != finished {
			t.Errorf("after done: running=%v current=%v last=%v", running, cur, last)
		}
	})
	t.Run("drops a status answer that sync:done overtook", func(t *testing.T) {
		var s syncStore
		s.progress(prog("songs", 400, 1075, false))
		at := s.count()
		s.done(finished)
		s.snapshot(true, at)
		if running, _, _, _ := s.state(); running {
			t.Error("a stale answer restarted the indicator")
		}
	})
	t.Run("drops a status answer that progress overtook", func(t *testing.T) {
		var s syncStore
		at := s.count()
		s.progress(prog("albums", 100, 653, false))
		s.snapshot(false, at)
		if running, cur, _, _ := s.state(); !running || cur == nil || cur.Domain != "albums" || cur.Done != 100 {
			t.Errorf("running=%v current=%v", running, cur)
		}
	})
	t.Run("follows a status answer no event overtook", func(t *testing.T) {
		var s syncStore
		s.snapshot(true, s.count())
		if running, cur, _, _ := s.state(); !running || cur != nil {
			t.Errorf("running=%v current=%v", running, cur)
		}
		s.progress(prog("songs", 1075, 1075, true))
		s.snapshot(false, s.count())
		if running, cur, _, _ := s.state(); running || cur != nil {
			t.Errorf("running=%v current=%v", running, cur)
		}
	})
}

// syncAPI answers the sync calls of the indicator and records Sync.
type syncAPI struct {
	libraryAPI
	mu       sync.Mutex
	status   services.SyncStatus
	syncs    int
	syncErr  error
	release  chan struct{} // holds SyncStatus until closed when set
	statuses int
}

func (a *syncAPI) SyncStatus(ctx context.Context) (services.SyncStatus, error) {
	a.mu.Lock()
	st, rel := a.status, a.release
	a.statuses++
	if a.release != nil && a.statuses > 1 {
		// Only the first answer is held; later ones are Go's current word.
		st, rel = services.SyncStatus{}, nil
	}
	a.mu.Unlock()
	if rel != nil {
		<-rel
	}
	return st, nil
}

func (a *syncAPI) Sync() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.syncs++
	return a.syncErr
}

func (a *syncAPI) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.syncs
}

// accountAPI is an auth service in one state.
type accountAPI struct{ state string }

func (a accountAPI) Status() services.AuthStatus { return services.AuthStatus{State: a.state} }

// syncRig is the page rig with the sync indicator as the shell mounts it.
func syncRig(t *testing.T, state string, api *syncAPI) *pageRig {
	t.Helper()
	return newPageRig(t, albumsPath, 1132, 800, func(l *library) {
		api.libraryAPI = l.lib
		l.lib = api
		l.auth = accountAPI{state}
	})
}

func TestSyncIndicatorNeedsAnAccount(t *testing.T) {
	r := syncRig(t, "signed_out", &syncAPI{})
	for i := 0; i < 20; i++ {
		r.tt.Drain()
		time.Sleep(5 * time.Millisecond)
	}
	for _, w := range []string{"Library not synced yet", "Sync now", "Synced"} {
		if r.tt.HasText(w) {
			t.Errorf("a signed-out sidebar shows %q", w)
		}
	}
}

func TestSyncIndicatorFollowsTheSync(t *testing.T) {
	api := &syncAPI{}
	r := syncRig(t, "signed_in", api)
	r.tt.Until("never synced", r.tt.Shows("Library not synced yet", "Sync now"))

	// Sync now asks Go to sync.
	if err := r.tt.Click("Sync now"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the call", func() bool { return api.count() == 1 })

	// Progress: the domain, the count and the bar.
	bus := r.h.Env.Bus
	bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "albums", Done: 3, Total: 12})
	r.tt.Until("syncing albums", r.tt.Shows("Syncing albums", "3 of 12"))
	if bar, ok := r.tt.Find("Library sync"); !ok || bar.W == 0 {
		t.Errorf("no progress bar: %v %v", bar, ok)
	}
	bus.Emit(services.EventSyncProgress, services.SyncProgress{Domain: "playlist-tracks", Done: 1, Total: 4})
	r.tt.Until("syncing playlist songs", r.tt.Shows("Syncing playlist songs", "1 of 4"))
	if r.tt.HasText("Sync now") {
		t.Error("Sync now shows while syncing")
	}

	// Done: when it finished.
	bus.Emit(services.EventSyncDone, services.SyncDone{OK: true, FinishedAt: time.Now().UnixMilli()})
	r.tt.Until("synced", r.tt.Shows("Synced just now", "Sync now"))

	// A failed sync says so and offers Try again; a cancelled one does not.
	bus.Emit(services.EventSyncDone, services.SyncDone{OK: false, ErrorCode: services.CodeCancelled, FinishedAt: 1})
	r.tt.Until("cancelled is not a failure", func() bool { return !r.tt.HasText("Library sync failed") && r.tt.HasText("Sync now") })
	bus.Emit(services.EventSyncDone, services.SyncDone{OK: false, ErrorCode: services.CodeOf(fmt.Errorf("x")), FinishedAt: 1})
	r.tt.Until("failed", r.tt.Shows("Library sync failed", "Try again"))
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the second call", func() bool { return api.count() == 2 })
}

func TestSyncIndicatorShowsTheLastSuccessFromGo(t *testing.T) {
	api := &syncAPI{status: services.SyncStatus{LastSuccess: time.Now().Add(-3 * time.Hour).UnixMilli()}}
	r := syncRig(t, "signed_in", api)
	r.tt.Until("synced hours ago", r.tt.Shows("Synced 3 hr. ago"))
}

func TestAStatusAnswerOvertakenByAnEventIsDropped(t *testing.T) {
	api := &syncAPI{status: services.SyncStatus{Running: true}, release: make(chan struct{})}
	r := syncRig(t, "signed_in", api)
	// The first SyncStatus is in flight (blocked): the sync ends meanwhile.
	r.tt.Until("the status asked", func() bool {
		api.mu.Lock()
		defer api.mu.Unlock()
		return api.statuses > 0
	})
	r.h.Env.Bus.Emit(services.EventSyncDone, services.SyncDone{OK: true, FinishedAt: time.Now().UnixMilli()})
	r.tt.Drain()
	close(api.release)
	for i := 0; i < 30; i++ {
		r.tt.Drain()
		time.Sleep(5 * time.Millisecond)
	}
	if running, _, _, _ := r.l.sync.state(); running {
		t.Error("the answer sync:done overtook restarted the indicator")
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{10 * time.Second, "just now"},
		{5 * time.Minute, "5 min. ago"},
		{3 * time.Hour, "3 hr. ago"},
		{24 * time.Hour, "yesterday"},
		{72 * time.Hour, "3 days ago"},
	} {
		if got := relativeTime(now.Add(-c.ago).UnixMilli(), now); got != c.want {
			t.Errorf("%v ago: %q, want %q", c.ago, got, c.want)
		}
	}
	if got := relativeTime(0, now); got != "never" {
		t.Errorf("no stamp: %q", got)
	}
}

// The domain table of v0.2.0's invalidations.test.ts.
func TestSyncInvalidationsPerDomain(t *testing.T) {
	of := func(domain string) string {
		return fmt.Sprint(syncInvalidations(services.SyncProgress{Domain: domain, Finished: true}))
	}
	for domain, want := range map[string][]string{
		"playlist-tracks": {"[library playlist]", "[library counts]", "[library sync-status]"},
		"ratings":         {"[library favorite-songs]", "[library state]"},
		"albums":          {"[library recently-added]", "[library album]"},
		"songs":           {"[library favorite-songs]", "[library songs]"},
	} {
		for _, w := range want {
			if !strings.Contains(of(domain), w) {
				t.Errorf("%s finished does not invalidate %s: %s", domain, w, of(domain))
			}
		}
	}
	if strings.Contains(of("playlist-tracks"), "[library playlists]") {
		t.Error("playlist tracks invalidate the playlists list")
	}
	if strings.Contains(of("ratings"), "recently-added") || strings.Contains(of("albums"), "favorite-songs") {
		t.Error("a domain invalidates another's lists")
	}
}

// The module's routes and sidebar (index.ts, made-for-you-redirect.test.ts).
func TestModuleRoutesAndNav(t *testing.T) {
	env := uitest.Env(t, uitest.Bare())
	l := newLibrary(env, nil, nil, nil)
	m := l.module()
	var paths []string
	for _, n := range m.Nav {
		paths = append(paths, fmt.Sprintf("%d %s %s %s", n.Order, n.Label(), n.Icon, n.To))
	}
	want := []string{
		"5 Recently added clock-3 /library/recently-added", "10 Artists mic-vocal /library/artists",
		"20 Albums disc-3 /library/albums", "30 Songs music /library/songs",
		"40 Music videos clapperboard /library/music-videos", "60 Favourite songs star /library/favourites",
		"65 History history /library/history", "70 Playlists list-music /library/playlists",
	}
	if strings.Join(paths, "\n") != strings.Join(want, "\n") {
		t.Errorf("nav:\n%s\nwant:\n%s", strings.Join(paths, "\n"), strings.Join(want, "\n"))
	}
	routes := map[string]bool{}
	for _, r := range m.Routes {
		routes[r.Pattern] = true
	}
	for _, p := range []string{recentPath, artistsPath, albumsPath, songsPath, videosPath, favouritePath, historyPath, playlistsPath, madeForYou} {
		if !routes[p] {
			t.Errorf("no route %s", p)
		}
	}
	for _, n := range m.Nav {
		if n.To == madeForYou {
			t.Error("Made for you is in the sidebar")
		}
	}
	if m.Slots.SidebarFooter == nil {
		t.Error("no sync indicator slot")
	}
}

func TestMadeForYouRedirectsHome(t *testing.T) {
	env := uitest.Env(t, uitest.Bare())
	router := ui.NewRouter(madeForYou)
	env.Nav = router
	l := newLibrary(env, nil, nil, nil)
	m := l.module()
	tt := uitest.Tester(t, env, func(c *ui.Context) {
		router.View(c, func(rt *ui.Route) {
			for _, r := range m.Routes {
				if rt.Match(r.Pattern) {
					r.Build(c, rt)
				}
			}
		})
	}, 400, 300)
	tt.Drain()
	if got := router.Location(); got != "/" {
		t.Errorf("opened %q, want Home", got)
	}
}

// The URL's sort, order and type are validated as v0.2.0's zod schemas do.
func TestListQueriesDropUnknownValues(t *testing.T) {
	router := ui.NewRouter("/")
	parse := func(spec listSpec, loc string) listQuery {
		router.Replace(loc)
		var q listQuery
		env := uitest.Env(t, uitest.Bare())
		env.Nav = router
		tt := uitest.Tester(t, env, func(c *ui.Context) {
			router.View(c, func(rt *ui.Route) { q = spec.parse(rt) })
		}, 100, 100)
		tt.Drain()
		return q
	}
	for _, c := range []struct {
		spec listSpec
		loc  string
		want listQuery
	}{
		{albumsSpec, albumsPath, listQuery{sort: "dateAdded", desc: true}},
		{albumsSpec, albumsPath + "?sort=bogus&desc=maybe&type=weird", listQuery{sort: "dateAdded", desc: true}},
		{albumsSpec, albumsPath + "?sort=title&type=singles", listQuery{sort: "title", kind: "singles"}},
		{albumsSpec, albumsPath + "?sort=releaseDate&desc=false", listQuery{sort: "releaseDate"}},
		{songsSpec, songsPath + "?sort=album", listQuery{sort: "album"}},
		{songsSpec, songsPath + "?sort=dateAdded", listQuery{sort: "dateAdded"}}, // songs never default descending
		{songsSpec, songsPath + "?sort=releaseDate", listQuery{sort: "title"}},   // not a song order
		{playlistsSpec, playlistsPath + "?sort=lastModified", listQuery{sort: "lastModified", desc: true}},
		{playlistsSpec, playlistsPath + "?sort=title&type=singles", listQuery{sort: "title"}},
	} {
		if got := parse(c.spec, c.loc); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.loc, got, c.want)
		}
	}
}
