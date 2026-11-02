package services

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
)

// EventAppleTranslations carries AppleTranslations when a locale's
// messages arrive or change (a download after the first paint).
const EventAppleTranslations = "translations:apple"

func init() { application.RegisterEvent[AppleTranslations](EventAppleTranslations) }

// AppleTranslations is Apple Music's own translation of Sona's mapped
// messages for one locale, fetched from Apple at run time and cached
// (internal/i18n/applestrings). The frontend layers it over the community
// catalogue and English and decides whether to show the locale.
type AppleTranslations struct {
	// Locale is Apple's locale in BCP 47 ("fr-FR"); "" when none is cached
	// for the asked tag (yet).
	Locale string `json:"locale"`
	// Messages are Lingui (ICU) messages keyed by Sona's message ID; text
	// only, rendered as text.
	Messages map[string]string `json:"messages"`
	// Coverage is the share of all Sona's messages in Messages (0-1).
	Coverage float64 `json:"coverage"`
	// Version is Apple's bundle the messages come from.
	Version string `json:"version"`
	// FetchedAt is when they were downloaded, Unix ms (0: none).
	FetchedAt int64 `json:"fetchedAt"`
}

func appleTranslations(l applestrings.Locale) AppleTranslations {
	out := AppleTranslations{Locale: l.Tag, Messages: l.Messages, Coverage: l.Coverage(), Version: l.Version}
	if out.Messages == nil {
		out.Messages = map[string]string{}
	}
	if !l.FetchedAt.IsZero() {
		out.FetchedAt = l.FetchedAt.UnixMilli()
	}
	return out
}
