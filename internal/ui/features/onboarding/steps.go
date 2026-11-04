package onboarding

import (
	"slices"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// setupSteps are the setup's steps, in order (steps.ts' setupSteps). The
// open step itself lives in store.Surfaces, outside the layer, so it
// survives a language change that rebuilds the whole tree.
var setupSteps = []string{
	store.SetupWelcome,
	store.SetupAccount,
	store.SetupPlayback,
	store.SetupConnections,
	store.SetupAppearance,
	store.SetupDesktop,
	store.SetupDone,
}

// stepIndex is the position of step in setupSteps; an unknown name is the
// welcome (setupSearch's catch).
func stepIndex(step string) int {
	return max(slices.Index(setupSteps, step), 0)
}

// stepAfter is the step after step; the last one stays where it is.
func stepAfter(step string) string {
	return setupSteps[min(stepIndex(step)+1, len(setupSteps)-1)]
}

// stepBefore is the step before step; the first one stays where it is.
func stepBefore(step string) string {
	return setupSteps[max(stepIndex(step)-1, 0)]
}

// parseStep reads a /welcome?step= value: a missing or unknown step starts
// at the beginning.
func parseStep(s string) string {
	if slices.Contains(setupSteps, s) {
		return s
	}
	return store.SetupWelcome
}

// stepName is each step's name, as the strip and the step titles say it.
func stepName(step string) string {
	switch step {
	case store.SetupAccount:
		return i18n.T("Apple Music")
	case store.SetupPlayback:
		return i18n.T("Playback")
	case store.SetupConnections:
		return i18n.T("Connections")
	case store.SetupAppearance:
		return i18n.T("Appearance")
	case store.SetupDesktop:
		return i18n.T("Desktop")
	case store.SetupDone:
		return i18n.T("Done")
	}
	return i18n.T("Welcome")
}

// widevineState is what the setup can say about Widevine, from the platform's
// capabilities and Go's CDM status:
//   - viewLoading: capabilities or the CDM status haven't come yet;
//   - viewUnavailable: this build has no playback engine here;
//   - viewNotNeeded: the engine plays without a CDM;
//   - viewUnsupported: the engine needs one but Google makes none for this
//     computer (Linux on arm64);
//   - viewWaiting: the download starts once the window has painted;
//   - viewDownloading, viewReady, viewFailed: the download's own states.
type widevineState int

const (
	viewLoading widevineState = iota
	viewUnavailable
	viewNotNeeded
	viewUnsupported
	viewWaiting
	viewDownloading
	viewReady
	viewFailed
)

// widevineView reads Widevine from the capabilities first, then the
// download (steps.ts' widevineView). nil is "hasn't come yet".
func widevineView(caps *services.Capabilities, status *services.CDMStatus) widevineState {
	if caps == nil {
		return viewLoading
	}
	if !caps.Playback.Available {
		return viewUnavailable
	}
	if caps.Playback.Engine != "cdm" {
		return viewNotNeeded
	}
	if status == nil {
		return viewLoading
	}
	if status.Code == "unsupported_platform" {
		return viewUnsupported
	}
	switch status.State {
	case "downloading":
		return viewDownloading
	case "ready":
		return viewReady
	case "failed":
		return viewFailed
	}
	return viewWaiting
}
