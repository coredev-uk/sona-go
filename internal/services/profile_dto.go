package services

// EventProfileChanged carries GreetingName when the stored profile name
// changes: after Apple was asked, and at sign-out (empty).
const EventProfileChanged = "profile:changed"

// GreetingName is what the For You greeting calls the person: the part of
// the name on their Apple Music profile that the rule in docs/design.md
// ("The greeting") picks, from SQLite. Name is "" when there is nothing to
// say: signed out, no profile, a profile not set up, or Apple not asked yet.
// The handle is never part of it.
type GreetingName struct {
	Name string `json:"name"`
}
