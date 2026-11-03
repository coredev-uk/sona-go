package services

// Features declare their events (name constants and payload DTOs) in their
// _dto.go file. Core-level events live here.

// EventAppOpen carries an OpenTarget for every Apple Music link Sona is
// launched with.
const EventAppOpen = "app:open"

// Emitter delivers events to the UI; the app's event bus implements it.
type Emitter interface {
	Emit(name string, data ...any) bool
}
