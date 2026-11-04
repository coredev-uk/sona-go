package services

import (
	"time"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
)

// TranslationsService serves Apple Music's own translations of Sona's
// messages (docs/ui-architecture.md, "Strings and locales"). Sona
// fetches them from music.apple.com once the network opens after the
// first paint, for the locales the UI asked for, and keeps them in its
// cache directory, checking for a new Apple release at most daily; the
// cache serves offline. "translations:apple" announces a locale's messages
// when they arrive or change, "translations:languages" Apple's locale
// table (Languages). Without a store it serves nothing.
type TranslationsService struct {
	e *Env
}

func newTranslationsService(e *Env) *TranslationsService {
	s := &TranslationsService{e: e}
	if e.AppleStrings == nil {
		return s
	}
	e.OnClose(e.AppleStrings.Subscribe(func(l applestrings.Locale) {
		e.Emit.Emit(EventAppleTranslations, appleTranslations(l))
	}))
	e.OnClose(e.AppleStrings.SubscribeIndex(func() {
		e.Emit.Emit(EventAppleLanguages, appleLanguages(e.AppleStrings.Available(0)))
	}))
	e.OnNetwork(func() {
		e.Logger.Info("startup: apple strings check starts", "since_start_ms", time.Since(e.Started).Milliseconds())
		e.AppleStrings.Start()
	})
	return s
}

// Apple returns the cached messages for Apple's locale matching a
// preferred tag (the account's storefront language or a system language,
// e.g. "fr-CA"), and has Sona fetch them when the cache lacks them or is
// out of date. English tags have none (English is Sona's source).
func (s *TranslationsService) Apple(tag string) AppleTranslations {
	if s.e.AppleStrings == nil {
		return appleTranslations(applestrings.Locale{})
	}
	return appleTranslations(s.e.AppleStrings.Lookup(tag))
}

// Languages lists Apple's locales for the Language setting (Settings →
// General), when Sona's mapping could translate at least minShare of its
// messages (the UI's selectable share, less what the community
// catalogues add); below that it only reports the mapped share and fetches
// nothing. Otherwise it serves the cached locale table and has Sona fetch
// it after the first paint when it is missing or due; it then arrives as
// "translations:languages".
func (s *TranslationsService) Languages(minShare float64) AppleLanguages {
	if s.e.AppleStrings == nil {
		return appleLanguages(applestrings.Available{})
	}
	return appleLanguages(s.e.AppleStrings.Available(minShare))
}
