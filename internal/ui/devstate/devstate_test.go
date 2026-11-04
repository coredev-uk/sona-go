//go:build dev

package devstate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func isolate(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SONA_DEV_INSTANCE", "")
	t.Setenv("SONA_DEV_FRESH", "")
	reg.mu.Lock()
	reg.loc, reg.offsets, reg.pending, reg.flags, reg.waiting = nil, nil, nil, nil, nil
	reg.mu.Unlock()
}

func TestSaveLoad(t *testing.T) {
	isolate(t)
	if _, ok := Load(); ok {
		t.Fatal("no state yet")
	}
	loc := "/library/albums?sort=title"
	SetLocation(func() string { return loc })
	open := true
	Flag("queue.open", &open)
	tt := ui.NewTester(func(c *ui.Context) {
		pos := ui.ScrollState{Y: 640}
		Scroll(c, &pos)
	}, 100, 100)
	tt.Frame()

	Save(loc, &Window{X: 10, Y: 20, Width: 1000, Height: 700, Maximised: true})
	st, ok := Load()
	if !ok {
		t.Fatal("state not saved")
	}
	if st.Route != loc || st.Scroll[loc] != 640 || !st.Flags["queue.open"] || st.Window == nil || st.Window.Width != 1000 || !st.Window.Maximised {
		t.Fatalf("got %+v", st)
	}

	t.Setenv("SONA_DEV_FRESH", "1")
	if _, ok := Load(); ok {
		t.Fatal("SONA_DEV_FRESH=1 ignores the file")
	}
}

func TestInstance(t *testing.T) {
	isolate(t)
	t.Setenv("SONA_DEV_INSTANCE", "feat/x y")
	if got := Instance(); got != "feat_x_y" {
		t.Fatalf("instance %q", got)
	}
	SetLocation(func() string { return "/a" })
	Save("/a", nil)
	path, _ := file()
	if filepath.Base(filepath.Dir(path)) != "sona-feat_x_y" {
		t.Fatalf("state dir %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

// A restored offset is asked for until the page is tall enough: its data
// arrives after the first frame.
func TestScrollWaitsForContent(t *testing.T) {
	isolate(t)
	loc := "/library/albums"
	SetLocation(func() string { return loc })
	var pos ui.ScrollState
	height := float32(200) // the content's height: short until the data is in
	tt := ui.NewTester(func(c *ui.Context) {
		Scroll(c, &pos)
		ui.Scroll(c).TrackScroll(&pos).Fill().Children(func() {
			ui.Box(c).Width(50).Height(height)
		})
	}, 100, 100)

	Restore(State{Scroll: map[string]float32{loc: 500}})
	tt.Frame()
	if pos.Y >= 500 {
		t.Fatalf("content too short to scroll to 500, at %v", pos.Y)
	}
	height = 2000
	tt.Frame()
	tt.Frame()
	if pos.Y != 500 {
		t.Fatalf("offset %v, want 500", pos.Y)
	}
	// Landed: the user's own scrolling is not overridden.
	pos.Y = 300
	tt.Frame()
	if pos.Y != 300 {
		t.Fatalf("offset %v, want the user's 300", pos.Y)
	}
}

func TestFlagRestoredBeforeAndAfterRegistering(t *testing.T) {
	isolate(t)
	early := false
	Flag("early", &early)
	Restore(State{Flags: map[string]bool{"early": true, "late": true}})
	if !early {
		t.Fatal("a registered flag takes the saved value at once")
	}
	late := false
	Flag("late", &late)
	if !late {
		t.Fatal("a flag registered later takes it when registering")
	}
}

// A page that stays shorter than the saved offset stops being asked once it
// has rested at its end: the wait does not spin for the full ten seconds.
func TestScrollSettlesAtTheEnd(t *testing.T) {
	isolate(t)
	loc := "/short"
	SetLocation(func() string { return loc })
	var pos ui.ScrollState
	tt := ui.NewTester(func(c *ui.Context) {
		Scroll(c, &pos)
		ui.Scroll(c).TrackScroll(&pos).Fill().Children(func() {
			ui.Box(c).Width(50).Height(180)
		})
	}, 100, 100)
	Restore(State{Scroll: map[string]float32{loc: 5000}})
	for range settleFrames + 5 {
		tt.Frame()
	}
	reg.mu.Lock()
	_, waiting := reg.pending[loc]
	reg.mu.Unlock()
	if waiting {
		t.Fatal("still waiting for a page that cannot grow to the offset")
	}
}

// The app and the gallery keep separate files.
func TestSetNameSeparatesPrograms(t *testing.T) {
	isolate(t)
	defer SetName("")
	SetLocation(func() string { return "/library/albums" })
	Save("/library/albums", nil)

	SetName("uigallery")
	if _, ok := Load(); ok {
		t.Fatal("the gallery must not read the app's state")
	}
	SetLocation(func() string { return "/Button" })
	Save("/Button", nil)

	SetName("")
	st, ok := Load()
	if !ok || st.Route != "/library/albums" {
		t.Fatalf("the app's state was overwritten: %+v %v", st, ok)
	}
	SetName("uigallery")
	if st, ok := Load(); !ok || st.Route != "/Button" {
		t.Fatalf("the gallery's state: %+v %v", st, ok)
	}
}
