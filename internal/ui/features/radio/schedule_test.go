package radio

import (
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The cases of v0.2.0's schedule.test.ts.

func ev(id string, start, end int64, description ...string) services.StationEvent {
	e := services.StationEvent{ID: id, Title: id, StartMs: start, EndMs: end}
	if len(description) > 0 {
		e.Description = description[0]
	}
	return e
}

const hourMs = int64(time.Hour / time.Millisecond)

// t0 is 10:00 local on 30 October 2026.
var t0 = time.Date(2026, 10, 30, 10, 0, 0, 0, time.Local).UnixMilli()

func TestOnAirFindsTheProgramme(t *testing.T) {
	sched := []services.StationEvent{ev("a", t0, t0+hourMs), ev("b", t0+hourMs, t0+3*hourMs)}
	for _, c := range []struct {
		now  int64
		want string
	}{{t0 + 10, "a"}, {t0 + hourMs, "b"}, {t0 + 3*hourMs, ""}, {t0 - 1, ""}} {
		got, ok := kit.OnAirProgramme(sched, c.now)
		if ok != (c.want != "") || got.ID != c.want {
			t.Errorf("onAir at %d = %q %v, want %q", c.now-t0, got.ID, ok, c.want)
		}
	}
	if _, ok := kit.OnAirProgramme(nil, t0); ok {
		t.Error("an empty schedule has nothing on air")
	}
}

func TestProgressOfASlotIsClampedToZeroAndOne(t *testing.T) {
	sched := []services.StationEvent{ev("a", t0, t0+hourMs), ev("b", t0+hourMs, t0+3*hourMs)}
	for _, c := range []struct {
		e    services.StationEvent
		now  int64
		want float64
	}{
		{sched[1], t0 + 2*hourMs, 0.5},
		{sched[0], t0 - 1, 0},
		{sched[0], t0 + 5*hourMs, 1},
		{ev("z", t0, t0), t0, 0},
	} {
		if got := progressOf(spanOf(c.e), c.now); got != c.want {
			t.Errorf("progress of %s at %d = %v, want %v", c.e.ID, c.now-t0, got, c.want)
		}
	}
}

func TestSlotLabelsAddTheWeekdayOnlyForAnotherDay(t *testing.T) {
	a := ev("a", t0, t0+hourMs)
	tomorrow := ev("c", t0+24*hourMs, t0+25*hourMs)
	same := slotLabel(spanOf(a), t0)
	if want := clockAt(t0) + "–" + clockAt(t0+hourMs); same != want {
		t.Errorf("a slot today reads %q, want %q", same, want)
	}
	weekday := weekdayShort(time.UnixMilli(tomorrow.StartMs))
	if weekday == "" {
		t.Fatal("no weekday")
	}
	if got, want := slotLabel(spanOf(tomorrow), t0), weekday+" "+slotLabel(spanOf(tomorrow), tomorrow.StartMs); got != want {
		t.Errorf("tomorrow's slot reads %q, want %q", got, want)
	}
	if got, want := startLabel(tomorrow, t0), weekday+" "+startLabel(tomorrow, tomorrow.StartMs); got != want {
		t.Errorf("tomorrow's start reads %q, want %q", got, want)
	}
	if !strings.HasPrefix(slotLabel(spanOf(a), t0), startLabel(a, t0)) {
		t.Error("a slot label starts with the start label")
	}
}

func TestWeekdayIsTheAbbreviatedName(t *testing.T) {
	// 30 October 2026 is a Friday.
	if got := weekdayShort(time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)); got != "Fri" {
		t.Errorf("weekday = %q, want Fri", got)
	}
}

var hits = struct{ name, tagline string }{"Hits", "Songs you know."}

func TestProgrammeNamePrefersTheTitleElseTheStationsOwnDescription(t *testing.T) {
	for _, c := range []struct {
		name string
		e    services.StationEvent
		want string
	}{
		{"a show", ev("The Zane Lowe Show", t0, t0+hourMs), "The Zane Lowe Show"},
		{"the station's hour, described", ev("hits", t0, t0+hourMs, "Club euphoria with NAINA."), "Club euphoria with NAINA."},
		{"the station's hour repeating the tagline", ev("Hits", t0, t0+hourMs, "Songs you know."), ""},
		{"the station's hour with nothing said", ev("Hits", t0, t0+hourMs), ""},
	} {
		if got := programmeName(hits.name, hits.tagline, c.e); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestNextProgrammeSkipsTheStationsOwnHoursAndTheShowOnAir(t *testing.T) {
	day := []services.StationEvent{
		ev("Hits", t0, t0+hourMs),
		ev("Hits", t0+hourMs, t0+2*hourMs),
		ev("The Matt Wilkinson Show", t0+2*hourMs, t0+4*hourMs),
		ev("The Matt Wilkinson Show", t0+4*hourMs, t0+5*hourMs),
		ev("Rebecca Judd", t0+5*hourMs, t0+6*hourMs),
	}
	if got, _ := nextProgramme(hits.name, day, t0+10); got.StartMs != t0+2*hourMs {
		t.Errorf("next at the start = %q", got.Title)
	}
	// The show on air is not its own "next", even when it runs on.
	if got, _ := nextProgramme(hits.name, day, t0+3*hourMs); got.Title != "Rebecca Judd" {
		t.Errorf("next during the show = %q", got.Title)
	}
	if got, ok := nextProgramme(hits.name, day, t0+5*hourMs); ok {
		t.Errorf("next during the last = %q", got.Title)
	}
	if got, ok := nextProgramme(hits.name, day[:2], t0); ok {
		t.Errorf("next with only the station's hours = %q", got.Title)
	}
	// Nothing on air (a gap in the schedule): the next named slot from now.
	if got, _ := nextProgramme(hits.name, day[2:], t0); got.StartMs != t0+2*hourMs {
		t.Errorf("next with nothing on air = %q", got.Title)
	}
}

func TestOnAirSpanCountsRunsOfSlotsAsOneStretch(t *testing.T) {
	day := []services.StationEvent{
		ev("Hits", t0, t0+hourMs),
		ev("Hits", t0+hourMs, t0+2*hourMs),
		ev("Matt", t0+2*hourMs, t0+3*hourMs),
		ev("Matt", t0+3*hourMs, t0+4*hourMs),
		ev("Hits", t0+4*hourMs, t0+5*hourMs),
	}
	if got, ok := onAirSpan(hits.name, day, t0+10); !ok || got != (span{t0, t0 + 2*hourMs}) {
		t.Errorf("the station's own hours = %v %v", got, ok)
	}
	if got, ok := onAirSpan(hits.name, day, t0+3*hourMs); !ok || got != (span{t0 + 2*hourMs, t0 + 4*hourMs}) {
		t.Errorf("a show's two slots = %v %v", got, ok)
	}
	// The station's own hours to the end of the schedule: nothing to count down to.
	if got, ok := onAirSpan(hits.name, day, t0+4*hourMs); ok {
		t.Errorf("the last hours = %v", got)
	}
	if got, ok := onAirSpan(hits.name, day[:2], t0); ok {
		t.Errorf("hours to the end of the schedule = %v", got)
	}
	// A break in the run ends it.
	gap := []services.StationEvent{ev("Hits", t0, t0+hourMs), ev("Hits", t0+2*hourMs, t0+3*hourMs), day[2]}
	if got, ok := onAirSpan(hits.name, gap, t0); !ok || got != (span{t0, t0 + hourMs}) {
		t.Errorf("a break in the run = %v %v", got, ok)
	}
	if got, ok := onAirSpan(hits.name, day, t0-1); ok {
		t.Errorf("nothing on air = %v", got)
	}
}

func TestStationLineNamesTheKindAndOwner(t *testing.T) {
	for _, c := range []struct {
		name string
		s    services.Station
		want string
	}{
		{"live", services.Station{Kind: services.StationLive, Provider: "Apple Music"}, "Live radio · Apple Music"},
		{"broadcast", services.Station{Kind: services.StationBroadcast, Provider: "TuneIn"}, "Live broadcast · TuneIn"},
		{"tracks", services.Station{Kind: services.StationTracks}, "Station"},
		{"episode", services.Station{
			Kind: services.StationEpisode, ShowName: "Zane Lowe", Provider: "Apple Music", DurationMs: 3_420_000,
		}, "Episode · Zane Lowe · 57 min"},
		{"episode without a show", services.Station{Kind: services.StationEpisode, Provider: "Apple Music"}, "Episode · Apple Music"},
	} {
		if got := stationLine(c.s); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestStationLineAddsAnEpisodesAirDate(t *testing.T) {
	air := time.Date(2026, 11, 2, 9, 0, 0, 0, time.Local)
	got := stationLine(services.Station{Kind: services.StationEpisode, ShowName: "Zane Lowe", AirTimeMs: air.UnixMilli(), DurationMs: 3_420_000})
	want := "Episode · Zane Lowe · " + i18n.FormatDate(air, i18n.DateMedium) + " · 57 min"
	if got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	if !strings.Contains(got, "Nov") {
		t.Errorf("the date is not written out: %q", got)
	}
}

func TestStationLineFollowsTheRegionalFormatLocale(t *testing.T) {
	// An English UI on an en-GB system writes "mins" and "hrs".
	i18n.SetLocale("en")
	i18n.SetRegional("en-GB")
	t.Cleanup(func() { i18n.SetRegional(); i18n.SetLocale("en") })
	if got := stationLine(services.Station{Kind: services.StationEpisode, DurationMs: 3_420_000}); got != "Episode · 57 mins" {
		t.Errorf("stationLine = %q", got)
	}
}
