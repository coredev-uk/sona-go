package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// OriginSliderOpts are the options of OriginSlider.
type OriginSliderOpts struct {
	SliderOpts
	// Origin is the value the range is drawn from: the fill runs between it
	// and the value, up or down (a fader's 0 dB line). It is clamped to lo..hi.
	Origin float64
	// Length fixes a vertical slider's height (Slider's is 176 at least);
	// 0 keeps it. Dim draws the whole control at this opacity when below 1
	// and above 0 (a control that is off but still editable).
	Length float32
	Dim    float32
}

// OriginSlider is Slider whose filled part starts at Origin instead of lo:
// a bipolar control (an equaliser band, a balance) fills from its centre
// line towards the value, in either direction. Everything else is Slider's:
// the keys, the drag, the 12px thumb, the focus ring and the labels.
func OriginSlider(c *ui.Context, value *float64, lo, hi float64, o OriginSliderOpts) ui.Element {
	s := Slider(c, value, lo, hi, o.SliderOpts)
	if o.Vertical && o.Length > 0 {
		s.Height(o.Length).MinHeight(o.Length)
	}
	if o.Dim > 0 && o.Dim < 1 {
		s.Opacity(o.Dim)
	}
	const thumb = 12
	pos := func(v float64) float32 {
		if hi <= lo {
			return 0
		}
		return float32(min(max((v-lo)/(hi-lo), 0), 1))
	}
	at, from := pos(*value), pos(min(max(o.Origin, lo), hi))
	rtl := c.Direction() == ui.RightToLeft && !o.Vertical
	if rtl {
		at, from = 1-at, 1-from
	}
	track, ink, ring := theme.C("ink-faint").Alpha(0.4), theme.C("ink"), theme.C("ring")
	rng := o.Range
	if rng == (ui.Color{}) {
		rng = theme.C("primary")
	}
	focus, vertical := s.FocusVisible(), o.Vertical
	s.Draw(func(p *ui.Painter, r ui.Rect) {
		a, b := min(at, from), max(at, from)
		var tr, fill, k ui.Rect
		if vertical {
			x := r.X + (r.W-4)/2
			y0, h := r.Y+thumb/2, r.H-thumb
			tr = ui.Rect{X: x, Y: y0, W: 4, H: h}
			fill = ui.Rect{X: x, Y: y0 + h*(1-b), W: 4, H: h * (b - a)}
			k = ui.Rect{X: r.X + (r.W-thumb)/2, Y: y0 + h*(1-at) - thumb/2, W: thumb, H: thumb}
		} else {
			y := r.Y + (r.H-4)/2
			x0, w := r.X+thumb/2, r.W-thumb
			tr = ui.Rect{X: x0, Y: y, W: w, H: 4}
			fill = ui.Rect{X: x0 + w*a, Y: y, W: w * (b - a), H: 4}
			k = ui.Rect{X: x0 + w*at - thumb/2, Y: r.Y + (r.H-thumb)/2, W: thumb, H: thumb}
		}
		p.Fill(tr, track, 2)
		if fill.W > 0 && fill.H > 0 {
			p.Fill(fill, rng, 2)
		}
		if focus {
			p.Fill(ui.Rect{X: k.X - 4, Y: k.Y - 4, W: k.W + 8, H: k.H + 8}, ring, thumb/2+4)
		}
		p.Fill(k, ink, thumb/2)
	})
	return s
}
