package services

import "github.com/wailsapp/wails/v3/pkg/application"

// EventRemindersChanged carries every release reminder ([]Reminder)
// whenever one is set, removed or has fired.
const EventRemindersChanged = "reminders:changed"

func init() { application.RegisterEvent[[]Reminder](EventRemindersChanged) }

// Reminder is a release reminder: a pre-release catalog album that gets a
// desktop notification when it comes out.
type Reminder struct {
	AlbumID   string `json:"albumId"` // catalog album ID
	Name      string `json:"name"`
	Artist    string `json:"artistName"`
	ReleaseAt int64  `json:"releaseAt"` // Unix ms (Album.ReleaseAt)
	Art       Art    `json:"art"`
}
