package kit

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// BackdropTone is the token a Backdrop's scrim is mixed from.
type BackdropTone int

const (
	// ToneSurface: the bar sits on --surface (the default).
	ToneSurface BackdropTone = iota
	// ToneGround: headers and views sit on --ground.
	ToneGround
)

// BackdropOpts are the options of Backdrop.
type BackdropOpts struct {
	// Src is the pre-blurred tiny JPEG of the cover (the DTO's backdrop
	// URL, "/art/<key>/backdrop.jpg"): the artwork cache makes it in Go,
	// the UI never blurs.
	Src string
	// Color is the cover's dominant colour ("#rrggbb"), drawn at once while
	// the image decodes.
	Color string
	// Tone is the scrim's token: ToneSurface or ToneGround.
	Tone BackdropTone
}

// backdropZoom is how much larger than its box the tiny image is drawn:
// its edges, which the blur leaves darker, stay out of sight.
const backdropZoom = 1.1

// backdropFade is how long a new image takes to fade in over the last.
const backdropFade = 300 * time.Millisecond

type backdropState struct {
	cur, prev *cover // the image shown, and the one it fades over
	gen       int    // counts the images shown: each fades in from nothing
}

// Backdrop is a box whose background is a wash of the cover behind children:
// the cover's dominant colour at once, then the Go pre-blurred tiny image
// scaled to 110% (the GPU's bilinear upscaling gives the softness, nothing is
// blurred live) fading in over 300 ms, under a solid scrim of the surface or
// ground token at the theme's backdrop-scrim opacity (92% light, 86% dark),
// so text keeps 4.5:1 over any cover. Size and place the returned element as
// any box; what it holds draws over the wash.
//
// A new source is decoded before it replaces the last, so a track change
// never flashes an empty frame, and fades in over it. With neither image nor
// colour it draws nothing. Reduce effects hides the wash entirely. It is
// decoration: no role, no input.
func Backdrop(c *ui.Context, l *ArtLoader, o BackdropOpts, children func()) ui.Element {
	box := ui.Box(c)
	finish := func() ui.Element {
		if children != nil {
			box.Children(children)
		}
		return box
	}
	if theme.ReduceEffects() {
		return finish()
	}
	st := ui.Local(box, "backdrop", func() backdropState { return backdropState{} })
	cv := l.backdrop(o.Src)
	switch {
	case o.Src == "":
		st.cur, st.prev = nil, nil
	case cv != nil && cv.bitmap() != nil && cv != st.cur:
		st.prev, st.cur = st.cur, cv
		st.gen++
	}
	fade := float32(1)
	if st.cur != nil {
		fade = box.Keyframes(st.gen, []ui.Keyframe{{At: 0, Value: 0}, {At: backdropFade, Value: 1, Ease: theme.Current().Motion["backdrop"].Ease}})
		if fade >= 1 {
			st.prev = nil
		}
	}
	colour, hasColour := hexColour(o.Color)
	if st.cur == nil && !hasColour {
		return finish()
	}
	tone := "surface"
	if o.Tone == ToneGround {
		tone = "ground"
	}
	scrim := theme.C(tone).Alpha(theme.Current().Scalar["backdrop-scrim"])
	cur, prev := st.cur, st.prev
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		if hasColour {
			p.Fill(r, colour, 0)
		}
		p.Clip(r, 0, func() {
			if prev != nil {
				drawBackdropImage(p, r, prev, 1)
			}
			if cur != nil {
				drawBackdropImage(p, r, cur, fade)
			}
		})
		p.Fill(r, scrim, 0)
	})
	return finish()
}

// drawBackdropImage draws the cover's bitmap over r at backdropZoom, filling
// it (cropped to r's shape) at opacity a.
func drawBackdropImage(p *ui.Painter, r ui.Rect, cv *cover, a float32) {
	bmp := cv.bitmap()
	if bmp == nil || a <= 0 || r.W <= 0 || r.H <= 0 {
		return
	}
	cv.drawn()
	w, h := bmp.Size()
	if w == 0 || h == 0 {
		return
	}
	// The part of the bitmap that covers r.
	src := ui.Rect{W: float32(w), H: float32(h)}
	if bw, bh := float32(w)/float32(h), r.W/r.H; bw > bh {
		src.W = float32(h) * r.W / r.H
		src.X = (float32(w) - src.W) / 2
	} else {
		src.H = float32(w) * r.H / r.W
		src.Y = (float32(h) - src.H) / 2
	}
	dst := ui.Rect{
		X: r.X - r.W*(backdropZoom-1)/2, Y: r.Y - r.H*(backdropZoom-1)/2,
		W: r.W * backdropZoom, H: r.H * backdropZoom,
	}
	var tint ui.Color
	if a < 1 {
		tint = ui.Color{R: 255, G: 255, B: 255, A: max(1, uint8(a*255+0.5))}
	}
	p.ImageRegion(bmp, src, dst, ui.ImageOptions{Tint: tint})
}

// hexColour parses "#rrggbb" (as the services send colours), reporting false
// for anything else rather than panicking as ui.Hex does.
func hexColour(s string) (c ui.Color, ok bool) {
	if s == "" {
		return c, false
	}
	defer func() {
		if recover() != nil {
			c, ok = ui.Color{}, false
		}
	}()
	return ui.Hex(s), true
}
