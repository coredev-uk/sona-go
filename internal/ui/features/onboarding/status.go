package onboarding

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
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
			if percent := i18n.CDMPercent(*status); percent != "" {
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
