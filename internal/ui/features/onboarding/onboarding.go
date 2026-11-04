// Package onboarding is the first-run setup (v0.2.0's features/onboarding): a
// layer over the whole window, opened by Go on a first start (/welcome,
// services.StartRoute), by its /welcome?step= links and by Settings →
// General's "Set up Sona again" (store.Surfaces.OpenSetup), that signs the
// user in and sets how Sona plays, looks and starts, in seven steps that can
// each be skipped: welcome, Apple Music, playback, connections, appearance,
// desktop and done. Every control saves its setting at once through
// kit.UpdateSettings; reaching the last step, or leaving earlier, marks the
// setup completed once.
package onboarding

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// welcomePath is the route that opens the setup.
const welcomePath = "/welcome"

// onboarding is the feature's state. The open setup and its step live in
// env.Surfaces, outside the layer (a language change rebuilds the whole
// tree and keeps the step); what is here is what one opening of the layer
// keeps from frame to frame.
type onboarding struct {
	env  *module.Env
	deps account.Deps

	// What the steps ask of Go besides settings and capabilities; tests
	// replace them with fakes.
	scrobbler    scrobbleAPI
	integrations integrationsAPI
	translations translationsAPI
	cdmSvc       cdmAPI

	// shown is the step the last frame built, "" while the layer is closed:
	// a step the frame has not seen is arrived at.
	shown string
	// sec is the Apple Music step's sign-in section.
	sec *account.Section

	// The Widevine block: Try again is running, and the focus goes back to
	// the state once.
	retrying, focusState bool

	// Crossfade's length: the value while the thumb is held, saved on release.
	fade     float64
	fadeHeld bool

	// Connections: Last.fm's approval call is in flight; the ListenBrainz
	// token form is open, with its state, and the focus goes back to Connect
	// once when it closes.
	lastfmBusy   bool
	lb           bool
	lbForm       tokenForm
	focusConnect bool

	// The language asked for, until Go confirms it (settings:changed): then
	// every query is fetched again in it.
	language      string
	languageAsked bool
}

// New is the setup's module.
func New(env *module.Env) module.Module {
	o := newOnboarding(env, account.NewDeps(env),
		services.Service[services.ScrobbleService](env.Core),
		services.Service[services.IntegrationsService](env.Core),
		services.Service[services.TranslationsService](env.Core),
		services.Service[services.CDMService](env.Core))
	return module.Module{
		ID: "onboarding",
		Routes: []module.Route{{
			Pattern: welcomePath,
			Title:   func(*ui.Route) string { return i18n.T("Set up Sona") },
			Build:   o.welcome,
		}},
		Slots: module.Slots{Layer: o.layer, LayerOpen: env.Surfaces.SetupOpen},
		Init:  o.init,
	}
}

func newOnboarding(env *module.Env, deps account.Deps, sc scrobbleAPI, in integrationsAPI, tr translationsAPI, cdm cdmAPI) *onboarding {
	return &onboarding{env: env, deps: deps, scrobbler: sc, integrations: in, translations: tr, cdmSvc: cdm}
}

// welcome is /welcome?step=…: it opens the setup over the start page at that
// step and redirects (replace) there; the route itself is never shown. A
// missing or unknown step starts at the beginning. Go opens the main window
// on it on a first start (services.StartRoute). "/" has no route of its own
// natively: the shell sends an unmatched path to the start page.
func (o *onboarding) welcome(c *ui.Context, r *ui.Route) {
	o.env.Surfaces.OpenSetup(parseStep(r.Query("step")))
	o.env.Nav.Replace("/")
}

// init follows the saved language: once Go has it, every query is fetched
// again, so Apple's content comes in the new language too (useSetLanguage's
// onSuccess).
func (o *onboarding) init() {
	store.On(o.env.Bus, services.EventSettingsChanged, func(s config.Settings) {
		if o.languageAsked && s.Language == o.language {
			o.languageAsked = false
			o.env.Queries.Invalidate()
		}
	})
}
