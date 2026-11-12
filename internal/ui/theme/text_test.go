package theme

import (
	"bytes"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestTextIsTheTypeStylesChain(t *testing.T) {
	shoot := func(build func(c *ui.Context)) *ui.Tester {
		tt := ui.NewTester(func(c *ui.Context) {
			Apply(c)
			ui.Column(c).Fill().Background(C("ground")).Padding(8).Children(func() { build(c) })
		}, 240, 80)
		tt.Frame()
		return tt
	}
	for _, style := range []string{"display", "title", "label", "meta"} {
		ty := Current().Type[style]
		want := shoot(func(c *ui.Context) {
			ui.Text(c, "Sona").FontSize(ty.Size).FixedLineHeight(ty.LineHeight).FontWeight(int(ty.Weight)).LetterSpacing(ty.Tracking).TextColor(C("ink"))
		})
		got := shoot(func(c *ui.Context) { Text(c, "Sona", style).TextColor(C("ink")) })
		styled := shoot(func(c *ui.Context) { StyleText(ui.Text(c, "Sona"), style).TextColor(C("ink")) })
		if !bytes.Equal(want.Image().Pix, got.Image().Pix) {
			t.Errorf("%s: Text differs from the chain it replaces", style)
		}
		if !bytes.Equal(want.Image().Pix, styled.Image().Pix) {
			t.Errorf("%s: StyleText differs from the chain it replaces", style)
		}
	}
}

func TestHairlineIsOnePixelOfLine(t *testing.T) {
	tt := ui.NewTester(func(c *ui.Context) {
		Apply(c)
		ui.Column(c).Fill().Background(C("ground")).Children(func() { Hairline(c) })
	}, 40, 10)
	tt.Frame()
	img := tt.Image()
	line, ground := C("line"), C("ground")
	if got := img.RGBAAt(20, 0); got.R != line.R || got.G != line.G || got.B != line.B {
		t.Errorf("row 0 is %v, want the line colour %v", got, line)
	}
	if got := img.RGBAAt(20, 5); got.R != ground.R || got.G != ground.G || got.B != ground.B {
		t.Errorf("row 5 is %v, want the ground %v", got, ground)
	}
}
