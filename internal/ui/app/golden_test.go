package app

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The shell with the real modules over the synthetic library, light and
// dark, at the default window size: the few pictures that say the visual
// language did not move by accident. `go test ./internal/ui/app -update`
// accepts a change on purpose.
func TestShellAlbumsGolden(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  []uitest.Option
		theme string // Settings.ThemeID; "" is Sona's own
	}{
		{"shell-albums-light", nil, ""},
		{"shell-albums-dark", []uitest.Option{uitest.Dark()}, ""},
		// One built-in theme end to end: its board, its text and its spot colour.
		{"shell-albums-graphite-dark", []uitest.Option{uitest.Dark()}, "graphite"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := uitest.New(t, tc.opts...)
			if tc.theme != "" {
				if _, err := h.Settings.Update(func(s *config.Settings) { s.ThemeID = tc.theme }); err != nil {
					t.Fatal(err)
				}
			}
			a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now, StartPath: "/library/albums"}, factories)
			t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
			a.firstPaint = nil // the network stays closed
			a.connect()
			tt := h.Tester(t, a.view, 1132, 1253)
			tt.Art = append(tt.Art, a.env.Art) // the shell's own loader
			tt.Until("the albums and the sidebar's playlists", tt.Shows("Glass Harbour", "24 albums", "Sunday morning"))
			tt.Until("the covers", func() bool { return h.Covers.Hits() > 0 })
			tt.Settle()
			uitest.Golden(t, tt.Image(), tc.name)
		})
	}
}

// The account side's pieces in the shell: the update notice in the sidebar's
// foot, the player bar's Private pill (on, with its word) and the line under
// the title while a song waits for Widevine. They were full-window goldens;
// what those pictures said that no page golden does is that each piece is
// there at a wide and at a narrow window, and where the pill's word goes: it
// shows at 1132 DIP and is dropped at 940 (the pill stays).
func TestAccountStatesInTheShell(t *testing.T) {
	for _, tc := range []struct {
		name     string
		w, h     int
		pillWord bool
	}{
		{"wide", 1132, 1253, true},
		{"narrow", 940, 600, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := uitest.New(t)
			a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now, StartPath: "/library/albums"}, factories)
			t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
			a.firstPaint = nil
			a.connect()
			tt := h.Tester(t, a.view, tc.w, tc.h)
			tt.Art = append(tt.Art, a.env.Art)
			tt.Until("the albums and the sidebar's playlists", tt.Shows("24 albums", "Sunday morning"))
			h.Bus.Emit(services.EventPrivateChanged, services.PrivateState{On: true, Source: "setting", Saved: true})
			h.Bus.Emit(services.EventUpdateAvailable, services.UpdateStatus{Current: "0.2.0", Checkable: true, Latest: "0.3.0", Available: true})
			h.Bus.Emit(services.EventCDMStatus, services.CDMStatus{State: "downloading", Received: 42, Total: 100})
			s := store.EmptyState()
			s.State = store.StatePaused
			s.Current = &services.QueueItem{ID: "i.1", Name: "Glass Harbour", Artist: "Dara Finch", Album: "Low Tide"}
			s.NoCDM = true
			a.env.Play.Seed(s, services.QueueState{Index: -1})
			tt.Until("the account pieces", tt.Shows("Sona 0.3.0 is available", "Waiting for Widevine to download", "42%"))
			tt.Settle()
			pill, ok := tt.Find("Private listening")
			if !ok {
				t.Fatalf("no Private pill; texts %q", tt.Texts())
			}
			// With its word the pill is wider than tall; without, a square button.
			if got := pill.W > pill.H; got != tc.pillWord {
				t.Errorf("the Private pill shows its word = %v (box %v), want %v", got, pill, tc.pillWord)
			}
		})
	}
}
