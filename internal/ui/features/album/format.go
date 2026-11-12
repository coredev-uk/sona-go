package album

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

// This file is the formatting the album page takes from v0.2.0's
// shared/lib/format.ts and release.ts.

// fullReleaseDate is a release day in full: "Thursday 19 November 2026".
func fullReleaseDate(releaseAt int64) string {
	return i18n.FormatDate(time.UnixMilli(releaseAt), i18n.DateFull)
}
