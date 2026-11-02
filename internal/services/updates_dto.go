package services

import "github.com/wailsapp/wails/v3/pkg/application"

// EventUpdateAvailable carries the UpdateStatus when the start-up check
// finds a newer release.
const EventUpdateAvailable = "updates:available"

func init() { application.RegisterEvent[UpdateStatus](EventUpdateAvailable) }

// UpdateStatus is the update check's latest result.
type UpdateStatus struct {
	// Current is this build's version.
	Current string `json:"current"`
	// Checkable is false for builds without a release version ("dev"):
	// they are never checked.
	Checkable bool `json:"checkable"`
	// Latest is the newest published release ("" when none is published
	// or no check has succeeded).
	Latest    string `json:"latest"`
	Available bool   `json:"available"` // Latest is newer than Current
	// URL is the release page on GitHub ("" until a release is known).
	URL       string `json:"url"`
	Published int64  `json:"published"` // Latest's publication, Unix ms
	Checked   int64  `json:"checked"`   // last successful check, Unix ms; 0 never
	// Error is why the last check failed ("" when it succeeded).
	Error ErrorCode `json:"error"`
	// AppImage: this process runs from an AppImage, which AppImageUpdate
	// can update in place; packages update through the package manager.
	AppImage bool `json:"appImage"`
}
