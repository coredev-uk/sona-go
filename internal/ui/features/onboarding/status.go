package onboarding

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// widevineShort is Widevine's state in a few words, for the welcome and the
// last step (status.ts' widevineShort).
func widevineShort(view widevineState, status *services.CDMStatus) string {
	switch view {
	case viewUnavailable, viewUnsupported:
		return i18n.T("Not available here")
	case viewWaiting:
		return i18n.T("Starting…")
	case viewDownloading:
		if status != nil {
			if percent := cdmPercent(*status); percent != "" {
				return i18n.T("Downloading, {percent}", "percent", percent)
			}
		}
		return i18n.T("Downloading…")
	case viewReady:
		if status != nil && status.Version != "" {
			return i18n.T("Ready, version {version}", "version", status.Version)
		}
		return i18n.T("Ready")
	case viewFailed:
		if status != nil {
			return i18n.CDMFailedState(status.Code)
		}
	}
	return ""
}

// cdmPercent is the running download's progress as a percentage in the shown
// language ("42%"), "" while its size is unknown (features/cdm's cdmPercent).
func cdmPercent(s services.CDMStatus) string {
	fraction, ok := store.CDMProgress(s)
	if !ok {
		return ""
	}
	return i18n.FormatPercent(fraction)
}
