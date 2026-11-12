package i18n

import (
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	mygoi18n "github.com/egoist/mygo/i18n"
)

// FormatPercent writes a fraction as a whole percentage in the shown
// language: 0.42 is "42%" (v0.2.0's numberFormat({style: "percent"})).
func FormatPercent(fraction float64) string {
	return mygoi18n.FormatNumber(current.Load().format, fraction, mygoi18n.NumberOptions{Percent: true})
}

// CDMPercent is a running CDM download's progress as a percentage in the
// shown language ("42%"), "" while its size is unknown.
func CDMPercent(s services.CDMStatus) string {
	f, ok := store.CDMProgress(s)
	if !ok {
		return ""
	}
	return FormatPercent(f)
}

// CDMProblem words why there is no Widevine CDM, or why its last update check
// failed: every failure code of internal/playback/cdm/errors.go
// (CDMStatus.Code), in plain words for people who have never heard of a CDM:
// what happened, then what to do, without blame. A code Sona doesn't know yet
// gets a general sentence (v0.2.0's features/cdm/messages.ts).
func CDMProblem(code string) string {
	switch code {
	case "network":
		return T("Sona couldn't reach Google's download server. Check your internet connection.")
	case "unsupported_platform":
		return T("Google doesn't make Widevine for this computer, so songs can't play here.")
	case "manifest_invalid":
		return T("Sona couldn't tell which version of Widevine to download. Try again later.")
	case "untrusted_url":
		return T("The download wasn't coming from Google, so Sona stopped it. Try again later.")
	case "too_large":
		return T("The download was larger than expected, so Sona stopped it. Try again later.")
	case "hash_mismatch":
		return T("The downloaded file wasn't the one expected, so Sona deleted it. Try again.")
	case "signature_invalid":
		return T("The download isn't signed by Google, so Sona deleted it. Try again later.")
	case "archive_invalid":
		return T("The download was damaged or incomplete, so Sona deleted it. Try again.")
	case "load_failed":
		return T("The downloaded version of Widevine didn't start on this computer, so Sona removed it.")
	case "load_check_incomplete":
		return T("Sona couldn't finish checking the downloaded Widevine. Try again.")
	case "disk":
		return T("Sona couldn't save Widevine on this computer. Make sure there's free disk space, then try again.")
	}
	return T("Widevine couldn't be downloaded. Try again later.")
}

// CDMFailedState is the short state of a CDM that failed (CDMStatus.State
// "failed"): its install, when the download arrived and only loading it failed
// or could not be finished; otherwise the download.
func CDMFailedState(code string) string {
	if code == "load_failed" || code == "load_check_incomplete" {
		return T("Couldn't install")
	}
	return T("Couldn't download")
}
