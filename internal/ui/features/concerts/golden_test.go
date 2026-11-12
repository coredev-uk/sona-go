package concerts

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The Concerts page, light and dark: the header with its location button,
// the filters, and the agenda by month and day.
func concertsGolden(t *testing.T, name string, dark bool) {
	t.Helper()
	var opts []uitest.Option
	if dark {
		opts = append(opts, uitest.Dark())
	}
	r := newRigOpts(t, "/concerts", 1132, 900, opts, exampleBand, sampleSinger, anotherAct)
	r.shows("April 2027", "June 2027", "Genres", "Your artists (2)", "Get tickets")
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), name)
}

func TestConcertsPageGoldenLight(t *testing.T) { concertsGolden(t, "concerts-light", false) }

func TestConcertsPageGoldenDark(t *testing.T) { concertsGolden(t, "concerts-dark", true) }

// The location dialog over the page, light.
func TestLocationDialogGolden(t *testing.T) {
	r := newRig(t, "/concerts", 1132, 900, exampleBand, sampleSinger, anotherAct)
	r.shows("Set location", "April 2027")
	r.tt.MustClick("Set location")
	r.shows("Search cities", "Manchester")
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "concerts-location-light")
}
