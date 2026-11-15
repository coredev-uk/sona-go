//go:build dev

package inspect

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

const (
	songArt  = "/art/songkey0001/1000x1000.jpg"
	albumArt = "/art/albumkey002/1000x1000.jpg"
	listArt  = "/art/listkey0003/1000x1000.jpg"
	radioArt = "/art/radiokey004/1000x1000.jpg"
	// A JWT-shaped value Apple's cached body never holds, to see the
	// redaction run on what the panel shows (built in parts: no literal
	// credential in the source).
	secret = "eyJ" + "hbGciOiJFUzI1NiJ9" + "." + "eyJpc3MiOiJ0ZXN0In0" + "." + "c2lnbmF0dXJl"
)

var items = map[string]kit.Item{
	"song": kit.SongItem(services.Song{
		ID: "1440", Source: services.SourceCatalog, CatalogID: "1440", Name: "Airbag", Art: services.Art{Large: songArt},
	}),
	"album": kit.AlbumItem(services.Album{
		ID: "1700", Source: services.SourceCatalog, CatalogID: "1700", Name: "OK Computer", Art: services.Art{Large: albumArt},
	}),
	// A Discover hero card is a playlist item.
	"hero": kit.PlaylistItem(services.Playlist{
		ID: "pl.hero", CatalogID: "pl.hero", Name: "Today's Hits", Art: services.Art{Large: listArt},
	}),
	"station": kit.StationItem(services.Station{
		ID: "ra.978194965", Name: "Apple Music 1", Art: services.Art{Large: radioArt},
	}),
}

type rig struct {
	h  *uitest.Harness
	tt *uitest.Session
	in *inspector
}

func newRig(t *testing.T) *rig {
	t.Helper()
	h := uitest.New(t, uitest.Apple(http.NotFoundHandler()))
	in := newInspector(h.Env)
	h.Env.Actions.Register(in.module().Actions...)
	r := &rig{h: h, in: in}
	r.tt = h.Tester(t, func(c *ui.Context) { in.panel(c) }, 1000, 800)
	return r
}

// An item of each kind offers Inspect, last in its menu, in the Developer group.
func TestEveryKindOffersInspectLastInItsMenu(t *testing.T) {
	r := newRig(t)
	for name, it := range items {
		defs := r.h.Env.Actions.For(it.Kind)
		last := defs[len(defs)-1]
		if last.ID != ActionID+string(it.Kind) || last.Group != kit.GroupDeveloper {
			t.Errorf("%s: last action = %s group %d", name, last.ID, last.Group)
		}
		if last.Available != nil && !last.Available(r.h.Env.Actions.Context([]kit.Item{it}, nil, nil)) {
			t.Errorf("%s: Inspect not available", name)
		}
	}
	for _, k := range []kit.ItemKind{kit.KindArtist, kit.KindMusicVideo} {
		if !r.h.Env.Actions.Has(k) {
			t.Errorf("%s has no actions", k)
		}
	}
}

// Running Inspect shows the item's IDs and artwork, from what Go's cache
// holds, redacted.
func TestInspectShowsIDsArtworkAndTheCachedAppleResource(t *testing.T) {
	r := newRig(t)
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
		{"song", "1440", songArt, true},
		{"album", "1700", albumArt, false},
		{"hero", "pl.hero", listArt, false},
		{"station", "ra.978194965", radioArt, false},
	}
	for _, tc := range cases {
		it := items[tc.name]
		for _, def := range r.h.Env.Actions.For(it.Kind) {
			if def.ID == ActionID+string(it.Kind) {
				def.Run(r.h.Env.Actions.Context([]kit.Item{it}, nil, nil))
			}
		}
		r.tt.Until(tc.name+" loaded", func() bool { r.tt.Drain(); return !r.in.loading })
		for _, want := range []string{"id: " + tc.id, "artwork: " + tc.art, `"id": "` + tc.id + `"`} {
			if !strings.Contains(r.in.text, want) {
				t.Errorf("%s: panel lacks %q:\n%s", tc.name, want, r.in.text)
			}
		}
		if got := strings.Contains(r.in.text, "Airbag\"") && strings.Contains(r.in.text, "catalog cache gb|song-artists|1440"); got != tc.raw {
			t.Errorf("%s: Apple's resource shown = %v, want %v:\n%s", tc.name, got, tc.raw, r.in.text)
		}
		if strings.Contains(r.in.text, secret) {
			t.Errorf("%s: a token reached the panel", tc.name)
		}
		if !r.tt.HasText("Copy as JSON") || !r.tt.HasText("Copy ID") || !r.tt.HasText("Copy artwork URL") {
			t.Errorf("%s: buttons missing: %q", tc.name, r.tt.Texts())
		}
	}
}

func TestInspectCopiesJSONIDAndArtworkURL(t *testing.T) {
	r := newRig(t)
	r.in.show(items["album"])
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
