//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/e2e/atspi"
	"github.com/coredev-uk/sona-go/e2e/automation"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// appName is the application name on the accessibility bus: the app ID
// (GLib's program name, cmd/sona/appid_linux.go).
const appName = "dev.coredev.Sona"

// TestAccessibility reads the shell's landmarks the way a screen reader
// does, from the AT-SPI bus: the application and its window, the sidebar's
// link, the history buttons, the page's region and controls, an album's play
// button, and the player bar's transport controls and sliders, each by AT-SPI
// role and name.
func TestAccessibility(t *testing.T) {
	a := startApp(t)
	first := fixture.Albums()[len(fixture.Albums())-1]
	if _, err := a.client.Wait(startTimeout, "the Albums page", func(tr *automation.Tree) bool { return tr.HasText("24 albums") }); err != nil {
		t.Fatal(err)
	}
	bus, err := atspi.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bus.Close() }()
	app, err := bus.App(appName, 14, 15*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ role, name string }{
		{"frame", "Sona"},
		{"panel", "Main"},                              // the sidebar
		{"link", "Albums"},                             // its page link
		{"button", "Back"},                             // history
		{"button", "Forward"},                          //
		{"panel", "Albums"},                            // the page
		{"label", "24 albums"},                         //
		{"panel", "Album type"},                        // the filter
		{"toggle button", "All"},                       //
		{"combo box", "Sort"},                          //
		{"table", "Albums in your library"},            // the grid
		{"button", "Play " + first.Name},               // its play button, beside the opener
		{"push button menu", "More for " + first.Name}, // and its more button
		{"panel", "Player"},                            // the player bar
		{"button", "Previous"},                         //
		{"button", "Play"},                             //
		{"button", "Next"},                             //
		{"toggle button", "Shuffle off"},
		{"slider", "Seek"},
		{"slider", "Volume"},
	} {
		if app.Find(want.role, want.name) == nil {
			t.Errorf("no %s named %q on the accessibility bus", want.role, want.name)
		}
	}
	if t.Failed() {
		t.Logf("accessibility tree:\n%s", app.Dump())
	}
}
