package kit

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// itemsApp is a page that builds one shared item component in the app's
// context, recording what it asks of the app: the harness of the
// collections_items tests.
type itemsApp struct {
	rec   *itemsRec
	fake  *fakeArt
	mt    *mainThread
	art   *ArtLoader
	menus *ActionMenus
	pad   float32
	// tweak changes the context of the frame (restricted, what is playing).
	tweak func(*ItemsCtx)
	build func(c *ui.Context, ctx ItemsCtx)
}

// itemsMenus is the actions of the tests: one for each kind with a menu.
func itemsMenus() *ActionMenus {
	var defs []ActionDef
	for _, k := range []ItemKind{KindSong, KindAlbum, KindStation, KindPlaylist, KindArtist} {
		defs = append(defs, ActionDef{
			ID: "test." + string(k), Target: k, Group: GroupItem,
			Label: func(*ActionContext) string { return "Do something" },
			Run:   func(*ActionContext) {},
		})
	}
	return NewActionMenus(NewActionRegistry(defs...), nil)
}

// newItemsApp is a window of w by h with the page's padding (24) around the
// component, light unless a test says otherwise; the art is the fake.
func newItemsApp(t *testing.T, w, h int, src artSource, build func(c *ui.Context, ctx ItemsCtx)) (*itemsApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	resetDrag()
	t.Cleanup(resetDrag)
	if src == nil {
		src = &fakeArt{}
	}
	mt := newMainThread()
	a := &itemsApp{rec: newItemsRec(), mt: mt, pad: 24, build: build, menus: itemsMenus()}
	a.fake, _ = src.(*fakeArt)
	a.art = newArtLoader(src, mt.post, 2)
	t.Cleanup(a.art.Close)
	return a, ui.NewTester(a.view, w, h)
}

func (a *itemsApp) view(c *ui.Context) {
	theme.Apply(c)
	ctx := a.rec.ctx()
	ctx.Art, ctx.Menus = a.art, a.menus
	if a.tweak != nil {
		a.tweak(&ctx)
	}
	ui.Column(c).Fill().Padding(a.pad).Background(theme.C("ground")).Children(func() { a.build(c, ctx) })
}

// load draws a frame, waits for n art decodes and draws the result.
func (a *itemsApp) load(t *testing.T, tt *ui.Tester, n int) {
	t.Helper()
	tt.Frame()
	if n > 0 {
		a.mt.run(t, n)
	}
	tt.Frame()
}

// box is the bounding box of the pixels near any of the colours.
func box(img *image.RGBA, match func(color.RGBA) bool) (r image.Rectangle) {
	b := img.Bounds()
	r = image.Rectangle{Min: b.Max, Max: b.Min} // empty, grown by each match
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if match(img.RGBAAt(x, y)) {
				r.Min.X, r.Min.Y = min(r.Min.X, x), min(r.Min.Y, y)
				r.Max.X, r.Max.Y = max(r.Max.X, x+1), max(r.Max.Y, y+1)
			}
		}
	}
	if r.Max.X < r.Min.X {
		return image.Rectangle{}
	}
	return r
}

// evidence writes the frame as a PNG next to the plan's other evidence, when
// SONA_EVIDENCE names the directory.
func evidence(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("SONA_EVIDENCE")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

// songItem is an Item holding a playable song.
func songItem(id, name, artist string, explicit bool) services.Item {
	s := &services.Song{ID: id, Name: name, Artist: artist, ArtistID: "ar" + id, Explicit: explicit, Playable: true,
		Art: services.Art{Small: "/art/ks" + id + "/80x80.jpg"}}
	return services.Item{Kind: services.ItemSong, ID: id, Name: name, Song: s}
}
