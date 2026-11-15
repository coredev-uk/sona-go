package kit

import (
	"bytes"
	"image"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// pixelsIn is the picture inside r, to tell one frame of a fade from another.
func pixelsIn(tt *ui.Tester, r ui.Rect) []byte {
	im := tt.Image()
	sub := im.SubImage(image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H))).(*image.RGBA)
	var out []byte
	for y := 0; y < sub.Rect.Dy(); y++ {
		out = append(out, sub.Pix[y*sub.Stride:y*sub.Stride+sub.Rect.Dx()*4]...)
	}
	return out
}

// fadesOver drives a control to show and checks the picture at half the press
// duration is neither the hidden nor the shown one, and settles on the shown
// one; with reduced motion the first frame is already the shown one.
func fadesOver(t *testing.T, tt *ui.Tester, rect ui.Rect, show func()) {
	t.Helper()
	press := theme.Current().Motion["press"].Duration
	tt.Frame()
	hidden := pixelsIn(tt, rect)
	show()
	tt.Frame()
	if theme.ReducedMotion() {
		if bytes.Equal(pixelsIn(tt, rect), hidden) {
			t.Error("reduced motion: the control is not shown at once")
		}
		return
	}
	tt.Advance(press / 2)
	mid := pixelsIn(tt, rect)
	tt.Advance(press)
	tt.Frame()
	shown := pixelsIn(tt, rect)
	if bytes.Equal(hidden, shown) {
		t.Fatal("the control never shows")
	}
	if bytes.Equal(mid, hidden) || bytes.Equal(mid, shown) {
		t.Errorf("no fade: halfway through the press duration the picture is already the hidden or the shown one")
	}
}

func TestTrackRowWashFadesInOnHoverAndShowsOnKeyboardFocus(t *testing.T) {
	collectionsFonts(t)
	for _, reduced := range []bool{false, true} {
		theme.SetReducedMotion(reduced)
		t.Cleanup(func() { theme.SetReducedMotion(false) })
		for _, how := range []string{"hover", "focus"} {
			_, tt := trackApp(t, trackItems(4), "", nil)
			tt.Frame()
			btn, ok := tt.Find("Play Song 1")
			if !ok {
				t.Fatalf("no play button on Song 1")
			}
			fadesOver(t, tt, btn, func() {
				if how == "hover" {
					tt.Move(btn.X+btn.W+30, btn.Y+btn.H/2)
					return
				}
				tt.Move(2, 2)
				tt.Key(0, ui.KeyTab)
			})
			if how == "focus" && !tt.Focused("Song 1") {
				t.Errorf("reduced=%v: Tab did not focus Song 1", reduced)
			}
		}
	}
}

func TestWideCardPlayDiscFadesInOnHoverAndFocus(t *testing.T) {
	collectionsFonts(t)
	items := []services.Item{wideItem("1", artPath("1", 1000, 250), ""), wideItem("2", artPath("2", 1000, 250), "")}
	for _, reduced := range []bool{false, true} {
		theme.SetReducedMotion(reduced)
		t.Cleanup(func() { theme.SetReducedMotion(false) })
		for _, how := range []string{"hover", "focus"} {
			a, tt, _ := wideApp(t, nil, items, 2)
			a.load(t, tt, 1)
			disc, ok := tt.Find("Play Item 1")
			if !ok {
				t.Fatal("no play disc on Item 1")
			}
			fadesOver(t, tt, disc, func() {
				if how == "hover" {
					tt.Move(disc.X+disc.W+20, disc.Y-20)
					return
				}
				tt.Move(2, 2)
				tt.Key(0, ui.KeyTab)
				tt.Key(0, ui.KeyTab)
			})
		}
	}
}
