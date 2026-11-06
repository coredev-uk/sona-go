package playlist

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// chooser is the desktop's file dialog, answering with the path a test sets
// ("" is the user cancelling).
type chooser struct {
	path string
	asks atomic.Int32
}

func (c *chooser) OpenFile(context.Context, app.OpenFileOptions) (string, error) {
	c.asks.Add(1)
	return c.path, nil
}
func (c *chooser) SaveFile(context.Context, app.SaveFileOptions) (string, error) { return "", nil }

// realRig is the playlist page and the cover picker over the real services,
// on the fixture library, its playlist "Road trip" wearing the cover Apple
// generates (so Apple's designs are offered), with a file chooser that
// answers with files.
type realRig struct {
	p      *playlists
	svc    *services.PlaylistsService
	h      *uitest.Harness
	tt     *uitest.Session
	router *ui.Router
	files  *chooser
	id     string
}

func newRealRig(t *testing.T, opts ...uitest.Option) *realRig {
	t.Helper()
	return newRealRigSized(t, 1132, 900, opts...)
}

// newRealRigSized is newRealRig in a window of w×h DIPs.
func newRealRigSized(t *testing.T, width, height int, opts ...uitest.Option) *realRig {
	t.Helper()
	goldenFont(t)
	files := &chooser{}
	opts = append(opts, uitest.Platform(func(p *services.Platform) { p.Files = files }))
	h := uitest.New(t, opts...)
	t.Cleanup(kit.CancelConfirm)
	svc := services.Service[services.PlaylistsService](h.Core)
	id := fixture.PlaylistLibraryID(0)
	if _, err := h.DB.W.ExecContext(t.Context(), `UPDATE playlists SET artwork_template = ? WHERE library_id = ?`,
		fixture.GeneratedTemplate("Road trip"), id); err != nil {
		t.Fatal(err)
	}
	router := ui.NewRouter("/playlist/" + id)
	h.Env.Nav = router
	h.Env.HasRoute = func(string) bool { return true }
	p := newPlaylists(h.Env, svc, newPrefs(config.Sidebar{}, nil))
	p.init()
	h.Env.Actions = kit.NewActionRegistry(p.actions()...)
	h.Env.Actions.SetPoster(h.Main.Post)
	r := &realRig{p: p, svc: svc, h: h, router: router, files: files, id: id}
	r.tt = h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		ui.Column(c).Fill().Children(func() {
			router.View(c, func(rt *ui.Route) {
				if rt.Match(playlistPath) {
					p.playlistPage(c, rt)
				}
			})
		})
		p.dialogHost(c)
		kit.OverlayLayer(c)
	}, width, height)
	r.tt.Art = append(r.tt.Art, h.Env.Art)
	r.tt.Until("the page", r.tt.Shows("Road trip", "Songs in Road trip"))
	return r
}

// cover is the cover the services give playlist r.id on each surface that
// lists or opens it, as the URL of its large artwork.
func (r *realRig) cover(t *testing.T) map[string]string {
	t.Helper()
	ctx := context.Background()
	out := map[string]string{}
	tree, err := r.svc.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, tp := range tree.Playlists {
		if tp.Playlist.ID == r.id {
			out["sidebar"] = tp.Playlist.Art.Large
		}
	}
	d, err := r.svc.Playlist(ctx, r.id)
	if err != nil {
		t.Fatal(err)
	}
	out["page"] = d.Playlist.Art.Large
	lib := services.Service[services.LibraryService](r.h.Core)
	page, err := lib.Playlists(ctx, "title", false, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range page.Items {
		if pl.ID == r.id {
			out["library"] = pl.Art.Large
		}
	}
	found, err := services.Service[services.SearchService](r.h.Core).Library(ctx, "Road trip", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, pl := range found.Playlists {
		if pl.ID == r.id {
			out["search"] = pl.Art.Large
		}
	}
	return out
}

func (r *realRig) open(t *testing.T) {
	t.Helper()
	r.p.changeCover(r.id, "Road trip")
	r.tt.Until("the picker", r.tt.Shows("Playlist cover", "Automatic", "Bars", "Choose an image…"))
}

func (r *realRig) chosen(t *testing.T) string {
	t.Helper()
	return (&dialogsRig{tt: r.tt}).chosenCover(t)
}

// writeImage writes a w×h picture of one colour as a PNG file.
func writeImage(t *testing.T, w, h int, c color.RGBA) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Rect, image.NewUniform(c), image.Point{}, draw.Src)
	path := filepath.Join(t.TempDir(), "cover.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeQuadrants writes a w×h picture of four colours as a PNG file: the
// user's photo of the golden image.
func writeQuadrants(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{0x2f, 0x5d, 0x8a, 255}
			switch {
			case x < w/2 && y < h/2:
				c = color.RGBA{0xe9, 0xc4, 0x6a, 255}
			case x >= w/2 && y >= h/2:
				c = color.RGBA{0xc4, 0x5a, 0x3b, 255}
			case x >= w/2:
				c = color.RGBA{0x3b, 0x8f, 0x6f, 255}
			}
			img.Set(x, y, c)
		}
	}
	path := filepath.Join(t.TempDir(), "photo.png")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// agree fails unless the covers of every surface are other than before,
// and the same as one another's: one choice, one cover, everywhere.
func agree(t *testing.T, before, after map[string]string) string {
	t.Helper()
	var want string
	for surface, got := range after {
		if got == before[surface] {
			t.Errorf("%s still shows %q", surface, got)
		}
		if want == "" {
			want = got
		}
		if got != want {
			t.Errorf("%s shows %q, the others %q", surface, got, want)
		}
	}
	if len(after) != 4 {
		t.Errorf("surfaces %v, want the sidebar, the library, the page and search", after)
	}
	return want
}

// The whole path over the real services: a design, none and an uploaded image
// each change the cover the sidebar, the library list, the page and search
// show, and the choice is still there for another picker.
func TestRealCoverChoicesReachEverySurface(t *testing.T) {
	r := newRealRig(t)
	before := r.cover(t)
	r.open(t)
	if got := r.chosen(t); got != "Automatic" {
		t.Fatalf("chosen = %q", got)
	}

	if err := r.tt.Click("Bars"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("Bars is chosen", func() bool { return r.chosen(t) == "Bars" })
	preset := agree(t, before, r.cover(t))
	if !strings.HasPrefix(preset, "/art/") {
		t.Fatalf("cover %q", preset)
	}
	// The page behind the picker shows it too (the event refreshed the query).
	r.tt.Until("the page's cover follows", func() bool {
		d := r.p.detail(r.id)
		return d.HasData && d.Data.Playlist.Art.Large == preset
	})

	if err := r.tt.Click("None"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("None is chosen", func() bool { return r.chosen(t) == "None" })
	for surface, got := range r.cover(t) {
		if got != "" {
			t.Errorf("%s shows %q for none", surface, got)
		}
	}

	r.files.path = writeImage(t, 640, 480, color.RGBA{190, 60, 50, 255})
	if err := r.tt.Click("Choose an image…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the image is chosen", func() bool { return r.chosen(t) == "Your image" })
	upload := agree(t, map[string]string{}, r.cover(t))
	if upload == preset || !strings.HasPrefix(upload, "/art/") {
		t.Fatalf("uploaded cover %q (preset %q)", upload, preset)
	}

	// Persistence: the choice is the service's, not the dialog's.
	_ = r.tt.Click("Done")
	r.tt.Until("closed", func() bool { return !r.p.dialogs.open })
	covers, err := r.svc.CoverOptions(context.Background(), r.id)
	if err != nil || covers.Current != services.CoverUpload {
		t.Fatalf("CoverOptions = %+v, %v", covers, err)
	}
	r.open(t)
	r.tt.Until("the picker remembers it", func() bool { return r.chosen(t) == "Your image" })

	// Back to Apple's cover.
	if err := r.tt.Click("Automatic"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("Automatic is chosen", func() bool { return r.chosen(t) == "Automatic" })
	for surface, got := range r.cover(t) {
		if got != before[surface] {
			t.Errorf("%s shows %q after Automatic, was %q", surface, got, before[surface])
		}
	}
}

// Cancelling the file chooser is not a failure and changes nothing.
func TestRealCoverUploadCancelledAndRefused(t *testing.T) {
	r := newRealRig(t)
	r.open(t)
	before := r.cover(t)
	if err := r.tt.Click("Choose an image…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the chooser was asked", func() bool { return r.files.asks.Load() == 1 })
	r.tt.Until("the picker is usable again", func() bool { return !r.p.dialogs.busy })
	if got := r.chosen(t); got != "Automatic" || r.tt.HasText("Your image") || r.tt.HasText("Sona can't use that image") {
		t.Errorf("a cancelled chooser left %q chosen", got)
	}

	r.files.path = writeImage(t, 120, 90, color.RGBA{1, 2, 3, 255})
	if err := r.tt.Click("Choose an image…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the reason", r.tt.Shows("Sona can't use that image. It must be a JPEG or PNG within the size limits."))
	for surface, got := range r.cover(t) {
		if got != before[surface] {
			t.Errorf("a refused image changed %s", surface)
		}
	}
}

// The picker over the page, and the page with one of Apple's designs, light
// and dark.
func TestCoverPickerGolden(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"cover-picker-light", nil},
		{"cover-picker-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRealRig(t, c.opts...)
			// Over the page: the picker with a design chosen and an image of the
			// user's offered beside the automatic cover.
			r.files.path = writeQuadrants(t, 600, 600)
			r.open(t)
			if err := r.tt.Click("Choose an image…"); err != nil {
				t.Fatal(err)
			}
			r.tt.Until("the image is offered", r.tt.Shows("Your image", "Remove my image"))
			if err := r.tt.Click("Dots"); err != nil {
				t.Fatal(err)
			}
			r.tt.Until("Dots is chosen", func() bool { return r.chosen(t) == "Dots" })
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), c.name)
		})
	}
}

// In the smallest window the grid scrolls instead of running off the dialog:
// the last cover is reachable from the keyboard and is on screen when it has
// the focus.
func TestCoverPickerScrollsInTheSmallestWindow(t *testing.T) {
	r := newRealRigSized(t, 940, 600)
	r.files.path = writeQuadrants(t, 600, 600)
	r.open(t)
	if err := r.tt.Click("Choose an image…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the image is offered", r.tt.Shows("Your image", "Remove my image"))
	if err := r.tt.Click("Bars"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("Bars is chosen", func() bool { return r.chosen(t) == "Bars" })
	r.tt.Key(0, ui.KeyEnd)
	r.tt.Until("the last cover has the focus", func() bool { return r.tt.Focused("Haze") })
	r.tt.Settle()
	tr := access.Read(t, r.tt.Tester)
	n, ok := tr.Find(ui.RoleRadio, "Haze")
	if !ok || n.Has(access.Offscreen) {
		t.Errorf("the focused cover is off screen: %+v (found %v)", n, ok)
	}
	if d, ok := tr.Find(ui.RoleButton, "Done"); !ok || d.Has(access.Offscreen) {
		t.Errorf("Done is off screen: %+v (found %v)", d, ok)
	}
}
