package radio

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// fakeRadio is the RadioService of a test: what each call answers, and what
// was asked of it.
type fakeRadio struct {
	mu sync.Mutex

	grouping    services.EditorialPage
	groupingErr error
	live        []services.Station
	liveErr     error
	recent      []services.Station
	recentErr   error
	details     map[string]services.StationDetail
	detailErr   error
	playErr     error
	createErr   error
	created     services.Station

	// hold, when not nil, keeps every read waiting until it is closed.
	hold chan struct{}

	asked   map[string]int
	played  []string
	creates [][2]string
}

func (f *fakeRadio) wait(ctx context.Context, what string) {
	f.mu.Lock()
	f.asked[what]++
	hold := f.hold
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
		}
	}
}

func (f *fakeRadio) Grouping(ctx context.Context) (services.EditorialPage, error) {
	f.wait(ctx, "grouping")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.grouping, f.groupingErr
}

func (f *fakeRadio) Live(ctx context.Context) ([]services.Station, error) {
	f.wait(ctx, "live")
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.live), f.liveErr
}

func (f *fakeRadio) Recent(ctx context.Context) ([]services.Station, error) {
	f.wait(ctx, "recent")
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.recent), f.recentErr
}

func (f *fakeRadio) Station(ctx context.Context, id string) (services.StationDetail, error) {
	f.wait(ctx, "station "+id)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.detailErr != nil {
		return services.StationDetail{}, f.detailErr
	}
	d, ok := f.details[id]
	if !ok {
		return services.StationDetail{}, errors.New("no such station")
	}
	return d, nil
}

func (f *fakeRadio) Play(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.played = append(f.played, id)
	return f.playErr
}

func (f *fakeRadio) CreateStation(_ context.Context, seed, catalogID string) (services.Station, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.creates = append(f.creates, [2]string{seed, catalogID})
	return f.created, f.createErr
}

func (f *fakeRadio) set(fn func(*fakeRadio)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *fakeRadio) plays() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.played)
}

func (f *fakeRadio) count(what string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.asked[what]
}

// fakeAuth is the sign-in state of a test.
type fakeAuth struct{ state string }

func (a *fakeAuth) Status() services.AuthStatus { return services.AuthStatus{State: a.state} }

// fakePlayback is the PlaybackService of a test.
type fakePlayback struct {
	mu      sync.Mutex
	toggled int
	undone  int
}

func (p *fakePlayback) Toggle(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.toggled++
	return nil
}

func (p *fakePlayback) UndoReplace(context.Context) (bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.undone++
	return true, nil
}

func (p *fakePlayback) counts() (toggled, undone int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.toggled, p.undone
}

// rig is the Radio and Station pages over a test Env, with fake services.
type rig struct {
	t      *testing.T
	h      *uitest.Harness
	r      *radio
	api    *fakeRadio
	pb     *fakePlayback
	auth   *fakeAuth
	router *ui.Router
	tt     *uitest.Session
	// now is the moment the pages read schedules against.
	now time.Time
}

// base is the top of the hour a test's schedules hang from: a fixed local
// time, never the wall clock.
func base() time.Time { return time.Date(2026, 11, 4, 10, 0, 0, 0, time.Local) }

// appleBase is base for the tests over the real services, which drop a
// programme that has ended by the real clock: far ahead of it.
func appleBase() time.Time { return time.Date(2099, 11, 4, 10, 0, 0, 0, time.Local) }

// newRig builds the pages over a test Env. setup changes the fakes and the
// clock before the first frame: the tester draws one as it is made, which
// asks the queries, and their answers are kept.
func newRig(t *testing.T, path string, width, height int, setup ...func(*rig)) *rig {
	t.Helper()
	kit.CloseToast("")
	h := uitest.New(t, uitest.Empty())
	rg := &rig{
		t: t, h: h, api: &fakeRadio{asked: map[string]int{}, details: map[string]services.StationDetail{}},
		pb: &fakePlayback{}, auth: &fakeAuth{state: "signed_out"}, router: ui.NewRouter(path), now: base().Add(15 * time.Minute),
	}
	env := h.Env
	env.Nav = rg.router
	env.HasRoute = func(p string) bool { return true }
	rg.r = newRadio(env, rg.api, rg.pb, rg.auth, nil)
	rg.r.now = func() time.Time { return rg.now }
	env.Actions.Register(rg.r.actions()...)
	rg.r.init()
	for _, f := range setup {
		f(rg)
	}
	rg.tt = h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen clock never finishes a transition: every input would cost 20 frames
		ui.Column(c).Fill().Children(func() {
			rg.router.View(c, func(rt *ui.Route) {
				switch {
				case rt.Match(radioPath):
					rg.r.radioView(c, rt)
				case rt.Match(stationPath):
					rg.r.stationView(c, rt)
				}
			})
		})
		kit.Toaster(c)
		kit.ConfirmHost(c)
	}, width, height)
	return rg
}

// playing says, as Go does through the bus, that station id plays (a live
// stream: it has no duration). Events Go queued before (the coordinator's
// first state) are delivered first, so the mirror ends as this says.
func (rg *rig) playing(id string, state string) {
	rg.playState(services.PlaybackState{
		State: state, Index: 0,
		Current: &services.QueueItem{ID: id, CatalogID: id, Name: id, Live: true},
	})
}

// playState sends a playback state and the queue that goes with it.
func (rg *rig) playState(s services.PlaybackState) {
	rg.h.Bus.Emit(services.EventPlaybackQueue, services.QueueState{})
	rg.h.Bus.Emit(services.EventPlaybackState, s)
	rg.tt.Drain()
}

// advance moves the pages' clock and the tester's together.
func (rg *rig) advance(d time.Duration) {
	rg.now = rg.now.Add(d)
	rg.tt.Advance(d)
	rg.tt.Drain()
}

// texts are the strings of the last frame that equal one of names, in the
// order the page shows them.
func (rg *rig) order(names ...string) []string {
	var got []string
	for _, w := range rg.tt.Texts() {
		if slices.Contains(names, w) && !slices.Contains(got, w) {
			got = append(got, w)
		}
	}
	return got
}

// count is how many times the frame shows text s exactly.
func (rg *rig) count(s string) int {
	n := 0
	for _, w := range rg.tt.Texts() {
		if w == s {
			n++
		}
	}
	return n
}

func (rg *rig) click(s string) {
	rg.t.Helper()
	if err := rg.tt.Click(s); err != nil {
		rg.t.Fatalf("click %q: %v (texts %q)", s, err, rg.tt.Texts())
	}
	rg.tt.Drain()
}

// toastShown draws frames until a toast titled s shows.
func (rg *rig) toastShown(s string) {
	rg.t.Helper()
	rg.tt.Until("a toast "+s, rg.tt.Shows(s))
}

// Fixtures.

func liveStation(id, name string, extra ...func(*services.Station)) services.Station {
	s := services.Station{
		ID: id, Name: name, Kind: services.StationLive, HasPage: true, Playable: true,
		Tagline: name + " tagline", Art: services.Art{Small: "/art/" + id + "/s.jpg", Medium: "/art/" + id + "/station.jpg", Large: "/art/" + id + "/l.jpg"},
	}
	for _, f := range extra {
		f(&s)
	}
	return s
}

func stationEvent(title string, start, end time.Time, description ...string) services.StationEvent {
	e := services.StationEvent{
		ID: title + "-" + start.Format("1504"), Title: title, StartMs: start.UnixMilli(), EndMs: end.UnixMilli(),
		Art: services.Art{Small: "/art/show/s.jpg", Medium: "/art/show/show.jpg"},
	}
	if len(description) > 0 {
		e.Description = description[0]
	}
	return e
}

func stationItemOf(s services.Station) services.Item {
	return services.Item{Kind: services.ItemStation, ID: s.ID, Name: s.Name, Station: &s}
}

func itemsOf(stations ...services.Station) []services.Item {
	out := make([]services.Item, len(stations))
	for i, s := range stations {
		out[i] = stationItemOf(s)
	}
	return out
}
