package equalizer

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// barRig is the player bar's controls with the equaliser's button in them.
func barRig(t *testing.T, supported bool, setup func(*fake)) (*uitest.Session, *ui.Router, *uitest.Harness) {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	f := newFake()
	if setup != nil {
		setup(f)
	}
	router := ui.NewRouter("/")
	h.Env.Nav = router
	e := newEqualizer(h.Env, f, func() bool { return supported })
	h.Env.PlayerControls = module.Point{}
	h.Env.PlayerControls.Add(controlOrder, e.button)
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Row(c).Fill().Children(func() { h.Env.PlayerControls.Build(c) })
	}, 300, 80)
	return tt, router, h
}

func TestTheBarsEqualiserButtonOpensThePage(t *testing.T) {
	tt, router, _ := barRig(t, true, nil)
	tt.Until("the button", tt.Shows("Equaliser"))
	if err := tt.Click("Equaliser"); err != nil {
		t.Fatal(err)
	}
	if got := router.Location(); got != "/equalizer" {
		t.Errorf("went to %q", got)
	}
}

func TestTheBarsEqualiserButtonSaysWhenItIsOn(t *testing.T) {
	tt, _, _ := barRig(t, true, func(f *fake) {
		f.settings.Audio.Equalizer = config.Equalizer{Enabled: true, Preset: "flat", Custom: f.settings.Audio.Equalizer.Custom}
	})
	tt.Until("the button, on", tt.Shows("Equaliser (on)"))
}

func TestNoEqualiserButtonWhereThePlatformHasNone(t *testing.T) {
	tt, _, _ := barRig(t, false, nil)
	tt.Advance(time.Second)
	if tt.HasText("Equaliser") {
		t.Errorf("a platform with no equaliser shows the button: %q", tt.Texts())
	}
	if _, ok := tt.Find("Equaliser"); ok {
		t.Error("the button is on the tree")
	}
}

func TestTheEqualiserButtonIsAfterTheMiniPlayerOrder(t *testing.T) {
	if controlOrder <= module.ControlsAfterQueue+10 {
		t.Errorf("order %d is not after the Mini player's", controlOrder)
	}
}
