package equalizer

import (
	"cmp"
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry, in DIPs. The bands panel is one graphic equaliser:
// every fader column is a value, the fader and a name, and the response
// curve is drawn in the box behind the faders' travel, so that its rules and
// the faders share one dB scale.
const (
	padTop     = 28
	padBottom  = 48
	narrow     = 960  // the gutters shrink below this width (as the library's)
	maxWidth   = 1120 // the page column's widest
	chipHeight = 32
	chipGap    = 8

	panelPadX   = 24
	panelPadTop = 24
	panelPadBot = 20
	faderLength = 224 // a fader's whole height, thumb included
	faderGap    = 8   // between a fader and the lines above and below it
	lineHeight  = 16  // the value above a fader and its name below (meta)
	// faderTop is where a fader starts in its column, under its value.
	faderTop = lineHeight + faderGap
	// stageHeight is a fader column: value, fader, name.
	stageHeight = faderTop + faderLength + faderGap + lineHeight
	// thumbInset is how far a fader's travel starts inside its box (half the
	// thumb), which the curve and the rules follow.
	thumbInset = 6
	preampW    = 64 // the preamp's column
	scaleW     = 48 // the dB scale, right of the bands
	ruleGap    = 20 // either side of the rule that sets the preamp apart
	noteGap    = 20
	panelH     = panelPadTop + stageHeight + noteGap + lineHeight + panelPadBot
)

// pageState is the view state: the scroll place and the width of the last
// frame, which the layout follows.
type pageState struct {
	pos   ui.ScrollState
	width float32
}

// page is EqualizerPage: the header, the presets as a row of chips, then the
// bands panel, a graphic equaliser the width of the column. Where the
// platform has no equaliser it says so instead.
func (e *equalizer) page(c *ui.Context, _ *ui.Route) {
	tk := theme.Current()
	pg := &e.body
	gutter := tk.Metric["gutter"]
	if pg.width > 0 && pg.width < narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	devstate.Scroll(c, &pg.pos) // dev builds: the offset survives a restart
	sc := ui.Scroll(c).TrackScroll(&pg.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	sc.Children(func() {
		ui.Column(c).Padding(padTop, gutter+tk.Metric["scrollbar"], padBottom, gutter).Children(func() {
			ui.Column(c).FillWidth().MaxWidth(maxWidth).Children(func() { e.content(c) })
		})
	})
	if b := sc.Bounds(); b.W != pg.width {
		pg.width = b.W
		c.Invalidate() // the layout follows the width
	}
}

// content is everything under the scroll container's padding.
func (e *equalizer) content(c *ui.Context) {
	if !e.supported() {
		kit.PageHeader(c, i18n.T("Equaliser"), i18n.T("Not available on this platform yet"), nil)
		e.unavailable(c)
		return
	}
	settings, info := e.settings(), e.info()
	if err := cmp.Or(settings.Err, info.Err); err != nil {
		kit.PageHeaderCounted(c, i18n.T("Equaliser"), "", nil)
		kit.ErrorBlock(c, i18n.T("Couldn't load the equaliser"), err, kit.ErrorBlockOpts{
			OnRetry:  e.refetch,
			Retrying: settings.Fetching || info.Fetching,
		})
		return
	}
	if !settings.HasData || !info.HasData {
		kit.PageHeaderCounted(c, i18n.T("Equaliser"), "", nil) // the line's place is kept
		e.skeleton(c)
		return
	}
	s, in := settings.Data, info.Data

	eq := s.Audio.Equalizer
	if e.draft != nil {
		eq = *e.draft
	}
	gains := gainsOf(eq, in.Presets)
	curveName := presetName(custom, "")
	for _, p := range in.Presets {
		if p.ID == eq.Preset {
			curveName = presetName(p.ID, p.Name)
		}
	}
	room := -headroom(gains)

	subline := i18n.T("Off · Sona plays the music as mastered")
	if eq.Enabled {
		subline = i18n.T("{curveName} · shapes everything Sona plays", "curveName", curveName)
	}
	kit.PageHeader(c, i18n.T("Equaliser"), subline, func() {
		on := eq.Enabled
		name := i18n.T("Off")
		if on {
			name = i18n.T("On")
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			if kit.Switch(c, &on, kit.SwitchOpts{A11yLabel: name}).Changed() {
				next := eq
				next.Enabled = on
				e.save(s, next)
			}
			// The word is part of the switch's label: a click on it toggles too.
			// A plain row, not a button: the switch is the one control (and
			// the one Tab stop) assistive technology finds.
			word := ui.Row(c.Key("switch:word")).Cursor(ui.CursorPointer)
			word.Children(func() { text(c, name, "body").TextColor(theme.C("ink")) })
			if word.Clicked() {
				next := eq
				next.Enabled = !eq.Enabled
				e.save(s, next)
			}
		})
	})

	e.presets(c, s, eq, in)
	e.bands(c, s, eq, in, gains, room)
}

// text is a single line of text in one of the theme's type styles.
func text(c *ui.Context, s, style string) ui.Element {
	return theme.Text(c, s, style).MinWidth(0)
}

// presets is the preset chips: the user's own curve first, then the built-in
// curves, wrapping over as many rows as the width needs. They are one radio
// group and one stop of Tab; the arrows choose in reading order, wrapping,
// and the focus goes with the choice. Choosing one saves it and turns the
// equaliser on. The chosen chip is filled with ink while the equaliser is
// on; off, it keeps the selected tone and an ink outline, so the fill always
// means "this is shaping the sound".
func (e *equalizer) presets(c *ui.Context, s config.Settings, eq config.Equalizer, in services.EqualizerInfo) {
	type item struct{ id, name string }
	items := []item{{custom, presetName(custom, "")}}
	for _, p := range in.Presets {
		items = append(items, item{p.ID, presetName(p.ID, p.Name)})
	}
	sel, picked := eq.Preset, ""
	radius := theme.Current().Radius["md"]
	ui.RadioGroup(c, func() {
		for i, it := range items {
			on := it.id == eq.Preset
			r := ui.RadioBase(c.Key(it.id), &sel, it.id).Height(chipHeight).PaddingX(12).AlignItems(ui.Center).Radius(radius).
				Cursor(ui.CursorPointer).Label(it.name).Border(1, ui.Transparent)
			if i == 0 {
				r.Margin(0, chipGap, 0, 0) // your curve stands apart from the built-in ones
			}
			bg, fg := theme.C("surface"), theme.C("ink-muted")
			switch {
			case on && eq.Enabled:
				bg, fg = theme.C("ink"), theme.C("ground")
			case on:
				bg, fg = theme.C("active"), theme.C("ink")
				r.Border(1, theme.C("ink"))
			case r.Hovered():
				bg, fg = theme.C("hover"), theme.C("ink")
			}
			r.Background(bg)
			r.Children(func() { text(c, it.name, "body").FontWeight(500).TextColor(fg).SingleLine() })
			if r.Changed() {
				picked = sel
			} else if r.Clicked() {
				picked = it.id
			}
		}
	}).Row().Wrap().Gap(chipGap).FillWidth().Margin(0, 0, 20, 0).Label(i18n.T("Preset"))
	if picked != "" {
		next := eq
		next.Preset, next.Enabled = picked, true
		e.save(s, next)
	}
}

// bands is the Bands panel: the preamp, a rule, then the ten band faders
// over their response curve with the dB scale to the right, and a note on
// the level change.
func (e *equalizer) bands(c *ui.Context, s config.Settings, eq config.Equalizer, in services.EqualizerInfo, gains []float64, room float64) {
	tk := theme.Current()
	dim := !eq.Enabled
	panel := ui.Column(c).FillWidth().Padding(panelPadTop, panelPadX, panelPadBot, panelPadX).Radius(tk.Radius["lg"]).
		Background(theme.C("surface")).Role(ui.RoleGroup).Label(i18n.T("Bands"))
	panel.Children(func() {
		ui.Row(c).FillWidth().Height(stageHeight).Children(func() {
			e.fader(c, fader{
				id: "preamp", label: i18n.T("Preamp"), name: i18n.T("Preamp"), value: eq.Preamp,
				lo: in.MinPreamp, hi: in.MaxPreamp, dim: dim, width: preampW,
			}, func(db float64) {
				next := eq
				next.Preamp = db
				e.live(next)
			}, func(db float64) {
				next := eq
				next.Preamp = db
				e.save(s, next)
			})
			ui.Box(c).Width(1).Height(stageHeight).Shrink(0).Margin(0, ruleGap, 0, ruleGap).Background(theme.C("line")).Role(ui.RoleNone)

			// The curve's box is in the flow and the faders lie over it.
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Height(stageHeight).Children(func() {
				e.graphView(c, in.Frequencies, gains, in.MaxGain, dim)
				ui.Row(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Children(func() {
					for i, hz := range in.Frequencies {
						freq := formatHz(hz)
						value := 0.0
						if i < len(gains) {
							value = gains[i]
						}
						e.fader(c, fader{
							id: freq, label: i18n.T("{freq} Hz", "freq", freq), name: freq, value: value,
							lo: in.MinGain, hi: in.MaxGain, dim: dim,
						}, func(db float64) {
							e.live(withBand(eq, in.Presets, i, db))
						}, func(db float64) {
							e.save(s, withBand(eq, in.Presets, i, db))
						})
					}
				})
			})
			e.scale(c, in.MaxGain, in.MinGain)
		})
		note := i18n.T("Cuts only: the level stays as it is. Preamp lowers it further.")
		if room > 0 {
			note = i18n.T("The level drops {drop} dB ahead of the bands, so the largest boost cannot clip. Preamp lowers it further.",
				"drop", i18n.FormatNumber(math.Round(room*10)/10))
		}
		text(c, note, "meta").TextColor(theme.C("ink-muted")).Margin(noteGap, 0, 0, 0).FillWidth()
	})
}

// scale is the dB labels at the rules' height, right of the bands, and the
// unit of the names under the faders.
func (e *equalizer) scale(c *ui.Context, top, bottom float64) {
	ui.Box(c).Width(scaleW).Height(stageHeight).Shrink(0).Role(ui.RoleNone).Children(func() {
		travel := float32(faderLength - 2*thumbInset)
		for i, label := range []string{formatDb(top), i18n.T("0 dB"), formatDb(bottom)} {
			y := faderTop + thumbInset + travel*float32(i)/2 - lineHeight/2
			text(c, label, "meta").TextColor(theme.C("ink-faint")).FontFeatures("tnum").SingleLine().Role(ui.RoleNone).
				Absolute().Left(10).Right(0).Top(y)
		}
		// translators: the unit of the frequencies under the equaliser's faders (hertz)
		text(c, i18n.T("Hz"), "meta").TextColor(theme.C("ink-faint")).SingleLine().Role(ui.RoleNone).
			Absolute().Left(10).Right(0).Bottom(0)
	})
}

// fader describes one vertical fader.
type fader struct {
	id string
	// label names it for assistive technology ("947 Hz", "Preamp"); name is
	// shown under it ("947", "Preamp").
	label, name string
	value       float64
	lo, hi      float64
	// dim: the equaliser is off, the fader is still editable, drawn quieter.
	dim bool
	// width fixes the column's width; 0 shares the row with its siblings.
	width float32
}

// fader is one vertical fader: its value above, the cap on a slim track
// filled from the 0 dB line (or from the top for a cut-only control), its
// name below. change is called as the value moves, commit once when the
// pointer lets go (at once for a key).
func (e *equalizer) fader(c *ui.Context, f fader, change, commit func(db float64)) {
	col := ui.Column(c.Key(f.id)).MinWidth(0).AlignItems(ui.Center).Gap(faderGap)
	if f.width > 0 {
		col.Width(f.width).Shrink(0)
	} else {
		col.Grow(1).Basis(0)
	}
	col.Children(func() {
		ink := theme.C("ink")
		if f.value == 0 {
			ink = theme.C("ink-faint")
		}
		db := formatDb(f.value)
		text(c, db, "meta").TextColor(ink).FontFeatures("tnum").SingleLine().Role(ui.RoleNone)
		v := f.value
		opts := kit.OriginSliderOpts{
			SliderOpts: kit.SliderOpts{A11yLabel: f.label, ValueText: i18n.T("{db} dB", "db", db), Step: 0.5, Vertical: true, Range: theme.C("ink-muted")},
			Origin:     0, Length: faderLength, Cap: true,
		}
		if f.dim {
			opts.Dim = 0.55
		}
		sl := kit.OriginSlider(c.Key("fader"), &v, f.lo, f.hi, opts)
		if sl.Changed() {
			e.held[f.id] = v
			change(v)
		}
		if last, ok := e.held[f.id]; ok && !sl.Pressed() {
			delete(e.held, f.id)
			commit(last)
		}
		text(c, f.name, "meta").TextColor(theme.C("ink-muted")).FontFeatures("tnum").SingleLine().Role(ui.RoleNone)
	})
}

// The curve's points: 160 frequencies on a log scale and where each falls
// on the axis, fixed.
var (
	graphFreqs = logFrequencies(160)
	graphX     = func() []float32 {
		x := make([]float32, len(graphFreqs))
		for i, f := range graphFreqs {
			x[i] = float32(logPosition(f))
		}
		return x
	}()
)

// graphCache keeps the curve of the last gains: it is computed again only
// when they change. The paths are kept to be refilled, not reallocated.
type graphCache struct {
	freqs, gains []float64
	rng          float64
	// y is the curve's height at each point, -1 (bottom) to 1 (top).
	y           []float32
	curve, area ui.Path
}

func (g *graphCache) of(freqs, gains []float64, rng float64) []float32 {
	if g.y != nil && g.rng == rng && slices.Equal(g.freqs, freqs) && slices.Equal(g.gains, gains) {
		return g.y
	}
	db := response(freqs, gains, graphFreqs)
	y := make([]float32, len(db))
	for i, d := range db {
		y[i] = float32(max(-rng, min(rng, d)) / rng)
	}
	g.freqs, g.gains, g.rng, g.y = slices.Clone(freqs), slices.Clone(gains), rng, y
	return y
}

// graphView is ResponseGraph: the box behind the faders' travel, with the
// rules at +rng, 0 and −rng dB and the curve the filters draw (without the
// level change ahead of them) on a log frequency axis. The box is as wide as
// the ten faders and as tall as one fader, so its top and bottom rule are the
// fader's ends and the curve passes through the thumbs. It is decoration:
// assistive technology has the faders.
func (e *equalizer) graphView(c *ui.Context, freqs, gains []float64, rng float64, dim bool) {
	ys := e.graph.of(freqs, gains, rng)
	g := &e.graph
	line, ink, faint := theme.C("line"), theme.C("ink"), theme.C("ink-faint")
	box := ui.Box(c).FillWidth().Height(faderLength).Margin(faderTop, 0, 0, 0).Shrink(0).Role(ui.RoleNone)
	// Off and on cross-fade over 150 ms (unless motion is reduced).
	off := float32(0)
	if dim {
		off = 1
	}
	if !theme.ReducedMotion() {
		off = box.Animate("dim", off, 150*time.Millisecond)
	}
	fillAlpha := 0.07 - 0.04*off
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		y0, h := r.Y+thumbInset, r.H-2*thumbInset
		mid := y0 + h/2
		p.Line(r.X, y0, r.X+r.W, y0, 1, line)
		p.Line(r.X, y0+h, r.X+r.W, y0+h, 1, line)
		p.Line(r.X, mid, r.X+r.W, mid, 1, faint.Alpha(0.5))
		g.curve.Reset()
		g.area.Reset()
		for i, x := range graphX {
			px, py := r.X+x*r.W, mid-ys[i]*h/2
			if i == 0 {
				g.curve.MoveTo(px, py)
				g.area.MoveTo(px, py)
			} else {
				g.curve.LineTo(px, py)
				g.area.LineTo(px, py)
			}
		}
		g.area.LineTo(r.X+r.W, mid).LineTo(r.X, mid).Close()
		p.FillPath(&g.area, ink.Alpha(fillAlpha))
		p.StrokePath(&g.curve, 2, faint)
		p.StrokePath(&g.curve, 2, ink.Alpha(1-off))
	})
}

// skeleton is the page while it loads, at its real sizes: the chips as bars
// of an average chip's width, and the panel with its rules.
func (e *equalizer) skeleton(c *ui.Context) {
	tk := theme.Current()
	ui.Column(c).FillWidth().Role(ui.RoleStatus).Label(i18n.T("Loading")).Children(func() {
		ui.Row(c).FillWidth().Wrap().Gap(chipGap).Margin(0, 0, 20, 0).Children(func() {
			for i := range 16 {
				ui.Box(c.Key(string(rune('a' + i)))).Width(88).Height(chipHeight).Shrink(0).Radius(tk.Radius["md"]).Background(theme.C("surface"))
			}
		})
		panel := ui.Box(c).FillWidth().Height(panelH).Radius(tk.Radius["lg"]).Background(theme.C("surface"))
		panel.Draw(func(p *ui.Painter, r ui.Rect) {
			y0 := r.Y + panelPadTop + faderTop + thumbInset
			h := float32(faderLength - 2*thumbInset)
			left, right := r.X+panelPadX+preampW+2*ruleGap+1, r.X+r.W-panelPadX-scaleW
			for i := range 3 {
				y := y0 + h*float32(i)/2
				p.Line(left, y, right, y, 1, theme.C("line"))
			}
			p.Fill(ui.Rect{X: left - ruleGap - 1, Y: r.Y + panelPadTop, W: 1, H: stageHeight}, theme.C("line"), 0)
		})
	})
}

// unavailable is the panel where the faders would be on a platform whose
// playback has no equaliser: it takes the panel's place and size, and says
// so, with the way to the sound settings that do exist.
func (e *equalizer) unavailable(c *ui.Context) {
	tk := theme.Current()
	ui.Column(c).FillWidth().Height(panelH).Justify(ui.Center).AlignItems(ui.Center).Gap(8).Padding(0, 24).
		Radius(tk.Radius["lg"]).Background(theme.C("surface")).Children(func() {
		ui.Icon(c, theme.Icon("sliders-vertical")).FontSize(24).TextColor(theme.C("ink-faint")).Margin(0, 0, 4, 0)
		text(c, i18n.T("Playback here has no equaliser, so there is nothing to adjust."), "body").
			TextColor(theme.C("ink")).TextAlign(ui.Center).MaxWidth(480)
		text(c, i18n.T("The other sound settings are in Settings, under Playback."), "secondary").
			TextColor(theme.C("ink-muted")).TextAlign(ui.Center).MaxWidth(480)
	})
}
