package app

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/features/onboarding"
	"github.com/coredev-uk/sona-go/internal/ui/features/player"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// TestShortcutsDoNotRunWhileTheSetupIsOpen runs the shell with the real
// player and onboarding modules: with the first-run setup open Space does not
// pause playback behind it, and once it is closed Space does.
func TestShortcutsDoNotRunWhileTheSetupIsOpen(t *testing.T) {
	h := uitest.New(t)
	ctx := context.Background()
	lib := services.Service[services.LibraryService](h.Core)
	pb := services.Service[services.PlaybackService](h.Core)
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
	if err := pb.PlayItems(ctx, refs, 0, services.PlayFrom{}); err != nil {
		t.Fatal(err)
	}

	a := newApp(Options{Core: h.Core, Bus: h.Bus, Art: h.Art, Now: h.Clock.Now, Poster: h.Main.Post}, []module.Factory{player.New, onboarding.New})
	t.Cleanup(a.env.Art.Close)
	a.connect()
	tt := h.Tester(t, a.view, 1132, 800)
	for still := 0; still < 10; time.Sleep(10 * time.Millisecond) { // the engine's start-up events
		p := h.Main.Pending()
		tt.Drain()
		if p {
			still = 0
		} else {
			still++
		}
	}
	toggles := func() (n int) {
		for _, c := range h.Engine.Calls() {
			if c == "pause" || c == "resume" {
				n++
			}
		}
		return n
	}

	a.env.Surfaces.OpenSetup(store.SetupWelcome)
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	time.Sleep(50 * time.Millisecond)
	h.Main.Drain()
	if n := toggles(); n != 0 {
		t.Fatalf("Space paused playback %d times behind the open setup", n)
	}

	a.env.Surfaces.CloseSetup()
	tt.Frame()
	tt.Key(0, ui.KeySpace)
	deadline := time.Now().Add(5 * time.Second)
	for toggles() == 0 && time.Now().Before(deadline) {
		h.Main.Drain()
		time.Sleep(5 * time.Millisecond)
	}
	if toggles() == 0 {
		t.Error("Space does nothing with the setup closed: the test does not exercise the shortcut")
	}
}
