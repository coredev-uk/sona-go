package settings

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// sectionRig is the page over the full Env opened on one section, with the
// toaster of the shell so a toast shows.
func sectionRig(t *testing.T, section string, opts ...uitest.Option) *rig {
	t.Helper()
	return sectionRigWith(t, section, nil, opts...)
}

// sectionRigWith is sectionRig with setup run before the first frame (a
// seam installed after it would be asked too late: a fetch the real service
// failed is not asked again for 5 s).
func sectionRigWith(t *testing.T, section string, setup func(*settings), opts ...uitest.Option) *rig {
	t.Helper()
	hn := uitest.New(t, opts...)
	r := &rig{t: t, h: hn, router: ui.NewRouter(settingsPath)}
	hn.Env.Nav = r.router
	hn.Env.Shortcuts = func() []module.BoundShortcut { return nil }
	r.s = newSettings(hn.Env,
		services.Service[services.SystemService](hn.Core), services.Service[services.PlatformService](hn.Core),
		fakeScrobbler{}, services.Service[services.IntegrationsService](hn.Core),
		services.Service[services.CDMService](hn.Core), services.Service[services.UpdatesService](hn.Core))
	r.s.init()
	followShell(hn)
	if setup != nil {
		setup(r.s)
	}
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(settingsPath) {
					r.s.page(c, rt)
				}
			})
		})
		kit.Toaster(c)
	}, 1132, 900)
	r.tt.Drain()
	r.router.Replace(settingsPath + "?section=" + section)
	r.tt.Drain()
	t.Cleanup(func() { theme.SetReduceEffects(false) })
	return r
}

// toggle moves the focus to a setting's switch (Find's landing) and presses Space.
func (r *rig) toggle(id string) {
	r.t.Helper()
	r.s.focusSetting(id)
	r.tt.Drain()
	r.tt.Key(0, ui.KeySpace)
}

// saved waits for the saved settings to satisfy ok.
func (r *rig) saved(what string, ok func(config.Settings) bool) {
	r.t.Helper()
	r.tt.Until(what, func() bool { return ok(r.h.Settings.Get()) })
}

func TestThemeChoiceTakesEffectAndSaves(t *testing.T) {
	r := sectionRig(t, "appearance")
	r.tt.Until("appearance", r.tt.Shows("Theme", "System", "Light", "Dark", "Reduce effects", "Motion artwork"))
	if theme.Dark() {
		t.Fatal("starts light")
	}
	if err := r.tt.Click("Dark"); err != nil {
		t.Fatal(err)
	}
	if !theme.Dark() {
		t.Error("Dark did not darken the theme")
	}
	r.saved("dark saved", func(s config.Settings) bool { return s.Theme == config.ThemeDark })

	if err := r.tt.Click("Light"); err != nil {
		t.Fatal(err)
	}
	if theme.Dark() {
		t.Error("Light did not lighten the theme")
	}
	r.saved("light saved", func(s config.Settings) bool { return s.Theme == config.ThemeLight })

	// System follows the desktop.
	r.tt.SetDark(true)
	if err := r.tt.Click("System"); err != nil {
		t.Fatal(err)
	}
	r.saved("system saved", func(s config.Settings) bool { return s.Theme == config.ThemeSystem })
	r.tt.Drain()
	if !theme.Dark() {
		t.Error("System did not follow a dark desktop")
	}
	r.tt.SetDark(false)
	r.tt.Drain()
	if theme.Dark() {
		t.Error("System did not follow a light desktop")
	}
}

func TestReduceEffectsTakesEffectAndTurnsMotionArtworkOff(t *testing.T) {
	r := sectionRig(t, "appearance")
	r.tt.Until("appearance", r.tt.Shows("Motion artwork", "Plays the looping covers"))
	r.toggle("reduce-effects")
	if !theme.ReduceEffects() {
		t.Error("Reduce effects did not reach the theme")
	}
	r.saved("reduce effects saved", func(s config.Settings) bool { return s.ReduceEffects })
	r.tt.Until("the motion row's note", r.tt.Shows("Off while Reduce effects is on."))
	before := r.h.Settings.Get().MotionArtwork.Enabled
	r.toggle("motion-artwork") // disabled: nothing happens
	r.tt.Drain()
	if got := r.h.Settings.Get().MotionArtwork.Enabled; got != before {
		t.Error("the disabled Motion artwork switch changed")
	}
	// Turned off again, the note is back.
	r.toggle("reduce-effects")
	r.saved("reduce effects off", func(s config.Settings) bool { return !s.ReduceEffects })
	if theme.ReduceEffects() {
		t.Error("Reduce effects stayed on")
	}
}

func TestMotionArtworkSaves(t *testing.T) {
	r := sectionRig(t, "appearance")
	r.tt.Until("appearance", r.tt.Shows("Motion artwork"))
	want := !r.h.Settings.Get().MotionArtwork.Enabled
	r.toggle("motion-artwork")
	r.saved("motion artwork saved", func(s config.Settings) bool { return s.MotionArtwork.Enabled == want })
}

func TestAppearanceRowsAreInTheFindIndex(t *testing.T) {
	r := sectionRig(t, "appearance")
	r.tt.Until("appearance", r.tt.Shows("Theme"))
	for _, id := range []string{"theme", "reduce-effects", "motion-artwork"} {
		if !r.s.seen[id] {
			t.Errorf("no row %q", id)
		}
	}
	if !slices.Contains(sections, "appearance") {
		t.Fatal("section")
	}
}
