package library

import (
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

func historyPlay(id string, playedAt int64) services.HistoryEntry {
	return services.HistoryEntry{PlayedAt: playedAt, Song: services.Song{ID: id}}
}

func TestHistoryGroupsFoldARepeatIntoTheLatestPlayAndCountOtherDevices(t *testing.T) {
	h := services.ListeningHistory{
		Apple: true,
		Days: []services.HistoryDay{
			{Day: 0, Entries: []services.HistoryEntry{historyPlay("x", 0)}},
			{Day: 1000, Entries: []services.HistoryEntry{historyPlay("a", 30), historyPlay("b", 0), historyPlay("a", 20), historyPlay("c", 10)}},
			{Day: 500, Entries: []services.HistoryEntry{historyPlay("a", 5)}},
		},
	}
	groups := historyGroups(h)
	var ids [][]string
	var counts [][2]int
	for _, g := range groups {
		var s []string
		for _, song := range g.songs {
			s = append(s, song.ID)
		}
		ids = append(ids, s)
		counts = append(counts, [2]int{g.plays, g.elsewhere})
	}
	if want := [][]string{{"x"}, {"a", "b", "c"}, {"a"}}; !slices.EqualFunc(ids, want, slices.Equal) {
		t.Errorf("songs %v, want %v", ids, want)
	}
	if want := [][2]int{{1, 1}, {4, 1}, {1, 0}}; !slices.Equal(counts, want) {
		t.Errorf("[plays elsewhere] %v, want %v", counts, want)
	}
}

func TestDayLabelNamesRecentDaysAndDatesOlderOnes(t *testing.T) {
	prev := i18n.Locale()
	i18n.SetLocale("en")
	t.Cleanup(func() { i18n.SetLocale(prev) })

	for _, loc := range []*time.Location{time.UTC, time.FixedZone("UTC+13", 13*3600), time.FixedZone("UTC-8", -8*3600)} {
		now := time.Date(2026, 10, 30, 14, 30, 0, 0, loc) // Friday 30 October 2026
		day := func(y int, m time.Month, d int) int64 { return time.Date(y, m, d, 0, 0, 0, 0, loc).UnixMilli() }
		for _, c := range []struct {
			day  int64
			want string
		}{
			{day(2026, 10, 30), "Today"},
			{day(2026, 10, 29), "Yesterday"},
			{day(2026, 10, 26), "Monday"},
			{day(2026, 10, 24), "Saturday"},
			{day(2026, 10, 23), "Friday, October 23"},
			{day(2025, 12, 31), "Wednesday, December 31, 2025"},
			{0, "Recently, on your other devices"},
		} {
			if got := dayLabel(c.day, now, loc); got != c.want {
				t.Errorf("%s: dayLabel = %q, want %q", loc, got, c.want)
			}
		}
	}
}

func TestDayLabelCountsWholeDaysAcrossADaylightSavingChange(t *testing.T) {
	prev := i18n.Locale()
	i18n.SetLocale("en")
	t.Cleanup(func() { i18n.SetLocale(prev) })
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tz database:", err)
	}
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, loc) // Monday, the day after the clocks went back
	day := time.Date(2026, 10, 25, 0, 0, 0, 0, loc).UnixMilli()
	if got := dayLabel(day, now, loc); got != "Yesterday" {
		t.Errorf("the 25-hour day before is %q, want Yesterday", got)
	}
}
