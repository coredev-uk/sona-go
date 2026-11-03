package shell

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
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
	skips := coordinator.Controls{Previous: true, Next: true}
	got := trayModel(coordinator.Snapshot{State: playback.StateLoading, Current: cur, Controls: skips})
	if got != (trayState{NowPlaying: "Song – Artist", Playing: true, HasItem: true, CanNext: true, CanPrevious: true}) {
		t.Fatalf("loading: %+v", got)
	}
	// A live station can't be skipped: Previous and Next stay off (the
	// controls, from the snapshot, decide).
	got = trayModel(coordinator.Snapshot{State: playback.StatePlaying, Current: &coordinator.Entry{Item: coordinator.Item{Title: "KISS"}}})
	if got != (trayState{NowPlaying: "KISS", Playing: true, HasItem: true}) {
		t.Fatalf("live station: %+v", got)
	}
	got = trayModel(coordinator.Snapshot{State: playback.StatePaused, Current: &coordinator.Entry{Item: coordinator.Item{Title: "Song"}}})
	if got != (trayState{NowPlaying: "Song", HasItem: true}) {
		t.Fatalf("paused, no artist: %+v", got)
	}
}

// The tray menu's text is in the printer's language; the song is not text
// of Sona's.
func TestTrayView(t *testing.T) {
	fr := l10n.NewPrinter("fr", map[l10n.Message]string{
		msgNotPlaying: "Aucune lecture",
		msgPlay:       "Lire",
		msgCloseMini:  "Fermer le mini lecteur",
		msgPrivateEnv: "Écoute privée (%s)",
	})
	tr := &tray{}
	want := trayView{
		now: "Aucune lecture", play: "Lire", next: "Next", prev: "Previous", private: "Private listening",
		show: "Show Sona", mini: "Mini player", quit: "Quit Sona", privateEnabled: true,
	}
	if v := tr.view(fr); v != want {
		t.Errorf("idle:\n%+v\nwant\n%+v", v, want)
	}
	tr.state = trayState{NowPlaying: "Song – Artist", Playing: true, HasItem: true, CanNext: true}
	tr.miniOpen = true
	tr.privateNow = privatemode.State{On: true, Source: privatemode.SourceEnvironment}
	want = trayView{
		now: "Song – Artist", play: "Pause", next: "Next", prev: "Previous", private: "Écoute privée (" + privatemode.EnvVar + "=1)",
		show: "Show Sona", mini: "Fermer le mini lecteur", quit: "Quit Sona", hasItem: true, canNext: true, privateOn: true,
	}
	if v := tr.view(fr); v != want {
		t.Errorf("playing:\n%+v\nwant\n%+v", v, want)
	}
}
