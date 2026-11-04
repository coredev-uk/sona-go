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

// TestAccessibilityPages walks the sidebar and reads each page the way a
// screen reader does: the sidebar link that opens it, the level-1 heading
// that names it, and what the page offers beside it (the library pages' lists
// and their rows' controls, Settings' section list). The pages that need an
// account (For you, Discover, Radio, Concerts, Search) have their trees
// checked by the UI tests (internal/ui/features/*/a11y_test.go): their
// network answers are not in the fixture profile.
func TestAccessibilityPages(t *testing.T) {
	a := startApp(t)
	c := a.client
	if _, err := c.Wait(startTimeout, "the Albums page", func(tr *automation.Tree) bool { return tr.HasText("24 albums") }); err != nil {
		t.Fatal(err)
	}
	bus, err := atspi.Connect()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = bus.Close() }()

	read := func(t *testing.T) *atspi.Node {
		t.Helper()
		app, err := bus.App(appName, 14, 15*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		return app
	}
	for _, page := range []struct {
		link, heading string
		also          []struct{ role, name string }
	}{
		{link: "Recently added", heading: "Recently added"},
		{link: "Artists", heading: "Artists"},
		{link: "Albums", heading: "Albums", also: []struct{ role, name string }{{"table", "Albums in your library"}}},
		{link: "Songs", heading: "Songs"},
		{link: "Music videos", heading: "Music videos"},
		{link: "Favourite songs", heading: "Favourite songs"},
		{link: "History", heading: "History"},
		{link: "Playlists", heading: "Playlists"},
		{link: "Settings", heading: "Settings"},
	} {
		t.Run(page.link, func(t *testing.T) {
			if err := c.PressRole(startTimeout, "link", page.link); err != nil {
				t.Fatal(err)
			}
			if _, err := c.Wait(startTimeout, "the "+page.link+" page", func(tr *automation.Tree) bool {
				_, ok := tr.Find("heading", page.heading)
				return ok
			}); err != nil {
				t.Fatal(err)
			}
			app := read(t)
			want := append([]struct{ role, name string }{
				{"link", page.link},       // the sidebar
				{"heading", page.heading}, // the page's title
			}, page.also...)
			for _, w := range want {
				if app.Find(w.role, w.name) == nil {
					t.Errorf("no %s named %q on the accessibility bus", w.role, w.name)
				}
			}
			// Every page leaves the shell's landmarks where they were.
			for _, w := range []struct{ role, name string }{{"panel", "Main"}, {"panel", "Player"}, {"button", "Back"}} {
				if app.Find(w.role, w.name) == nil {
					t.Errorf("no %s named %q once on %s", w.role, w.name, page.link)
				}
			}
			if t.Failed() {
				t.Logf("accessibility tree:\n%s", app.Dump())
			}
		})
	}
}
