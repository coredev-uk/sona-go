package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

func webItem() services.Item {
	return services.Item{Kind: services.ItemLink, Name: "Web One", URL: "https://example.com/c"}
}

func TestOpensInBrowserIsAnItemWithAWebPageAndNoSonaPage(t *testing.T) {
	p := &ItemPlayers{}
	p.Register(services.ItemStation, func(services.Item) {})
	for name, c := range map[string]struct {
		it   services.Item
		want bool
	}{
		"web link":                        {webItem(), true},
		"unplayable station without page": {stationItem(false, false), true},
		"playable station without page":   {stationItem(false, true), false},
		"station with a page":             {stationItem(true, true), false},
		"album":                           {albumItem(1), false},
	} {
		if got := OpensInBrowser(c.it, p); got != c.want {
			t.Errorf("%s: OpensInBrowser = %v, want %v", name, got, c.want)
		}
	}
}

// Every surface that shows a web-only item draws one arrow out and names the
// opener the same way; an item that stays in Sona has neither.
func TestWebOnlyItemsDrawOneCueOnEverySurface(t *testing.T) {
	surfaces := map[string]func(it services.Item) (*itemsApp, *ui.Tester){
		"cover tile": func(it services.Item) (*itemsApp, *ui.Tester) {
			st := &CoverShelfState{}
			a, tt := newItemsApp(t, 400, 400, nil, func(c *ui.Context, ctx ItemsCtx) {
				ItemShelf(c, st, "Things", []services.Item{it}, ctx, ItemShelfOpts{First: true})
			})
			a.load(t, tt, 0)
			return a, tt
		},
		"wide card": func(it services.Item) (*itemsApp, *ui.Tester) {
			st := &WideRowState{}
			a, tt := newItemsApp(t, 600, 400, nil, func(c *ui.Context, ctx ItemsCtx) {
				WideRow(c, st, []services.Item{it}, ctx, WideRowOpts{Title: "Hero", First: true, Min: 240, Ratio: 2})
			})
			a.load(t, tt, 0)
			return a, tt
		},
		"link tile": func(it services.Item) (*itemsApp, *ui.Tester) {
			return linkListApp(t, 600, "Links", []services.Item{it})
		},
	}
	for name, build := range surfaces {
		t.Run(name, func(t *testing.T) {
			a, tt := build(webItem())
			if n := countText(tt, "Opens in your browser"); n != 1 {
				t.Errorf("a web link has %d arrows out, want 1: %q", n, tt.Texts())
			}
			if err := tt.Click("Web One"); err != nil {
				t.Fatalf("no opener: %v (%q)", err, tt.Texts())
			}
			if len(a.rec.urls) != 1 {
				t.Errorf("urls %v, want the browser asked once", a.rec.urls)
			}
			_, tt = build(albumItem(1))
			if n := countText(tt, "Opens in your browser"); n != 0 {
				t.Errorf("an album has %d arrows out: %q", n, tt.Texts())
			}
		})
	}
}
