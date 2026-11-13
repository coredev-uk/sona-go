package services

// feature is one service: build constructs it from the shared Env
// (registering any hooks it needs) and returns the typed instance (a *T);
// its exported methods are the UI-facing API.
type feature struct {
	name  string
	build func(*Env) any
}

// svc adapts a typed constructor for the registry.
func svc[T any](build func(*Env) *T) func(*Env) any {
	return func(e *Env) any { return build(e) }
}

// features are the bound services, built in this order (which is also the
// order of their sign-out steps). Adding a feature: its files (feature.go,
// feature_dto.go) plus one line here (doc.go).
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
	{"lyrics", svc(newLyricsService)},
	{"concerts", svc(newConcertsService)},
	{"themes", svc(newThemesService)},
}
