package services

// EventUpdateAvailable carries the UpdateStatus when the start-up check
// finds a newer release.
const EventUpdateAvailable = "updates:available"

// EventWhatsNew carries the WhatsNew of the version just installed, once its
// notes are fetched after the first start of that version.
const EventWhatsNew = "updates:whatsnew"

// WhatsNew is the release notes of a version: what the What's New dialog
// shows after an update (UpdatesService.WhatsNew, "updates:whatsnew").
type WhatsNew struct {
	// Version is the release's number ("0.8.0"); "" when there is none to show.
	Version string `json:"version"`
	// Published is its publication, Unix ms (0 when unknown).
	Published int64 `json:"published"`
	// Notes are the release notes (Markdown), without the install
	// instructions and the version's own heading. Never empty with a Version.
	Notes string `json:"notes"`
}

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
	// RetryAt, with Error CodeRateLimited, is when GitHub's limit for this
	// network is over and Sona checks again, Unix ms; 0 otherwise. Until then
	// no call to GitHub is made.
	RetryAt int64 `json:"retryAt"`
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
	// Notes are Latest's release notes (Markdown): the signed update
	// manifest's where Sona updates itself, else the signed release.json's
	// (the body of the GitHub release for a release published without it),
	// without the install instructions. "" when the release has none. The UI
	// shows them in a dialog (kit.ReleaseNotes).
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
	// Declined is the version the user went back from with "Restore…"
	// (Settings → Diagnostics): this run is the one they went back to, and
	// the sidebar notice doesn't offer that version again. A check by hand
	// (UpdatesService.Check) clears it; a newer release than Declined is
	// offered as usual. "" when nothing was declined.
	Declined string `json:"declined"`
}

// Notify reports whether the sidebar notice offers Latest: it is newer than
// this build and not the version the user just went back from. Settings →
// General says Latest is available either way.
func (u UpdateStatus) Notify() bool {
	return u.Available && (u.Declined == "" || u.Latest != u.Declined)
}

// PreviousVersion is the version the last update replaced, kept beside the
// install to go back to (UpdatesService.Previous).
type PreviousVersion struct {
	// Available: a previous version is kept and Sona can restore it here:
	// macOS and Windows installs that update themselves, with no update
	// installing and no restore under way. False elsewhere (Linux, a
	// development build, nothing kept).
	Available bool `json:"available"`
	// Version is its number ("" when it can't be read: a snapshot made by a
	// Sona that didn't record it).
	Version string `json:"version"`
}
