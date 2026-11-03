package services

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
)

// fakeConcerts serves the concert fixtures and keeps the queries it saw.
type fakeConcerts struct {
	t    *testing.T
	mu   sync.Mutex
	seen []url.Values
	fail bool
}

func (f *fakeConcerts) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.seen = append(f.seen, r.URL.Query())
	fail := f.fail
	f.mu.Unlock()
	if fail {
		http.Error(w, `{"errors":[]}`, http.StatusInternalServerError)
		return
	}
	file := "hub.json"
	switch {
	case r.URL.Path == "/v1/catalog/gb/genres":
		_, _ = w.Write([]byte(`{"data":[{"id":"34","type":"genres","attributes":{"name":"Music"}},{"id":"20","type":"genres","attributes":{"name":"Alternative","parentId":"34"}}]}`))
		return
	case strings.HasPrefix(r.URL.Path, "/v1/catalog/gb/artists/"):
		file = "artist.json"
	case r.URL.Path != "/v1/concerts/gb/hub":
		http.NotFound(w, r)
		return
	}
	b, err := os.ReadFile(filepath.Join("testdata", "concerts", file))
	if err != nil {
		f.t.Error(err)
	}
	// Apple answers the user's own artists only when asked for them.
	if file == "hub.json" && !strings.Contains(r.URL.Query().Get("containers"), "favorite-artists-concerts") {
		b = []byte(strings.Replace(string(b), `"favorite-artists-concerts"`, `"not-asked-for"`, 1))
	}
	_, _ = w.Write(b)
}

func (f *fakeConcerts) queries() []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.seen)
}

type concertsEnv struct {
	s        *ConcertsService
	amp      *fakeConcerts
	ev       *recorder
	settings *config.Store
	now      *time.Time
}

func newConcertsEnv(t *testing.T, signedIn bool) concertsEnv {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	amp := &fakeConcerts{t: t}
	srv := httptest.NewServer(amp)
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string {
		if signedIn {
			return "mut"
		}
		return ""
	})
	api.Base, api.AMPBase = srv.URL, srv.URL
	art, err := artwork.New(artwork.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	st, err := config.OpenStore(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2027, 3, 1, 9, 0, 0, 0, time.UTC)
	ev := &recorder{}
	e := &Env{
		Deps: Deps{
			DB: d, API: api, Auth: &auth.Manager{}, Art: art, Gate: NewGate(time.Hour, nil), Emit: ev, Settings: st,
			Storefront: func() string { return "gb" },
			Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			Now:        func() time.Time { return now },
		},
		m: mapper{art: art},
	}
	return concertsEnv{s: newConcertsService(e), amp: amp, ev: ev, settings: st, now: &now}
}

func TestConcertsPageMapsTheListing(t *testing.T) {
	env := newConcertsEnv(t, true)
	page, err := env.s.Concerts(t.Context(), ConcertsRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Nothing chosen: the storefront's main city, asked for by its geohash only.
	if l := page.Location; l.ID != "gb-london" || !l.IsDefault || l.Name != "London" || l.CountryCode != "gb" {
		t.Errorf("location = %+v", l)
	}
	q := env.amp.queries()[0]
	if q.Get("geoHashLocation") != "gcpv" {
		t.Errorf("geohash = %q", q.Get("geoHashLocation"))
	}
	for k := range q {
		if lk := strings.ToLower(k); strings.HasPrefix(lk, "lat") || strings.HasPrefix(lk, "lon") || strings.Contains(lk, "coord") {
			t.Errorf("a coordinate went to Apple: %s", k)
		}
	}
	if !strings.Contains(q.Get("containers"), "favorite-artists-concerts") {
		t.Errorf("signed in, the user's artists are asked for: %s", q.Get("containers"))
	}

	if len(page.Concerts) != 3 || page.More || page.Limit != 100 {
		t.Fatalf("concerts = %d, more %v, limit %d", len(page.Concerts), page.More, page.Limit)
	}
	first := page.Concerts[0]
	if first.Lineup != "Example Band with Opener Duo" || first.Name != "Example Band with Opener Duo @ Example Hall" {
		t.Errorf("lineup = %q (name %q)", first.Lineup, first.Name)
	}
	if want := time.Date(2027, 4, 14, 17, 0, 0, 0, time.UTC).UnixMilli(); first.StartsAt != want {
		t.Errorf("startsAt = %d, want %d", first.StartsAt, want)
	}
	if first.Timezone != "Europe/London" || first.EndsAt <= first.StartsAt {
		t.Errorf("zone %q, ends %d", first.Timezone, first.EndsAt)
	}
	if v := first.Venue; v == nil || v.Name != "Example Hall" || v.City != "London" || v.CountryCode != "GB" {
		t.Errorf("venue = %+v", v)
	}
	// Example Hall is about 2 km from the middle of London.
	if first.DistanceKm < 1 || first.DistanceKm > 6 {
		t.Errorf("distance = %d km", first.DistanceKm)
	}
	if len(first.Tickets) != 1 || first.Tickets[0].URL != "https://tickets.example.com/e/1" {
		t.Errorf("tickets = %+v", first.Tickets)
	}
	if len(first.Artists) != 1 || first.Artists[0].Name != "Example Band" || first.Artists[0].Art.Medium == "" {
		t.Errorf("artists = %+v", first.Artists)
	}
	if first.YourArtist || !page.Concerts[1].YourArtist || !page.Concerts[2].YourArtist || page.YourArtists != 2 {
		t.Errorf("your artists: %v %v %v, count %d", first.YourArtist, page.Concerts[1].YourArtist, page.Concerts[2].YourArtist, page.YourArtists)
	}
	if v := page.Concerts[2].Venue; v != nil {
		t.Errorf("a concert without a venue has one: %+v", v)
	}
	if len(page.Genres) != 2 || page.Genres[0].ID != "" || page.Genres[1].Name != "Alternative" {
		t.Errorf("genres = %+v", page.Genres)
	}
}

func TestConcertsSignedOutAskForNoPersonalList(t *testing.T) {
	env := newConcertsEnv(t, false)
	page, err := env.s.Concerts(t.Context(), ConcertsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if c := env.amp.queries()[0].Get("containers"); c != "popular-concerts" {
		t.Errorf("containers = %s", c)
	}
	if page.YourArtists != 0 {
		t.Errorf("your artists = %d", page.YourArtists)
	}
}

func TestConcertsRequestIsValidatedAndPassedOn(t *testing.T) {
	env := newConcertsEnv(t, false)
	ctx := t.Context()
	for _, bad := range []ConcertsRequest{
		{From: "2027-04-01"},
		{To: "2027-04-01"},
		{From: "2027-04-09", To: "2027-04-01"},
		{From: "tomorrow", To: "next week"},
		{Genre: "20,21"},
		{Genre: "rock"},
	} {
		if _, err := env.s.Concerts(ctx, bad); !strings.Contains(errString(err), "invalid argument") {
			t.Errorf("%+v: err = %v, want invalid argument", bad, err)
		}
	}
	if n := len(env.amp.queries()); n != 0 {
		t.Errorf("%d requests for invalid ones", n)
	}
	page, err := env.s.Concerts(ctx, ConcertsRequest{From: "2027-04-01", To: "2027-04-30", Genre: "20", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	q := env.amp.queries()[0]
	if q.Get("filter[date]") != "2027-04-01..2027-04-30" || q.Get("filter[genre]") != "20" || q.Get("limit[popular-concerts]") != "2" {
		t.Errorf("query = %v", q)
	}
	// As many as asked for came back, so there may be more.
	if !page.More || page.Limit != 2 {
		t.Errorf("more %v, limit %d", page.More, page.Limit)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestConcertsLocationIsChosenSavedAndUsed(t *testing.T) {
	env := newConcertsEnv(t, false)
	ctx := t.Context()

	if _, err := env.s.SetLocation("xx-nowhere"); err == nil {
		t.Error("an unknown city was accepted")
	}
	loc, err := env.s.SetLocation("fr-paris")
	if err != nil {
		t.Fatal(err)
	}
	if loc.ID != "fr-paris" || loc.IsDefault || loc.Name != "Paris" {
		t.Errorf("location = %+v", loc)
	}
	if got := env.settings.Get().Concerts.City; got != "fr-paris" {
		t.Errorf("saved city = %q", got)
	}
	if !slices.Contains(env.ev.events, EventSettingsChanged) {
		t.Error("the settings change was not announced (the frontend's copy would overwrite it)")
	}

	page, err := env.s.Concerts(ctx, ConcertsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Location.ID != "fr-paris" || env.amp.queries()[0].Get("geoHashLocation") != "u09t" {
		t.Errorf("asked for %s with %q", page.Location.ID, env.amp.queries()[0].Get("geoHashLocation"))
	}

	// Back to the storefront's city.
	loc, err = env.s.SetLocation("")
	if err != nil || loc.ID != "gb-london" || !loc.IsDefault {
		t.Errorf("reset: %+v, %v", loc, err)
	}
}

func TestConcertsPlacesPicker(t *testing.T) {
	env := newConcertsEnv(t, false)
	if _, err := env.s.SetLocation("gb-leeds"); err != nil {
		t.Fatal(err)
	}
	sug := env.s.Places("")
	if len(sug) != concertsPickerRows || sug[0].CountryCode != "gb" {
		t.Fatalf("suggestions = %+v", sug)
	}
	found := env.s.Places("  lee")
	if len(found) == 0 || found[0].ID != "gb-leeds" || !found[0].Selected {
		t.Errorf("found = %+v", found)
	}
	if got := env.s.Places("zzzzzz"); got == nil || len(got) != 0 {
		t.Errorf("nothing found = %#v, want an empty list", got)
	}
}

func TestConcertsAreCachedPerQueryAndServedWhenAppleFails(t *testing.T) {
	env := newConcertsEnv(t, false)
	ctx := t.Context()
	if _, err := env.s.Concerts(ctx, ConcertsRequest{}); err != nil {
		t.Fatal(err)
	}
	n := len(env.amp.queries()) // the list and the genres
	if _, err := env.s.Concerts(ctx, ConcertsRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := len(env.amp.queries()); got != n {
		t.Errorf("a fresh list asked Apple again (%d → %d requests)", n, got)
	}

	// Another city is another list.
	if _, err := env.s.SetLocation("de-berlin"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.s.Concerts(ctx, ConcertsRequest{}); err != nil {
		t.Fatal(err)
	}
	if got := len(env.amp.queries()); got != n+1 {
		t.Errorf("a new city made %d requests, want 1", got-n)
	}

	// Past its lifetime, with Apple down, the copy is served and says so.
	*env.now = env.now.Add(4 * time.Hour)
	env.amp.fail = true
	env.s.e.Gate.Open()
	page, err := env.s.Concerts(ctx, ConcertsRequest{})
	if err != nil || !page.Stale || len(page.Concerts) != 2 {
		t.Errorf("expired list with Apple down: stale %v, %d concerts, %v", page.Stale, len(page.Concerts), err)
	}

	// The next day's list is another one: yesterday's is not offered as today's.
	*env.now = env.now.Add(24 * time.Hour)
	if _, err := env.s.Concerts(ctx, ConcertsRequest{}); err == nil {
		t.Error("a list from another day was served")
	}
}

func TestArtistConcertsSplitNearbyFromTheRest(t *testing.T) {
	env := newConcertsEnv(t, false)
	if _, err := env.s.ForArtist(t.Context(), "../x"); err == nil {
		t.Error("a bad artist ID was accepted")
	}
	got, err := env.s.ForArtist(t.Context(), "900001")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Nearby) != 1 || got.Nearby[0].ID != "ce.0001" || len(got.Elsewhere) != 1 || got.Elsewhere[0].ID != "ce.0009" {
		t.Fatalf("nearby %+v elsewhere %+v", got.Nearby, got.Elsewhere)
	}
	far := got.Elsewhere[0]
	if far.Timezone != "America/New_York" || far.Venue.Region != "NY" || far.DistanceKm < 5000 {
		t.Errorf("elsewhere = %+v", far)
	}
	if far.EndsAt != 0 || far.Lineup != "Example Band" || len(far.Artists) != 1 || far.Artists[0].ID != "900001" {
		t.Errorf("elsewhere (no end time, artist from the page) = %+v", far)
	}
	if q := env.amp.queries()[0]; q.Get("geoHashLocation") != "gcpv" {
		t.Errorf("geohash = %q", q.Get("geoHashLocation"))
	}
}

func TestOnlySecureTicketLinksReachTheFrontend(t *testing.T) {
	for raw, want := range map[string]bool{
		"https://tickets.example.com/e/1": true,
		"http://tickets.example.com/e/1":  false,
		"javascript:alert(1)":             false,
		"file:///etc/passwd":              false,
		"https://":                        false,
		"":                                false,
	} {
		if got := isHTTPS(raw); got != want {
			t.Errorf("isHTTPS(%q) = %v, want %v", raw, got, want)
		}
	}
}
