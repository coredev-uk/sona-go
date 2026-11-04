package kit

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// controlsText is a Text in one of the theme's type styles ("body", "label",
// "secondary", "meta", "heading"…): its size, fixed line height, weight and
// tracking.
func controlsText(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// controlsRadius is a radius token of the current theme.
func controlsRadius(name string) float32 { return theme.Current().Radius[name] }

// controlsDisabled disables an element: pointer and keyboard pass it by, and
// MyGo paints it at 50% (the CSS disabled:opacity-50 + pointer-events-none).
func controlsDisabled(e ui.Element, off bool) {
	if off {
		e.Disabled(true)
	}
}

// controlsUndim is the colour that, painted at 50% over the ground, comes
// out as want: for the one disabled thing that keeps full opacity in
// v0.2.0 (the Play disc).
func controlsUndim(want ui.Color) ui.Color {
	g := theme.C("ground")
	f := func(w, g uint8) uint8 { return uint8(min(max(2*int(w)-int(g), 0), 255)) }
	return ui.RGB(f(want.R, g.R), f(want.G, g.G), f(want.B, g.B))
}

// controlsFilledSVG are the solid forms of a few Lucide glyphs (ISC, from the
// icons the theme embeds with their fill turned on and their stroke off), for
// what is "on": the play disc's triangle, a favourite star.
var controlsFilledSVG = map[string]string{
	"play":        `<path stroke="none" d="M5 5a2 2 0 0 1 3.008-1.728l11.997 6.998a2 2 0 0 1 .003 3.458l-12 7A2 2 0 0 1 5 19z"/>`,
	"star":        `<path d="M11.525 2.295a.53.53 0 0 1 .95 0l2.31 4.679a2.123 2.123 0 0 0 1.595 1.16l5.166.756a.53.53 0 0 1 .294.904l-3.736 3.638a2.123 2.123 0 0 0-.611 1.878l.882 5.14a.53.53 0 0 1-.771.56l-4.618-2.428a2.122 2.122 0 0 0-1.973 0L6.396 21.01a.53.53 0 0 1-.77-.56l.881-5.139a2.122 2.122 0 0 0-.611-1.879L2.16 9.795a.53.53 0 0 1 .294-.906l5.165-.755a2.122 2.122 0 0 0 1.597-1.16z"/>`,
	"thumbs-down": `<path d="M9 18.12 10 14H4.17a2 2 0 0 1-1.92-2.56l2.33-8A2 2 0 0 1 6.5 2H20a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-2.76a2 2 0 0 0-1.79 1.11L12 22a3.13 3.13 0 0 1-3-3.88Z"/><path d="M17 14V2" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>`,
	"pause":       `<rect stroke="none" x="14" y="3" width="5" height="18" rx="1"/><rect stroke="none" x="5" y="3" width="5" height="18" rx="1"/>`,
}

var controlsFilledIcons struct {
	mu sync.Mutex
	m  map[string]*ui.SVG
}

// controlsIcon is the Lucide icon name, in its solid form when filled and it
// has one.
func controlsIcon(name string, filled bool) *ui.SVG {
	if !filled {
		return theme.Icon(name)
	}
	canon, ok := theme.IconName(name)
	if !ok {
		return theme.Icon(name) // a bug: reports it as theme does
	}
	body, ok := controlsFilledSVG[canon]
	if !ok {
		return theme.Icon(name)
	}
	controlsFilledIcons.mu.Lock()
	defer controlsFilledIcons.mu.Unlock()
	if s := controlsFilledIcons.m[canon]; s != nil {
		return s
	}
	if controlsFilledIcons.m == nil {
		controlsFilledIcons.m = map[string]*ui.SVG{}
	}
	s := ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
	controlsFilledIcons.m[canon] = s
	return s
}

// controlsWatchWidth is e's width in the last frame, and asks for another
// frame when it is not the one the frame before saw (or there was none yet),
// so what is laid out from a measured width settles: the clamp of Notes, the
// slots of an empty page.
func controlsWatchWidth(c *ui.Context, e ui.Element) float32 {
	last := ui.Local(e, "watched", func() float32 { return -1 })
	w := e.Bounds().W
	if w != *last {
		*last = w
		c.After(time.Millisecond)
	}
	return w
}
