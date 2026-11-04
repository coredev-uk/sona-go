package onboarding

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// TestGoldenWelcome compares the welcome step, light and dark, with its images
// in testdata/golden (written by running the test with -update): the sheet
// over the app behind a scrim, the strip at its first step, the two things
// whole songs need (the second ready), "Get started" on the foot.
func TestGoldenWelcome(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"welcome-light", nil},
		{"welcome-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, 940, 700, c.opts...)
			store.Set(r.h.Env.Queries, store.KeyCDM, services.CDMStatus{State: "ready", Version: "4.10.3112.0"})
			// Automatic, as on a first start: with English alone there is no Language row.
			update(r, func(s *config.Settings) { s.Language = "" })
			r.open("welcome")
			r.tt.Until("the step", r.tt.Shows("Welcome to Sona", "What whole songs need", "Ready, version 4.10.3112.0", "Get started"))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), c.name)
		})
	}
}

// TestGoldenAppearance compares the theme picker's miniature windows, which
// are drawn from the tokens of both palettes whatever the current one is.
func TestGoldenAppearance(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"appearance-light", nil},
		{"appearance-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, 940, 700, c.opts...)
			r.open("appearance")
			r.tt.Until("the step", r.tt.Shows("Theme", "Always dark", "Reduce effects"))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), c.name)
		})
	}
}
