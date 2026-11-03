package services

// EventRemindersChanged carries every release reminder ([]Reminder)
// whenever one is set, removed or has fired.
const EventRemindersChanged = "reminders:changed"

// Reminder is a release reminder: a pre-release catalog album that gets a
// desktop notification when it comes out.
type Reminder struct {
	AlbumID   string `json:"albumId"` // catalog album ID
	Name      string `json:"name"`
	Artist    string `json:"artistName"`
	ReleaseAt int64  `json:"releaseAt"` // Unix ms (Album.ReleaseAt)
	Art       Art    `json:"art"`
}
