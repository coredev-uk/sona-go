package concerts

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// lateShow is a third show in April, a week after the 14th: the strip marks
// two days of the fortnight from the 9th.
var lateShow = spec{
	id: "ce.4", lineup: "Late Show", venue: "Corner Room", city: "London", zone: "Europe/London",
	start: "2027-04-21T19:00:00Z", genre: "20", artist: "900004", lat: 51.5, lon: -0.12,
	tickets: []string{"Example Tickets", "https://tickets.example.com/e/4"},
}

// onDay moves the page's today for a test: the fixtures are in April.
func onDay(t *testing.T, y int, m time.Month, d int) {
	t.Helper()
	old := today
	today = time.Date(y, m, d, 12, 0, 0, 0, time.UTC)
	t.Cleanup(func() { today = old })
}

// The Concerts page, light and dark: the title with its location button, the
// filters, the strip of the coming days over the agenda's ticket stubs.
func concertsGolden(t *testing.T, name string, dark bool, w, h int) {
	t.Helper()
	onDay(t, 2027, time.April, 9)
	var opts []uitest.Option
	if dark {
		opts = append(opts, uitest.Dark())
	}
	r := newRigOpts(t, "/concerts", w, h, opts, exampleBand, sampleSinger, lateShow, anotherAct)
	r.shows("April 2027", "June 2027", "Genres", "Your artists (2)", "Late Show")
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), name)
}

func TestConcertsPageGoldenLight(t *testing.T) { concertsGolden(t, "concerts-light", false, 1132, 900) }

func TestConcertsPageGoldenDark(t *testing.T) { concertsGolden(t, "concerts-dark", true, 1132, 900) }

// The narrowest the page gets beside the sidebar: the strip's cells and the
// rows' columns narrow, the line-up and the venue give way first.
func TestConcertsPageGoldenNarrow(t *testing.T) {
	concertsGolden(t, "concerts-narrow-light", false, 708, 900)
}

// The location dialog over the page, light.
func TestLocationDialogGolden(t *testing.T) {
	onDay(t, 2027, time.April, 9)
	r := newRig(t, "/concerts", 1132, 900, exampleBand, sampleSinger, lateShow, anotherAct)
	r.shows("Set location", "April 2027")
	r.click("Set location")
	r.shows("Search cities", "Manchester")
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "concerts-location-light")
}
