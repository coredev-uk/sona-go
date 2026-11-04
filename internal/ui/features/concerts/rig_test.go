package concerts

import (
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// spec is a concert of the fake Apple listing.
type spec struct {
	id, lineup, venue, city, zone, start, genre string
	artist                                      string   // the headliner's catalogue ID
	yours                                       bool     // by an artist the user follows
	near                                        bool     // within reach of the chosen place (an artist's list)
	tickets                                     []string // vendor, URL pairs
	lat, lon                                    float64
}

// fakeApple answers Apple's concert listings (the amp-api host) from specs,
// as the real ones are shaped, and keeps what it was asked.
type fakeApple struct {
	mu    sync.Mutex
	specs []spec
	seen  []url.Values
	fail  atomic.Bool
	// gate, when set, holds every hub answer until it is closed.
	gate atomic.Pointer[chan struct{}]
}

// hold makes the hub answer only when release is called.
func (f *fakeApple) hold() (release func()) {
	ch := make(chan struct{})
	f.gate.Store(&ch)
	return func() { f.gate.Store(nil); close(ch) }
}

const ampHost = "amp-api.music.apple.com"

func (f *fakeApple) set(specs ...spec) {
	f.mu.Lock()
	f.specs = specs
	f.mu.Unlock()
}

// hubs are the hub queries seen, oldest first.
func (f *fakeApple) hubs() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []url.Values
	for _, q := range f.seen {
		if q.Get("geoHashLocation") != "" {
			out = append(out, q)
		}
	}
	return out
}

func (f *fakeApple) last() url.Values {
	h := f.hubs()
	if len(h) == 0 {
		return url.Values{}
	}
	return h[len(h)-1]
}

func (f *fakeApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f.mu.Lock()
	f.seen = append(f.seen, q)
	specs := slices.Clone(f.specs)
	f.mu.Unlock()
	if f.fail.Load() {
		http.Error(w, `{"errors":[]}`, http.StatusBadRequest)
		return
	}
	if g := f.gate.Load(); g != nil && r.URL.Path == "/v1/concerts/gb/hub" {
		select {
		case <-*g:
		case <-r.Context().Done():
			// A request given up on is an error, never an empty listing the
			// service could cache under the key a refetch will ask for.
			http.Error(w, `{"errors":[]}`, http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/v1/catalog/gb/genres":
		_, _ = w.Write([]byte(genres))
	case r.URL.Path == "/v1/concerts/gb/hub":
		_ = json.NewEncoder(w).Encode(hub(specs, q))
	case strings.HasPrefix(r.URL.Path, "/v1/catalog/gb/artists/"):
		_ = json.NewEncoder(w).Encode(artistPage(specs, strings.TrimPrefix(r.URL.Path, "/v1/catalog/gb/artists/")))
	default:
		http.NotFound(w, r)
	}
}

type obj = map[string]any

func resources(specs []spec) (concerts, venues, artists obj) {
	concerts, venues, artists = obj{}, obj{}, obj{}
	for _, s := range specs {
		var tickets []obj
		for i := 0; i+1 < len(s.tickets); i += 2 {
			tickets = append(tickets, obj{"vendor": s.tickets[i], "url": s.tickets[i+1]})
		}
		concerts[s.id] = obj{"id": s.id, "type": "concerts", "attributes": obj{
			"name": s.lineup + " @ " + s.venue, "startISODateTime": s.start, "timezone": s.zone, "tickets": tickets,
		}, "relationships": obj{
			"artists": obj{"data": []obj{{"id": s.artist, "type": "artists"}}},
			"venues":  obj{"data": []obj{{"id": "v." + s.id, "type": "venues"}}},
		}}
		venues["v."+s.id] = obj{"id": "v." + s.id, "type": "venues", "attributes": obj{
			"name": s.venue, "geoLocation": obj{"latitude": s.lat, "longitude": s.lon},
			"structuredAddress": obj{"city": s.city, "countryIsoCode": "GB"},
		}}
		name := strings.SplitN(s.lineup, " with ", 2)[0]
		artists[s.artist] = obj{"id": s.artist, "type": "artists", "attributes": obj{"name": name}}
	}
	return concerts, venues, artists
}

func refs(specs []spec, keep func(spec) bool) []obj {
	out := []obj{}
	for _, s := range specs {
		if keep(s) {
			out = append(out, obj{"id": s.id, "type": "concerts"})
		}
	}
	return out
}

// hub is Apple's concerts hub for the query: the date range, genre and
// limit the page asks for are honoured.
func hub(specs []spec, q url.Values) obj {
	var kept []spec
	from, to, _ := strings.Cut(q.Get("filter[date]"), "..")
	for _, s := range specs {
		if g := q.Get("filter[genre]"); g != "" && s.genre != g {
			continue
		}
		if from != "" && (s.start[:10] < from || s.start[:10] > to) {
			continue
		}
		kept = append(kept, s)
	}
	if n, _ := strconv.Atoi(q.Get("limit[popular-concerts]")); n > 0 && len(kept) > n {
		kept = kept[:n]
	}
	containers := obj{"popular-concerts": obj{"data": refs(kept, func(spec) bool { return true })}}
	if strings.Contains(q.Get("containers"), "favorite-artists-concerts") {
		containers["favorite-artists-concerts"] = obj{"data": refs(kept, func(s spec) bool { return s.yours })}
	}
	c, v, a := resources(kept)
	return obj{"results": obj{"containers": containers}, "resources": obj{"concerts": c, "venues": v, "artists": a}}
}

// artistPage is an artist's catalogue page with their concerts' views.
func artistPage(specs []spec, id string) obj {
	var mine []spec
	for _, s := range specs {
		if s.artist == id {
			mine = append(mine, s)
		}
	}
	c, v, a := resources(mine)
	art := a[id]
	if art == nil {
		art = obj{"id": id, "type": "artists", "attributes": obj{"name": "Artist"}}
	}
	art.(obj)["meta"] = obj{"views": obj{"order": []string{"nearby-upcoming-concerts", "all-upcoming-concerts"}}}
	art.(obj)["views"] = obj{
		"nearby-upcoming-concerts": obj{"attributes": obj{"title": "Nearby Concerts"}, "data": refs(mine, func(s spec) bool { return s.near })},
		"all-upcoming-concerts":    obj{"attributes": obj{"title": "All Upcoming Concerts"}, "data": refs(mine, func(spec) bool { return true })},
	}
	return obj{"data": []obj{{"id": id, "type": "artists"}}, "resources": obj{"artists": obj{id: art}, "concerts": c, "venues": v}}
}

// genres is Apple's genre list, the filter's choices.
const genres = `{"data":[{"id":"34","type":"genres","attributes":{"name":"Music"}},` +
	`{"id":"20","type":"genres","attributes":{"name":"Alternative","parentId":"34"}},` +
	`{"id":"21","type":"genres","attributes":{"name":"Rock","parentId":"34"}}]}`

// The fixture's concerts: three shows over two months, two by followed artists.
var (
	exampleBand = spec{
		id: "ce.1", lineup: "Example Band with Opener Duo", venue: "Example Hall", city: "London", zone: "Europe/London",
		start: "2027-04-14T17:00:00Z", genre: "20", artist: "900001", lat: 51.6, lon: -0.1,
		tickets: []string{"Example Tickets", "https://tickets.example.com/e/1"},
	}
	sampleSinger = spec{
		id: "ce.2", lineup: "Sample Singer", venue: "Placeholder Arena", city: "Watford", zone: "Europe/London",
		start: "2027-04-14T19:30:00Z", genre: "21", artist: "900002", yours: true, lat: 51.65, lon: -0.4,
		tickets: []string{"First Seller", "https://first.example.com/1", "Second Seller", "https://second.example.com/2"},
	}
	anotherAct = spec{
		id: "ce.3", lineup: "Another Act", venue: "Nowhere Club", city: "Reading", zone: "Europe/London",
		start: "2027-06-01T19:00:00Z", genre: "21", artist: "900003", yours: true, lat: 51.45, lon: -0.97,
	}
)

type rig struct {
	t      *testing.T
	h      *uitest.Harness
	router *ui.Router
	s      *concerts
	tt     *uitest.Session
	amp    *fakeApple
}

// newRig shows the Concerts page over the real services, on a fake Apple
// serving specs, in a w×h window. The page is at its address.
func newRig(t *testing.T, at string, w, h int, specs ...spec) *rig {
	t.Helper()
	return newRigOpts(t, at, w, h, nil, specs...)
}

// newRigOpts is newRig with more uitest options (Dark).
func newRigOpts(t *testing.T, at string, w, h int, opts []uitest.Option, specs ...spec) *rig {
	t.Helper()
	amp := &fakeApple{}
	amp.set(specs...)
	return newRigAmp(t, amp, at, w, h, opts, nil)
}

// newRigAmp is newRigOpts over a fake Apple the caller shaped (a failure
// is on before the first frame asks) and, when wrap is set, a stand-in in
// front of the service.
func newRigAmp(t *testing.T, amp *fakeApple, at string, w, h int, opts []uitest.Option, wrap func(concertsAPI) concertsAPI) *rig {
	t.Helper()
	hn := uitest.New(t, append([]uitest.Option{uitest.Apple(http.NotFoundHandler())}, opts...)...)
	hn.Network.ServeHandler(ampHost, amp)
	return newRigOver(t, hn, amp, at, w, h, wrap)
}

// today is the day the page believes it is: tests do not read the wall
// clock (the fixtures' shows are in 2027).
var today = time.Date(2027, 2, 10, 12, 0, 0, 0, time.UTC)

// newRigOver is newRig over a harness of the caller's; wrap, when set,
// puts a stand-in in front of the service before the first frame.
func newRigOver(t *testing.T, hn *uitest.Harness, amp *fakeApple, at string, w, h int, wrap func(concertsAPI) concertsAPI) *rig {
	t.Helper()
	r := &rig{t: t, h: hn, router: ui.NewRouter(at), amp: amp}
	computerIn(t, "Europe/London") // the venues' clock is this computer's: no zone names
	hn.Env.Nav = r.router
	// The artists' pages are another PR's: only their route is faked.
	hn.Env.HasRoute = func(p string) bool { return strings.HasPrefix(p, "/artist/") || p == concertsPath }
	// The fake account is signed in before the page is drawn: its sign-in
	// events would otherwise reach the page's auth handler in the middle of
	// a test (a change of account refetches the listing, dropping the one
	// held or in flight).
	acct := services.Service[services.AuthService](hn.Core)
	for deadline := time.Now().Add(10 * time.Second); acct.Status().State != "signed_in"; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the fake account never signed in: %q", acct.Status().State)
		}
	}
	r.s = newConcerts(hn.Env, services.Service[services.ConcertsService](hn.Core))
	if wrap != nil {
		r.s.api = wrap(r.s.api)
	}
	r.s.today = func() time.Time { return today }
	r.s.init()
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(concertsPath) {
					r.s.concertsPage(c, rt)
				}
			})
		})
		kit.Toaster(c)
	}, w, h)
	return r
}

func (r *rig) click(name string) {
	r.t.Helper()
	if err := r.tt.Click(name); err != nil {
		r.t.Fatalf("click %q: %v", name, err)
	}
}

func (r *rig) shows(texts ...string) {
	r.t.Helper()
	r.tt.Until("texts "+strings.Join(texts, " | "), r.tt.Shows(texts...))
}

// lacks fails when the page shows text.
func (r *rig) lacks(texts ...string) {
	r.t.Helper()
	for _, w := range texts {
		if r.tt.HasText(w) {
			r.t.Errorf("page shows %q; texts %q", w, r.tt.Texts())
		}
	}
}

// clickField clicks the text field after its label (the label and the field
// are both named for it).
func (r *rig) clickField(label string) {
	r.t.Helper()
	b, ok := r.tt.Find(label)
	if !ok {
		r.t.Fatalf("no %q; texts %q", label, r.tt.Texts())
	}
	r.tt.ClickAt(b.X+b.W+40, b.Y+b.H/2)
}

// bulk is n concerts, one a day from 1 January 2027.
func bulk(n int) []spec {
	out := make([]spec, n)
	for i := range out {
		day := time.Date(2027, 1, 1+i, 19, 0, 0, 0, time.UTC)
		out[i] = spec{
			id: "bulk." + strconv.Itoa(i), lineup: "Band " + strconv.Itoa(i), venue: "Hall " + strconv.Itoa(i), city: "London",
			zone: "Europe/London", start: day.Format(time.RFC3339), artist: strconv.Itoa(800000 + i),
		}
	}
	return out
}

// answered waits for the fake Apple to have been asked and for the answer
// (or its failure) to reach the view: what a test about a shelf that draws
// nothing must do before it may say nothing is drawn.
func (r *rig) answered() {
	r.t.Helper()
	r.tt.Until("a request to Apple", func() bool {
		r.amp.mu.Lock()
		defer r.amp.mu.Unlock()
		return len(r.amp.seen) > 0
	})
	time.Sleep(100 * time.Millisecond) // the fetch goroutine's post
	r.tt.Drain()
	r.tt.Drain()
}

// openedURLs is the URLs the system browser was asked to open (the
// harness records Env.OpenURL).
func (r *rig) openedURLs() []string { return r.h.Opened() }

// lacksExactly fails when some text on the page is exactly one of texts.
func (r *rig) lacksExactly(texts ...string) {
	r.t.Helper()
	for _, w := range r.tt.Texts() {
		if slices.Contains(texts, w) {
			r.t.Errorf("page shows exactly %q; texts %q", w, r.tt.Texts())
		}
	}
}
