package discover

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// chartTitle is a chart's name in Sona's words, by its key (v0.2.0's
// chart-titles.ts); Apple's title for any other chart.
func chartTitle(c services.Chart) string {
	switch c.Key {
	case services.ChartSongs:
		return i18n.T("Top songs")
	case services.ChartAlbums:
		return i18n.T("Top albums")
	case services.ChartPlaylists:
		return i18n.T("Top playlists")
	case services.ChartMusicVideos:
		return i18n.T("Top music videos")
	case services.ChartCities:
		return i18n.T("Top 25 songs by city")
	case services.ChartDailyGlobal:
		return i18n.T("Top 100 songs by country")
	}
	return c.Title
}

// freshness is "Updated 12 min. ago", or the honest stale line when Apple
// couldn't be reached (v0.2.0's freshness.ts); "" when the page was never
// fetched. fetchedAt is Unix milliseconds.
func freshness(fetchedAt int64, stale bool, now time.Time) string {
	if fetchedAt == 0 {
		return ""
	}
	when := relativeTime(fetchedAt, now)
	if stale {
		return i18n.T("Couldn't reach Apple Music · showing the copy from {when}", "when", when)
	}
	return i18n.T("Updated {when}", "when", when)
}

// relativeTime is v0.2.0's relativeTime: "just now", then minutes, hours and
// days ago in short form; stamp is Unix milliseconds, 0 for never.
func relativeTime(stamp int64, now time.Time) string {
	if stamp <= 0 {
		return i18n.T("never")
	}
	s := int64(now.Sub(time.UnixMilli(stamp)).Round(time.Second) / time.Second)
	if s < 45 {
		return i18n.T("just now")
	}
	m := (s + 30) / 60
	if m < 60 {
		return i18n.T("{minutes, plural, one {# min. ago} other {# min. ago}}", "minutes", m)
	}
	h := (m + 30) / 60
	if h < 24 {
		return i18n.T("{hours, plural, one {# hr. ago} other {# hr. ago}}", "hours", h)
	}
	d := (h + 12) / 24
	if d == 1 {
		return i18n.T("yesterday")
	}
	return i18n.T("{days, plural, one {# day ago} other {# days ago}}", "days", d)
}
