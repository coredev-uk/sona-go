package app

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/features/player"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// A page with one focusable button, the page the room opens over.
func pageWithButton(*module.Env) module.Module {
	return module.Module{
		ID: "behind",
		Routes: []module.Route{{Pattern: "/", Build: func(c *ui.Context, _ *ui.Route) {
			kit.Button(c.Key("go"), "Page button", kit.ButtonOpts{})
		}}},
		Nav: []module.NavItem{{Section: module.NavPrimary, Label: func() string { return "Home" }, Icon: "music", To: "/", Order: 1}},
	}
}

// With the full-screen lyrics room of Now Playing open, Tab walks the room's
// own controls and never reaches the page, its sidebar or the player bar
// behind it.
func TestTabInTheNowPlayingRoomNeverReachesThePageBehind(t *testing.T) {
	h := uitest.New(t)
	a := newApp(Options{Core: h.Core, Bus: h.Bus, Art: h.Art, Now: h.Clock.Now, Poster: h.Main.Post}, []module.Factory{pageWithButton, player.New})
	t.Cleanup(a.env.Art.Close)
	a.connect()
	tt := h.Tester(t, a.view, 1132, 800)

	// The real engine plays a fixture album: its events fill the mirror.
	ctx := context.Background()
	lib := services.Service[services.LibraryService](h.Core)
	page, err := lib.Albums(ctx, "dateAdded", true, "", 0, 1)
	if err != nil || len(page.Items) == 0 {
		t.Fatalf("no fixture album: %v", err)
	}
	detail, err := lib.Album(ctx, page.Items[0].ID)
	if err != nil || len(detail.Tracks) == 0 {
		t.Fatalf("no tracks: %v", err)
	}
	var refs []services.TrackRef
	for _, s := range detail.Tracks {
		refs = append(refs, services.TrackRef{ID: s.ID, Source: s.Source})
	}
	if err := services.Service[services.PlaybackService](h.Core).PlayItems(ctx, refs, 0, services.PlayFrom{}); err != nil {
		t.Fatal(err)
	}
	tt.Until("a song playing", tt.Shows("Open now playing"))

	behind := []string{"Page button", "Home", "Open now playing", "Close now playing", "Play", "Pause", "Next", "Previous"}
	// Before the room: the page takes the focus.
	reached := false
	for range 40 {
		tt.Key(0, ui.KeyTab)
		if tt.Focused("Page button") {
			reached = true
			break
		}
	}
	if !reached {
		t.Fatal("Tab never reached the page with the room closed")
	}

	tt.Key(ui.Shift, ui.KeyL) // Shift+L: full-screen lyrics
	tt.Until("the room", tt.Shows("Exit full screen"))
	seenRoom := false
	for range 60 {
		tt.Key(0, ui.KeyTab)
		for _, l := range behind {
			if tt.Focused(l) {
				t.Fatalf("Tab reached %q behind the open room", l)
			}
		}
		if tt.Focused("Exit full screen") {
			seenRoom = true
		}
	}
	if !seenRoom {
		t.Error("Tab never reached the room's own controls")
	}
}
