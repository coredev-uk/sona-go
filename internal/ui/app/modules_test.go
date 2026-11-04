package app

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The real factories, in v0.2.0's modules.ts order: routes, event handlers,
// shortcuts and shell slots follow it, so a change of order is a decision.
func TestRegisteredModulesAreEachOnceInV020sOrder(t *testing.T) {
	h := uitest.New(t)
	a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now}, factories)
	t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
	var got []string
	for _, m := range a.mods {
		got = append(got, m.ID)
	}
	want := []string{
		"home", "discover", "search", "concerts", "library", "album", "artist", "song", "video", "playlist",
		"radio", "player", "equalizer", "settings", "auth", "commands", "updates", "private", "cdm", "reminders", "onboarding",
	}
	if !slices.Equal(got, want) {
		t.Errorf("modules %v, want %v", got, want)
	}
}

func TestDuplicateModuleIDPanicsAtStartup(t *testing.T) {
	defer func() {
		if v := recover(); v != "module a registered twice" {
			t.Errorf("recovered %v", v)
		}
	}()
	plain := func(*module.Env) module.Module { return module.Module{ID: "a"} }
	newApp(Options{}, []module.Factory{plain, plain})
}
