package services

// EventUpdateAvailable carries the UpdateStatus when the start-up check
// finds a newer release.
const EventUpdateAvailable = "updates:available"

// EventUpdateInstall carries the UpdateStatus while an update installs:
// at the start, on download progress, and when it finishes or fails.
const EventUpdateInstall = "updates:install"

// Why a self-updating platform doesn't update this install itself
// (UpdateStatus.Disabled).
const (
	// UpdatesUnsigned: the build carries no update signing key, so nothing
	// it downloads could be trusted.
	UpdatesUnsigned = "unsigned"
	// UpdatesNotWritable: Sona can't write where it is installed (a
	// read-only volume, a folder that needs administrator rights, or a
	// build run outside an app bundle).
	UpdatesNotWritable = "not_writable"
)

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
	// SelfUpdate: this install can download and install the update itself
	// ("Install and restart"): macOS and Windows, a signed build, a place
	// Sona can write.
	SelfUpdate bool `json:"selfUpdate"`
	// Disabled says why not on macOS and Windows: UpdatesUnsigned or
	// UpdatesNotWritable ("" when SelfUpdate, on Linux, or for a
	// development build). Linux updates through the package manager.
	Disabled string `json:"disabled"`
	// Notes are Latest's release notes (Markdown) when the manifest has
	// them.
	Notes string `json:"notes"`
	// Installing, Downloaded and Total are the install under way: Total is
	// the archive's size in bytes.
	Installing bool  `json:"installing"`
	Downloaded int64 `json:"downloaded"`
	Total      int64 `json:"total"`
	// Installed: the update is in place and Sona is about to restart.
	Installed bool `json:"installed"`
	// InstallError is why the last install failed ("" when none did).
	InstallError ErrorCode `json:"installError"`
}
