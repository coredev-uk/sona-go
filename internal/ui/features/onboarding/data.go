package onboarding

import (
	"context"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// What the steps read and ask of Go. The query keys are v0.2.0's, which
// Settings shares: Go's scrobble:changed and integrations:status events
// (followed by the settings module) replace them.
var (
	keyScrobble     = store.Key{"scrobble", "status"}
	keyIntegrations = store.Key{"integrations", "status"}
	// keyLanguages is the language list asked for with the share the UI
	// needs of Apple's locales (the prefix {"translations","languages"} is
	// what Settings invalidates when Go's locale table arrives).
	keyLanguages = store.Key{"translations", "languages", i18n.SelectableShare}
)

// scrobbleAPI is what the Connections step asks of ScrobbleService.
type scrobbleAPI interface {
	Status() services.ScrobbleStatus
	ConnectLastFM(ctx context.Context) (services.ScrobbleStatus, error)
	CancelConnect()
	ConnectListenBrainz(ctx context.Context, token string) (services.ScrobbleStatus, error)
}

// integrationsAPI is what it asks of IntegrationsService.
type integrationsAPI interface {
	Status() services.IntegrationsStatus
}

// translationsAPI is what the Welcome step's language asks of
// TranslationsService.
type translationsAPI interface {
	Languages(minShare float64) services.AppleLanguages
}

// cdmAPI is what the Widevine block asks of CDMService.
type cdmAPI interface {
	Retry() error
}

// settings is the settings (queries.ts useSettings): Go's, cached under
// ["settings"] and kept current by settings:changed.
func (o *onboarding) settings() store.Result[config.Settings] {
	return store.UseSettings(o.env.Queries, o.env.Core)
}

// caps is what this platform supports (capabilities.ts).
func (o *onboarding) caps() store.Result[services.Capabilities] {
	return store.UseCapabilities(o.env.Queries, o.env.Core)
}

// cdm is the Widevine CDM as Go reports it; nil until Go answers.
func (o *onboarding) cdm() *services.CDMStatus {
	r := store.UseCDM(o.env.Queries, o.env.Core)
	if !r.HasData {
		return nil
	}
	return &r.Data
}

// capsData is the capabilities, nil until they have come.
func (o *onboarding) capsData() *services.Capabilities {
	r := o.caps()
	if !r.HasData {
		return nil
	}
	return &r.Data
}

// authState is the sign-in state (useAuth().data?.state); "" until known.
func (o *onboarding) authState() string {
	r := account.UseStatus(o.deps)
	if !r.HasData {
		return ""
	}
	return r.Data.State
}

// scrobbleStatus is the scrobblers' live state (useScrobbleStatus).
func (o *onboarding) scrobbleStatus() store.Result[services.ScrobbleStatus] {
	api := o.scrobbler
	return store.Use(o.env.Queries, keyScrobble, func(context.Context) (services.ScrobbleStatus, error) {
		return api.Status(), nil
	})
}

// integrationsStatus is the integrations' live state (useIntegrationsStatus).
func (o *onboarding) integrationsStatus() store.Result[services.IntegrationsStatus] {
	api := o.integrations
	return store.Use(o.env.Queries, keyIntegrations, func(context.Context) (services.IntegrationsStatus, error) {
		return api.Status(), nil
	})
}

// languageOptions are the languages the Language setting offers for the
// chosen one, once Go has answered; ok is false until then (language-queries.ts
// useLanguageOptions). Go lists Apple's locales only when they could be
// complete enough (i18n.SelectableShare), so most of the time the answer is
// English alone and the setting stays hidden.
func (o *onboarding) languageOptions(current string) (options []i18n.LanguageOption, ok bool) {
	api := o.translations
	r := store.Use(o.env.Queries, keyLanguages,
		func(context.Context) (services.AppleLanguages, error) {
			return api.Languages(i18n.SelectableShare), nil
		})
	if !r.HasData {
		return nil, false
	}
	return i18n.LanguageOptions(r.Data, nil, current, i18n.Locale()), true
}
