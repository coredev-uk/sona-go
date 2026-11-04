package i18n

import (
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
