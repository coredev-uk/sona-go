package newreleases

import (
	"context"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// now is the clock of the page: noon on Sunday 15 November 2026.
var now = time.Date(2026, time.November, 15, 12, 0, 0, 0, time.Local)

// fakeAPI is ArtistTrackerService without the database: it records what the
// page asked.
type fakeAPI struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeAPI) record(s string) {
	f.mu.Lock()
	f.calls = append(f.calls, s)
	f.mu.Unlock()
}

func (f *fakeAPI) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeAPI) MarkRead(_ context.Context, id string) error { f.record("read " + id); return nil }
func (f *fakeAPI) MarkAllRead(context.Context) error           { f.record("read-all"); return nil }
func (f *fakeAPI) Dismiss(_ context.Context, id string) error  { f.record("dismiss " + id); return nil }
func (f *fakeAPI) Clear(context.Context) error                 { f.record("clear"); return nil }

// coverOf is the generated cover of a release: the fixture's, which its CDN
// serves to the artwork cache.
func coverOf(id string) services.Art {
	n, _ := strconv.Atoi(id)
	t := fixture.Template(n)
	return services.Art{Small: t, Medium: t, Large: t}
}

// release is a tracked release by an invented artist.
func release(id, name, artist, kind, date string, upcoming, read bool) services.NewRelease {
	day, err := time.ParseInLocation(time.DateOnly, date, time.Local)
	if err != nil {
		panic(err)
	}
	return services.NewRelease{
		Album: services.Album{
			ID: id, CatalogID: id, Source: services.SourceCatalog, Name: name, Artist: artist, ReleaseDate: date,
			ReleaseAt: day.UnixMilli(), Prerelease: upcoming, Single: kind != services.ReleaseAlbum,
			Art: coverOf(id),
		},
		Kind: kind, FoundAt: now.Add(-time.Hour).UnixMilli(), Read: read,
	}
}

// sample is a list of every kind of row: one still to come, two out this
// week (one read), an EP and a single, and an older album.
func sample() services.NewReleases {
	items := []services.NewRelease{
		release("303", "Night Signals - EP", "Mia Vale", services.ReleaseEP, "2026-11-14", false, false),
		release("304", "Paper Boats", "Hollow Pines", services.ReleaseSingle, "2026-11-13", false, true),
		release("302", "Lantern Season", "The Quiet Hours", services.ReleaseAlbum, "2026-11-06", false, false),
		release("301", "Slow Harbour", "Mia Vale", services.ReleaseAlbum, "2026-10-30", false, true),
	}
	soon := release("305", "Winter Rooms", "Hollow Pines", services.ReleaseAlbum, "2026-11-27", true, false)
	items = append([]services.NewRelease{soon}, items...)
	n := services.NewReleases{Items: items, CheckedAt: now.Add(-3 * time.Hour).UnixMilli()}
	for _, r := range items {
		if !r.Read {
			n.Unread++
		}
	}
	return n
}

type rig struct {
	t      *testing.T
	h      *uitest.Harness
	n      *newReleases
	api    *fakeAPI
	router *ui.Router
	tt     *uitest.Session
}

// newRig is the page over a list and whether the tracker is on, in a w×h
// window. Album pages are registered so that a row opens one.
func newRig(t *testing.T, list services.NewReleases, on bool, w, h int, opts ...uitest.Option) *rig {
	t.Helper()
	return buildRig(t, path, list, on, w, h, opts...)
}

// newShelfRig is For You's shelf over a list, standing where the page's
// route is not: its "See all" opens the page in the same window.
func newShelfRig(t *testing.T, list services.NewReleases, on bool, w, h int, opts ...uitest.Option) *rig {
	t.Helper()
	return buildRig(t, "/", list, on, w, h, opts...)
}

func buildRig(t *testing.T, start string, list services.NewReleases, on bool, w, h int, opts ...uitest.Option) *rig {
	t.Helper()
	hn := uitest.New(t, opts...)
	hn.Env.HasRoute = func(p string) bool { return p != "" }
	r := &rig{t: t, h: hn, api: &fakeAPI{}, router: ui.NewRouter(start)}
	hn.Env.Nav = r.router
	r.n = newNewReleases(hn.Env, r.api)
	r.n.now = func() time.Time { return now }
	cfg := hn.Settings.Get()
	cfg.ArtistTracker.Enabled = on
	store.Set(hn.Env.Queries, store.KeySettings, cfg)
	store.Set(hn.Env.Queries, store.KeyNewReleases, list)
	r.n.module().Init()
	hn.Env.Actions.Register(r.n.actions()...)
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(path) {
					r.n.releasesPage(c, rt)
					return
				}
				ui.Column(c).Padding(24).Children(func() {
					r.n.forYouShelf(c, func(title string, more func(), body func()) {
						kit.Shelf(c, kit.ShelfOpts{Title: title, More: more, First: true}, body)
					})
				})
			})
			kit.Toaster(c)
		})
	}, w, h)
	return r
}

func (r *rig) until(what string, ok func() bool) {
	r.t.Helper()
	r.tt.Until(what, ok)
}

func (r *rig) click(label string) {
	r.t.Helper()
	if err := r.tt.Click(label); err != nil {
		r.t.Fatal(err)
	}
	r.tt.Drain()
}
