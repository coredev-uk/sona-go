// Package store is the UI's data layer: the event bus the Go services emit
// into, cached queries over the typed services, and the playback mirror
// (docs/ui-architecture.md, "Store, queries and events").
//
// Everything here is safe to call from any goroutine unless noted, and every
// callback a UI author passes runs on the main thread, through the Poster
// the app supplies (MyGo's Window.Update fanned out to every window), so a
// result that arrives off the main thread also redraws the windows.
package store

// Poster runs fn on the main thread and redraws every window. app supplies
// it (mygo.Window.Update fan-out); tests supply a synchronous one.
type Poster func(fn func())

// runNow is the Poster that runs fn at once, for a nil one.
func runNow(fn func()) { fn() }
