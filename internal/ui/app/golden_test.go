package app

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The shell with the real modules over the synthetic library, light and
// dark, at the default window size: the few pictures that say the visual
// language did not move by accident. `go test ./internal/ui/app -update`
// accepts a change on purpose.
func TestShellAlbumsGolden(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []uitest.Option
	}{
		{"shell-albums-light", nil},
		{"shell-albums-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := uitest.New(t, tc.opts...)
			a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art}, factories)
			t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
			a.firstPaint = nil // the network stays closed
			a.connect()
			tt := h.Tester(t, a.view, 1132, 1253)
			tt.Art = append(tt.Art, a.env.Art) // the shell's own loader
			tt.Until("the albums", tt.Shows("Glass Harbour", "24 albums"))
			tt.Until("the covers", func() bool { return h.Covers.Hits() > 0 })
			tt.Settle()
			uitest.Golden(t, tt.Image(), tc.name)
		})
	}
}
