package kit

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// -update-controls rewrites the controls goldens (internal/ui/uitest's Golden
// takes this over; the flag carries the area so the kit areas' helpers don't
// collide in one test binary).
var updateControls = flag.Bool("update-controls", false, "rewrite internal/ui/kit/testdata/golden/controls-*.png")

var controlsFont sync.Once

// controlsSheet renders build in a w×h window light and dark and compares
// each image with testdata/golden/controls-<name>-<mode>.png (tolerance: a
// pixel may differ by 6 per channel, 0.2% of pixels by more). setup may drive
// the tester (open a popup, move the pointer) before the picture is taken.
func controlsSheet(t *testing.T, name string, w, h int, build func(c *ui.Context), setup func(tt *ui.Tester)) {
	t.Helper()
	controlsFont.Do(func() {
		if err := theme.RegisterFonts(); err != nil {
			t.Fatal(err)
		}
	})
	for _, mode := range []struct {
		name string
		m    theme.Mode
	}{{"light", theme.ModeLight}, {"dark", theme.ModeDark}} {
		t.Run(mode.name, func(t *testing.T) {
			theme.SetMode(mode.m)
			// Still pictures: the skeleton's pulse and the colour eases run on
			// the clock.
			theme.SetReducedMotion(true)
			defer func() {
				theme.SetMode(theme.ModeSystem)
				theme.SetReducedMotion(false)
			}()
			tt := ui.NewTester(func(c *ui.Context) {
				theme.Apply(c)
				ui.Column(c).Fill().Padding(20).Gap(14).Background(theme.C("ground")).TextColor(theme.C("ink")).Children(func() { build(c) })
			}, w, h)
			if setup != nil {
				setup(tt)
			}
			tt.Advance(time.Second)
			controlsCompare(t, fmt.Sprintf("controls-%s-%s.png", name, mode.name), tt.Image())
		})
	}
}

func controlsCompare(t *testing.T, file string, got *image.RGBA) {
	t.Helper()
	path := filepath.Join("testdata", "golden", file)
	if *updateControls {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, got); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("no golden %s (run with -update-controls): %v", path, err)
	}
	defer func() { _ = f.Close() }()
	want, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if want.Bounds() != got.Bounds() {
		t.Fatalf("%s: size %v, golden %v", file, got.Bounds(), want.Bounds())
	}
	var off int
	b := got.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			wr, wg, wb, _ := want.At(x, y).RGBA()
			g := got.RGBAAt(x, y)
			if absDiff(int(wr>>8), int(g.R)) > 6 || absDiff(int(wg>>8), int(g.G)) > 6 || absDiff(int(wb>>8), int(g.B)) > 6 {
				off++
			}
		}
	}
	if limit := b.Dx() * b.Dy() / 500; off > limit {
		t.Errorf("%s: %d pixels differ (allowed %d); run with -update-controls and look at it if the change is meant", file, off, limit)
		// Leave the picture beside the golden for a look.
		if out, err := os.Create(filepath.Join(os.TempDir(), file)); err == nil {
			_ = png.Encode(out, got)
			_ = out.Close()
			t.Logf("got: %s", out.Name())
		}
	}
}

func absDiff(a, b int) int {
	if a < b {
		return b - a
	}
	return a - b
}
