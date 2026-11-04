package radio

import (
	"encoding/json"
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
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// appleFake is Apple's catalogue as the radio pages see it: live stations
// with schedules, the user's recent stations, stations by ID, the station
// page, the editorial radio grouping (the services' trimmed real grouping
// response) and the next songs of a station of tracks. Every request it
// answers is recorded; what it does not know is a 404.
type appleFake struct {
	t *testing.T

	mu   sync.Mutex
	seen []string
}

func (a *appleFake) requests(prefix string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, r := range a.seen {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}

func artwork(n int) map[string]any {
	return map[string]any{"url": fixture.Template(n), "width": 600, "height": 600}
}

// station is Apple's JSON of a station.
func station(id, name string, art int, attrs map[string]any) map[string]any {
	a := map[string]any{
		"name": name, "artwork": artwork(art), "mediaKind": "audio", "stationProviderName": "Apple Music",
		"editorialNotes": map[string]any{"tagline": name + " tagline", "standard": ""},
	}
	for k, v := range attrs {
		a[k] = v
	}
	return map[string]any{"id": id, "type": "stations", "attributes": a}
}

var live = map[string]any{
	"isLive": true, "kind": "streaming", "supportedDrms": []string{"widevine"},
	"playParams": map[string]any{"id": "x", "kind": "radioStation", "format": "stream", "hasDrm": true},
}

var tracks = map[string]any{
	"kind":       "songSeeded",
	"playParams": map[string]any{"id": "x", "kind": "radioStation", "format": "tracks"},
}

func liveStations() []any {
	return []any{
		station("ra.1", "Apple Music 1", 1, live),
		station("ra.2", "Hits", 2, live),
	}
}

func (a *appleFake) json(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (a *appleFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.seen = append(a.seen, r.Method+" "+r.URL.Path)
	a.mu.Unlock()
	p, q := r.URL.Path, r.URL.Query()
	hour := appleBase()
	event := func(id, title string, from, to time.Time) map[string]any {
		return map[string]any{"id": id, "type": "station-events", "attributes": map[string]any{
			"title": title, "description": map[string]any{"standard": title + " on air."},
			"startTime": from.UTC().Format(time.RFC3339), "endTime": to.UTC().Format(time.RFC3339), "artwork": artwork(5),
		}}
	}
	switch {
	case p == "/v1/catalog/gb/stations" && q.Get("filter[featured]") != "":
		a.json(w, map[string]any{"data": liveStations()})
	case p == "/v1/catalog/gb/stations" && q.Get("ids") != "":
		var out []any
		for _, id := range strings.Split(q.Get("ids"), ",") {
			switch id {
			case "ra.1":
				out = append(out, liveStations()[0])
			case "ra.2":
				out = append(out, liveStations()[1])
			case "ra.123": // Create Station on song 123
				out = append(out, station("ra.123", "Airbag Station", 7, tracks))
			case "ra.mine":
				out = append(out, station("ra.mine", "My Station", 8, tracks))
			}
		}
		a.json(w, map[string]any{"data": out})
	case p == "/v1/catalog/gb/stations/ra.1":
		st := liveStations()[0].(map[string]any)
		st["relationships"] = map[string]any{"events": map[string]any{"data": []any{
			event("e1", "The Zane Lowe Show", hour, hour.Add(time.Hour)),
			event("e2", "Apple Music 1", hour.Add(time.Hour), hour.Add(2*time.Hour)),
		}}}
		a.json(w, map[string]any{"data": []any{st}})
	case p == "/v1/catalog/gb/stations/ra.mine":
		a.json(w, map[string]any{"data": []any{station("ra.mine", "My Station", 8, tracks)}})
	case strings.HasPrefix(p, "/v1/catalog/gb/stations/"):
		for _, st := range liveStations() {
			if "/v1/catalog/gb/stations/"+st.(map[string]any)["id"].(string) == p {
				a.json(w, map[string]any{"data": []any{st}})
				return
			}
		}
		http.NotFound(w, r)
	case p == "/v1/me/recent/radio-stations":
		a.json(w, map[string]any{"data": []any{station("ra.mine", "My Station", 8, tracks)}})
	case p == "/v1/editorial/gb/groupings":
		b, err := os.ReadFile(filepath.Join("..", "..", "..", "services", "testdata", "discover", "new.json"))
		if err != nil {
			a.t.Error(err)
		}
		_, _ = w.Write(b)
	case r.Method == http.MethodPost && strings.HasPrefix(p, "/v1/me/stations/next-tracks/"):
		song := func(id, name string) map[string]any {
			return map[string]any{"id": id, "type": "songs", "attributes": map[string]any{
				"name": name, "artistName": "Radiohead", "albumName": "OK Computer", "durationInMillis": 284000,
				"playParams": map[string]any{"id": id, "kind": "song"}, "artwork": artwork(9),
			}, "relationships": map[string]any{
				"albums":  map[string]any{"data": []any{map[string]any{"id": "al1", "type": "albums"}}},
				"artists": map[string]any{"data": []any{map[string]any{"id": "ar1", "type": "artists"}}},
			}}
		}
		a.json(w, map[string]any{"data": []any{song("900", "Airbag"), song("901", "Paranoid Android")}})
	default:
		http.NotFound(w, r)
	}
}

// appleRig is the pages over the real services (the Core on the fixture
// library, signed in to a fake account) answered by appleFake.
func appleRig(t *testing.T, path string, width, height int) (*rig, *appleFake) {
	t.Helper()
	kit.CloseToast("")
	fake := &appleFake{t: t}
	h := uitest.New(t, uitest.Apple(fake), uitest.Empty())
	rg := &rig{t: t, h: h, router: ui.NewRouter(path), now: appleBase().Add(15 * time.Minute)}
	env := h.Env
	env.Nav = rg.router
	env.HasRoute = func(string) bool { return true }
	core := env.Core
	sys := services.Service[services.SystemService](core)
	rg.r = newRadio(env, services.Service[services.RadioService](core), services.Service[services.PlaybackService](core),
		services.Service[services.AuthService](core), func() string { return sys.Settings().Queue.ReplaceConfirm })
	rg.r.now = func() time.Time { return rg.now }
	env.Actions.Register(rg.r.actions()...)
	rg.r.init()
	// The account is validated before the first frame: a station page read
	// without the user's token has no songs to list.
	acct := services.Service[services.AuthService](core)
	for deadline := time.Now().Add(10 * time.Second); acct.Status().State != "signed_in"; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the fake account never signed in: %q", acct.Status().State)
		}
	}
	// The account's feature keeps the shared ["auth"] query current.
	store.On(h.Bus, services.EventAuthChanged, func(s services.AuthStatus) { store.Set(env.Queries, keyAuth, s) })
	rg.tt = h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
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
	}, width, height)
	return rg, fake
}

func TestRadioPageOverTheServices(t *testing.T) {
	rg, apple := appleRig(t, radioPath, 1132, 1253)
	// The board lists Apple's live stations and, from each station's own
	// page, the programme on air; the signed-in account's stations lead the
	// Stations band; Apple's grouping fills the bands.
	rg.tt.Until("the page", rg.tt.Shows("Apple Music 1", "The Zane Lowe Show", "My Station", "Hits"))
	if rg.tt.HasText("Sign in to see the stations you play") {
		t.Errorf("a signed-in account is asked to sign in: %q", rg.tt.Texts())
	}
	for _, want := range []string{"Live now", "Your stations", "Stations"} {
		if !rg.tt.HasText(want) {
			t.Errorf("no %q: %q", want, rg.tt.Texts())
		}
	}
	for _, prefix := range []string{
		"GET /v1/catalog/gb/stations", "GET /v1/me/recent/radio-stations", "GET /v1/editorial/gb/groupings", "GET /v1/catalog/gb/stations/ra.1",
	} {
		if apple.requests(prefix) == 0 {
			t.Errorf("nothing asked Apple for %s: %v", prefix, apple.seen)
		}
	}
}

func TestPlayingALiveStationThroughTheServicesMarksItTuned(t *testing.T) {
	rg, _ := appleRig(t, radioPath, 1132, 1253)
	// Hits has no schedule on its own page: it has no page, and plays when opened.
	rg.tt.Until("the board", rg.tt.Shows("Play Apple Music 1", "Hits Play"))
	rg.click("Play Apple Music 1")
	// The service queues the stream; the mirror follows its events: the
	// row's play button gives way to the equaliser.
	rg.tt.Until("the station tuned", func() bool { _, ok := rg.tt.Find("Play Apple Music 1"); return !ok })
	if !rg.tt.HasText("Now playing") && !rg.tt.HasText("Paused") {
		t.Errorf("no equaliser on the station that plays: %q", rg.tt.Texts())
	}
	if got := rg.h.Engine.Played(); !slices.Equal(got, []string{"ra.1"}) {
		t.Errorf("the engine played %v", got)
	}
	cur := rg.h.Env.Play.State().Current
	if cur == nil || cur.ID != "ra.1" || !cur.Live {
		t.Fatalf("current item %+v", cur)
	}
	// Another station is not marked.
	if _, ok := rg.tt.Find("Hits Play"); !ok {
		t.Errorf("another station lost its control: %q", rg.tt.Texts())
	}
}

func TestCreateStationThroughTheServicesStartsItAndRefreshesTheUsersStations(t *testing.T) {
	rg, apple := appleRig(t, radioPath, 1132, 1253)
	rg.tt.Until("the page", rg.tt.Shows("My Station"))
	before := apple.requests("GET /v1/me/recent/radio-stations")
	create := actionOf(t, rg, kit.KindSong, "radio.create-station")
	create.Run(rg.h.Env.Actions.Context([]kit.Item{kit.SongItem(services.Song{CatalogID: "123", Name: "Airbag"})}, nil, nil))
	rg.toastShown("Playing Airbag Station")
	// The station that feeds the queue is a new one: Apple's history of the
	// user's stations changed, so it is asked again.
	rg.tt.Until("the history read again", func() bool { return apple.requests("GET /v1/me/recent/radio-stations") > before })
	st := rg.h.Env.Play.State().Station
	if st == nil || st.ID != "ra.123" {
		t.Errorf("the queue's station is %+v", st)
	}
	if got := rg.h.Engine.Played(); len(got) == 0 || got[0] != "900" {
		t.Errorf("the engine played %v, want the station's first song", got)
	}
}

func TestStationPageOverTheServices(t *testing.T) {
	rg, _ := appleRig(t, "/station/ra.1", 1132, 1253)
	rg.tt.Until("the page", rg.tt.Shows("Apple Music 1", "Live radio · Apple Music", "Schedule", "The Zane Lowe Show", "Apple Music 1 on air."))
	if _, ok := rg.tt.Find("Time through this programme"); !ok {
		t.Errorf("no progress on the programme on air: %q", rg.tt.Texts())
	}
	rg.click("Play Apple Music 1")
	rg.tt.Until("playing", func() bool { return slices.Equal(rg.h.Engine.Played(), []string{"ra.1"}) })
}

func TestStationPageOfAStationOfSongsListsWhatItPlaysFirst(t *testing.T) {
	rg, _ := appleRig(t, "/station/ra.mine", 1132, 1253)
	rg.tt.Until("the page", rg.tt.Shows("My Station", "Station · Apple Music", "Plays first", "Airbag", "Paranoid Android", "OK Computer", "4:44"))
	// The first song leads to its album, with the song to show.
	rg.click("Airbag")
	if got := rg.router.Location(); got != "/album/al1?song=900" {
		t.Errorf("the song went to %q", got)
	}
}

func TestStationPageFailsInPlaceWhenAppleDoesNotKnowTheStation(t *testing.T) {
	rg, _ := appleRig(t, "/station/ra.nobody", 1132, 1253)
	rg.tt.Until("the error", rg.tt.Shows("Couldn't load this station"))
	if !rg.tt.HasText("It couldn't be found. It may have been removed.") {
		t.Errorf("no reason: %q", rg.tt.Texts())
	}
}
