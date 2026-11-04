package kit

import (
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The shared Golden helper arrives with PR 0.3 (internal/ui/uitest); until
// then this is a tiny one of the area's own: images in
// testdata/golden/collections-<scene>-<light|dark>.png, compared with a
// tolerance, rewritten by `go test -update`. KIT_EVIDENCE_DIR also writes
// every scene there (the PR's evidence).

func init() {
	if flag.Lookup("update") == nil {
		flag.Bool("update", false, "rewrite the golden images")
	}
}

func collectionsUpdate() bool { return flag.Lookup("update").Value.String() == "true" }

func collectionsAbsDiff(a, b uint32) uint32 {
	if a > b {
		return a - b
	}
	return b - a
}

func collectionsWritePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}

// collectionsGolden compares img with the stored golden: a channel may differ
// by 6 of 255 (the CPU renderer's anti-aliasing across machines) in at most
// 0.5 % of the pixels.
func collectionsGolden(t *testing.T, name string, img *image.RGBA) {
	t.Helper()
	if dir := os.Getenv("KIT_EVIDENCE_DIR"); dir != "" {
		if err := collectionsWritePNG(filepath.Join(dir, "collections-"+name+".png"), img); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join("testdata", "golden", "collections-"+name+".png")
	if collectionsUpdate() {
		if err := collectionsWritePNG(path, img); err != nil {
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
	bad, b := 0, img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g := img.RGBAAt(x, y)
			w0, w1, w2, w3 := want.At(x, y).RGBA()
			if collectionsAbsDiff(uint32(g.R), w0>>8) > 6 || collectionsAbsDiff(uint32(g.G), w1>>8) > 6 ||
				collectionsAbsDiff(uint32(g.B), w2>>8) > 6 || collectionsAbsDiff(uint32(g.A), w3>>8) > 6 {
				bad++
			}
		}
	}
	if limit := b.Dx() * b.Dy() / 200; bad > limit {
		t.Errorf("%s: %d pixels differ (limit %d); run `go test -update` if the change is intended", name, bad, limit)
	}
}

// collectionsScenes are the components in a state worth looking at, over the
// views the behaviour tests drive.
var collectionsScenes = []struct {
	name  string
	build func(t *testing.T) *ui.Tester
}{
	{"grid", func(t *testing.T) *ui.Tester {
		a, tt := newCoverApp(t, 12, 820, 560)
		inner := a.items
		a.items = func(i int) *CoverItem {
			it := inner(i)
			switch i {
			case 1:
				it.Round, it.Explicit, it.Play = true, true, nil
			case 2:
				it.Play, it.Badge = nil, "Out 19 Nov"
			case 3:
				it.Subtitle = ""
			case 5:
				it.PlaysOnOpen, it.Open = true, nil
			}
			return it
		}
		tt.Frame()
		a.st.Sel.Click(6, ClickMods{})
		a.st.Sel.Click(7, ClickMods{Toggle: true})
		tt.Move(100, 120)
		tt.Advance(300 * time.Millisecond)
		return tt
	}},
	{"grid-loading", func(t *testing.T) *ui.Tester {
		a, tt := newCoverApp(t, 8, 820, 360)
		inner := a.items
		a.items = func(i int) *CoverItem {
			if i >= 4 {
				return nil
			}
			return inner(i)
		}
		tt.Frame()
		return tt
	}},
	{"shelf", func(t *testing.T) *ui.Tester {
		a, tt := newShelfApp(t, 12, 820)
		a.items[0].Play = &CoverPlay{Label: "Album 0", Run: func() {}}
		a.items[1].Round = true
		a.items[2].Explicit = true
		tt.SetSize(820, 330)
		tt.Frame()
		tt.Frame()
		tt.Move(100, 140)
		tt.Advance(300 * time.Millisecond)
		return tt
	}},
	{"tracks", func(t *testing.T) *ui.Tester {
		a, tt := newTrackApp(t, 9, 820, 460)
		a.current = "s1"
		a.songs[2].Explicit = true
		a.songs[4].Playable = false
		a.opts.Popular = map[string]bool{"c3": true}
		tt.Frame()
		a.st.Sel.Click(3, ClickMods{})
		a.st.Sel.Click(5, ClickMods{Range: true})
		x, y := collRowPos(t, tt, "Song 0")
		tt.Move(x, y)
		tt.Advance(300 * time.Millisecond)
		return tt
	}},
	{"tracks-playlist", func(t *testing.T) *ui.Tester {
		a, tt := newTrackApp(t, 7, 820, 420)
		a.opts.Columns = TrackColumns{RowNumbers: true, Artwork: true, Album: true}
		a.opts.Discs = true
		for i := 4; i < 7; i++ {
			a.songs[i].DiscNumber = 2
		}
		a.opts.AddedBy = func(i int) *services.SocialProfile {
			if i%2 == 0 {
				return &services.SocialProfile{ID: "sp." + strconv.Itoa(i), Name: "Kim Lee"}
			}
			return nil
		}
		a.opts.Sort = &TrackSortState{Key: SortAlbum, Desc: true}
		tt.Frame()
		return tt
	}},
	{"drag", func(t *testing.T) *ui.Tester {
		d, tt := newDropApp(t)
		_ = d
		tt.SetSize(820, 420)
		x, y := collPos(t, tt, "Artist 2")
		y -= 100
		tx, ty := collPos(t, tt, "Road trip")
		tt.Press(x, y)
		tt.Move(x+10, y)
		tt.Move(x+20, y+10)
		tt.Move(tx-40, ty+80)
		tt.Move(tx-39, ty+81)
		return tt
	}},
}

func TestCollectionsGolden(t *testing.T) {
	for _, sc := range collectionsScenes {
		for _, dark := range []bool{false, true} {
			mode := "light"
			if dark {
				mode = "dark"
			}
			t.Run(sc.name+"-"+mode, func(t *testing.T) {
				tt := sc.build(t)
				if dark {
					theme.SetMode(theme.ModeDark)
					tt.SetDark(true)
				}
				tt.Frame()
				tt.Advance(50 * time.Millisecond)
				collectionsGolden(t, sc.name+"-"+mode, tt.Image())
			})
		}
	}
}
