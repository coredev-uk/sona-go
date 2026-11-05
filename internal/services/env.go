package services

import (
	"context"
	"net/http"
	"slices"
	"sync"

	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/diag"
)

// Env is what every feature constructor receives: the core components
// (Deps, embedded), the shared DTO mappers, library membership and the
// lifecycle hooks. Features keep a *Env and must not reach into other
// features' services.
type Env struct {
	Deps

	m       mapper      // shared DTO mappers (mappers.go)
	members *membership // catalog item → in the user's library?
	// previews parks the first batch of a track station shown on its page
	// (station_preview.go).
	previews stationPreviews

	// coverFiles is held from the moment a chosen image is stored until the
	// playlist that chose it has recorded it, and while the covers directory
	// is tidied, so that the tidying never takes an image nothing refers to
	// yet (playlists_covers.go).
	coverFiles sync.Mutex

	// firstPaint is set by Core; SystemService.FirstPaint calls it.
	firstPaint func()

	mu       sync.Mutex
	closers  []func()
	sections []bundleSection
}

// bundleSection is a feature's section of the bug-report bundle.
type bundleSection struct {
	title string
	value func() any
}

// Report passes a failed Apple request to auth triage: a 401/403 that
// survived the client's token refresh may move the account to
// needs_reauth; every other error is ignored there.
func (e *Env) Report(err error) {
	if err != nil {
		e.Auth.ReportUnauthorized(err)
	}
}

// Fail reports err (see Report) and returns it, for one-line returns.
func (e *Env) Fail(err error) error {
	e.Report(err)
	return err
}

// updateSettings saves fn's change and announces the saved settings as
// EventSettingsChanged. The UI replaces its copy with the latest event, so
// the events must follow the saves: the announcement is made inside the
// store's update (Store.UpdateThen), not after it returns, where a later
// save (an autostart undo, a tray toggle) could be announced first and the
// UI would end on the older settings. Every save that announces goes through
// it, never Settings.Update followed by Emit.
func (e *Env) updateSettings(fn func(*config.Settings)) (config.Settings, error) {
	return e.updateSettingsIf(fn, func(_, _ config.Settings) bool { return true })
}

// updateSettingsIf is updateSettings that announces only when announce
// (given the settings before and after) says so.
func (e *Env) updateSettingsIf(fn func(*config.Settings), announce func(prev, saved config.Settings) bool) (config.Settings, error) {
	return e.Settings.UpdateThen(fn, func(prev, saved config.Settings) {
		if e.Emit != nil && announce(prev, saved) {
			e.Emit.Emit(EventSettingsChanged, saved)
		}
	})
}

// OnNetwork runs fn once the network opens after the first paint (at
// once if it is already open), on the goroutine that opened it: start a
// goroutine for anything slow. Cold start must not do network work before.
func (e *Env) OnNetwork(fn func()) { e.Gate.OnOpen(fn) }

// transport is the HTTP transport of Go's own network services: Deps.Transport,
// http.DefaultTransport when it is nil.
func (e *Env) transport() http.RoundTripper {
	if e.Transport != nil {
		return e.Transport
	}
	return http.DefaultTransport
}

// gated is transport held back until the first paint.
func (e *Env) gated() http.RoundTripper { return e.Gate.Transport(e.transport()) }

// openSecret opens the keyring secret named key (Deps.Keyring, the system
// keyring when it is nil), with its file fallback under the config directory.
func (e *Env) openSecret(key string) (store.Secret, error) {
	var kr store.Keyring = store.SystemKeyring{}
	if e.Keyring != nil {
		kr = e.Keyring
	}
	return store.OpenSecretWith(e.Paths.Config, key, kr, e.Logger)
}

// Go runs fn on a new goroutine; a panic in it is recovered and logged
// with its stack (name labels it) instead of crashing the app. Start every
// background goroutine in a feature with it.
func (e *Env) Go(name string, fn func()) { diag.Go(e.Logger, name, fn) }

// OnSignOut registers a sign-out step. Steps run in feature registry
// order, before the user-data wipe; name labels its error.
func (e *Env) OnSignOut(name string, fn func(context.Context) error) { e.Auth.OnSignOut(name, fn) }

// OnClose registers a shutdown step. Core.Close runs them in reverse
// registration order, before main closes the coordinator, engine and DB.
func (e *Env) OnClose(fn func()) {
	e.mu.Lock()
	e.closers = append(e.closers, fn)
	e.mu.Unlock()
}

// OnDiagnostics adds a section to the bug-report bundle (Settings →
// Diagnostics → Copy diagnostics), after the Diagnostics report and in
// registration order; value is called each time a bundle is built.
func (e *Env) OnDiagnostics(title string, value func() any) {
	e.mu.Lock()
	e.sections = append(e.sections, bundleSection{title, value})
	e.mu.Unlock()
}

// diagnosticSections evaluates the features' bundle sections.
func (e *Env) diagnosticSections() []diag.Section {
	e.mu.Lock()
	registered := slices.Clone(e.sections)
	e.mu.Unlock()
	out := make([]diag.Section, 0, len(registered))
	for _, s := range registered {
		out = append(out, diag.Section{Title: s.title, Value: s.value()})
	}
	return out
}

func (e *Env) close() {
	e.mu.Lock()
	fns := e.closers
	e.closers = nil
	e.mu.Unlock()
	for i := len(fns) - 1; i >= 0; i-- {
		fns[i]()
	}
}
