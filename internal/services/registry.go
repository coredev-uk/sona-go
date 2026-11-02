package services

import "github.com/wailsapp/wails/v3/pkg/application"

// feature is one bound Wails service: build constructs it from the shared
// Env (registering any hooks it needs) and wraps it for Wails; its
// exported methods become bindings.
type feature struct {
	name  string
	build func(*Env) application.Service
}

// svc adapts a typed constructor for the registry. Wails v3 marshals a
// bound method's error with its service's MarshalError only (the
// application-level option does not reach bound methods), so every service
// carries MarshalError for err.cause = {code, message}.
func svc[T any](build func(*Env) *T) func(*Env) application.Service {
	return func(e *Env) application.Service {
		return application.NewServiceWithOptions(build(e), application.ServiceOptions{MarshalError: MarshalError})
	}
}

// features are the bound services, built in this order (which is also the
// order of their sign-out steps). Adding a feature: its files (feature.go,
// feature_dto.go) plus one line here.
var features = []feature{
	{"auth", svc(newAuthService)},
	{"catalog", svc(newCatalogService)},
	{"library", svc(newLibraryService)},
	{"playback", svc(newPlaybackService)},
	{"cdm", svc(newCDMService)},
	{"search", svc(newSearchService)},
	{"discover", svc(newDiscoverService)},
	{"playlists", svc(newPlaylistsService)},
	{"radio", svc(newRadioService)},
	{"system", svc(newSystemService)},
	{"updates", svc(newUpdatesService)},
	{"integrations", svc(newIntegrationsService)},
	{"motion", svc(newMotionService)},
	{"desktop", svc(newDesktopService)},
	{"scrobble", svc(newScrobbleService)},
	{"audio", svc(newAudioService)},
	{"storage", svc(newStorageService)},
	{"private", svc(newPrivateService)},
	{"platform", svc(newPlatformService)},
	{"reminders", svc(newRemindersService)},
	{"translations", svc(newTranslationsService)},
}
