package services

import "github.com/wailsapp/wails/v3/pkg/application"

// EventPrivateChanged carries PrivateState whenever private listening turns
// on or off (Settings, the player bar, the tray, the command palette or
// `sona private`).
const EventPrivateChanged = "private:changed"

func init() { application.RegisterEvent[PrivateState](EventPrivateChanged) }

// PrivateState is private listening as it applies now.
type PrivateState struct {
	On bool `json:"on"`
	// Source is where On comes from: "setting" (the saved setting; toggling
	// saves it), "development" (a development build starts private;
	// toggling changes this session only and never the saved setting) or
	// "environment" (SONA_PRIVATE=1 forces it on; it cannot be turned off).
	Source string `json:"source"`
	// Saved is the saved setting, which differs from On only while
	// Source is not "setting".
	Saved bool `json:"saved"`
}
