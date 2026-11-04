package app

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/features/player"
	"github.com/coredev-uk/sona-go/internal/ui/features/video"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// TestVideoViewKeysDoNotAlsoRunThePlayersShortcut runs the shell with the
// real video and player modules: with the focus in the video view, Space
// reaches the view only, so playback is toggled once, not twice.
func TestVideoViewKeysDoNotAlsoRunThePlayersShortcut(t *testing.T) {
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

	a := newApp(Options{Core: h.Core, Bus: h.Bus, Art: h.Art, Now: h.Clock.Now, Poster: h.Main.Post}, []module.Factory{video.New, player.New})
	t.Cleanup(a.env.Art.Close)
	a.connect()
	tt := h.Tester(t, a.view, 1132, 800)

	// The engine plays a song; the mirror is made to show a music video
	// playing, as the engine's events would once a video does.
	showVideo := func() {
		s := store.EmptyState()
		s.State = store.StatePlaying
		s.Current = &services.QueueItem{UID: 1, ID: "mv.1", CatalogID: "mv.1", Name: "Sky Video", Artist: "Mia", Video: true, DurationMs: 200_000}
		s.DurationMs, s.PositionMs, s.At = 200_000, 1000, h.Clock.Now().UnixMilli()
		a.env.Play.Seed(s, services.QueueState{Index: -1})
		a.env.Video.Open()
		tt.Frame()
	}
	quiet := func() { // the engine's start-up events have all arrived
		for still := 0; still < 10; time.Sleep(10 * time.Millisecond) {
			p := h.Main.Pending()
			tt.Drain()
			if p {
				still = 0
			} else {
				still++
			}
		}
	}
	quiet()
	showVideo()
	tt.Until("the view", tt.Shows("Close video"))
	before := len(h.Engine.Calls())

	tt.Key(0, ui.KeySpace)
	deadline := time.Now().Add(5 * time.Second)
	for len(h.Engine.Calls()) == before && time.Now().Before(deadline) {
		h.Main.Drain()
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // a second toggle, if there were one
	got := h.Engine.Calls()[before:]
	toggles := 0
	for _, c := range got {
		if c == "pause" || c == "resume" {
			toggles++
		}
	}
	if toggles != 1 {
		t.Errorf("Space in the video view toggled playback %d times: %v", toggles, slices.Clone(got))
	}
}
