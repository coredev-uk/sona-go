package services

// Events of the add-ons service.
const (
	// EventAddonsChanged carries the []AddonInfo (as List returns it) each
	// time the list of add-ons, or the state of one, changes.
	EventAddonsChanged = "addons:changed"
	// EventAddonsDevAsk carries an AddonDevAsk: `sona addons dev` waits for the
	// person to confirm in the main window (AddonsService.AnswerDev).
	EventAddonsDevAsk = "addons:dev-ask"
	// EventAddonsDevAskEnd carries the token (a string) of a question that is
	// over, answered or not: the window closes it.
	EventAddonsDevAskEnd = "addons:dev-ask-end"
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
	// Never one of Sona's own add-ons: they are started again for as long as
	// Sona runs (Restarting, with a Reason).
	AddonCrashed AddonState = "crashed"
	// AddonInvalid: its folder or manifest is refused (Problem says why).
	AddonInvalid AddonState = "invalid"
	// AddonIdle: one of Sona's own add-ons that is on and has nothing to do
	// yet (no account connected, the status off): it starts when it has, so an
	// integration nobody uses costs no process.
	AddonIdle AddonState = "idle"
	// AddonNoSandbox: the OS sandbox can't be set up on this computer, so the
	// add-on does not run, however it is switched (Sandbox says why). Only
	// Sona's own add-ons run without the sandbox (Unsandboxed); no other.
	AddonNoSandbox AddonState = "no_sandbox"
)

// AddonReason is why an add-on that should be running isn't
// (AddonInfo.Reason), a code the UI words.
type AddonReason string

const (
	// AddonReasonStartFailed: Sona couldn't start its program (the file is
	// gone or can't be run, or its data folder couldn't be made).
	AddonReasonStartFailed AddonReason = "start_failed"
	// AddonReasonEndedAtStart: the program ended before it said hello: it
	// couldn't load (a library the system or its environment doesn't give
	// it), or it crashed at once.
	AddonReasonEndedAtStart AddonReason = "ended_at_start"
	// AddonReasonNoHello: the program runs but didn't say hello in time, and
	// was killed.
	AddonReasonNoHello AddonReason = "no_hello"
	// AddonReasonStopped: it said hello and ran, then stopped on its own.
	AddonReasonStopped AddonReason = "stopped"
)

// AddonInfo is one add-on of the user's add-ons folder.
type AddonInfo struct {
	// ID is the add-on's ID; for a refused add-on whose manifest has none,
	// the folder's name.
	ID          string `json:"id"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	// Runtime is how it runs: "native", a process of its own, or "wasm", a
	// WebAssembly module inside Sona with no files or network of its own.
	Runtime string `json:"runtime"`
	// Permissions are what the manifest asks for; Granted what the user
	// approved. The OS sandbox enforces `network`; the protocol checks the
	// rest at every call.
	Permissions []string   `json:"permissions"`
	Granted     []string   `json:"granted"`
	Enabled     bool       `json:"enabled"`
	State       AddonState `json:"state"`
	// Problem is why an invalid add-on is refused: one of the
	// addons.Problem* codes ("bad_id", "unsafe_owner", …); "" otherwise.
	Problem string `json:"problem"`
	// Sandbox is why the sandbox can't be set up when State is
	// AddonNoSandbox: one of the addons.Sandbox* codes ("userns_restricted",
	// …); "" otherwise.
	Sandbox string `json:"sandbox"`
	// Unsandboxed is why one of Sona's own add-ons runs as a plain process
	// with Sona's rights while State is starting, running or restarting: the
	// sandbox couldn't be set up, and it is one of the addons.Sandbox* codes;
	// "" when it runs inside the sandbox.
	Unsandboxed string `json:"unsandboxed"`
	// Restarts counts the times it stopped on its own since it was enabled.
	Restarts int `json:"restarts"`
	// Reason is why an add-on that should be running isn't, one of the
	// AddonReason* codes: how its last run ended, or why it never began,
	// while State is starting or restarting (it is tried again, one of
	// Sona's own for as long as Sona runs) or crashed. "" while it runs or
	// before a run has ended. The add-on's log has the details.
	Reason AddonReason `json:"reason"`
	// Bundled is one of Sona's own add-ons (Last.fm, ListenBrainz, Discord):
	// part of Sona, always on (Enabled is true; it runs while its integration
	// is in use, which Settings → Connections switches), never switched off by
	// the crash rule, with nothing to approve and no permission to revoke.
	Bundled bool `json:"bundled"`
}

// AddonResult is an add-on's answer to an invoked contribution. Both fields
// are plain text and may be empty.
type AddonResult struct {
	// Toast is a short message to show.
	Toast string `json:"toast"`
	// URL is a link to offer the user; it opens only after a click.
	URL string `json:"url"`
}

// AddonTarget is what an item action was chosen on. Name is the item's
// name as Sona shows it; ID is its Apple Music catalogue ID, or the library
// ID of a song only in the library.
type AddonTarget struct {
	// Kind is "song", "album", "artist" or "playlist".
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AddonContribution is what one add-on adds to Sona's surfaces, in the
// locale asked for (AddonsService.Contributions). Every text is plain and
// clean of control and bidirectional characters.
type AddonContribution struct {
	// ID and Name are the add-on's; Name is shown beside everything it adds.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Running: the add-on is up. Its actions and commands are offered only
	// then; its settings rows always.
	Running  bool           `json:"running"`
	Actions  []AddonAction  `json:"actions"`
	Commands []AddonCommand `json:"commands"`
	Rows     []AddonRow     `json:"rows"`
}

// AddonAction is an item action. ID is the full action ID
// ("addon.<add-on>.<name>"); Kinds are "song", "album", "artist", "playlist".
type AddonAction struct {
	ID    string   `json:"id"`
	Label string   `json:"label"`
	Kinds []string `json:"kinds"`
	Icon  string   `json:"icon"`
}

// AddonCommand is a palette command.
type AddonCommand struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Keywords string `json:"keywords"`
	Icon     string `json:"icon"`
}

// AddonRow is a settings row. Kind is "toggle", "text", "choice" or
// "secret". Value is the row's current value ("" for a secret: it is never
// sent to the UI; Saved says one is stored).
type AddonRow struct {
	ID          string        `json:"id"`
	Kind        string        `json:"kind"`
	Label       string        `json:"label"`
	Description string        `json:"description"`
	Value       string        `json:"value"`
	Saved       bool          `json:"saved"`
	Options     []AddonOption `json:"options"`
}

// AddonOption is one choice of a choice row.
type AddonOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// AddonDevAsk is the question of EventAddonsDevAsk: run the add-on in Dir for
// this session, with these permissions? Every text is plain and clean.
type AddonDevAsk struct {
	// Token names the question; AnswerDev takes it back.
	Token       string   `json:"token"`
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Dir         string   `json:"dir"`
	Permissions []string `json:"permissions"`
}
