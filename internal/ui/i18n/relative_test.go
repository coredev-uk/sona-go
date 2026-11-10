package i18n

import (
	"strings"
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) string { return RelativeTime(now.Add(-d), now) }
	for _, c := range []struct {
		name string
		got  string
		want string
	}{
		{"zero is never", RelativeTime(time.Time{}, now), "never"},
		{"epoch is never", RelativeTime(time.UnixMilli(0), now), "never"},
		{"seconds", ago(10 * time.Second), "just now"},
		{"44 s", ago(44 * time.Second), "just now"},
		{"future", RelativeTime(now.Add(time.Minute), now), "just now"},
		{"45 s", ago(45 * time.Second), "1 min. ago"},
		{"3 min", ago(3 * time.Minute), "3 min. ago"},
		{"59.5 min rounds to an hour", ago(59*time.Minute + 30*time.Second), "1 hr. ago"},
		{"2 h", ago(2 * time.Hour), "2 hr. ago"},
		{"23 h", ago(23 * time.Hour), "23 hr. ago"},
		{"24 h", ago(24 * time.Hour), "yesterday"},
		{"3 d", ago(72 * time.Hour), "3 days ago"},
	} {
		if c.got != c.want {
			t.Errorf("%s: %q, want %q", c.name, c.got, c.want)
		}
	}
}

func TestRelativeTimeRoundsLikeV020(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{
		{89 * time.Second, "1 min. ago"},
		{90 * time.Second, "2 min. ago"},
		{59 * time.Minute, "59 min. ago"},
		{89 * time.Minute, "1 hr. ago"},
		{36 * time.Hour, "2 days ago"},
	} {
		if got := RelativeTime(now.Add(-c.ago), now); got != c.want {
			t.Errorf("%v ago = %q, want %q", c.ago, got, c.want)
		}
	}
}

func TestRelativeDayBuckets(t *testing.T) {
	reset(t)
	// Every bucket in UTC and in zones either side of it: the dates are read
	// in loc, so the labels are the same.
	for _, loc := range []*time.Location{time.UTC, time.FixedZone("UTC+13", 13*3600), time.FixedZone("UTC-8", -8*3600)} {
		now := time.Date(2026, 11, 10, 14, 30, 0, 0, loc) // Tuesday 10 November 2026
		at := func(m time.Month, d, h, min int) time.Time { return time.Date(2026, m, d, h, min, 0, 0, loc) }
		for _, c := range []struct {
			name string
			t    time.Time
			want string
		}{
			{"zero", time.Time{}, "never"},
			{"epoch", time.UnixMilli(0), "never"},
			{"future, today", now.Add(time.Hour), "just now"},
			{"future, another day", now.AddDate(0, 0, 3), "just now"},
			{"seconds", now.Add(-10 * time.Second), "just now"},
			{"minutes", now.Add(-3 * time.Minute), "3 min. ago"},
			{"hours", now.Add(-2 * time.Hour), "2 hr. ago"},
			{"this morning", at(11, 10, 0, 5), "14 hr. ago"},
			{"late yesterday, under a day", at(11, 9, 23, 59), "yesterday"},
			{"early yesterday", at(11, 9, 0, 0), "yesterday"},
			{"two days", at(11, 8, 18, 0), "on Sunday"},
			{"six days", at(11, 4, 9, 0), "on Wednesday"},
			{"seven days", at(11, 3, 9, 0), "last week"},
			{"13 days, last month", at(10, 28, 9, 0), "last week"},
			{"14 days", at(10, 27, 9, 0), "in October"},
			{"start of the year", at(1, 1, 0, 0), "in January"},
			{"last day of last year", time.Date(2025, 12, 31, 23, 0, 0, 0, loc), "last year"},
			{"start of last year", time.Date(2025, 1, 1, 0, 0, 0, 0, loc), "last year"},
			{"two years", time.Date(2024, 12, 31, 23, 0, 0, 0, loc), "in 2024"},
			{"long ago", time.Date(2003, 6, 1, 0, 0, 0, 0, loc), "in 2003"},
		} {
			if got := RelativeDay(c.t, now, loc); got != c.want {
				t.Errorf("%s, %s: %q, want %q", loc, c.name, got, c.want)
			}
		}
	}
}

func TestRelativeDayMonthAndYearBoundaries(t *testing.T) {
	reset(t)
	loc := time.UTC
	for _, c := range []struct {
		name   string
		t, now time.Time
		want   string
	}{
		{"across a month", time.Date(2026, 11, 30, 23, 0, 0, 0, loc), time.Date(2026, 12, 1, 0, 30, 0, 0, loc), "yesterday"},
		{"across a year", time.Date(2026, 12, 31, 22, 0, 0, 0, loc), time.Date(2027, 1, 1, 9, 0, 0, 0, loc), "yesterday"},
		{"a week across a year", time.Date(2026, 12, 25, 9, 0, 0, 0, loc), time.Date(2027, 1, 1, 9, 0, 0, 0, loc), "last week"},
		{"two weeks across a year", time.Date(2026, 12, 18, 9, 0, 0, 0, loc), time.Date(2027, 1, 1, 9, 0, 0, 0, loc), "last year"},
		{"earlier this month", time.Date(2026, 11, 1, 9, 0, 0, 0, loc), time.Date(2026, 11, 28, 9, 0, 0, 0, loc), "earlier this month"},
		{"the month before", time.Date(2026, 10, 31, 9, 0, 0, 0, loc), time.Date(2026, 11, 28, 9, 0, 0, 0, loc), "in October"},
		{"last year in January", time.Date(2026, 12, 1, 9, 0, 0, 0, loc), time.Date(2027, 1, 20, 9, 0, 0, 0, loc), "last year"},
	} {
		if got := RelativeDay(c.t, c.now, loc); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestRelativeDayReadsTheDayInItsZone(t *testing.T) {
	reset(t)
	// 23:30 and 00:30 UTC: a day apart in UTC, the same afternoon in
	// UTC-8 and the same morning in UTC+13.
	ts := time.Date(2026, 11, 9, 23, 30, 0, 0, time.UTC)
	now := time.Date(2026, 11, 10, 0, 30, 0, 0, time.UTC)
	for _, c := range []struct {
		loc  *time.Location
		want string
	}{
		{time.UTC, "yesterday"},
		{time.FixedZone("UTC-8", -8*3600), "1 hr. ago"},
		{time.FixedZone("UTC+13", 13*3600), "1 hr. ago"},
	} {
		if got := RelativeDay(ts, now, c.loc); got != c.want {
			t.Errorf("%s: %q, want %q", c.loc, got, c.want)
		}
	}
}

func TestRelativeDayAcrossADaylightSavingChange(t *testing.T) {
	reset(t)
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tz database:", err)
	}
	// The clocks went back on Sunday 25 October 2026: a 25-hour day.
	for _, c := range []struct {
		name   string
		t, now time.Time
		want   string
	}{
		{"the long day before", time.Date(2026, 10, 25, 0, 0, 0, 0, loc), time.Date(2026, 10, 26, 9, 0, 0, 0, loc), "yesterday"},
		{"the day before it", time.Date(2026, 10, 24, 23, 0, 0, 0, loc), time.Date(2026, 10, 26, 0, 30, 0, 0, loc), "on Saturday"},
		{"24 hours into the long day", time.Date(2026, 10, 25, 0, 10, 0, 0, loc), time.Date(2026, 10, 25, 23, 40, 0, 0, loc), "24 hr. ago"},
		{"the short day in March", time.Date(2026, 3, 28, 23, 0, 0, 0, loc), time.Date(2026, 3, 29, 23, 30, 0, 0, loc), "yesterday"},
	} {
		if got := RelativeDay(c.t, c.now, loc); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCalendarDaysAndNextMidnight(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("no tz database:", err)
	}
	now := time.Date(2026, 10, 26, 9, 0, 0, 0, loc)
	for _, c := range []struct {
		t    time.Time
		want int
	}{
		{now, 0},
		{time.Date(2026, 10, 25, 23, 59, 0, 0, loc), 1},
		{time.Date(2026, 10, 25, 0, 0, 0, 0, loc), 1},
		{time.Date(2026, 3, 29, 12, 0, 0, 0, loc), 211},
		{time.Date(2026, 10, 27, 0, 0, 0, 0, loc), -1},
	} {
		if got := CalendarDays(c.t, now, loc); got != c.want {
			t.Errorf("CalendarDays(%v) = %d, want %d", c.t, got, c.want)
		}
	}
	// Saturday evening before the clocks go back: the next midnight is
	// Sunday's, and Sunday lasts 25 hours to Monday's.
	sat := time.Date(2026, 10, 24, 22, 0, 0, 0, loc)
	if got, want := NextMidnight(sat, loc), time.Date(2026, 10, 25, 0, 0, 0, 0, loc); !got.Equal(want) {
		t.Errorf("NextMidnight(%v) = %v, want %v", sat, got, want)
	}
	if got := NextMidnight(time.Date(2026, 10, 25, 12, 0, 0, 0, loc), loc).Sub(time.Date(2026, 10, 25, 0, 0, 0, 0, loc)); got != 25*time.Hour {
		t.Errorf("the long day lasts %v, want 25h", got)
	}
	if got := NextMidnight(time.Date(2026, 12, 31, 23, 59, 0, 0, time.UTC), time.UTC); !got.Equal(time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("NextMidnight at the year's end = %v", got)
	}
	// West of UTC, Chile's clocks skip from 23:59:59 on Saturday to 01:00 on
	// Sunday: the next day starts at 01:00, not an hour before the jump.
	if scl, err := time.LoadLocation("America/Santiago"); err == nil {
		got := NextMidnight(time.Date(2026, 9, 5, 22, 0, 0, 0, scl), scl)
		if want := time.Date(2026, 9, 6, 1, 0, 0, 0, scl); !got.Equal(want) {
			t.Errorf("NextMidnight across Santiago's skipped midnight = %v, want %v", got, want)
		}
	}
}

func TestFormatDateTimeWritesTheDayAndTheClock(t *testing.T) {
	reset(t)
	got := FormatDateTime(time.Date(2026, 11, 10, 14, 30, 0, 0, time.UTC))
	if !strings.Contains(got, "Tuesday, November 10, 2026") || !strings.Contains(got, "2:30") {
		t.Errorf("FormatDateTime = %q", got)
	}
}
