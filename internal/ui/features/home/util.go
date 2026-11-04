package home

import (
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// widthTracker follows the width an element laid out at, for what depends on
// it (v0.2.0's useElementWidth): the width of the last frame, 0 before the
// first.
type widthTracker struct{ w float32 }

// track reads root's width and asks for another frame when it changes. Call
// it on the element before its children are built.
func (m *widthTracker) track(c *ui.Context, root ui.Element) float32 {
	root.Draw(func(_ *ui.Painter, r ui.Rect) {
		if r.W != m.w {
			m.w = r.W
			c.Invalidate() // the columns follow the width
		}
	})
	if w := root.Bounds().W; w > 0 && m.w == 0 {
		m.w = w
	}
	return m.w
}

// text is a text in one of the theme's type styles.
func text(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// fade shows or hides e over the theme's press duration, at once while
// motion is reduced.
func fade(e ui.Element, show bool) {
	to := float32(0)
	if show {
		to = 1
	}
	if theme.ReducedMotion() {
		e.Opacity(to)
		return
	}
	e.Opacity(e.Animate("home.fade", to, theme.Current().Motion["press"].Duration))
}

// playGlyph is the solid play triangle (Lucide's, ISC, as kit's play
// button draws it), for the play mark over a cover.
var playGlyph = sync.OnceValue(func() *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="currentColor" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path stroke="none" d="M5 5a2 2 0 0 1 3.008-1.728l11.997 6.998a2 2 0 0 1 .003 3.458l-12 7A2 2 0 0 1 5 19z"/></svg>`))
})

// hexColour parses an item's "#rrggbb" colour; false for anything else.
func hexColour(s string) (ui.Color, bool) {
	if len(s) != 7 || s[0] != '#' {
		return ui.Color{}, false
	}
	for i := 1; i < 7; i++ {
		if ch := s[i]; (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return ui.Color{}, false
		}
	}
	return ui.Hex(s), true
}

// relativeTime is v0.2.0's relativeTime: "just now", then minutes, hours and
// days ago in short form; stamp is Unix milliseconds (0: never).
// Stand-in for the shared formatter: drop for i18n's when it lands.
func relativeTime(stamp int64, now time.Time) string {
	if stamp <= 0 {
		return i18n.T("never")
	}
	s := int64(now.Sub(time.UnixMilli(stamp)).Round(time.Second) / time.Second)
	if s < 45 {
		return i18n.T("just now")
	}
	m := (s + 30) / 60
	if m < 60 {
		return i18n.T("{minutes, plural, one {# min. ago} other {# min. ago}}", "minutes", m)
	}
	h := (m + 30) / 60
	if h < 24 {
		return i18n.T("{hours, plural, one {# hr. ago} other {# hr. ago}}", "hours", h)
	}
	d := (h + 12) / 24
	if d == 1 {
		return i18n.T("yesterday")
	}
	return i18n.T("{days, plural, one {# day ago} other {# days ago}}", "days", d)
}
