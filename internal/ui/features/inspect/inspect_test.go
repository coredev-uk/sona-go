package inspect

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

const (
	albumArt = "/art/albumkey002/1000x1000.jpg"
	listArt  = "/art/listkey0003/1000x1000.jpg"
	radioArt = "/art/radiokey004/1000x1000.jpg"
	// songTemplate is Apple's artwork template for the song: the add-on's
	// track carries it at 600 pixels.
	songTemplate = "https://is1-ssl.mzstatic.com/image/thumb/Music/v4/ab/cd/ef/airbag.jpg/{w}x{h}bb.jpg"
	// A JWT-shaped value Apple's cached body never holds, to see the
	// redaction run on what the panel shows (built in parts: no literal
	// credential in the source).
	secret = "eyJ" + "hbGciOiJFUzI1NiJ9" + "." + "eyJpc3MiOiJ0ZXN0In0" + "." + "c2lnbmF0dXJl"
)

type rig struct {
	h     *uitest.Harness
	tt    *uitest.Session
	in    *inspector
	items map[string]kit.Item
	// songArt is the song's Go-served artwork URL at the large size.
	songArt string
}

// newRig is the inspector over a full Env. dev says whether it is a
// development build (Env.Dev); the setting starts off.
func newRig(t *testing.T, dev bool, opts ...uitest.Option) *rig {
	t.Helper()
	h := uitest.New(t, append([]uitest.Option{uitest.Apple(http.NotFoundHandler())}, opts...)...)
	h.Env.Dev = dev
	in := newInspector(h.Env)
	in.init()
	r := &rig{h: h, in: in}
	songURLs := h.Art.URLs(songTemplate, 1, 1, [3]int{96, 400, 1000})
	r.songArt = songURLs[2]
	r.items = map[string]kit.Item{
		"song": kit.SongItem(services.Song{
			ID: "1440", Source: services.SourceCatalog, CatalogID: "1440", Name: "Airbag", Artist: "Radiohead", Album: "OK Computer",
			DurationMs: 284000, Explicit: true, Art: services.Art{Large: r.songArt},
		}),
		"album": kit.AlbumItem(services.Album{
			ID: "1700", Source: services.SourceCatalog, CatalogID: "1700", Name: "OK Computer", Art: services.Art{Large: albumArt},
		}),
		// A Discover hero card is a playlist item.
		"playlist": kit.PlaylistItem(services.Playlist{
			ID: "pl.hero", CatalogID: "pl.hero", Name: "Today's Hits", Art: services.Art{Large: listArt},
		}),
		"station": kit.StationItem(services.Station{
			ID: "ra.978194965", Name: "Apple Music 1", Art: services.Art{Large: radioArt},
		}),
	}
	r.tt = h.Tester(t, func(c *ui.Context) { in.panel(c) }, 1000, 900)
	return r
}

// setDeveloperTools saves the setting as the Settings row does and lets the
// event reach the UI.
func (r *rig) setDeveloperTools(t *testing.T, on bool) {
	t.Helper()
	st := r.h.Settings.Get()
	st.DeveloperTools = on
	if _, err := services.Service[services.SystemService](r.h.Core).UpdateSettings(st); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
}

// offered is whether the item's menu lists Inspect, and last.
func (r *rig) offered(it kit.Item) (listed, last bool) {
	defs := r.h.Env.Actions.For(it.Kind)
	for i, d := range defs {
		if d.ID == ActionID+string(it.Kind) {
			return true, i == len(defs)-1 && d.Group == kit.GroupDeveloper
		}
	}
	return false, false
}

// run chooses Inspect on it and waits for the report.
func (r *rig) run(t *testing.T, it kit.Item) {
	t.Helper()
	for _, def := range r.h.Env.Actions.For(it.Kind) {
		if def.ID == ActionID+string(it.Kind) {
			def.Run(r.h.Env.Actions.Context([]kit.Item{it}, nil, nil))
		}
	}
	r.tt.Until("loaded", func() bool { r.tt.Drain(); return !r.in.loading })
}

// A release build offers Inspect only while Developer tools is on, and takes
// it away again at once when it goes off.
func TestReleaseBuildOffersInspectOnlyWhileTheSettingIsOn(t *testing.T) {
	r := newRig(t, false)
	for name, it := range r.items {
		if listed, _ := r.offered(it); listed {
			t.Errorf("%s: Inspect offered with the setting off", name)
		}
	}
	r.setDeveloperTools(t, true)
	for name, it := range r.items {
		listed, last := r.offered(it)
		if !listed || !last {
			t.Errorf("%s: Inspect listed = %v, last in the Developer group = %v, with the setting on", name, listed, last)
		}
	}
	r.setDeveloperTools(t, false)
	for name, it := range r.items {
		if listed, _ := r.offered(it); listed {
			t.Errorf("%s: Inspect still offered after the setting went off", name)
		}
	}
}

// A release build that starts with the setting already on offers Inspect from
// the first menu.
func TestInspectIsOfferedFromStartWhenTheSettingWasSaved(t *testing.T) {
	h := uitest.New(t)
	if _, err := h.Settings.Update(func(s *config.Settings) { s.DeveloperTools = true }); err != nil {
		t.Fatal(err)
	}
	newInspector(h.Env).init()
	if !h.Env.Actions.Has(kit.KindSong) || !slices.ContainsFunc(h.Env.Actions.For(kit.KindSong), func(d *kit.ActionDef) bool { return d.ID == ActionID+"song" }) {
		t.Error("a saved setting did not offer Inspect")
	}
}

// A development build offers Inspect whatever the setting says.
func TestDevelopmentBuildOffersInspectWhateverTheSettingSays(t *testing.T) {
	r := newRig(t, true)
	for name, it := range r.items {
		if listed, last := r.offered(it); !listed || !last {
			t.Errorf("%s: Inspect listed = %v, last = %v", name, listed, last)
		}
	}
	r.setDeveloperTools(t, false)
	for name, it := range r.items {
		if listed, _ := r.offered(it); !listed {
			t.Errorf("%s: Inspect left a development build when the setting went off", name)
		}
	}
}

// An item of each kind has Inspect, as an action no other group follows.
func TestArtistAndMusicVideoMenusOfferInspectToo(t *testing.T) {
	r := newRig(t, true)
	for _, k := range []kit.ItemKind{kit.KindArtist, kit.KindMusicVideo} {
		if !r.h.Env.Actions.Has(k) {
			t.Errorf("%s has no actions", k)
		}
	}
}

// Running Inspect shows the item's IDs and artwork, from what Go's cache
// holds, redacted.
func TestInspectShowsIDsArtworkAndTheCachedAppleResource(t *testing.T) {
	r := newRig(t, false)
	r.setDeveloperTools(t, true)
	body := `{"id":"1440","type":"songs","attributes":{"name":"Airbag","url":"` + secret + `","artwork":{"url":"https://is1-ssl.mzstatic.com/image/thumb/Music/v4/{w}x{h}bb.jpg"}}}`
	if err := r.h.DB.Q.PutCache(context.Background(), dbq.PutCacheParams{
		Key: "gb|song-artists|1440", Storefront: "gb", Body: []byte(body), FetchedAt: 1, ExpiresAt: 1 << 50,
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		id, art string
		raw     bool
	}{
		{"song", "1440", r.songArt, true},
		{"album", "1700", albumArt, false},
		{"playlist", "pl.hero", listArt, false},
		{"station", "ra.978194965", radioArt, false},
	}
	for _, tc := range cases {
		r.run(t, r.items[tc.name])
		for _, want := range []string{"id: " + tc.id, "artwork: " + tc.art, `"id": "` + tc.id + `"`} {
			if !strings.Contains(r.in.text, want) {
				t.Errorf("%s: panel lacks %q:\n%s", tc.name, want, r.in.text)
			}
		}
		if got := strings.Contains(r.in.text, "Airbag\"") && strings.Contains(r.in.text, "catalog cache gb|song-artists|1440"); got != tc.raw {
			t.Errorf("%s: Apple's resource shown = %v, want %v:\n%s", tc.name, got, tc.raw, r.in.text)
		}
		for _, text := range []string{r.in.text, r.in.addonText, r.in.asJSON(), r.in.addonJSON()} {
			if strings.Contains(text, secret) {
				t.Errorf("%s: a token reached the panel", tc.name)
			}
			for _, header := range []string{"Authorization", "Bearer", "Media-User-Token", "Music-User-Token", "Origin:"} {
				if strings.Contains(text, header) {
					t.Errorf("%s: %q reached the panel:\n%s", tc.name, header, text)
				}
			}
		}
		if !r.tt.HasText("Copy as JSON") || !r.tt.HasText("Copy ID") || !r.tt.HasText("Copy artwork URL") || !r.tt.HasText("What an add-on gets") {
			t.Errorf("%s: panel parts missing: %q", tc.name, r.tt.Texts())
		}
	}
}

// "What an add-on gets" is the protocol's JSON: the invoke target for the
// item, and for a song the now-playing event's track. The text and the copied
// JSON hold what the protocol code produces, not a copy of it.
func TestWhatAnAddonGetsIsTheProtocolsJSONForTheItem(t *testing.T) {
	r := newRig(t, false)
	r.setDeveloperTools(t, true)
	type want struct{ target, track string }
	cases := map[string]want{
		"song": {
			target: `{"kind":"song","id":"1440","name":"Airbag"}`,
			track: `{"id":"1440","title":"Airbag","artist":"Radiohead","album":"OK Computer","duration":284,"explicit":true,` +
				`"artwork":"https://is1-ssl.mzstatic.com/image/thumb/Music/v4/ab/cd/ef/airbag.jpg/600x600bb.jpg"}`,
		},
		"album":    {target: `{"kind":"album","id":"1700","name":"OK Computer"}`},
		"playlist": {target: `{"kind":"playlist","id":"pl.hero","name":"Today's Hits"}`},
		"station":  {},
	}
	for name, w := range cases {
		it := r.items[name]
		r.run(t, it)
		var doc struct {
			Invoke     *struct{ Target json.RawMessage } `json:"invoke"`
			NowPlaying *struct{ Track json.RawMessage }  `json:"now-playing"`
		}
		r.tt.MustClick("Copy what an add-on gets as JSON")
		copied := r.h.Copied()
		if err := json.Unmarshal([]byte(copied[len(copied)-1]), &doc); err != nil {
			t.Fatalf("%s: copied JSON: %v", name, err)
		}
		same := func(what string, got json.RawMessage, want string) {
			t.Helper()
			if want == "" {
				if len(got) != 0 {
					t.Errorf("%s: %s = %s, want none", name, what, got)
				}
				return
			}
			var a, b any
			if err := json.Unmarshal(got, &a); err != nil {
				t.Fatalf("%s: %s: %v", name, what, err)
			}
			_ = json.Unmarshal([]byte(want), &b)
			if !jsonEqual(a, b) {
				t.Errorf("%s: %s = %s, want %s", name, what, got, want)
			}
		}
		var target, track json.RawMessage
		if doc.Invoke != nil {
			target = doc.Invoke.Target
		}
		if doc.NowPlaying != nil {
			track = doc.NowPlaying.Track
		}
		same("invoke target", target, w.target)
		same("now-playing track", track, w.track)
		if !strings.Contains(r.in.addonText, "private listens") && !strings.Contains(addonNote, "private listens") {
			t.Errorf("%s: the section does not say add-ons never see private listens", name)
		}
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestInspectCopiesJSONIDAndArtworkURL(t *testing.T) {
	r := newRig(t, true)
	r.in.show(r.items["album"])
	r.tt.Until("loaded", func() bool { r.tt.Drain(); return !r.in.loading })
	for _, label := range []string{"Copy as JSON", "Copy ID", "Copy artwork URL"} {
		r.tt.MustClick(label)
	}
	got := r.h.Copied()
	if len(got) != 3 || got[1] != "1700" || got[2] != albumArt || !strings.Contains(got[0], `"kind": "album"`) {
		t.Errorf("copied %q", got)
	}
	if !slices.ContainsFunc(r.tt.Texts(), func(s string) bool { return strings.Contains(s, "Inspect") }) {
		t.Errorf("panel has no title: %q", r.tt.Texts())
	}
}

// The panel as it looks, light and dark: the report, the buttons and the
// section of what an add-on gets.
func TestPanelGolden(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts []uitest.Option
	}{{"inspect-light", nil}, {"inspect-dark", []uitest.Option{uitest.Dark()}}} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, true, tc.opts...)
			body := `{"id":"1440","type":"songs","attributes":{"name":"Airbag","artistName":"Radiohead","durationInMillis":284000,"artwork":{"url":"https://is1-ssl.mzstatic.com/image/thumb/Music/v4/{w}x{h}bb.jpg"}}}`
			if err := r.h.DB.Q.PutCache(context.Background(), dbq.PutCacheParams{
				Key: "gb|song-artists|1440", Storefront: "gb", Body: []byte(body), FetchedAt: 1, ExpiresAt: 1 << 50,
			}); err != nil {
				t.Fatal(err)
			}
			r.run(t, r.items["song"])
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), tc.name)
		})
	}
}
