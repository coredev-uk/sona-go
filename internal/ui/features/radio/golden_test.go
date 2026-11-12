package radio

import (
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

var fontsOnce sync.Once

// goldenRig is the Radio page over fixed data: covers the fixture CDN serves,
// schedules and a clock that do not move, a signed-in account.
func goldenRig(t *testing.T, path string, dark bool) *rig {
	return goldenRigAt(t, path, dark, 1132, 1253)
}

// goldenRigAt is goldenRig in a window of this size: the narrowest Sona
// opens at leaves the content about 650 DIPs.
func goldenRigAt(t *testing.T, path string, dark bool, width, height int) *rig {
	t.Helper()
	fontsOnce.Do(func() {
		if err := theme.RegisterFonts(); err != nil {
			t.Fatal(err)
		}
	})
	t0 := time.Date(2026, 11, 4, 10, 0, 0, 0, time.Local)
	return newRig(t, path, width, height, func(rg *rig) {
		if dark {
			theme.SetMode(theme.ModeDark)
		}
		rg.now = t0.Add(15 * time.Minute)
		rg.auth.state = "signed_in"
		cover := func(n int) services.Art {
			u := func(w int) string { return rg.h.Art.URL(fixture.Template(n), w, w) }
			return services.Art{Small: u(96), Medium: u(400), Large: u(1000)}
		}
		withArt := func(n int) func(*services.Station) { return func(s *services.Station) { s.Art = cover(n) } }
		one := liveStation("ra.1", "Apple Music 1", withArt(1))
		hits := liveStation("ra.2", "Hits", withArt(2))
		country := liveStation("ra.3", "Country", withArt(3))
		mine := trackStation("ra.mine", "My Station", withArt(4))
		kiss := broadcaster("ra.kiss", "KISS")
		kiss.Art = cover(5)
		ep := episodeStation("ra.ep", "Episode One")
		ep.Art = cover(6)
		prog := func(title string, from, to time.Time, art int, d ...string) services.StationEvent {
			e := stationEvent(title, from, to, d...)
			e.Art = cover(art)
			return e
		}
		sched := []services.StationEvent{
			prog("The Zane Lowe Show", t0, t0.Add(time.Hour), 7, "New music with Zane."),
			prog("Apple Music 1", t0.Add(time.Hour), t0.Add(2*time.Hour), 1),
			prog("The Matt Wilkinson Show", t0.Add(2*time.Hour), t0.Add(3*time.Hour), 8, "Matt plays the best of the week."),
		}
		rg.api.set(func(f *fakeRadio) {
			f.live = []services.Station{one, hits, country}
			f.recent = []services.Station{mine}
			f.details["ra.1"] = services.StationDetail{Station: one, Schedule: sched}
			f.details["ra.2"] = services.StationDetail{Station: hits, Schedule: []services.StationEvent{
				prog("Hits", t0, t0.Add(time.Hour), 2, "Songs you know."), prog("Hits", t0.Add(time.Hour), t0.Add(2*time.Hour), 2),
				prog("Club Hits", t0.Add(2*time.Hour), t0.Add(3*time.Hour), 3),
			}}
			f.details["ra.3"] = services.StationDetail{Station: country}
			f.grouping = services.EditorialPage{Sections: []services.Section{
				section("Local Broadcasters", itemsOf(kiss), services.SectionStations),
				section("Stations by Genre", itemsOf(mine, trackStation("ra.pop", "Pop", withArt(9)))),
				section("Latest Episodes", itemsOf(ep)),
			}}
		})
	})
}

func TestRadioPageGolden(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "radio-light"
		if dark {
			name = "radio-dark"
		}
		t.Run(name, func(t *testing.T) {
			rg := goldenRig(t, radioPath, dark)
			rg.tt.Until("the page", rg.tt.Shows("The Zane Lowe Show", "Latest Episodes", "My Station", "KISS"))
			if dark {
				// The dark picture has a station tuned (paused, so the equaliser
				// holds still): amber is its name's ink and its bars.
				rg.playing("ra.2", "paused")
				rg.tt.Until("the equaliser", rg.tt.Shows("Paused"))
			}
			rg.tt.Settle()
			uitestGolden(t, rg, name)
		})
	}
}

func TestStationPageGolden(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "station-light"
		if dark {
			name = "station-dark"
		}
		t.Run(name, func(t *testing.T) {
			rg := goldenRig(t, "/station/ra.1", dark)
			rg.tt.Until("the page", rg.tt.Shows("Schedule", "The Matt Wilkinson Show"))
			rg.tt.Settle()
			uitestGolden(t, rg, name)
		})
	}
}

// TestPagesGoldenAtTheNarrowestWindow pins the guide with a shorter stretch of
// time and the bands stacked under their headings, and the station page with
// its identity over the schedule.
func TestPagesGoldenAtTheNarrowestWindow(t *testing.T) {
	t.Run("radio-narrow", func(t *testing.T) {
		rg := goldenRigAt(t, radioPath, false, 708, 1100)
		rg.tt.Until("the page", rg.tt.Shows("The Zane Lowe Show", "Latest Episodes", "My Station", "KISS"))
		rg.tt.Settle()
		uitestGolden(t, rg, "radio-narrow")
	})
	t.Run("station-narrow", func(t *testing.T) {
		rg := goldenRigAt(t, "/station/ra.1", false, 708, 800)
		rg.tt.Until("the page", rg.tt.Shows("Schedule", "The Matt Wilkinson Show"))
		rg.tt.Settle()
		uitestGolden(t, rg, "station-narrow")
	})
}

func uitestGolden(t *testing.T, rg *rig, name string) {
	t.Helper()
	uitest.Golden(t, rg.tt.Image(), name)
}
