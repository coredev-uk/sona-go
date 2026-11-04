package app

import (
	"path/filepath"
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
)

func TestObserveWindowKeepsTheRestoredSize(t *testing.T) {
	w := config.Window{Width: 1400, Height: 900}
	// Maximising keeps the size to return to.
	w = observeWindow(w, 2560, 1440, true, false)
	if w != (config.Window{Width: 1400, Height: 900, Maximised: true}) {
		t.Fatalf("maximised: %+v", w)
	}
	// Fullscreen changes nothing, not even the maximised flag.
	if got := observeWindow(w, 2560, 1440, false, true); got != w {
		t.Fatalf("fullscreen: %+v", got)
	}
	// Un-maximised, a resize is the new size.
	w = observeWindow(w, 1200, 800, false, false)
	if w != (config.Window{Width: 1200, Height: 800}) {
		t.Fatalf("resized: %+v", w)
	}
	// A hidden window reports no size; keep the last one.
	if got := observeWindow(w, 0, 0, false, false); got != w {
		t.Fatalf("zero size: %+v", got)
	}
}

// The remembered size reaches settings.json once the window has stopped
// changing, and a pending change is written when the app closes.
func TestWindowMemorySavesToTheSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := config.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	m := newWindowMemory(s, nil)
	m.log = discardLog()
	m.observe(1500, 950, false, false)
	m.observe(1501, 951, false, false) // a drag: only the last is saved
	if got := s.Get().Window; got == (config.Window{Width: 1501, Height: 951}) {
		t.Fatal("saved before the delay")
	}
	m.flush()
	if got := s.Get().Window; got != (config.Window{Width: 1501, Height: 951}) {
		t.Fatalf("after flush: %+v", got)
	}
	m.observe(1600, 1000, true, false)
	m.flush()
	if got := s.Get().Window; got != (config.Window{Width: 1501, Height: 951, Maximised: true}) {
		t.Fatalf("maximised: %+v", got)
	}
	// Reset: the default size, for now and for the next start.
	m.reset()
	m.flush()
	if got := s.Get().Window; got != (config.Window{Width: config.DefaultWindowWidth, Height: config.DefaultWindowHeight}) {
		t.Fatalf("reset: %+v", got)
	}
	// And it is read back by the next start.
	s2, err := config.OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := newWindowMemory(s2, discardLog()).current(); got.Width != config.DefaultWindowWidth {
		t.Fatalf("reloaded: %+v", got)
	}
}
