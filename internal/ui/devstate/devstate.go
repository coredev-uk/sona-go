// Package devstate is what development builds (-tags dev) add to the native
// UI so that a restart from cmd/devloop lands where the last run stopped,
// and so that edits to design tokens and catalogues show without one
// (docs/ui-architecture.md, "The development loop").
//
//   - State: the route, the scroll offset of each page, the flags pages
//     register (an open overlay, an aside) and the window's place, written
//     when the app quits and read when the next one starts
//     ($XDG_STATE_HOME/sona/dev-restore.json; SONA_DEV_INSTANCE=<name> makes
//     it sona-<name>/…, so two worktrees keep their own).
//   - Live: tokens.json and the locales' .po files are read from the source
//     tree and reloaded as they change.
//
// In a production build every function here does nothing and compiles away.
// Pages call Scroll and Flag; the shell (internal/ui/app/restore_dev.go)
// calls the rest.
package devstate

// State is what a development run leaves for the next one.
type State struct {
	// Route is the router's location, path and query.
	Route string `json:"route"`
	// Window is the main window's place.
	Window *Window `json:"window,omitempty"`
	// Scroll is each page's scroll offset in DIPs, by location.
	Scroll map[string]float32 `json:"scroll,omitempty"`
	// Flags are the registered flags by name.
	Flags map[string]bool `json:"flags,omitempty"`
}

// Window is a window's bounds in DIPs and whether it was maximised.
type Window struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	Width     int  `json:"width"`
	Height    int  `json:"height"`
	Maximised bool `json:"maximised,omitempty"`
}
