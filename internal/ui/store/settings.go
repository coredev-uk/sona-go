package store

import (
	"context"

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
	return Use(cl, KeySettings, func(context.Context) (config.Settings, error) {
		if core == nil {
			return config.Settings{}, errNoCore
		}
		return services.Service[services.SystemService](core).Settings(), nil
	}, Options{Stale: Forever})
}

// UseCapabilities is what this platform supports (PlatformService.Capabilities);
// it is fixed for the process.
func UseCapabilities(cl *Client, core *services.Core) Result[services.Capabilities] {
	return Use(cl, KeyCapabilities, func(context.Context) (services.Capabilities, error) {
		if core == nil {
			return services.Capabilities{}, errNoCore
		}
		return services.Service[services.PlatformService](core).Capabilities(), nil
	}, Options{Stale: Forever, Keep: Forever})
}
