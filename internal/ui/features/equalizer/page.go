package equalizer

import (
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

// The page's geometry, from EqualizerPage, in DIPs.
const (
	padTop       = 28   // pt-7
	padBottom    = 48   // pb-12
	narrow       = 960  // the gutters shrink below this width (as the library's)
	stackBelow   = 727  // the presets stack over the bands below 960 DIPs of window (the sidebar takes 233)
	maxWidth     = 1040 // max-w-[1040px]
	presetsWidth = 176
	panelGap     = 32
	scaleWidth   = 56 // the dB scale and the preamp column
	graphHeight  = 112
	faderLength  = 160 // h-40
	loadingH     = 320 // h-80
)

// pageState is the view state: the scroll place and the width of the last
// frame, which the layout follows.
type pageState struct {
	pos   ui.ScrollState
	width float32
}

// page is EqualizerPage: the header, then the presets beside the bands.
// Where the platform has no equaliser it says so instead.
func (e *equalizer) page(c *ui.Context, _ *ui.Route) {
	tk := theme.Current()
	pg := &e.body
	gutter := tk.Metric["gutter"]
	if pg.width > 0 && pg.width < narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	devstate.Scroll(c, &pg.pos) // dev builds: the offset survives a restart
	sc := kit.GlassScroll(ui.Scroll(c).TrackScroll(&pg.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0))
	sc.Children(func() {
		ui.Column(c).Padding(padTop, gutter+tk.Metric["scrollbar"], padBottom, gutter).Children(func() { e.content(c) })
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
		return
	}
	settings, info := e.settings(), e.info()
	if err := firstErr(settings.Err, info.Err); err != nil {
		kit.PageHeader(c, i18n.T("Equaliser"), "", nil)
		kit.ErrorBlock(c, i18n.T("Couldn't load the equaliser"), err, kit.ErrorBlockOpts{
			OnRetry:  e.refetch,
			Retrying: settings.Fetching || info.Fetching,
		})
		return
	}
	if !settings.HasData || !info.HasData {
		kit.PageHeader(c, i18n.T("Equaliser"), "", nil)
		ui.Box(c).FillWidth().Height(loadingH).Radius(theme.Current().Radius["lg"]).Background(theme.C("surface"))
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

	stacked := e.body.width > 0 && e.body.width < stackBelow
	layout := ui.Row(c).FillWidth().MaxWidth(maxWidth).Gap(panelGap).AlignItems(ui.Start)
	if stacked {
		layout = ui.Column(c).FillWidth().MaxWidth(maxWidth).Gap(panelGap).AlignItems(ui.Stretch)
	}
	layout.Children(func() {
		e.presets(c, s, eq, in, stacked)
		e.bands(c, s, eq, in, gains, room)
	})
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// text is a single line of text in one of the theme's type styles.
func text(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).
		LetterSpacing(t.Tracking).MinWidth(0)
}

// presets is the preset list: the user's own curve first, then the
// built-in curves, one radio group and one stop of Tab; the arrows choose,
// wrapping, and the focus goes with the choice. Choosing one saves it and
// turns the equaliser on.
func (e *equalizer) presets(c *ui.Context, s config.Settings, eq config.Equalizer, in services.EqualizerInfo, stacked bool) {
	type item struct{ id, name string }
	items := []item{{custom, presetName(custom, "")}}
	for _, p := range in.Presets {
		items = append(items, item{p.ID, presetName(p.ID, p.Name)})
	}
	sel, picked := eq.Preset, ""
	radius := theme.Current().Radius["md"]
	group := ui.RadioGroup(c, func() {
		for _, it := range items {
			on := it.id == eq.Preset
			r := ui.RadioBase(c.Key(it.id), &sel, it.id).Height(32).PaddingX(10).AlignItems(ui.Center).Radius(radius).
				Cursor(ui.CursorPointer).Label(it.name)
			fg := theme.C("ink-muted")
			switch {
			case on:
				r.Background(theme.C("active"))
				fg = theme.C("ink")
			case r.Hovered():
				r.Background(theme.C("hover"))
				fg = theme.C("ink")
			}
			weight := 400
			if on {
				weight = 500
			}
			r.Children(func() { text(c, it.name, "body").FontWeight(weight).TextColor(fg).SingleLine() })
			if r.Changed() {
				picked = sel
			} else if r.Clicked() {
				picked = it.id
			}
		}
	}).Gap(2).Label(i18n.T("Preset"))
	if !stacked {
		group.Width(presetsWidth).Shrink(0)
	}
	if picked != "" {
		next := eq
		next.Preset, next.Enabled = picked, true
		e.save(s, next)
	}
}

// bands is the Bands panel: the dB scale beside the response curve, then
// the preamp and the ten band faders, each band's centre over its fader,
// and a note on the level change.
func (e *equalizer) bands(c *ui.Context, s config.Settings, eq config.Equalizer, in services.EqualizerInfo, gains []float64, room float64) {
	tk := theme.Current()
	dim := !eq.Enabled
	panel := ui.Column(c).Grow(1).Basis(0).MinWidth(0).Padding(20, 20, 16, 20).Radius(tk.Radius["lg"]).
		Background(theme.C("surface")).Role(ui.RoleGroup).Label(i18n.T("Bands"))
	panel.Children(func() {
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Width(scaleWidth).Shrink(0).Height(graphHeight).Justify(ui.SpaceBetween).PaddingY(4).Children(func() {
				for _, label := range []string{formatDb(in.MaxGain), i18n.T("0 dB"), formatDb(in.MinGain)} {
					text(c, label, "meta").TextColor(theme.C("ink-faint")).FontFeatures("tnum").TextAlign(ui.End).FillWidth().
						SingleLine().Role(ui.RoleNone)
				}
			})
			e.graphView(c, in.Frequencies, gains, in.MaxGain, dim)
		})
		ui.Row(c).FillWidth().Gap(16).AlignItems(ui.Start).Margin(20, 0, 0, 0).Children(func() {
			e.fader(c, fader{
				id: "preamp", label: i18n.T("Preamp"), name: i18n.T("Preamp"), value: eq.Preamp,
				lo: in.MinPreamp, hi: in.MaxPreamp, dim: dim, width: scaleWidth,
			}, func(db float64) {
				next := eq
				next.Preamp = db
				e.live(next)
			}, func(db float64) {
				next := eq
				next.Preamp = db
				e.save(s, next)
			})
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
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
		note := i18n.T("Cuts only: the level stays as it is. Preamp lowers it further.")
		if room > 0 {
			note = i18n.T("The level drops {drop} dB ahead of the bands, so the largest boost cannot clip. Preamp lowers it further.",
				"drop", i18n.FormatNumber(math.Round(room*10)/10))
		}
		text(c, note, "meta").TextColor(theme.C("ink-muted")).Margin(16, 0, 0, 0).FillWidth()
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
	col := ui.Column(c.Key(f.id)).MinWidth(0).AlignItems(ui.Center).Gap(8)
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
			SliderOpts: kit.SliderOpts{A11yLabel: f.label, ValueText: i18n.T("{db} dB", "db", db), Step: 0.5, Vertical: true},
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
// when they change.
type graphCache struct {
	freqs, gains []float64
	rng          float64
	// y is the curve's height at each point, -1 (bottom) to 1 (top).
	y []float32
}

func (g *graphCache) of(freqs, gains []float64, rng float64) []float32 {
	if g.y != nil && g.rng == rng && slices.Equal(g.freqs, freqs) && slices.Equal(g.gains, gains) {
		return g.y
	}
	db := response(freqs, gains, 0, graphFreqs)
	y := make([]float32, len(db))
	for i, d := range db {
		y[i] = float32(max(-rng, min(rng, d)) / rng)
	}
	g.freqs, g.gains, g.rng, g.y = slices.Clone(freqs), slices.Clone(gains), rng, y
	return y
}

// graphView is ResponseGraph: the curve the filters draw (without the level
// change ahead of them) on a log frequency axis that lines up with the
// faders below, rng dB at the top edge and its negative at the bottom. It is
// decoration: assistive technology has the faders.
func (e *equalizer) graphView(c *ui.Context, freqs, gains []float64, rng float64, dim bool) {
	ys := e.graph.of(freqs, gains, rng)
	line, ink, faint := theme.C("line"), theme.C("ink"), theme.C("ink-faint")
	box := ui.Box(c).Grow(1).Basis(0).MinWidth(0).Height(graphHeight).Role(ui.RoleNone)
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
		mid := r.Y + r.H/2
		p.Line(r.X, mid, r.X+r.W, mid, 1, line)
		var curve, area ui.Path
		for i, x := range graphX {
			px, py := r.X+x*r.W, mid-ys[i]*r.H/2
			if i == 0 {
				curve.MoveTo(px, py)
				area.MoveTo(px, py)
			} else {
				curve.LineTo(px, py)
				area.LineTo(px, py)
			}
		}
		area.LineTo(r.X+r.W, mid).LineTo(r.X, mid).Close()
		p.FillPath(&area, ink.Alpha(fillAlpha))
		p.StrokePath(&curve, 2, faint)
		p.StrokePath(&curve, 2, ink.Alpha(1-off))
	})
}
