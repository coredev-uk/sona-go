package services

// Events of the add-ons service.
const (
	// EventAddonsChanged carries the []AddonInfo (as List returns it) each
	// time the list of add-ons, or the state of one, changes.
	EventAddonsChanged = "addons:changed"
	// EventAddonsNotice carries an AddonNotice: an add-on Sona stopped
	// without being asked.
	EventAddonsNotice = "addons:notice"
)

// AddonState is where an add-on is. It is a string enum the UI words.
type AddonState string

const (
	// AddonOff: not enabled.
	AddonOff AddonState = "off"
	// AddonNeedsApproval: enabled, but its manifest asks for a permission the
	// user hasn't granted (it was added since): it doesn't run until the user
	// approves again.
	AddonNeedsApproval AddonState = "needs_approval"
	// AddonStarting: the process runs and hasn't said hello yet.
	AddonStarting AddonState = "starting"
	AddonRunning  AddonState = "running"
	// AddonRestarting: it stopped on its own and Sona starts it again after a
	// wait.
	AddonRestarting AddonState = "restarting"
	// AddonCrashed: it stopped on its own too often and was switched off.
	AddonCrashed AddonState = "crashed"
	// AddonInvalid: its folder or manifest is refused (Problem says why).
	AddonInvalid AddonState = "invalid"
)

// AddonInfo is one add-on of the user's add-ons folder.
type AddonInfo struct {
	// ID is the add-on's ID; for a refused add-on whose manifest has none,
	// the folder's name.
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	// Permissions are what the manifest asks for; Granted what the user
	// approved. Until the OS sandbox (#304) they are advisory: an add-on runs
	// with the user's rights.
	Permissions []string   `json:"permissions"`
	Granted     []string   `json:"granted"`
	Enabled     bool       `json:"enabled"`
	State       AddonState `json:"state"`
	// Problem is why an invalid add-on is refused: one of the
	// addons.Problem* codes ("bad_id", "unsafe_owner", …); "" otherwise.
	Problem string `json:"problem"`
	// Restarts counts the times it stopped on its own since it was enabled.
	Restarts int `json:"restarts"`
}

// AddonNotice is the payload of EventAddonsNotice.
type AddonNotice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Kind is "crashed": the add-on stopped on its own too many times and was
	// switched off.
	Kind string `json:"kind"`
}

// AddonResult is an add-on's answer to an invoked contribution. Both fields
// are plain text and may be empty.
type AddonResult struct {
	// Toast is a short message to show.
	Toast string `json:"toast"`
	// URL is a link to offer the user; it opens only after a click.
	URL string `json:"url"`
}
