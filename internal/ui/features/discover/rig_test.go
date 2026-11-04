package discover

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// testdata is where the services' tests keep the trimmed real Apple
// responses; no catalogue snapshot is committed here.
const testdata = "../../../services/testdata/discover"

// fakeApple answers the Apple Music API in process with the trimmed
// responses of the services' tests, and notes the requests it saw.
type fakeApple struct {
	t      *testing.T
	mu     sync.Mutex
	seen   []string
	status int // non-zero: every request fails with it
}

func (f *fakeApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.seen = append(f.seen, r.URL.Path)
	status := f.status
	f.mu.Unlock()
	if status != 0 {
		http.Error(w, `{"errors":[]}`, status)
		return
	}
	file := ""
	switch p := r.URL.Path; {
	case p == "/v1/editorial/gb/groupings":
		file = "new.json"
	case p == "/v1/catalog/gb":
		file = "curator.json"
	case strings.HasPrefix(p, "/v1/editorial/gb/rooms/"):
		file = "room.json"
	case strings.HasPrefix(p, "/v1/editorial/gb/multirooms/"):
		file = "multiroom.json"
	case p == "/v1/catalog/gb/charts":
		file = "charts.json"
	case p == "/v1/catalog/gb/genres":
		_, _ = w.Write([]byte(`{"data":[{"id":"34","type":"genres","attributes":{"name":"Music"}},{"id":"14","type":"genres","attributes":{"name":"Pop","parentId":"34"}}]}`))
		return
	default:
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(filepath.Join(testdata, file))
	if err != nil {
		f.t.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}

// fakeAPI is DiscoverService with the pages a test gives it. Every call is
// noted; block holds the calls until it is closed.
type fakeAPI struct {
	mu    sync.Mutex
	calls []string
	block chan struct{}
	err   error
	// blockItems holds the calls for further pages (RoomItems, ChartItems)
	// until it is closed.
	blockItems chan struct{}

	pages     map[string]services.EditorialPage // by "new", "curator/<id>"…
	room      services.RoomPage
	roomItems func(offset int) services.ItemPage
	charts    services.ChartsPage
	chartItem func(key string, offset int) services.ItemPage
}

func (f *fakeAPI) note(call string) error {
	f.mu.Lock()
	f.calls = append(f.calls, call)
	block, err := f.block, f.err
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	return err
}

func (f *fakeAPI) waitItems() {
	f.mu.Lock()
	block := f.blockItems
	f.mu.Unlock()
	if block != nil {
		<-block
	}
}

func (f *fakeAPI) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// count is how many calls start with prefix.
func (f *fakeAPI) count(prefix string) int {
	n := 0
	for _, c := range f.asked() {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (f *fakeAPI) page(key string) (services.EditorialPage, error) {
	if err := f.note(key); err != nil {
		return services.EditorialPage{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pages[key], nil
}

func (f *fakeAPI) New(context.Context) (services.EditorialPage, error) { return f.page("new") }
func (f *fakeAPI) Curator(_ context.Context, id string) (services.EditorialPage, error) {
	return f.page("curator/" + id)
}

func (f *fakeAPI) Multiroom(_ context.Context, id string) (services.EditorialPage, error) {
	return f.page("multiroom/" + id)
}

func (f *fakeAPI) Grouping(_ context.Context, id string) (services.EditorialPage, error) {
	return f.page("grouping/" + id)
}

func (f *fakeAPI) Room(_ context.Context, id, sort string) (services.RoomPage, error) {
	if err := f.note("room/" + id + "?" + sort); err != nil {
		return services.RoomPage{}, err
	}
	room := f.room
	room.ID = id
	if sort != "" {
		room.Sort = sort
	}
	return room, nil
}

func (f *fakeAPI) RoomItems(_ context.Context, id, sort string, offset int) (services.ItemPage, error) {
	if err := f.note(fmt.Sprintf("room-items/%s?%s@%d", id, sort, offset)); err != nil {
		return services.ItemPage{}, err
	}
	f.waitItems()
	return f.roomItems(offset), nil
}

func (f *fakeAPI) Charts(_ context.Context, genre string) (services.ChartsPage, error) {
	if err := f.note("charts?" + genre); err != nil {
		return services.ChartsPage{}, err
	}
	p := f.charts
	p.Genre = genre
	return p, nil
}

func (f *fakeAPI) ChartItems(_ context.Context, key, chartID, genre string, offset int) (services.ItemPage, error) {
	if err := f.note(fmt.Sprintf("chart-items/%s/%s?%s@%d", key, chartID, genre, offset)); err != nil {
		return services.ItemPage{}, err
	}
	f.waitItems()
	return f.chartItem(key, offset), nil
}

// fakePlayback is PlaybackService: what it was asked to play.
type fakePlayback struct {
	mu      sync.Mutex
	played  []played
	shuffle []bool
	undone  int
	err     error
}

type played struct {
	refs  []services.TrackRef
	start int
	from  services.PlayFrom
}

func (f *fakePlayback) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.played = append(f.played, played{refs, start, from})
	return nil
}

func (f *fakePlayback) PlayNext(context.Context, []services.TrackRef) error { return nil }

func (f *fakePlayback) Append(context.Context, []services.TrackRef) error { return nil }

func (f *fakePlayback) Insert(context.Context, int, []services.TrackRef) error { return nil }

func (f *fakePlayback) UndoReplace(context.Context) (bool, error) {
	f.mu.Lock()
	f.undone++
	f.mu.Unlock()
	return true, nil
}

func (f *fakePlayback) SetShuffle(on bool) {
	f.mu.Lock()
	f.shuffle = append(f.shuffle, on)
	f.mu.Unlock()
}

func (f *fakePlayback) plays() []played {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.played)
}

// fakeTracks answers the songs of albums and playlists.
type fakeTracks struct {
	songs map[string][]services.Song // by "album:<id>" or "playlist:<id>"
	err   error
}

func (f fakeTracks) Tracks(_ context.Context, it services.Item) ([]services.Song, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.songs[it.Kind+":"+it.ID], nil
}

// routePatterns are the pages a test's router knows: this module's and the
// ones other features port, so a link to them is a link.
var ported = []string{"/album/{id}", "/artist/{id}", "/playlist/{id}", "/station/{id}", "/song/{id}"}

// matches is the router's pattern match on paths, without a router.
func matches(pattern, path string) bool {
	path, _, _ = strings.Cut(path, "?")
	p, q := strings.Split(strings.Trim(pattern, "/"), "/"), strings.Split(strings.Trim(path, "/"), "/")
	if len(p) != len(q) {
		return false
	}
	for i := range p {
		if !strings.HasPrefix(p[i], "{") && p[i] != q[i] {
			return false
		}
	}
	return true
}

// rig is the feature on a router over the Env of uitest, with its fakes.
type rig struct {
	t      *testing.T
	h      *uitest.Harness
	env    *module.Env
	router *ui.Router
	d      *discover
	mod    module.Module
	api    *fakeAPI
	play   *fakePlayback
	// replaceMode is the queue.replaceConfirm setting the queue commands read.
	replaceMode string
	tt          *session
	start       string
	opened      bool
}

// session is the tester of the rig. The page opens when the test first draws
// a frame, so the pages a test gives its fake after building the rig are the
// ones the first fetch sees.
type session struct {
	*uitest.Session
	r *rig
}

func (s *session) open() {
	if !s.r.opened {
		s.r.opened = true
		s.r.router.Replace(s.r.start)
	}
}

func (s *session) Until(what string, ok func() bool) { s.open(); s.Session.Until(what, ok) }
func (s *session) Frame()                            { s.open(); s.Session.Frame() }
func (s *session) Drain()                            { s.open(); s.Session.Drain() }
func (s *session) Settle()                           { s.open(); s.Session.Settle() }
func (s *session) Advance(d time.Duration)           { s.open(); s.Session.Advance(d) }

type rigOption func(*rigConfig)

type rigConfig struct {
	apple   http.Handler
	api     *fakeAPI
	tracks  tracksAPI
	noRoute []string
	dark    bool
	w, h    int
}

// withApple serves the pages from the fake Apple Music API through the real
// service.
func withApple(a http.Handler) rigOption { return func(c *rigConfig) { c.apple = a } }

// withAPI serves the pages from a fake DiscoverService.
func withAPI(a *fakeAPI) rigOption { return func(c *rigConfig) { c.api = a } }

// withoutRoutes makes these patterns pages that are not ported.
func withoutRoutes(p ...string) rigOption { return func(c *rigConfig) { c.noRoute = p } }

// withTracks gives the songs of albums and playlists.
func withTracks(t tracksAPI) rigOption { return func(c *rigConfig) { c.tracks = t } }

// withDark runs the theme dark.
func withDark() rigOption { return func(c *rigConfig) { c.dark = true } }

// withSize sets the window.
func withSize(w, h int) rigOption { return func(c *rigConfig) { c.w, c.h = w, h } }

// newRig builds the module at start. The window is 1132 wide: the content
// column is then 1132 less the gutters and the scroll bar.
func newRig(t *testing.T, start string, opts ...rigOption) *rig {
	t.Helper()
	cfg := rigConfig{w: 1132, h: 1200}
	for _, o := range opts {
		o(&cfg)
	}
	var uopts []uitest.Option
	if cfg.apple != nil {
		uopts = append(uopts, uitest.Apple(cfg.apple))
	}
	if cfg.dark {
		uopts = append(uopts, uitest.Dark())
	}
	h := uitest.New(t, uopts...)
	env := h.Env
	// The shell's motion policy: visible, nothing covering the page.
	env.Motion = kit.NewMotionPolicy(h.Bus, h.Main.Post, kit.MotionOptions{})
	t.Cleanup(env.Motion.Close)
	r := &rig{t: t, h: h, env: env, router: ui.NewRouter("/blank"), play: &fakePlayback{}, start: start}
	env.Nav = r.router
	r.d = newFor(env)
	r.mod = r.d.module()
	patterns := append([]string(nil), ported...)
	for _, route := range r.mod.Routes {
		patterns = append(patterns, route.Pattern)
	}
	patterns = slices.DeleteFunc(patterns, func(p string) bool { return slices.Contains(cfg.noRoute, p) })
	env.HasRoute = func(path string) bool {
		return slices.ContainsFunc(patterns, func(p string) bool { return matches(p, path) })
	}
	r.d.now = h.Clock.Now
	env.Queue = kit.NewQueueCommands(kit.QueueDeps{Svc: r.play, Play: env.Play, Post: env.Post, ReplaceMode: func() string { return r.replaceMode }})
	if cfg.api != nil {
		r.api = cfg.api
		r.d.api = cfg.api
	}
	if cfg.tracks != nil {
		r.d.tracks = cfg.tracks
	}
	r.tt = &session{r: r, Session: h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // transitions never finish on the frozen clock: each would cost frames
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				for _, route := range r.mod.Routes {
					if rt.Match(route.Pattern) {
						route.Build(c, rt)
						return
					}
				}
			})
		})
		kit.ConfirmHost(c)
		kit.Toaster(c)
	}, cfg.w, cfg.h)}
	return r
}

// settle draws frames until the picture stops changing.
func (r *rig) settle() { r.tt.Settle() }

// path is where the router is, with its query.
func (r *rig) path() string { return r.router.Location() }

// reduceMotion sets the setting for the test.
func reduceMotion(t *testing.T, on bool) {
	t.Helper()
	theme.SetReducedMotion(on)
	t.Cleanup(func() { theme.SetReducedMotion(false) })
}

// reduceEffects sets the Reduce effects setting for the test.
func reduceEffects(t *testing.T, on bool) {
	t.Helper()
	theme.SetReduceEffects(on)
	t.Cleanup(func() { theme.SetReduceEffects(false) })
}

// advance moves the tester's clock on by d, a second at a time so that what
// is asked for in a frame (a deadline, an animation frame) runs in order.
func (r *rig) advance(d time.Duration) {
	r.t.Helper()
	for d > 0 {
		step := min(d, time.Second)
		r.tt.Advance(step)
		d -= step
	}
}
