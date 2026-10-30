package services

import "github.com/wailsapp/wails/v3/pkg/application"

// Features declare their events (name constants plus an init registering
// the payload type with application.RegisterEvent, so the generated
// bindings carry it) in their _dto.go file. Core-level events live here.

// EventAppOpen carries an OpenTarget for every Apple Music link Sona is
// launched with.
const EventAppOpen = "app:open"

func init() { application.RegisterEvent[OpenTarget](EventAppOpen) }

// Emitter delivers events to the frontend; *application.EventManager
// (app.Event) implements it.
type Emitter interface {
	Emit(name string, data ...any) bool
}
