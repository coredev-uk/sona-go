package home

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeAMP is Apple Music's API as far as Home reads it: the trimmed real
// recommendations response the services' own tests use. Anything else is
// not found; it never reaches Apple.
type fakeAMP struct {
	t    *testing.T
	mu   sync.Mutex
	seen []string
}

func (f *fakeAMP) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.seen = append(f.seen, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	if r.URL.Path != "/v1/me/recommendations" || r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "services", "testdata", "discover", "home.json"))
	if err != nil {
		f.t.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(b)
}

func (f *fakeAMP) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, s := range f.seen {
		if s == "GET "+path {
			n++
		}
	}
	return n
}

// TestHomeOverTheRealDiscoverService runs the module as the app builds it, over
// the real services and Apple's recorded response: the lead, the row shelf,
// the hero and the feature shelf of an account's Home, the nav entry and the
// routes.
func TestHomeOverTheRealDiscoverService(t *testing.T) {
	amp := &fakeAMP{t: t}
	h := uitest.New(t, uitest.Apple(amp))
	env := new(module.Env)
	*env = *h.Env
	router := ui.NewRouter("/")
	env.Nav = router
	env.HasRoute = func(string) bool { return true }
	env.Surfaces = store.NewSurfaces()
	played := &recorder{}
	for _, kind := range []string{"album", "playlist", "station"} {
		env.Players.Register(kind, func(it services.Item) { played.add(it.Kind + ":" + it.Name) })
	}
	m := New(env)

	if m.ID != "home" {
		t.Errorf("module ID %q", m.ID)
	}
	if len(m.Nav) != 1 || m.Nav[0].To != "/" || m.Nav[0].Icon != "sparkles" || m.Nav[0].Order != 10 || !m.Nav[0].Exact ||
		m.Nav[0].Section != module.NavPrimary || m.Nav[0].Label() != "For you" {
		t.Errorf("nav %+v", m.Nav)
	}
	var patterns []string
	for _, r := range m.Routes {
		patterns = append(patterns, r.Pattern)
	}
	if !slices.Equal(patterns, []string{"/", "/for-you/{id}"}) {
		t.Errorf("routes %q", patterns)
	}
	m.Init()

	tt := h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		ui.Column(c).Fill().Children(func() {
			router.View(c, func(rt *ui.Route) {
				for _, route := range m.Routes {
					if rt.Match(route.Pattern) {
						route.Build(c, rt)
						return
					}
				}
			})
		})
	}, 1132, 3000)

	tt.Until("Home", tt.Shows("For you", "Top Picks for You", "New Music", "Grand Theft Auto VI: The Album", "Hits Station",
		"Replay: Your Top Music", "Replay All Time", "Recently Played", "Apple Music 1", "Parachutes"))
	// The concerts shelf is the concerts feature's: nothing stands in for it.
	if tt.HasText("Concerts") {
		t.Error("a concerts placeholder is shown")
	}
	if tt.HasText("Recommendations need Apple Music") {
		t.Error("a signed-in Home shows the sign-in notice")
	}
	// The lead's play control plays the first pick through the item players.
	if err := tt.Click("Play New Music"); err != nil {
		t.Fatal(err)
	}
	tt.Drain()
	if got := played.all(); !slices.Equal(got, []string{"playlist:New Music"}) {
		t.Errorf("played %q", got)
	}
	// Recently Played has a page of its own: See all leads to it.
	if err := tt.Click("See all"); err != nil {
		t.Fatal(err)
	}
	tt.Drain()
	if got := router.Path(); got != "/for-you/7-2Tqlz47h9yro" {
		t.Errorf("See all went to %q", got)
	}
	if n := amp.count("/v1/me/recommendations"); n != 1 {
		t.Errorf("Apple was asked for Home %d times, want once (the cache serves the rest)", n)
	}
}

// TestRefreshAsksAppleAgain runs Refresh over the real service.
func TestRefreshAsksAppleAgain(t *testing.T) {
	amp := &fakeAMP{t: t}
	h := uitest.New(t, uitest.Apple(amp))
	env := new(module.Env)
	*env = *h.Env
	router := ui.NewRouter("/")
	env.Nav = router
	env.HasRoute = func(string) bool { return true }
	env.Surfaces = store.NewSurfaces()
	m := New(env)
	m.Init()
	tt := h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		ui.Column(c).Fill().Children(func() { m.Routes[0].Build(c, nil) })
	}, 1132, 3000)
	tt.Until("Home", tt.Shows("Top Picks for You", "Refresh"))
	before := amp.count("/v1/me/recommendations")
	if err := tt.Click("Refresh"); err != nil {
		t.Fatal(err)
	}
	tt.Until("a second fetch", func() bool { return amp.count("/v1/me/recommendations") > before && !tt.HasText("Refreshing…") })
	if !tt.HasText("Top Picks for You") {
		t.Errorf("the page is gone after a refresh; texts %q", tt.Texts())
	}
}
