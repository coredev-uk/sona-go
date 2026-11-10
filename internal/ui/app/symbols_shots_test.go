package app

// TEMPORARY (removed before merge): the pictures of the SF Symbols PR, taken
// on a macOS runner by .github/workflows/tmp-sf-shots.yml. Runs only with
// SONA_SHOTS set to a directory.

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

func TestSymbolShots(t *testing.T) {
	dir := os.Getenv("SONA_SHOTS")
	if dir == "" {
		t.Skip("SONA_SHOTS is not set")
	}
	for _, tc := range []struct {
		name string
		opts []uitest.Option
	}{
		{"light", nil},
		{"dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := uitest.New(t, tc.opts...)
			a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now, StartPath: "/library/albums"}, factories)
			t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
			a.firstPaint = nil
			a.connect()
			tt := h.Tester(t, a.view, 1132, 760)
			tt.Art = append(tt.Art, a.env.Art)
			tt.Until("the albums", tt.Shows("Glass Harbour", "24 albums"))
			tt.Until("the covers", func() bool { return h.Covers.Hits() > 0 })
			s := store.EmptyState()
			s.State = store.StatePaused
			s.PositionMs = 71000
			s.Volume = 0.8
			s.Current = &services.QueueItem{ID: "i.1", Name: "Glass Harbour", Artist: "Dara Finch", Album: "Low Tide", DurationMs: 214000}
			a.env.Play.Seed(s, services.QueueState{Index: -1})
			tt.Settle()
			save(t, dir, "shell-"+tc.name, tt)
			tt.Key(ui.Cmd, ui.KeyK)
			tt.Settle()
			save(t, dir, "palette-"+tc.name, tt)
		})
	}
}

func save(t *testing.T, dir, name string, tt *uitest.Session) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}
