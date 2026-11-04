package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// artKnobs is the Artwork playground's state.
var artKnobs = struct {
	seed, size, delay, radius, aspect float64
	round, edge, fail                 bool
	placeholder                       string
	nonce                             int
}{seed: 3, size: 160, delay: 600, edge: true, placeholder: "music"}

func init() {
	register(section{name: "Artwork", build: artwork})
}

func artwork(c *ui.Context, g *gallery) {
	k := &artKnobs
	note(c, "kit.Artwork shows a cover at an exact size: a placeholder while it loads (and if it fails), the decoded picture once it is ready. Covers here are generated.")

	heading(c, "Playground")
	panel(c, func() {
		knob(c, "Cover", func() { ui.StepSlider(c, &k.seed, 0, 40, 1).Label("Cover").Width(240) })
		knob(c, "Size", func() { ui.StepSlider(c, &k.size, 24, 320, 8).Label("Size").Width(240) })
		knob(c, "Radius", func() { ui.StepSlider(c, &k.radius, 0, 40, 2).Label("Radius").Width(240) })
		knob(c, "Delay (ms)", func() { ui.StepSlider(c, &k.delay, 0, 3000, 100).Label("Delay").Width(240) })
		knob(c, "Aspect", func() {
			i := 0
			if k.aspect > 0 {
				i = 1
			}
			if ui.Segmented(c, &i, "1:1", "16:9").Label("Aspect").Changed() {
				k.aspect = 0
				if i == 1 {
					k.aspect = 16.0 / 9
				}
			}
		})
		knob(c, "Placeholder", func() {
			ui.Select(c, &k.placeholder, []string{"music", "user", "disc-3", "play"}).Label("Placeholder glyph").Width(160)
		})
		knob(c, "Options", func() {
			ui.Checkbox(c, &k.round, "Round")
			ui.Checkbox(c, &k.edge, "Edge")
			ui.Checkbox(c, &k.fail, "Fails")
		})
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			if kit.Button(c.Key("reload"), "Load again", kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "shuffle"}).Clicked() {
				k.nonce++ // a src of its own: the loader asks again, the placeholder shows for the delay
			}
			text(c, fmt.Sprintf("%.0f×%.0f DIPs", k.size, k.size/maxf(k.aspect, 1)), "meta").TextColor(theme.C("ink-faint"))
		})
		src := coverURL(int(k.seed), time.Duration(k.delay)*time.Millisecond, k.fail, k.nonce)
		kit.Artwork(c, g.art, src, float32(k.size), kit.ArtOpts{
			Radius: float32(k.radius), Round: k.round, Edge: k.edge, Placeholder: k.placeholder, Aspect: float32(k.aspect), A11yLabel: "Playground cover",
		})
	})

	heading(c, "States")
	note(c, "Failed covers are asked for again after 5 s and 30 s, then the placeholder stays.")
	ui.Row(c).Gap(24).Wrap().Children(func() {
		labelled(c, "Placeholder (no src)", func() { kit.Artwork(c, g.art, "", 120, kit.ArtOpts{}) })
		labelled(c, "Loaded", func() { kit.Artwork(c, g.art, coverURL(1, 0, false, 0), 120, kit.ArtOpts{}) })
		labelled(c, "Loaded, edge", func() { kit.Artwork(c, g.art, coverURL(2, 0, false, 0), 120, kit.ArtOpts{Edge: true}) })
		labelled(c, "Failed", func() { kit.Artwork(c, g.art, coverURL(3, 0, true, 0), 120, kit.ArtOpts{}) })
		labelled(c, "Round", func() { kit.Artwork(c, g.art, coverURL(4, 0, false, 0), 120, kit.ArtOpts{Round: true, Edge: true}) })
		labelled(c, "Round placeholder", func() { kit.Artwork(c, g.art, "", 120, kit.ArtOpts{Round: true}) })
		labelled(c, "16:9", func() { kit.Artwork(c, g.art, coverURL(5, 0, false, 0), 213, kit.ArtOpts{Aspect: 16.0 / 9}) })
	})

	heading(c, "Sizes")
	ui.Row(c).Gap(16).AlignItems(ui.End).Wrap().Children(func() {
		for i, size := range []float32{24, 32, 40, 56, 80, 120, 176, 240} {
			labelled(c, fmt.Sprintf("%.0f", size), func() { kit.Artwork(c, g.art, coverURL(10+i, 0, false, 0), size, kit.ArtOpts{Edge: true}) })
		}
	})
}

func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
