package store

import (
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
)

// The settings and the platform's capabilities, shared by every feature
// that shows or saves one (v0.2.0's useSettings and useCapabilities).
var (
	KeySettings     = Key{"settings"}
	KeyCapabilities = Key{"capabilities"}
)

// UseSettings is SystemService.Settings, never stale: a save (kit.UpdateSettings)
// and "settings:changed" write the new settings into it.
func UseSettings(cl *Client, core *services.Core) Result[config.Settings] {
	return useCore(cl, core, KeySettings, func(s *services.SystemService) config.Settings { return s.Settings() })
}

// UseCapabilities is what this platform supports (PlatformService.Capabilities);
// it is fixed for the process.
func UseCapabilities(cl *Client, core *services.Core) Result[services.Capabilities] {
	return useCore(cl, core, KeyCapabilities, func(s *services.PlatformService) services.Capabilities { return s.Capabilities() },
		Options{Stale: Forever, Keep: Forever})
}
