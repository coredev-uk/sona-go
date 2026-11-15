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
		return i18n.T("Top Songs")
	case services.ChartAlbums:
		return i18n.T("Top Albums")
	case services.ChartPlaylists:
		return i18n.T("Top Playlists")
	case services.ChartMusicVideos:
		return i18n.T("Top Music Videos")
	case services.ChartCities:
		return i18n.T("Top 25 Songs by City")
	case services.ChartDailyGlobal:
		return i18n.T("Top 100 Songs by Country")
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
	when := i18n.RelativeTime(time.UnixMilli(fetchedAt), now)
	if stale {
		return i18n.T("Couldn't reach Apple Music · showing the copy from {when}", "when", when)
	}
	return i18n.T("Updated {when}", "when", when)
}
