package newreleases

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenPage draws the page over the invented list at the reference window
// size, the covers loaded. `go test ./internal/ui/features/newreleases
// -update` accepts a change on purpose.
func goldenPage(t *testing.T, list services.NewReleases, on bool, w, h int, dark bool) *rig {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, list, on, w, h, uitest.Empty())
	if dark {
		theme.SetMode(theme.ModeDark)
		t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	}
	return r
}

func TestGoldenListLight(t *testing.T) {
	r := goldenPage(t, sample(), true, 1132, 700, false)
	r.until("the list", r.tt.Shows("Winter Rooms", "Slow Harbour"))
	r.until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "list-light")
}

func TestGoldenListDark(t *testing.T) {
	r := goldenPage(t, sample(), true, 1132, 700, true)
	r.until("the list", r.tt.Shows("Winter Rooms", "Slow Harbour"))
	r.until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "list-dark")
}

func TestGoldenListNarrow(t *testing.T) {
	r := goldenPage(t, sample(), true, 700, 700, false)
	r.until("the list", r.tt.Shows("Winter Rooms", "Slow Harbour"))
	r.until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "list-narrow-light")
}

func TestGoldenOffLight(t *testing.T) {
	r := goldenPage(t, services.NewReleases{}, false, 1132, 360, false)
	r.until("the state", r.tt.Shows("Release tracking is off"))
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "off-light")
}

func TestGoldenOffDark(t *testing.T) {
	r := goldenPage(t, services.NewReleases{}, false, 1132, 360, true)
	r.until("the state", r.tt.Shows("Release tracking is off"))
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "off-dark")
}

// goldenShelf draws For You's shelf over the invented list, as goldenPage does
// the page.
func goldenShelf(t *testing.T, w, h int, dark bool) *rig {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	r := newShelfRig(t, sample(), true, w, h, uitest.Empty())
	if dark {
		theme.SetMode(theme.ModeDark)
		t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	}
	return r
}

func TestGoldenShelfLight(t *testing.T) {
	r := goldenShelf(t, 900, 320, false)
	r.until("the shelf", r.tt.Shows("Winter Rooms", "Lantern Season"))
	r.until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "shelf-light")
}

func TestGoldenShelfDark(t *testing.T) {
	r := goldenShelf(t, 900, 320, true)
	r.until("the shelf", r.tt.Shows("Winter Rooms", "Lantern Season"))
	r.until("the covers", func() bool { return r.h.Covers.Hits() > 0 })
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "shelf-dark")
}
