package kit

import (
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The shared Golden helper arrives with PR 0.3 (internal/ui/uitest); until
// then this is a tiny one of the area's own: images in
// testdata/golden/overlays-<scene>-<light|dark>.png, compared with a
// tolerance, rewritten by `go test -update`.
func overlaysUpdate() bool {
	f := flag.Lookup("update")
	if f == nil {
		f = flagRegisterUpdate()
	}
	return f.Value.String() == "true"
}

func flagRegisterUpdate() *flag.Flag {
	flag.Bool("update", false, "rewrite the golden images")
	return flag.Lookup("update")
}

func init() {
	if flag.Lookup("update") == nil {
		flagRegisterUpdate()
	}
}

// overlaysGolden compares img with the stored golden: a channel may differ
// by 6 of 255 (the CPU renderer's anti-aliasing across machines) in at most
// 0.5 % of the pixels.
func overlaysGolden(t *testing.T, name string, img *image.RGBA) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".png")
	if overlaysUpdate() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("%v (run `go test -update` to write it)", err)
	}
	defer func() { _ = f.Close() }()
	want, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if want.Bounds() != img.Bounds() {
		t.Fatalf("%s: size %v, golden %v", name, img.Bounds(), want.Bounds())
	}
	bad := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g := img.RGBAAt(x, y)
			w0, w1, w2, w3 := want.At(x, y).RGBA()
			if absDiff(uint32(g.R), w0>>8) > 6 || absDiff(uint32(g.G), w1>>8) > 6 || absDiff(uint32(g.B), w2>>8) > 6 || absDiff(uint32(g.A), w3>>8) > 6 {
				bad++
			}
		}
	}
	if limit := b.Dx() * b.Dy() / 200; bad > limit {
		t.Errorf("%s: %d pixels differ (limit %d); run `go test -update` if the change is intended", name, bad, limit)
	}
}

func absDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

// overlaysScenes are the components in a state worth looking at, built
// over the same views the behaviour tests drive. The gallery and the PR's
// evidence use the same images.
var overlaysScenes = []struct {
	name  string
	build func(t *testing.T) *ui.Tester
}{
	{"menu", func(t *testing.T) *ui.Tester {
		a, tt := newMenuApp(t)
		a.items = []MenuItem{
			{Key: "l", Label: "More for Airbag", Kind: MenuLabel},
			{Key: "play", Label: "Play next", Icon: "list-start", Hint: "Ctrl+Shift+N"},
			{Key: "last", Label: "Play last", Icon: "list-end"},
			{Key: "off", Label: "Station", Icon: "radio", Reason: "Not available while listening to a live station"},
			{Key: "s1", Kind: MenuSeparator},
			{Key: "fav", Label: "Undo favourite", Icon: "star", Filled: true},
			{Key: "less", Label: "Suggest less", Icon: "thumbs-down"},
			{Key: "s2", Kind: MenuSeparator},
			{Key: "more", Label: "Add to playlist", Icon: "list-plus", Kind: MenuSub, Sub: []MenuItem{{Key: "a", Label: "New playlist"}}},
			{Key: "del", Label: "Delete from library", Icon: "trash", Destructive: true},
		}
		tt.Key(0, ui.KeyTab)
		tt.Key(0, ui.KeyEnter)
		tt.Key(0, ui.KeyDown)
		tt.Key(0, ui.KeyDown)
		tt.Advance(400 * time.Millisecond)
		return tt
	}},
	{"menu-marks", func(t *testing.T) *ui.Tester {
		a, tt := newMenuApp(t)
		a.items = []MenuItem{
			{Key: "l", Label: "View", Kind: MenuLabel},
			{Key: "q", Label: "Show queue", Kind: MenuCheckbox, Checked: true, Hint: "Ctrl+Q"},
			{Key: "m", Label: "Mini player", Kind: MenuCheckbox},
			{Key: "s", Kind: MenuSeparator},
			{Key: "r1", Label: "Compact", Kind: MenuRadio, Checked: true},
			{Key: "r2", Label: "Roomy", Kind: MenuRadio},
			{Key: "x", Label: "Inset row", Inset: true},
		}
		_ = tt.Click("Open")
		tt.Advance(400 * time.Millisecond)
		return tt
	}},
	{"dialog", func(t *testing.T) *ui.Tester {
		a, tt := newDialogApp(t, DialogOpts{Title: "Rename playlist", Description: "Pick a name that says what is in it."})
		a.footer, a.open = true, true
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		return tt
	}},
	{"confirm", func(t *testing.T) *ui.Tester {
		_, tt := newConfirmApp(t)
		_ = tt.Click("Delete…")
		tt.Advance(400 * time.Millisecond)
		return tt
	}},
	{"toasts", func(t *testing.T) *ui.Tester {
		a, tt := newToastApp(t)
		a.show = func() {
			ShowToast(ToastSpec{ID: "1", Title: "Added “Airbag” to your library", Kind: ToastSuccess, Duration: -1})
			ShowToast(ToastSpec{ID: "2", Title: "Couldn't favourite “Karma Police”", Description: "Check your connection and try again.", Kind: ToastError, Duration: -1})
			ShowToast(ToastSpec{ID: "3", Title: "Not available while listening to a live station", Kind: ToastWarning, Duration: -1})
		}
		_ = tt.Click("Notify")
		tt.Advance(400 * time.Millisecond)
		return tt
	}},
	{"toasts-loading", func(t *testing.T) *ui.Tester {
		a, tt := newToastApp(t)
		a.show = func() {
			ShowToast(ToastSpec{ID: "1", Title: "Syncing your library", Kind: ToastLoading, Duration: -1})
			ShowToast(ToastSpec{ID: "2", Title: "Removed “Airbag”", Kind: ToastInfo, Action: "Undo", Duration: -1})
		}
		_ = tt.Click("Notify")
		tt.Advance(400 * time.Millisecond)
		return tt
	}},
	{"palette", func(t *testing.T) *ui.Tester {
		a, tt := newPaletteApp(t)
		a.p.Open()
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		tt.Key(0, ui.KeyDown)
		return tt
	}},
	{"popover", func(t *testing.T) *ui.Tester {
		a, tt := newPaletteApp(t)
		a.popOpen = true
		tt.Frame()
		tt.Advance(400 * time.Millisecond)
		return tt
	}},
	{"actions-bulk", func(t *testing.T) *ui.Tester {
		a, tt := newListApp(t)
		a.scope.sel = []int{0, 2}
		tt.Frame()
		_ = tt.RightClick("Karma Police")
		tt.Advance(400 * time.Millisecond)
		return tt
	}},
	{"actions-row", func(t *testing.T) *ui.Tester {
		a, tt := newListApp(t)
		_ = tt.Click("More for Airbag")
		a.drain(t, tt, 1)
		tt.Advance(400 * time.Millisecond)
		return tt
	}},
}

func TestOverlaysGoldens(t *testing.T) {
	for _, sc := range overlaysScenes {
		for _, dark := range []bool{false, true} {
			name := "overlays-" + sc.name + "-light"
			mode := theme.ModeLight
			if dark {
				name, mode = "overlays-"+sc.name+"-dark", theme.ModeDark
			}
			t.Run(name, func(t *testing.T) {
				tt := sc.build(t)
				theme.SetMode(mode)
				tt.SetDark(dark)
				tt.Frame()
				tt.Advance(400 * time.Millisecond)
				overlaysGolden(t, name, tt.Image())
			})
		}
	}
}
