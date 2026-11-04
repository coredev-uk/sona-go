package app

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
)

// actionModule registers one song action under id.
func actionModule(id, action string) module.Factory {
	return func(*module.Env) module.Module {
		return module.Module{ID: id, Actions: []kit.ActionDef{{
			ID: action, Target: kit.KindSong, Group: kit.GroupQueue, Order: 1, Icon: "plus",
			Label: func(*kit.ActionContext) string { return action }, Run: func(*kit.ActionContext) {},
		}}}
	}
}

func TestEnvActionsCollectsEveryModulesActions(t *testing.T) {
	a, _ := newShell(t, Options{}, actionModule("a", "a.one"), actionModule("b", "b.two"))
	got := a.env.Actions.For(kit.KindSong)
	if len(got) != 2 || got[0].ID != "a.one" || got[1].ID != "b.two" {
		t.Errorf("registered %v", got)
	}
	var nav string
	a.env.Actions.SetNavigator(func(p string) { nav = p })
	a.env.Actions.Navigate("/album/x")
	if nav != "/album/x" {
		t.Errorf("navigate %q", nav)
	}
}

func TestDuplicateActionIDAcrossModulesPanicsAtStartup(t *testing.T) {
	defer func() {
		if v := recover(); v != "action x registered twice" {
			t.Errorf("recovered %v", v)
		}
	}()
	newApp(Options{}, []module.Factory{actionModule("a", "x"), actionModule("b", "x")})
}

func TestShellMountsTheToasterAndConfirmHost(t *testing.T) {
	_, tt := newShell(t, Options{}, pageModule("a", pageDef{"/albums", "Albums", module.NavLibrary, 1}))
	t.Cleanup(kit.CancelConfirm)
	kit.Confirm(kit.ConfirmOptions{Title: "Delete it?", Body: "Gone.", ConfirmLabel: "Delete"}, func(bool) {})
	kit.ShowToast(kit.ToastSpec{ID: "t", Title: "Saved", Kind: kit.ToastSuccess})
	tt.Frame()
	for _, s := range []string{"Delete it?", "Saved", "Notifications"} {
		if !tt.HasText(s) {
			t.Errorf("shell does not show %q", s)
		}
	}
	tt.Advance(10 * time.Second)
}
