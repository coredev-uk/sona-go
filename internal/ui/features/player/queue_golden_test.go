package player

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The bar over the queue panel, light and dark, playing the fixture's second
// album: the pictures that say the bar and the queue did not move by accident
// (`go test ./internal/ui/features/player -update` accepts a change on purpose).
func TestBarAndQueueGolden(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		opts []uitest.Option
	}{
		{"player-queue-light", nil},
		{"player-queue-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFlow(t, tc.opts...)
			f.tt.MustClick("Queue")
			f.tt.Until("the queue's rows", f.tt.Shows("Up next: "+f.queueRow(1), f.queueRow(2)))
			// A playing bar ticks and the equaliser moves: pause it for the picture.
			f.tt.MustClick("Pause")
			f.tt.Until("paused", f.tt.Shows("Play"))
			f.tt.Until("the covers", func() bool { return f.h.Covers.Hits() > 0 })
			f.tt.Settle()
			uitest.Golden(t, f.tt.Image(), tc.name)
		})
	}
}
