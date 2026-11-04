// Package settings is the Settings feature: the page of v0.2.0's
// features/settings, one route (/settings?section=…) with General,
// Playback, Appearance, Connections, Shortcuts, Storage and Diagnostics,
// Find a setting over them, and the live state the sections share. Every
// control saves at once through SystemService.UpdateSettings.
package settings

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// settingsPath is the page's route.
const settingsPath = "/settings"

// settings is the feature's state.
type settings struct {
	env          *module.Env
	sys          systemAPI
	plat         platformAPI
	scrobbler    scrobbleAPI
	integrations integrationsAPI
	cdm          cdmAPI
	updates      updatesAPI
	// clock is time.Now; tests move it.
	clock func() time.Time

	// last is the section shown last in this run: /settings without a
	// section opens it again (not persisted).
	last string
	// seen records the rows built, by ID: the test of the find index
	// checks that each entry has its row.
	seen map[string]bool
	// pending is a Find landing waiting for its row, found the row washed
	// after one.
	pending *landing
	found   found
	find    findState
	// lyrics is the lyrics settings last seen, to refetch lyrics when they
	// change; "" until the first load.
	lyrics string

	// state is the sections' own state that outlives a frame (a call in
	// flight, a form's text): stateOf, main thread only.
	state   map[string]any
	stateMu sync.Mutex // a fetch goroutine reads a seam through stateOf

	// The page's layout of the last frame (window size, the section's
	// scroll place).
	pos ui.ScrollState
}

// New is the settings module.
func New(env *module.Env) module.Module {
	s := newSettings(env,
		services.Service[services.SystemService](env.Core),
		services.Service[services.PlatformService](env.Core),
		services.Service[services.ScrobbleService](env.Core),
		services.Service[services.IntegrationsService](env.Core),
		services.Service[services.CDMService](env.Core),
		services.Service[services.UpdatesService](env.Core))
	return module.Module{
		ID: "settings",
		Routes: []module.Route{{
			Pattern: settingsPath,
			Title:   func(*ui.Route) string { return i18n.T("Settings") },
			Build:   s.page,
		}},
		Nav: []module.NavItem{{
			Section: module.NavFooter,
			Label:   func() string { return i18n.T("Settings") },
			Icon:    "settings",
			To:      settingsPath,
			Order:   10,
			Keys:    []string{"Ctrl+,"},
		}},
		Init: s.init,
	}
}

func newSettings(env *module.Env, sys systemAPI, plat platformAPI, scrobbler scrobbleAPI, integrations integrationsAPI,
	cdm cdmAPI, updates updatesAPI,
) *settings {
	return &settings{
		env: env, sys: sys, plat: plat, scrobbler: scrobbler, integrations: integrations, cdm: cdm, updates: updates,
		clock: time.Now, last: sections[0], seen: map[string]bool{}, state: map[string]any{}, find: findState{focus: focusNone},
	}
}

func (s *settings) now() time.Time { return s.clock() }

// init follows what changes Settings' data (index.ts eventHandlers, init).
func (s *settings) init() {
	q := s.env.Queries
	if s.sys != nil {
		s.lyrics = lyricsOf(s.sys.Settings())
	}
	store.On(s.env.Bus, services.EventSettingsChanged, func(v config.Settings) {
		store.Set(q, store.KeySettings, v)
		// Another source, order or language can pick other lyrics.
		if l := lyricsOf(v); s.lyrics != "" && l != s.lyrics {
			q.Invalidate(store.Key{"catalog", "lyrics"})
			s.lyrics = l
		} else {
			s.lyrics = l
		}
	})
	store.On(s.env.Bus, services.EventIntegrationsStatus, func(v services.IntegrationsStatus) {
		store.Set(q, keyIntegrations, v)
	})
	store.On(s.env.Bus, services.EventCDMStatus, func(v services.CDMStatus) { store.Set(q, keyCDM, v) })
	store.On(s.env.Bus, services.EventUpdateAvailable, func(v services.UpdateStatus) { store.Set(q, keyUpdates, v) })
	store.On(s.env.Bus, services.EventScrobbleChanged, func(v services.ScrobbleStatus) {
		store.Set(q, keyScrobble, v)
	})
	store.On(s.env.Bus, services.EventAppleLanguages, func(services.AppleLanguages) {
		q.Invalidate(store.Key{"translations", "languages"})
	})
}

// lyricsOf is the lyrics settings as a comparable string.
func lyricsOf(v config.Settings) string {
	b, _ := json.Marshal(v.Lyrics)
	return string(b)
}

// stateOf is the section's own state of type T under key, made zero on first
// use (main thread only: hand results back through env.Post).
func stateOf[T any](s *settings, key string) *T {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if v, ok := s.state[key]; ok {
		return v.(*T)
	}
	v := new(T)
	s.state[key] = v
	return v
}
