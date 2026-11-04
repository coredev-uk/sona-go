package app

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// layerWithDialog is a page with a button and a layer, open while *open,
// that shows a dialog-like button of its own.
func layerWithDialog(open *bool) module.Factory {
	return func(*module.Env) module.Module {
		return module.Module{
			ID: "layer",
			Routes: []module.Route{{Pattern: "/", Build: func(c *ui.Context, _ *ui.Route) {
				kit.Button(c.Key("go"), "Page button", kit.ButtonOpts{})
			}}},
			Nav: []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "Home" }, Icon: "music", To: "/", Order: 1}},
			Slots: module.Slots{
				Layer: func(c *ui.Context) {
					if *open {
						kit.Button(c.Key("close"), "Layer button", kit.ButtonOpts{})
					}
				},
				LayerOpen: func() bool { return *open },
			},
		}
	}
}

// While a layer is open assistive technology sees it alone: the page behind
// is inert, and reads again once the layer closes.
func TestAnOpenLayerLeavesThePageOffTheAccessibilityTree(t *testing.T) {
	open := false
	_, tt := newShell(t, Options{}, layerWithDialog(&open))
	tt.Frame()
	if _, ok := access.Read(t, tt.Tester).Find(ui.RoleButton, "Page button"); !ok {
		t.Fatal("the page is not on the tree with the layer closed")
	}
	open = true
	tt.Frame()
	tree := access.Read(t, tt.Tester)
	if n, ok := tree.Find(ui.RoleButton, "Page button"); ok {
		t.Errorf("the page is on the tree behind an open layer: %v", n)
	}
	if _, ok := tree.Find(ui.RoleButton, "Layer button"); !ok {
		t.Errorf("the layer is not on the tree:\n%s", tree.Dump())
	}
	open = false
	tt.Frame()
	if _, ok := access.Read(t, tt.Tester).Find(ui.RoleButton, "Page button"); !ok {
		t.Error("the page did not come back once the layer closed")
	}
}

// A first run opens the setup layer at once: the first paint, which starts the
// network, is still reported, once.
func TestFirstPaintRunsOnceWithALayerOpenFromTheStart(t *testing.T) {
	open, n := true, 0
	_, tt := newShellPaints(t, Options{}, &n, layerWithDialog(&open))
	if n != 1 {
		t.Fatalf("first paint reported %d times with a layer open from the start, want once", n)
	}
	tt.Frame()
	tt.Frame()
	if n != 1 {
		t.Errorf("first paint reported %d times, want once", n)
	}
}
