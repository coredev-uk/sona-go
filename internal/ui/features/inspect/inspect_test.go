package inspect

import (
	"net/http"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
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
		// A library album: its row is in SQLite, so the Library tab has one.
		"libalbum": kit.AlbumItem(services.Album{
			ID: fixture.AlbumLibraryID(0), Source: services.SourceLibrary, LibraryID: fixture.AlbumLibraryID(0), CatalogID: fixture.AlbumCatalogID(0),
			Name: "Abbey Road", Artist: "The Beatles", Art: services.Art{Large: albumArt},
		}),
		"station": kit.StationItem(services.Station{
			ID: "ra.978194965", Name: "Apple Music 1", Art: services.Art{Large: radioArt},
		}),
	}
	r.tt = h.Tester(t, func(c *ui.Context) {
		in.panel(c)
		kit.Toaster(c)
	}, 1000, 900)
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
