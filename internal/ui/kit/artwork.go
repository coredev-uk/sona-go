package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// ArtOpts are the options of Artwork.
type ArtOpts struct {
	// Radius rounds the corners in DIPs; 0 is the theme's artwork radius
	// (4), Round makes a circle.
	Radius float32
	// Round is a circle (people, artists) with the person glyph as the
	// placeholder.
	Round bool
	// Edge draws the 1px line at 6% ink inside the edge, so pale covers do
	// not dissolve into the ground (docs/design.md, "Artwork").
	Edge bool
	// Placeholder is the Lucide glyph shown while there is no picture: "music"
	// (and "user" for Round) by default.
	Placeholder string
	// A11yLabel names a cover that matters for assistive technology; covers
	// are decorations without it.
	A11yLabel string
	// Aspect is the box's width over its height (16.0/9 for a video frame);
	// 0 is square. size is the width.
	Aspect float32
}

// Artwork shows a cover at an exact size: src is the artwork's Go-served
// URL from a DTO ("/art/<key>/…", the size in it ignored) or an Apple
// template, size the box's width in DIPs. The picture is fetched from the
// artwork cache and decoded at size × the window's scale device pixels off
// the main thread (ArtLoader), so it is never resampled while painting; a
// --hover square with a 40%-ink glyph shows until it is ready, and stays if
// it fails (it is asked for again after 5 s and 30 s). The CPU copy of the
// pixels is dropped once the cover has been drawn twice.
func Artwork(c *ui.Context, l *ArtLoader, src string, size float32, o ArtOpts) ui.Element {
	w, h := size, size
	if o.Aspect > 0 {
		h = size / o.Aspect
	}
	tk := theme.Current()
	radius := o.Radius
	if radius == 0 {
		radius = tk.Radius["sm"]
	}
	if o.Round {
		radius = tk.Radius["full"]
	}

	cv := l.cover(src, w, h)
	box := ui.Box(c).Size(w, h).Radius(radius).ClipX().ClipY()
	box.Draw(func(p *ui.Painter, _ ui.Rect) { l.noteScale(p.Scale()) })
	if o.A11yLabel != "" {
		box.Label(o.A11yLabel)
	}
	bmp := cv.bitmap()
	box.Children(func() {
		if bmp == nil {
			placeholder(c, w, h, o)
			return
		}
		ui.Image(c, bmp).Size(w, h).Fit(ui.Cover).Radius(radius).
			Draw(func(*ui.Painter, ui.Rect) { cv.drawn() })
	})
	if o.Edge && bmp != nil {
		// Over the picture, inside its edge, as an outline with a -1px
		// offset (a border would take room from it, and the picture is
		// decoded for the whole box).
		edge := theme.C("edge")
		box.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, edge, radius, 1) })
	}
	if bmp == nil {
		box.Background(theme.C("hover"))
	}
	return box
}

// placeholder is the missing-art glyph, centred: 40% ink, 1.5 stroke, 40%
// of the box (at least 12) on a --hover square.
func placeholder(c *ui.Context, w, h float32, o ArtOpts) {
	name := o.Placeholder
	if name == "" {
		name = "music"
		if o.Round {
			name = "user"
		}
	}
	glyph := max(12, min(w, h)*0.4)
	ui.Row(c).Size(w, h).Center().Children(func() {
		ui.Icon(c, theme.Icon(name)).FontSize(glyph).TextColor(theme.C("ink").Alpha(0.4))
	})
}
