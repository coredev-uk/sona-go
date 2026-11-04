package app

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The playlist dialogs live in a Layer slot with no LayerOpen: the layer
// takes no input itself, and a dialog built in it still does.
func TestPlaylistDialogTakesInputInTheShell(t *testing.T) {
	h := uitest.New(t)
	a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now}, factories)
	t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
	a.firstPaint = nil
	a.connect()
	tt := h.Tester(t, a.view, 1132, 800)
	tt.Art = append(tt.Art, a.env.Art)
	tt.Until("the playlists", tt.Shows("Road trip"))
	click := func(s string) {
		t.Helper()
		if err := tt.Click(s); err != nil {
			t.Fatalf("%v; texts %q", err, tt.Texts())
		}
		tt.Advance(300 * time.Millisecond)
		tt.Drain()
	}
	click("New playlist or folder")
	tt.Until("the menu", tt.Shows("New folder…"))
	click("New folder…")
	tt.Until("the dialog", tt.Shows("Folders group playlists in the sidebar on all your devices."))
	tt.Advance(100 * time.Millisecond)
	tt.Drain()
	if !tt.Focused("Name") {
		t.Fatalf("the name field has no focus")
	}
	click("Cancel")
	if tt.HasText("Folders group playlists in the sidebar on all your devices.") {
		t.Error("Cancel did not close the dialog")
	}
	// The window behind is live again.
	click("Road trip")
	tt.Until("the page", tt.Shows("Songs in Road trip"))
}
