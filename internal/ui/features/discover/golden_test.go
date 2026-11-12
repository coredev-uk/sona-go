package discover

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenNew draws Discover over the trimmed real page, with the rotation
// paused so the picture is still.
func goldenNew(t *testing.T, name string, opts ...rigOption) {
	t.Helper()
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, "/new", append([]rigOption{withApple(&fakeApple{t: t}), withSize(1132, 1253)}, opts...)...)
	r.tt.Until("the page", r.tt.Shows("On this page", "Best New Songs", "New This Week"))
	r.d.front.paused = true
	r.settle()
	uitest.Golden(t, r.tt.Image(), name)
}

func TestGoldenDiscoverLight(t *testing.T) { goldenNew(t, "discover-new-light") }

func TestGoldenDiscoverDark(t *testing.T) { goldenNew(t, "discover-new-dark", withDark()) }

func TestGoldenChartsLight(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, "/charts", withApple(&fakeApple{t: t}), withSize(1132, 1253))
	r.tt.Until("the charts", r.tt.Shows("Top songs", "Top albums"))
	r.settle()
	uitest.Golden(t, r.tt.Image(), "discover-charts-light")
}

// TestGoldenDiscoverNarrow is the front page and the first band at the
// minimum window's content width (940 less the sidebar): the lead over the
// strip's two rows of two, the band's heading above its shelves.
func TestGoldenDiscoverNarrow(t *testing.T) {
	goldenNew(t, "discover-new-narrow-light", withSize(708, 1253))
}
