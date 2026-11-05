package kit

import (
	"regexp"
	"strconv"

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
	// Fallback replaces the glyph and its --hover square while there is no
	// picture: it builds elements that fill the cover's box (PersonAvatar's
	// initials). The picture still shows when it arrives.
	Fallback func(c *ui.Context)
	// A11yLabel names a cover that matters for assistive technology; covers
	// are decorations without it.
	A11yLabel string
	// Aspect is the box's width over its height (16.0/9 for a video frame);
	// 0 is square. size is the width.
	Aspect float32
	// Contain shows the whole picture inside the box at its own shape, read
	// from the "<w>x<h>.jpg" of the URL (a banner stays wide), where the
	// default fills the box (Apple crops artwork to the box asked for). The
	// picture is decoded at the contained size.
	Contain bool
}

// sizePart is the "<w>x<h>.jpg" (or "<n>.jpg", n×n) a Go-served art URL
// ends in: the shape Apple serves the artwork in (lib/art.ts).
var sizePart = regexp.MustCompile(`/(\d+)(?:x(\d+))?\.jpg(?:$|\?)`)

// artAspect is the width over the height of the artwork behind a Go URL; 1
// when it names no size.
func artAspect(src string) float32 {
	m := sizePart.FindStringSubmatch(src)
	if m == nil {
		return 1
	}
	w, _ := strconv.Atoi(m[1])
	h := w
	if m[2] != "" {
		h, _ = strconv.Atoi(m[2])
	}
	if w <= 0 || h <= 0 {
		return 1
	}
	return float32(w) / float32(h)
}

// containBox is the largest box of the given aspect that fits w×h.
func containBox(w, h, aspect float32) (float32, float32) {
	if aspect >= w/h {
		return w, w / aspect
	}
	return h * aspect, h
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
	w, h := artBox(size, o)
	return artFrame(c, l, src, w, h, o, false)
}

// FluidArtwork is a cover that fills its container's width (grid tiles) at
// the aspect of o (square, or 16:9 for a music video's frame). size is the
// largest width it renders at: the picture is decoded for it, so the request
// stays exact, and the box scales it down to the width it gets. The
// placeholder glyph is 40% of size.
func FluidArtwork(c *ui.Context, l *ArtLoader, src string, size float32, o ArtOpts) ui.Element {
	w, h := artBox(size, o)
	return artFrame(c, l, src, w, h, o, true)
}

// PrefetchArtwork has the loader fetch and decode the cover that Artwork would
// show for the same arguments before anything shows it: a lead that is about to
// rotate in, the tiles a list will scroll to. It builds nothing and draws
// nothing. The cover comes after every cover on screen or built into the
// page, and like them it is called off when a pass of the view does not ask
// for it again: call it in the build, every pass, for as long as the cover is
// worth having.
func PrefetchArtwork(c *ui.Context, l *ArtLoader, src string, size float32, o ArtOpts) {
	if l == nil {
		return
	}
	w, h := artBox(size, o)
	pw, ph := artPicture(src, w, h, o)
	l.cover(l.view(c), rankAhead, src, pw, ph)
}

// artBox is the box of the width size that Artwork builds for o.
func artBox(size float32, o ArtOpts) (w, h float32) {
	w, h = size, size
	if o.Aspect > 0 {
		h = size / o.Aspect
	}
	return w, h
}

// artPicture is the size the picture of src is asked for in a box of w×h: the
// box, or for Contain the artwork's own shape inside it.
func artPicture(src string, w, h float32, o ArtOpts) (float32, float32) {
	if o.Contain {
		return containBox(w, h, artAspect(src))
	}
	return w, h
}

func artFrame(c *ui.Context, l *ArtLoader, src string, w, h float32, o ArtOpts, fluid bool) ui.Element {
	tk := theme.Current()
	radius := o.Radius
	if radius == 0 {
		radius = tk.Radius["sm"]
	}
	if o.Round {
		radius = tk.Radius["full"]
	}

	box := ui.Box(c).Radius(radius).ClipX().ClipY()
	// The picture is asked for at the box, or for Contain at the artwork's
	// own shape inside it. It is wanted first when the box showed in the
	// last frame.
	pw, ph := artPicture(src, w, h, o)
	r := rankBuilt
	if box.InView() {
		r = rankView
	}
	v := l.view(c)
	cv := l.cover(v, r, src, pw, ph)
	if fluid {
		box.FillWidth().AspectRatio(w / h)
	} else {
		box.Size(w, h)
	}
	box.Draw(func(p *ui.Painter, _ ui.Rect) { l.painted(v, p.Scale()) })
	if o.A11yLabel != "" {
		box.Label(o.A11yLabel)
	}
	bmp := cv.bitmap()
	box.Children(func() {
		switch {
		case bmp != nil:
			img := ui.Image(c, bmp).Radius(radius).Draw(func(*ui.Painter, ui.Rect) { cv.drawn() })
			fit := ui.Cover
			if o.Contain {
				fit = ui.Contain
			}
			if fluid {
				img.Fill().Fit(fit)
			} else {
				img.Size(w, h).Fit(fit)
			}
		case o.Fallback != nil:
			o.Fallback(c)
		default:
			placeholder(c, w, h, o, fluid)
		}
	})
	if o.Edge && (bmp != nil || o.Fallback != nil) {
		// Over the picture (or the fallback), inside its edge, as an
		// outline with a -1px offset (a border would take room from it,
		// and the picture is decoded for the whole box).
		edge := theme.C("edge")
		box.DrawOver(func(p *ui.Painter, r ui.Rect) { p.Stroke(r, edge, radius, 1) })
	}
	if bmp == nil && o.Fallback == nil {
		box.Background(theme.C("hover"))
	}
	return box
}

// placeholder is the missing-art glyph, centred: 40% ink, 1.5 stroke, 40%
// of the box (at least 12) on a --hover square.
func placeholder(c *ui.Context, w, h float32, o ArtOpts, fluid bool) {
	name := o.Placeholder
	if name == "" {
		name = "music"
		if o.Round {
			name = "user"
		}
	}
	glyph := max(12, min(w, h)*0.4)
	row := ui.Row(c).Center()
	if fluid {
		row.Fill()
	} else {
		row.Size(w, h)
	}
	row.Children(func() {
		ui.Icon(c, theme.Icon(name)).FontSize(glyph).TextColor(theme.C("ink").Alpha(0.4))
	})
}
