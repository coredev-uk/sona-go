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
		0:                               "",
		-time.Minute:                    "",
		29 * time.Second:                "",
	} {
		if got := FormatRunningTime(d); got != want {
			t.Errorf("FormatRunningTime(%v) = %q, want %q", d, got, want)
		}
	}
}

// durationRows are what Intl.DurationFormat (Node 24, CLDR 48) wrote for
// each locale: 1 h 12 min and 42 min short; 18 d 6 h, 21 h 1 min and 1 min
// long. The strings are Intl's, not this table's; the en-CA and fr-CA rows,
// which Node's build may not carry, are CLDR 48.2.0's own (units.json).
var durationRows = []struct {
	tag                                  string
	short72, short42, long18d, long21h1m string
	long1m                               string
}{
	{"en", "1 hr, 12 min", "42 min", "18 days, 6 hours", "21 hours, 1 minute", "1 minute"},
	{"en-US", "1 hr, 12 min", "42 min", "18 days, 6 hours", "21 hours, 1 minute", "1 minute"},
	{"en-GB", "1 hr, 12 mins", "42 mins", "18 days, 6 hours", "21 hours, 1 minute", "1 minute"},
	{"en-AU", "1 hr, 12 mins", "42 mins", "18 days, 6 hours", "21 hours, 1 minute", "1 minute"},
	{"en-IN", "1 hr, 12 mins", "42 mins", "18 days, 6 hours", "21 hours, 1 minute", "1 minute"},
	{"en-PR", "1 hr, 12 min", "42 min", "18 days, 6 hours", "21 hours, 1 minute", "1 minute"},
	{"en-CA", "1 hr, 12 mins", "42 mins", "18 days, 6 hours", "21 hours, 1 minute", "1 minute"},
	{"de", "1 Std., 12 Min.", "42 Min.", "18 Tage, 6 Stunden", "21 Stunden, 1 Minute", "1 Minute"},
	{"fr", "1 h et 12 min", "42 min", "18 jours et 6 heures", "21 heures et 1 minute", "1 minute"},
	// Canadian French: U+00A0 throughout, where France writes U+202F before h.
	{"fr-CA", "1\u00a0h et 12\u00a0min", "42\u00a0min", "18\u00a0jours et 6\u00a0heures", "21\u00a0heures et 1\u00a0minute", "1\u00a0minute"},
	{"es", "1 h y 12 min", "42 min", "18 días y 6 horas", "21 horas y 1 minuto", "1 minuto"},
	{"it", "1 h e 12 min", "42 min", "18 giorni e 6 ore", "21 ore e 1 minuto", "1 minuto"},
	{"pt", "1 h e 12 min", "42 min", "18 dias e 6 horas", "21 horas e 1 minuto", "1 minuto"},
	{"nl", "1 uur, 12 min", "42 min", "18 dagen en 6 uur", "21 uur en 1 minuut", "1 minuut"},
	{"sv", "1 tim, 12 min", "42 min", "18 dygn, 6 timmar", "21 timmar, 1 minut", "1 minut"},
	{"pl", "1 godz. i 12 min", "42 min", "18 dni i 6 godzin", "21 godzin i 1 minuta", "1 minuta"},
	{"ru", "1 ч 12 мин", "42 мин", "18 дней 6 часов", "21 час 1 минута", "1 минута"},
	{"tr", "1 sa. 12 dk.", "42 dk.", "18 gün 6 saat", "21 saat 1 dakika", "1 dakika"},
	{"ja", "1 時間 12 分", "42 分", "18 日 6 時間", "21 時間 1 分", "1 分"},
	{"zh", "1小时12分钟", "42分钟", "18天6小时", "21小时1分钟", "1分钟"},
	{"zh-Hans-CN", "1小时12分钟", "42分钟", "18天6小时", "21小时1分钟", "1分钟"},
	{"zh-TW", "1 小時 12 分鐘", "42 分鐘", "18 天 6 小時", "21 小時 1 分鐘", "1 分鐘"},
	{"zh-HK", "1 小時 12 分鐘", "42 分鐘", "18 日 6 小時", "21 小時 1 分鐘", "1 分鐘"},
	{"zh-Hant-TW", "1 小時 12 分鐘", "42 分鐘", "18 天 6 小時", "21 小時 1 分鐘", "1 分鐘"},
	{"ko", "1시간 12분", "42분", "18일 6시간", "21시간 1분", "1분"},
}

// Words, joins and plurals follow the regional format locale, not the
// catalogue's language; a regional tag applies to its own language's catalogue.
func TestDurationsMatchIntl(t *testing.T) {
	for _, r := range durationRows {
		t.Run(r.tag, func(t *testing.T) {
			reset(t)
			SetLocale(langOf(r.tag))
			SetRegional(r.tag)
			if got := Locales()[0]; got != r.tag {
				t.Fatalf("format locale %q, want %q", got, r.tag)
			}
			for _, c := range []struct {
				name string
				got  string
				want string
			}{
				{"72 min", FormatRunningTime(72 * time.Minute), r.short72},
				{"42 min", FormatRunningTime(42 * time.Minute), r.short42},
				{"18 d 6 h", FormatTimeLeft(18*24*time.Hour + 6*time.Hour), r.long18d},
				{"21 h 1 min", FormatTimeLeft(21*time.Hour + time.Minute), r.long21h1m},
				{"20 s", FormatTimeLeft(20 * time.Second), r.long1m},
			} {
				if c.got != c.want {
					t.Errorf("%s: %q, want %q", c.name, c.got, c.want)
				}
			}
		})
	}
}

// A language with no row reads in English, counted by English's rules
// (Ukrainian counts 21 as "one").
func TestDurationsFallBackToEnglish(t *testing.T) {
	reset(t)
	SetLocale("uk")
	SetRegional("uk-UA")
	if got := FormatTimeLeft(21*time.Hour + 5*time.Minute); got != "21 hours, 5 minutes" {
		t.Errorf("uk: %q", got)
	}
	if got := FormatRunningTime(72 * time.Minute); got != "1 hr, 12 min" {
		t.Errorf("uk short: %q", got)
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
