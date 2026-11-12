package app

import (
	"strconv"
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// equalizerShell is the real shell on /equalizer over an engine that has an
// equaliser.
func equalizerShell(t *testing.T, w, h int, opts ...uitest.Option) (*uitest.Harness, *uitest.Session) {
	t.Helper()
	hh := uitest.New(t, append([]uitest.Option{uitest.Equalizer()}, opts...)...)
	a := newApp(Options{Core: hh.Core, Settings: hh.Settings, Bus: hh.Bus, Poster: hh.Main.Post, Art: hh.Art, Now: hh.Clock.Now, StartPath: "/equalizer"}, factories)
	t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
	a.firstPaint = nil
	a.connect()
	tt := hh.Tester(t, a.view, w, h)
	tt.Art = append(tt.Art, a.env.Art)
	tt.Until("the equaliser", tt.Shows("Equaliser", "Preset", "Bands"))
	return hh, tt
}

// Choosing a preset in the shell saves it, turns the equaliser on, and the
// audio service hands the engine the curve.
func TestEqualizerPresetReachesTheEngineThroughTheShell(t *testing.T) {
	h, tt := equalizerShell(t, 1132, 1253)
	if err := tt.Click("Rock"); err != nil {
		t.Fatal(err)
	}
	tt.Until("the engine's curve", func() bool { return len(h.Engine.Curves()) > 0 })
	eq := h.Settings.Get().Audio.Equalizer
	if !eq.Enabled || eq.Preset != "rock" {
		t.Errorf("saved %+v", eq)
	}
	tt.Until("the subline", tt.Shows("Rock · shapes everything Sona plays"))
}

// The page in the shell at both window sizes: choosing a preset (narrow, the
// presets are below the fold, so it switches the equaliser on instead) saves
// it and the page's subline says so. The page's look is the equalizer
// package's goldens; the shell's, shell-albums'.
func TestEqualizerShellSublineAtBothSizes(t *testing.T) {
	for _, sz := range [][2]int{{1132, 1253}, {940, 600}} {
		t.Run(strconv.Itoa(sz[0])+"x"+strconv.Itoa(sz[1]), func(t *testing.T) {
			_, tt := equalizerShell(t, sz[0], sz[1])
			pick, subline := "Rock", "Rock · shapes everything Sona plays"
			if sz[1] < 800 {
				pick, subline = "Off", "Flat · shapes everything Sona plays"
			}
			if err := tt.Click(pick); err != nil {
				t.Fatal(err)
			}
			tt.Until("the page", tt.Shows(subline))
		})
	}
}
