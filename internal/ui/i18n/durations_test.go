package i18n

import (
	"testing"
	"time"
)

func TestFormatTimeLeft(t *testing.T) {
	reset(t)
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{18*24*time.Hour + 6*time.Hour, "18 days, 6 hours"},
		{5*time.Hour + 12*time.Minute, "5 hours, 12 minutes"},
		{20 * time.Second, "1 minute"},
		{0, "1 minute"},
		{time.Hour, "1 hour"},
		{24*time.Hour + time.Second, "1 day"},
	} {
		if got := FormatTimeLeft(c.d); got != c.want {
			t.Errorf("FormatTimeLeft(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestFormatRunningTime(t *testing.T) {
	reset(t)
	for d, want := range map[time.Duration]string{
		42 * time.Minute:                "42 min",
		72 * time.Minute:                "1 hr, 12 min",
		time.Hour:                       "1 hr",
		2*time.Hour + 5*time.Minute:     "2 hr, 5 min",
		41*time.Minute + 40*time.Second: "42 min",
		0:                               "0 min",
		-time.Minute:                    "0 min",
	} {
		if got := FormatRunningTime(d); got != want {
			t.Errorf("FormatRunningTime(%v) = %q, want %q", d, got, want)
		}
	}
}

// The words follow the regional format locale, not the catalogue's language.
func TestDurationsFollowTheFormatLocale(t *testing.T) {
	reset(t)
	SetRegional("en-GB")
	if got := FormatRunningTime(125 * time.Minute); got != "2 hrs, 5 mins" {
		t.Errorf("en-GB: %q", got)
	}
	// Regional tags apply to their own language's catalogue.
	SetLocale("de")
	SetRegional("de-DE")
	if got := FormatTimeLeft(26 * time.Hour); got != "1 Tag, 2 Stunden" {
		t.Errorf("de-DE: %q", got)
	}
	if got := FormatRunningTime(72 * time.Minute); got != "1 Std., 12 Min." {
		t.Errorf("de short: %q", got)
	}
	SetLocale("ru")
	SetRegional("ru-RU")
	if got := FormatTimeLeft(5*time.Hour + 2*time.Minute); got != "5 часов, 2 минуты" {
		t.Errorf("ru: %q", got)
	}
	SetLocale("ja")
	SetRegional("ja-JP")
	if got := FormatRunningTime(72 * time.Minute); got != "1時間 12分" {
		t.Errorf("ja: %q", got)
	}
	// A language without a row reads in English.
	SetLocale("sw")
	SetRegional("sw-KE")
	if got := FormatRunningTime(72 * time.Minute); got != "1 hr, 12 min" {
		t.Errorf("fallback: %q", got)
	}
}

func TestShortReleaseDate(t *testing.T) {
	reset(t)
	now := time.Date(2026, 11, 1, 14, 0, 0, 0, time.UTC)
	same := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	next := time.Date(2027, 1, 8, 0, 0, 0, 0, time.UTC)
	if got := ShortReleaseDate(same, now); got != "Nov 19" {
		t.Errorf("en this year: %q", got)
	}
	if got := ShortReleaseDate(next, now); got != "Jan 8, 2027" {
		t.Errorf("en next year: %q", got)
	}
	SetRegional("en-GB")
	if got := ShortReleaseDate(same, now); got != "19 Nov" {
		t.Errorf("en-GB: %q", got)
	}
	if got := ShortReleaseDate(next, now); got != "8 Jan 2027" {
		t.Errorf("en-GB next year: %q", got)
	}
}
