package concerts

import (
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

func show(t *testing.T, iso, zone, id string) services.Concert {
	t.Helper()
	at, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t.Fatal(err)
	}
	return services.Concert{ID: id, StartsAt: at.UnixMilli(), Timezone: zone}
}

// regional writes dates for en-GB (24-hour clocks, day before month, miles)
// until the test ends.
func regional(t *testing.T, tag string) {
	t.Helper()
	i18n.SetRegional(tag)
	t.Cleanup(func() { i18n.SetRegional() })
}

// computerIn is this computer's zone until the test ends.
func computerIn(t *testing.T, zone string) {
	t.Helper()
	old := here
	here = zoneOf(zone)
	t.Cleanup(func() { here = old })
}

func TestPresetRange(t *testing.T) {
	wed := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC) // Wednesday
	day := func(m time.Month, d int) time.Time { return time.Date(2026, m, d, 0, 0, 0, 0, time.UTC) }
	for _, tc := range []struct {
		name string
		when string
		now  time.Time
		want dayRange
		ok   bool
	}{
		{"any time is nothing", "", wed, dayRange{}, false},
		{"week runs to Sunday", whenWeek, wed, dayRange{"2026-11-04", "2026-11-08"}, true},
		{"on Sunday the week is today", whenWeek, day(11, 8), dayRange{"2026-11-08", "2026-11-08"}, true},
		{"weekend is Friday to Sunday", whenWeekend, wed, dayRange{"2026-11-06", "2026-11-08"}, true},
		{"what is left on Saturday", whenWeekend, day(11, 7), dayRange{"2026-11-07", "2026-11-08"}, true},
		{"what is left on Sunday", whenWeekend, day(11, 8), dayRange{"2026-11-08", "2026-11-08"}, true},
		{"from today on Friday", whenWeekend, day(11, 6), dayRange{"2026-11-06", "2026-11-08"}, true},
		{"month is thirty days", whenMonth, wed, dayRange{"2026-11-04", "2026-12-03"}, true},
		{"month across a year end", whenMonth, day(12, 20), dayRange{"2026-12-20", "2027-01-18"}, true},
	} {
		got, ok := presetRange(tc.when, tc.now)
		if ok != tc.ok || got != tc.want {
			t.Errorf("%s: presetRange(%q) = %+v, %v; want %+v, %v", tc.name, tc.when, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRangeFor(t *testing.T) {
	now := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	got, ok := rangeFor(params{when: whenDates, from: "2026-12-01", to: "2026-12-24"}, now)
	if !ok || got != (dayRange{"2026-12-01", "2026-12-24"}) {
		t.Errorf("typed dates = %+v, %v", got, ok)
	}
	if _, ok := rangeFor(params{when: whenDates, from: "2026-12-01"}, now); ok {
		t.Error("one date made a range")
	}
	if _, ok := rangeFor(params{when: whenDates, from: "2026-12-24", to: "2026-12-01"}, now); ok {
		t.Error("dates out of order made a range")
	}
	// Typed dates are ignored for a preset.
	if got, _ := rangeFor(params{when: whenMonth, from: "2020-01-01", to: "2020-01-02"}, now); got.from != "2026-11-04" {
		t.Errorf("preset with typed dates from %q", got.from)
	}
	if _, ok := rangeFor(params{}, now); ok {
		t.Error("any date has a range")
	}
}

func TestShowReadsTheVenuesCalendarDay(t *testing.T) {
	regional(t, "en-GB")
	// 23:30 in New York on the 14th is already the 15th in UTC.
	late := show(t, "2027-04-15T03:30:00Z", "America/New_York", "late")
	if got := showDay(late); got != "2027-04-14" {
		t.Errorf("showDay = %q", got)
	}
	if got := showDate(late); got != "14" {
		t.Errorf("showDate = %q", got)
	}
	// 00:30 on the 15th in Tokyo is still the 14th in UTC.
	early := show(t, "2027-04-14T15:30:00Z", "Asia/Tokyo", "early")
	if got := showDay(early); got != "2027-04-15" {
		t.Errorf("showDay = %q", got)
	}
	if got := showWeekday(early); got != "Thu" {
		t.Errorf("showWeekday = %q", got)
	}
	london := show(t, "2027-04-14T17:00:00Z", "Europe/London", "london")
	for got, want := range map[string]string{
		showMonth(london):      "April 2027",
		showMonthShort(london): "Apr",
		showDayLong(london):    "Wed 14 Apr",
	} {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func TestShowTimeIsTheVenuesClock(t *testing.T) {
	regional(t, "en-GB")
	// London in April is on summer time, one hour ahead of UTC.
	london := show(t, "2027-04-14T17:00:00Z", "Europe/London", "london")
	computerIn(t, "Europe/London")
	if got := showTime(london); got != "18:00" {
		t.Errorf("same clock: showTime = %q", got)
	}
	// A computer on another clock reads the venue's, with its zone's name.
	computerIn(t, "Pacific/Auckland")
	if got := showTime(london); got != "18:00 BST" {
		t.Errorf("another clock: showTime = %q", got)
	}
	computerIn(t, "UTC")
	ny := show(t, "2027-04-14T22:00:00Z", "America/New_York", "ny")
	if got := showTime(ny); got != "18:00 EDT" {
		t.Errorf("showTime = %q", got)
	}
}

func TestGroupShowsByVenueMonthAndDay(t *testing.T) {
	regional(t, "en-GB")
	groups := groupShows([]services.Concert{
		show(t, "2027-04-29T18:00:00Z", "Europe/London", "a"),
		show(t, "2027-04-29T20:00:00Z", "Europe/London", "b"),
		// 23:30 on 30 April in London is 1 May in Tokyo, but the venue decides.
		show(t, "2027-04-30T22:30:00Z", "Europe/London", "c"),
		show(t, "2027-05-01T19:00:00Z", "Europe/London", "d"),
	})
	var titles []string
	for _, m := range groups {
		titles = append(titles, m.title)
	}
	if !slices.Equal(titles, []string{"April 2027", "May 2027"}) {
		t.Fatalf("months %q", titles)
	}
	type dayIDs struct {
		day string
		ids []string
	}
	var got []dayIDs
	for _, d := range groups[0].days {
		g := dayIDs{day: d.day}
		for _, c := range d.shows {
			g.ids = append(g.ids, c.ID)
		}
		got = append(got, g)
	}
	want := []dayIDs{{"2027-04-29", []string{"a", "b"}}, {"2027-04-30", []string{"c"}}}
	if len(got) != 2 || got[0].day != want[0].day || !slices.Equal(got[0].ids, want[0].ids) || got[1].day != want[1].day || !slices.Equal(got[1].ids, want[1].ids) {
		t.Errorf("days %+v, want %+v", got, want)
	}
	if d := groups[1].days; len(d) != 1 || len(d[0].shows) != 1 || d[0].shows[0].ID != "d" {
		t.Errorf("May %+v", d)
	}
	if len(groupShows(nil)) != 0 {
		t.Error("no shows made groups")
	}
}

func TestDistanceLabelSaysKilometresOrMiles(t *testing.T) {
	regional(t, "en-GB")
	if got := distanceLabel(16); got != "10\u00a0mi away" {
		t.Errorf("en-GB: %q", got)
	}
	// A distance under a mile still says one.
	if got := distanceLabel(1); got != "1\u00a0mi away" {
		t.Errorf("en-GB, 1 km: %q", got)
	}
	i18n.SetLocale("de")
	t.Cleanup(func() { i18n.SetLocale("en") })
	if got := distanceLabel(16); got != "16\u00a0km away" {
		t.Errorf("de: %q", got)
	}
	if got := distanceLabel(0); got != "" {
		t.Errorf("unknown distance: %q", got)
	}
}

func TestCountryNameIsInTheInterfaceLanguage(t *testing.T) {
	if got := countryName("gb"); got != "United Kingdom" {
		t.Errorf("en: %q", got)
	}
	i18n.SetLocale("de")
	t.Cleanup(func() { i18n.SetLocale("en") })
	if got := countryName("GB"); got != "Vereinigtes Königreich" {
		t.Errorf("de: %q", got)
	}
	// What the system lacks is the code.
	if got := countryName("zzzz"); got != "ZZZZ" {
		t.Errorf("unknown: %q", got)
	}
}

func TestVenueLine(t *testing.T) {
	c := services.Concert{Venue: &services.ConcertVenue{Name: "Example Hall", City: "London"}}
	if got := venueLine(c); got != "Example Hall · London" {
		t.Errorf("venueLine = %q", got)
	}
	c.Venue.City = ""
	if got := venueLine(c); got != "Example Hall" {
		t.Errorf("without a city: %q", got)
	}
	if got := venueLine(services.Concert{}); got != "" {
		t.Errorf("no venue: %q", got)
	}
}
