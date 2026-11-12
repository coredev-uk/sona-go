package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// ThemePreview is a picture of Sona's window in miniature drawn from tk, at
// 1.6:1 and as wide as its parent: what the first-run setup's theme cards and
// Settings → Appearance's theme gallery show. Decorative, so it has no name.
func ThemePreview(c *ui.Context, tk *theme.Tokens) ui.Element {
	radius := theme.Current().Radius["md"]
	e := ui.Box(c).FillWidth().AspectRatio(1.6).Radius(radius).Clip()
	ink := theme.C("ink")
	e.Draw(func(p *ui.Painter, r ui.Rect) {
		MiniWindow(p, r, tk)
		// The edge, over the picture so a dark window still shows on a dark card.
		p.Stroke(r, ink.Alpha(0.2), radius, 1)
	})
	return e
}

// MiniWindow draws Sona's window in miniature in one palette: the sidebar with
// the mark, the top bar, a few rows, and the player bar with its play
// control in the theme's spot colour. Drawn from the tokens, so it follows any
// change to them.
func MiniWindow(p *ui.Painter, r ui.Rect, tk *theme.Tokens) {
	col := func(name string) ui.Color { return tk.Color[name] }
	at := func(fx, fy, fw, fh float32) ui.Rect {
		return ui.Rect{X: r.X + fx*r.W, Y: r.Y + fy*r.H, W: fw * r.W, H: fh * r.H}
	}
	pill := func(rc ui.Rect, c ui.Color) { p.Fill(rc, c, rc.H/2) }

	p.Fill(r, col("ground"), 0)

	// The sidebar: the mark and three rows.
	side := at(0, 0, 0.28, 1)
	p.Fill(side, col("surface"), 0)
	d := 0.22 * side.W
	p.Fill(ui.Rect{X: side.X + 0.14*side.W, Y: r.Y + 0.08*r.H, W: d, H: d}, col("play"), d/2)
	faint := col("ink-faint").Alpha(0.4)
	for _, row := range [...]struct{ top, w float32 }{{0.30, 0.58}, {0.42, 0.44}, {0.54, 0.52}} {
		pill(ui.Rect{X: side.X + 0.14*side.W, Y: r.Y + row.top*r.H, W: row.w * side.W, H: 0.05 * r.H}, faint)
	}

	// The top bar, with its line under it.
	bar := at(0.28, 0, 0.72, 0.16)
	p.Fill(bar, col("surface"), 0)
	p.Fill(ui.Rect{X: bar.X, Y: bar.Y + bar.H - 1, W: bar.W, H: 1}, col("line"), 0)

	// A heading and a few rows.
	pill(at(0.34, 0.26, 0.30, 0.08), col("ink").Alpha(0.8))
	muted := col("ink-muted").Alpha(0.4)
	for _, row := range [...]struct{ top, w float32 }{{0.42, 0.50}, {0.52, 0.42}, {0.62, 0.46}} {
		pill(at(0.34, row.top, row.w, 0.05), muted)
	}

	// The player bar and its amber play control.
	foot := at(0, 0.82, 1, 0.18)
	p.Fill(foot, col("surface"), 0)
	p.Fill(ui.Rect{X: foot.X, Y: foot.Y, W: foot.W, H: 1}, col("line"), 0)
	disc := 0.48 * foot.H
	p.Fill(ui.Rect{X: foot.X + (foot.W-disc)/2, Y: foot.Y + (foot.H-disc)/2, W: disc, H: disc}, col("play"), disc/2)
}
