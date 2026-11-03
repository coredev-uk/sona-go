package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
)

// registerEvents registers every event payload type with Wails so the
// generated bindings carry it. Call before application.New.
func registerEvents() {
	application.RegisterEvent[services.AuthStatus](services.EventAuthChanged)
	application.RegisterEvent[services.CDMStatus](services.EventCDMStatus)
	application.RegisterEvent[services.WindowState](services.EventWindowState)
	application.RegisterEvent[string](services.EventNavigate)
	application.RegisterEvent[services.HomePage](services.EventHome)
	application.RegisterEvent[services.OpenTarget](services.EventAppOpen)
	application.RegisterEvent[services.IntegrationsStatus](services.EventIntegrationsStatus)
	application.RegisterEvent[services.SyncProgress](services.EventSyncProgress)
	application.RegisterEvent[services.SyncDone](services.EventSyncDone)
	application.RegisterEvent[services.ItemState](services.EventItemState)
	application.RegisterEvent[services.PowerState](services.EventPower)
	application.RegisterEvent[services.PlaybackState](services.EventPlaybackState)
	application.RegisterEvent[services.QueueState](services.EventPlaybackQueue)
	application.RegisterEvent[services.PlaylistsChanged](services.EventPlaylistsChanged)
	application.RegisterEvent[services.PrivateState](services.EventPrivateChanged)
	application.RegisterEvent[[]services.Reminder](services.EventRemindersChanged)
	application.RegisterEvent[services.ScrobbleStatus](services.EventScrobbleChanged)
	application.RegisterEvent[config.Settings](services.EventSettingsChanged)
	application.RegisterEvent[services.AppleTranslations](services.EventAppleTranslations)
	application.RegisterEvent[services.AppleLanguages](services.EventAppleLanguages)
	application.RegisterEvent[services.UpdateStatus](services.EventUpdateAvailable)
}
