package radio

import (
	"image/color"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// v0.2.0's OnAir.test.tsx: the on-air board over a page of live stations.

// boardRig is the Radio page with these live stations, and each one's own
// page (its schedule) as given; no editorial page.
func boardRig(t *testing.T, stations []services.Station, details map[string]services.StationDetail, setup ...func(*rig)) *rig {
	t.Helper()
	return newRig(t, radioPath, 1132, 1253, append([]func(*rig){fake(func(f *fakeRadio) {
		f.live = stations
		for id, d := range details {
			f.details[id] = d
		}
	})}, setup...)...)
}

func TestOnAirRowShowsTheProgrammeHowLongIsLeftAndWhatComesAfter(t *testing.T) {
	t0 := base()
	one := appleMusic1
	rg := boardRig(t, []services.Station{one}, map[string]services.StationDetail{"ra.1": {Station: one, Schedule: []services.StationEvent{
		stationEvent("The Zane Lowe Show", t0, t0.Add(time.Hour)),
		stationEvent("Apple Music 1", t0.Add(time.Hour), t0.Add(2*time.Hour)),
		stationEvent("The Matt Wilkinson Show", t0.Add(2*time.Hour), t0.Add(3*time.Hour)),
	}}})
	rg.tt.Until("the programme", rg.tt.Shows("The Zane Lowe Show", "The Matt Wilkinson Show"))
	slot := clockAt(t0.UnixMilli()) + "–" + clockAt(t0.Add(time.Hour).UnixMilli())
	for _, want := range []string{slot, "45 min left", "Time through this programme"} {
		if !rg.tt.HasText(want) {
			t.Errorf("the row lacks %q: %q", want, rg.tt.Texts())
		}
	}
	// The station's own hour comes between: it is a box of its own (quiet,
	// the tagline), and the later show starts where its slot says.
	if !rg.tt.HasText("Apple Music 1 tagline") {
		t.Errorf("the station's own hour is not on the board: %q", rg.tt.Texts())
	}
	if !rg.tt.HasText(clockAt(t0.Add(2*time.Hour).UnixMilli()) + "–" + clockAt(t0.Add(3*time.Hour).UnixMilli())) {
		t.Errorf("the later show lacks its slot: %q", rg.tt.Texts())
	}
}

func TestOnAirRowJoinsTheStationsOwnHoursIntoOneBoxAndCountsThemDownToTheNextShow(t *testing.T) {
	t0 := base()
	one := appleMusic1
	rg := boardRig(t, []services.Station{one}, map[string]services.StationDetail{"ra.1": {Station: one, Schedule: []services.StationEvent{
		stationEvent("Apple Music 1", t0, t0.Add(time.Hour), "New music."),
		stationEvent("Apple Music 1", t0.Add(time.Hour), t0.Add(2*time.Hour)),
		stationEvent("The Ebro Show", t0.Add(2*time.Hour), t0.Add(3*time.Hour)),
	}}})
	slot := clockAt(t0.UnixMilli()) + "–" + clockAt(t0.Add(2*time.Hour).UnixMilli())
	rg.tt.Until("the row", rg.tt.Shows("New music.", slot, "The Ebro Show"))
	// 1 h 45 min to the show, as one stretch.
	if !rg.tt.HasText("1 hr, 45 min left") {
		t.Errorf("no count down: %q", rg.tt.Texts())
	}
}

func TestOnAirRowFallsBackToTheTaglineForItsOwnHoursAndWithoutASchedule(t *testing.T) {
	t0 := base()
	hits, chill := liveStation("ra.2", "Hits"), liveStation("ra.3", "Chill")
	rg := boardRig(t, []services.Station{hits, chill}, map[string]services.StationDetail{"ra.2": {Station: hits, Schedule: []services.StationEvent{
		stationEvent("Hits", t0, t0.Add(time.Hour), "Hits tagline"),
		stationEvent("Hits", t0.Add(time.Hour), t0.Add(2*time.Hour)),
	}}})
	rg.tt.Until("the rows", rg.tt.Shows("Hits tagline", "Chill tagline"))
	// Hours to the end of the schedule have nothing to count down to; no
	// schedule has no bar.
	if rg.tt.HasText("Time through this programme") {
		t.Errorf("a progress bar without a stretch to count down: %q", rg.tt.Texts())
	}
}

func TestOnAirRowSaysWhyAStationCannotPlay(t *testing.T) {
	off := liveStation("ra.4", "Off", func(s *services.Station) { s.Playable, s.Unavailable = false, services.StationNoWidevine })
	plain := liveStation("ra.5", "Plain", func(s *services.Station) { s.Playable, s.Unavailable = false, "" })
	rg := boardRig(t, []services.Station{off, plain}, nil)
	rg.tt.Until("the rows", rg.tt.Shows("Off", "Plain"))
	for _, want := range []string{
		"This stream isn't offered with Widevine, the only protection Sona can play.",
		"Sona can't play this station",
	} {
		if !rg.tt.HasText(want) {
			t.Errorf("no %q: %q", want, rg.tt.Texts())
		}
	}
	for _, name := range []string{"Play Off", "Play Plain"} {
		if _, ok := rg.tt.Find(name); ok {
			t.Errorf("%q has a play button", name)
		}
	}
}

func TestOnAirRowPlaysFromItsButtonAndMarksTheStationPlaying(t *testing.T) {
	rg := boardRig(t, []services.Station{appleMusic1, appleHits}, nil)
	rg.playing("ra.2", "playing")
	rg.tt.Until("the rows", rg.tt.Shows("Apple Music 1", "Hits", "Now playing"))
	rg.click("Play Apple Music 1")
	rg.tt.Until("the play", func() bool { return slices.Equal(rg.api.plays(), []string{"ra.1"}) })
	// The tuned station shows the equaliser, not the button.
	if _, ok := rg.tt.Find("Play Hits"); ok {
		t.Error("the station that plays has a play button")
	}
	if _, ok := rg.tt.Find("Now playing"); !ok {
		t.Errorf("no equaliser: %q", rg.tt.Texts())
	}
	// Focus stays on the row's link, which the equaliser's arrival leaves
	// in place.
	if !rg.tt.Focused("Apple Music 1") {
		t.Error("focus left the row")
	}
}

func TestOnAirRowOfAStationWithNoPagePlaysAndOffersItsMenu(t *testing.T) {
	quiet := liveStation("ra.2", "Quiet FM", func(s *services.Station) { s.HasPage = false })
	rg := boardRig(t, []services.Station{quiet}, map[string]services.StationDetail{"ra.2": {Station: quiet, Schedule: []services.StationEvent{}}})
	rg.tt.Until("the row", rg.tt.Shows("Quiet FM", "More for Quiet FM"))
	// Its name is a button that plays it; there is no page link and no play
	// button of the row's.
	if _, ok := rg.tt.Find("Play Quiet FM"); ok {
		t.Error("a station with no page has a second play control")
	}
	rg.click("Quiet FM Play")
	rg.tt.Until("the play", func() bool { return slices.Equal(rg.api.plays(), []string{"ra.2"}) })
	if rg.router.Path() != radioPath {
		t.Errorf("went to %q", rg.router.Path())
	}
	// Once tuned, the same button pauses.
	rg.playing("ra.2", "playing")
	rg.tt.Drain()
	rg.click("Quiet FM Pause")
	rg.tt.Until("the toggle", func() bool { n, _ := rg.pb.counts(); return n == 1 })
	if got := rg.api.plays(); len(got) != 1 {
		t.Errorf("a tuned station was played again: %v", got)
	}
}

func TestOnAirRowOpensItsPage(t *testing.T) {
	rg := boardRig(t, []services.Station{appleMusic1}, nil)
	rg.tt.Until("the row", rg.tt.Shows("Apple Music 1"))
	rg.click("Apple Music 1")
	if rg.router.Path() != "/station/ra.1" {
		t.Errorf("went to %q", rg.router.Path())
	}
}

func TestOnAirRowKeepsItsPageOnceItsOwnScheduleSaysItHasOne(t *testing.T) {
	t0 := base()
	// The list says no page; the station's own page (its schedule) wins.
	listed := liveStation("ra.3", "Show FM", func(s *services.Station) { s.HasPage = false })
	page := listed
	page.HasPage = true
	rg := boardRig(t, []services.Station{listed}, map[string]services.StationDetail{"ra.3": {Station: page, Schedule: []services.StationEvent{
		stationEvent("Morning", t0, t0.Add(time.Hour)),
	}}})
	rg.tt.Until("the schedule", rg.tt.Shows("Morning"))
	if _, ok := rg.tt.Find("Show FM Play"); ok {
		t.Error("a station with a schedule plays when opened")
	}
	rg.click("Show FM")
	if rg.router.Path() != "/station/ra.3" {
		t.Errorf("went to %q", rg.router.Path())
	}
}

func TestOnAirBoardReadsItsSchedulesEvery30SecondsAndNeverTicks(t *testing.T) {
	t0 := base()
	one := appleMusic1
	rg := boardRig(t, []services.Station{one}, map[string]services.StationDetail{"ra.1": {Station: one, Schedule: []services.StationEvent{
		stationEvent("The Zane Lowe Show", t0, t0.Add(time.Hour)),
		stationEvent("The Matt Wilkinson Show", t0.Add(time.Hour), t0.Add(2*time.Hour)),
	}}}, func(rg *rig) {
		// Ten seconds before the show ends.
		rg.now = t0.Add(time.Hour - 10*time.Second)
	})
	rg.tt.Until("the show", rg.tt.Shows("The Zane Lowe Show", "1 min left"))
	// Twenty seconds on the slot is over, but the board has not read the
	// clock again: no ticking counter, the show is still the one on air.
	rg.advance(20 * time.Second)
	if !rg.tt.HasText("1 min left") || rg.tt.HasText("1 hr left") {
		t.Errorf("the board moved before its 30 seconds: %q", rg.tt.Texts())
	}
	// Thirty seconds in, it reads again and the next show is on air.
	rg.advance(10 * time.Second)
	if !rg.tt.HasText("1 hr left") || rg.tt.HasText("1 min left") {
		t.Errorf("the board did not move on after 30 seconds: %q", rg.tt.Texts())
	}
}

func TestOnAirBoardKeepsWorkingAtTheNarrowestWindowWithAShorterStretchOfTime(t *testing.T) {
	t0 := base()
	one, hits := appleMusic1, appleHits
	details := map[string]services.StationDetail{
		"ra.1": {Station: one, Schedule: []services.StationEvent{
			stationEvent("The Zane Lowe Show", t0, t0.Add(time.Hour)),
			stationEvent("The Matt Wilkinson Show", t0.Add(time.Hour), t0.Add(2*time.Hour)),
			stationEvent("The Ebro Show", t0.Add(2*time.Hour), t0.Add(3*time.Hour)),
		}},
		"ra.2": {Station: hits},
	}
	stations := []services.Station{one, hits}
	// The lanes are stacked at every width; the wider the window, the more
	// hours its track holds.
	wide := boardRig(t, stations, details)
	wide.tt.Until("the lanes", wide.tt.Shows("Apple Music 1", "Hits", "The Ebro Show"))
	a, _ := wide.tt.Find("Apple Music 1")
	b, _ := wide.tt.Find("Hits")
	if a.X != b.X || b.Y <= a.Y {
		t.Errorf("wide: %v and %v are not stacked", a, b)
	}
	narrow := newRig(t, radioPath, 700, 1253, fake(func(f *fakeRadio) {
		f.live = stations
		for id, d := range details {
			f.details[id] = d
		}
	}))
	narrow.tt.Until("the lanes", narrow.tt.Shows("Apple Music 1", "Hits", "The Zane Lowe Show"))
	a, _ = narrow.tt.Find("Apple Music 1")
	b, _ = narrow.tt.Find("Hits")
	if a.X != b.X || b.Y <= a.Y {
		t.Errorf("narrow: %v and %v are not stacked", a, b)
	}
	// The narrow track is two hours at most: the third show is out of view.
	if narrow.tt.HasText("The Ebro Show") {
		t.Errorf("a show three hours ahead is on a narrow board: %q", narrow.tt.Texts())
	}
}

// pixel is the colour of the frame at a point in DIPs (scale 1).
func pixel(rg *rig, x, y float32) color.RGBA {
	return rg.tt.Image().RGBAAt(int(x), int(y))
}

func near(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) < 12 && d(a.G, b.G) < 12 && d(a.B, b.B) < 12
}

func rgba(c ui.Color) color.RGBA { return color.RGBA{c.R, c.G, c.B, 255} }

func TestOnAirProgressIsAmberOnlyWhileThisStationPlays(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("pixel readings are asserted on Linux only")
	}
	t0 := base()
	one := appleMusic1
	details := map[string]services.StationDetail{"ra.1": {Station: one, Schedule: []services.StationEvent{
		stationEvent("The Zane Lowe Show", t0, t0.Add(time.Hour)),
		stationEvent("The Matt Wilkinson Show", t0.Add(time.Hour), t0.Add(2*time.Hour)),
	}}}
	fill := func(rg *rig) color.RGBA {
		t.Helper()
		bar, ok := rg.tt.Find("Time through this programme")
		if !ok {
			t.Fatalf("no progress bar: %q", rg.tt.Texts())
		}
		// 25% through: the bar's first pixels (past the box's rounded corner) are the fill.
		return pixel(rg, bar.X+12, bar.Y+bar.H/2)
	}
	still := boardRig(t, []services.Station{one}, details)
	still.tt.Until("the bar", still.tt.Shows("Time through this programme"))
	if got := fill(still); !near(got, rgba(theme.C("ink-muted"))) {
		t.Errorf("fill of a station that is not playing = %v, want --ink-muted %v", got, rgba(theme.C("ink-muted")))
	}
	playing := boardRig(t, []services.Station{one}, details)
	playing.playing("ra.1", "playing")
	playing.tt.Until("the bar", playing.tt.Shows("Time through this programme", "Now playing"))
	if got := fill(playing); !near(got, rgba(theme.C("play"))) {
		t.Errorf("fill of the station playing = %v, want --play %v", got, rgba(theme.C("play")))
	}
	paused := boardRig(t, []services.Station{one}, details)
	paused.playing("ra.1", "paused")
	paused.tt.Until("the bar", paused.tt.Shows("Time through this programme"))
	if got := fill(paused); !near(got, rgba(theme.C("ink-muted"))) {
		t.Errorf("fill of the station paused = %v, want --ink-muted", got)
	}
}

func TestOnAirProgrammeBoxOpensTheStationsPage(t *testing.T) {
	t0 := base()
	one := appleMusic1
	rg := boardRig(t, []services.Station{one}, map[string]services.StationDetail{"ra.1": {Station: one, Schedule: []services.StationEvent{
		stationEvent("The Zane Lowe Show", t0, t0.Add(time.Hour)),
		stationEvent("The Matt Wilkinson Show", t0.Add(time.Hour), t0.Add(2*time.Hour)),
	}}})
	rg.tt.Until("the boxes", rg.tt.Shows("The Zane Lowe Show", "The Matt Wilkinson Show"))
	// A box to come is as much a link as the one on air.
	rg.click("The Matt Wilkinson Show, " + clockAt(t0.Add(time.Hour).UnixMilli()) + "–" + clockAt(t0.Add(2*time.Hour).UnixMilli()))
	if rg.router.Path() != "/station/ra.1" {
		t.Errorf("went to %q", rg.router.Path())
	}
}

func TestOnAirNowLineAndFlagSitAtTheClockTimeOfTheBoard(t *testing.T) {
	rg := boardRig(t, []services.Station{appleMusic1}, nil)
	now := clockAt(rg.now.UnixMilli())
	rg.tt.Until("the flag", rg.tt.Shows(now))
	// The ruler names every half hour but the one the flag covers.
	if !rg.tt.HasText(clockAt(base().UnixMilli())) {
		t.Errorf("the ruler lacks the hour: %q", rg.tt.Texts())
	}
	if n := rg.count(now); n != 1 {
		t.Errorf("the clock time of now shows %d times", n)
	}
}
