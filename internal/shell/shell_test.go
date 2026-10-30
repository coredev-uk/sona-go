package shell

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
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

func TestTrayModel(t *testing.T) {
	if got := trayModel(coordinator.Snapshot{State: playback.StateIdle}); got != (trayState{}) {
		t.Fatalf("idle: %+v", got)
	}
	cur := &coordinator.Entry{Item: coordinator.Item{Title: "Song", Artist: "Artist"}}
	got := trayModel(coordinator.Snapshot{State: playback.StateLoading, Current: cur})
	if got != (trayState{NowPlaying: "Song – Artist", Playing: true, HasItem: true}) {
		t.Fatalf("loading: %+v", got)
	}
	got = trayModel(coordinator.Snapshot{State: playback.StatePaused, Current: &coordinator.Entry{Item: coordinator.Item{Title: "Song"}}})
	if got != (trayState{NowPlaying: "Song", HasItem: true}) {
		t.Fatalf("paused, no artist: %+v", got)
	}
}
