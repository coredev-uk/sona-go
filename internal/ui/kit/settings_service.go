package kit

import (
	"maps"
	"slices"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// UpdateSettings saves a settings change in Go (each control saves itself):
// v0.2.0's useUpdateSettings. change edits a copy of the current settings
// (what the cache holds, else Go's); the cache and the theme follow at once,
// the save runs off the main thread, and a failure puts the previous
// settings back and toasts "Couldn't save the setting". "settings:changed"
// confirms, and the saved settings replace the cache on success.
func UpdateSettings(cl *store.Client, core *services.Core, change func(*config.Settings)) {
	svc := services.Service[services.SystemService](core)
	prev, ok := store.Peek[config.Settings](cl, store.KeySettings)
	if !ok {
		prev = svc.Settings()
	}
	next := CloneSettings(prev)
	change(&next)
	store.Set(cl, store.KeySettings, next)
	applyAppearance(next)
	go func() {
		saved, err := svc.UpdateSettings(next)
		if err != nil {
			store.Set(cl, store.KeySettings, prev)
			applyAppearance(prev)
			ToastErrorf(i18n.T("Couldn't save the setting"), i18n.ErrorMessage(err))
			return
		}
		store.Set(cl, store.KeySettings, saved)
	}()
}

// CloneSettings copies s deeply enough that editing the copy never changes
// the cached settings: the maps and slices it holds are cloned.
func CloneSettings(s config.Settings) config.Settings {
	s.Shortcuts = maps.Clone(s.Shortcuts)
	s.Audio.Equalizer.Custom = slices.Clone(s.Audio.Equalizer.Custom)
	s.Lyrics.Providers = slices.Clone(s.Lyrics.Providers)
	return s
}

// applyAppearance applies the theme and reduce effects at once (the window's
// own follow-up on "settings:changed" confirms it).
func applyAppearance(s config.Settings) {
	switch s.Theme {
	case config.ThemeLight:
		theme.SetMode(theme.ModeLight)
	case config.ThemeDark:
		theme.SetMode(theme.ModeDark)
	default:
		theme.SetMode(theme.ModeSystem)
	}
	theme.SetReduceEffects(s.ReduceEffects)
}
