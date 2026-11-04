package settings

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// systemAPI is what the page asks of SystemService.
type systemAPI interface {
	Settings() config.Settings
}

// platformAPI is what the page asks of PlatformService.
type platformAPI interface {
	Capabilities() services.Capabilities
}

// cfg is the settings (queries.ts useSettings): Go's, cached under
// ["settings"] and kept current by settings:changed.
func (s *settings) cfg() store.Result[config.Settings] {
	return store.UseSettings(s.env.Queries, s.env.Core)
}

// caps is what this platform supports (capabilities.ts): fixed for the
// process, so fetched once.
func (s *settings) caps() store.Result[services.Capabilities] {
	return store.UseCapabilities(s.env.Queries, s.env.Core)
}

// ready is both answers, once they are there: what a section needs before
// it builds its rows (until then it shows its skeleton).
func (s *settings) ready() (config.Settings, services.Capabilities, bool) {
	st, c := s.cfg(), s.caps()
	if !st.HasData || !c.HasData {
		return config.Settings{}, services.Capabilities{}, false
	}
	return st.Data, c.Data, true
}

// save applies change to a copy of the settings and saves it (kit.UpdateSettings,
// v0.2.0's useUpdateSettings): the cache and the appearance update at once,
// Go's answer replaces them, and on failure the old value comes back with a
// toast "Couldn't save the setting". Every control of the page saves itself
// with it.
func (s *settings) save(change func(*config.Settings)) {
	kit.UpdateSettings(s.env.Queries, s.env.Core, change)
}

// Query keys of the live state the sections show (scrobble.ts,
// integrations.ts); Go's scrobble:changed and integrations:status replace
// them.
var (
	keyScrobble     = store.Key{"scrobble", "status"}
	keyIntegrations = store.Key{"integrations", "status"}
)

// scrobbleAPI and integrationsAPI are what the sections ask of
// ScrobbleService and IntegrationsService.
type scrobbleAPI interface {
	Status() services.ScrobbleStatus
	ConnectLastFM(ctx context.Context) (services.ScrobbleStatus, error)
	CancelConnect()
	ConnectListenBrainz(ctx context.Context, token string) (services.ScrobbleStatus, error)
	Disconnect(provider string) (services.ScrobbleStatus, error)
}

type integrationsAPI interface {
	Status() services.IntegrationsStatus
}

// scrobbleStatus is the scrobblers' live state (useScrobbleStatus).
func (s *settings) scrobbleStatus() store.Result[services.ScrobbleStatus] {
	return store.Use(s.env.Queries, keyScrobble, func(context.Context) (services.ScrobbleStatus, error) {
		return s.scrobbler.Status(), nil
	})
}

// integrationsStatus is the integrations' live state (useIntegrationsStatus).
func (s *settings) integrationsStatus() store.Result[services.IntegrationsStatus] {
	return store.Use(s.env.Queries, keyIntegrations, func(context.Context) (services.IntegrationsStatus, error) {
		return s.integrations.Status(), nil
	})
}

// cdmAPI and updatesAPI are what the sections ask of CDMService and
// UpdatesService.
type cdmAPI interface {
	Retry() error
}

type updatesAPI interface {
	Check(ctx context.Context) (services.UpdateStatus, error)
	OpenRelease() error
	Install(ctx context.Context) (services.UpdateStatus, error)
}

// cdmStatus is the Widevine CDM's state: the shared mirror (store.KeyCDM)
// the cdm module keeps current from Go's cdm:status.
func (s *settings) cdmStatus() store.Result[services.CDMStatus] {
	return store.UseCDM(s.env.Queries, s.env.Core)
}

// updateStatus is the update check's latest result: the shared mirror
// (store.KeyUpdates) the updates module keeps current; a manual check writes
// its answer to it too.
func (s *settings) updateStatus() store.Result[services.UpdateStatus] {
	return store.UseUpdates(s.env.Queries, s.env.Core)
}
