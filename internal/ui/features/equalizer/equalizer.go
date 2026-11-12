// Package equalizer is the equaliser page: a response curve over ten band
// faders and a preamp, and the presets, previewed live while a fader is held
// and saved in Go's settings when it is let go (v0.2.0's features/equalizer).
// It is reached from Settings → Playback, the player bar and Ctrl+Shift+E,
// and says so plainly where the platform's playback has no equaliser.
//
// Every service call runs off the main thread and comes back through
// Env.Post; the audio host is told of the curve through
// services.AudioService.PreviewEqualizer, one call at a time.
package equalizer

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// path is the page's route.
const path = "/equalizer"

// Query keys, as v0.2.0's: the equaliser's bands, limits and presets
// (fixed for the app's lifetime). The settings the page edits are the shared
// store.KeySettings, which the shell keeps as Go's on "settings:changed".
var keyInfo = store.Key{"audio", "equalizer"}

// backend is what the page asks of Go: AudioService for the bands and the
// live preview, SystemService for the settings.
type backend interface {
	Info(ctx context.Context) (services.EqualizerInfo, error)
	Settings(ctx context.Context) (config.Settings, error)
	Preview(ctx context.Context, eq config.Equalizer) error
	Save(ctx context.Context, s config.Settings) (config.Settings, error)
}

// host is the backend over the real services.
type host struct {
	audio  *services.AudioService
	system *services.SystemService
}

func (h host) Info(context.Context) (services.EqualizerInfo, error) { return h.audio.Equalizer(), nil }

func (h host) Settings(context.Context) (config.Settings, error) { return h.system.Settings(), nil }

func (h host) Preview(ctx context.Context, eq config.Equalizer) error {
	return h.audio.PreviewEqualizer(ctx, eq)
}

func (h host) Save(_ context.Context, s config.Settings) (config.Settings, error) {
	return h.system.UpdateSettings(s)
}

// equalizer is the feature's state.
type equalizer struct {
	env *module.Env
	svc backend
	// supported reports whether this platform's playback has an equaliser
	// (PlatformService capabilities).
	supported func() bool

	// preview sends the curve being dragged to the playing audio; saves
	// writes the settings, the newest only when several arrive at once.
	preview store.Latest[config.Equalizer]
	saves   store.Latest[config.Settings]

	// draft is the curve while a fader is held: shown and heard, saved on
	// release. Main thread.
	draft *config.Equalizer
	// held are the faders under the pointer, by ID, with the value they
	// were last moved to. Main thread.
	held map[string]float64

	graph graphCache
	body  pageState
}

// New is the equaliser's module.
func New(env *module.Env) module.Module {
	plat := services.Service[services.PlatformService](env.Core)
	e := newEqualizer(env,
		host{audio: services.Service[services.AudioService](env.Core), system: services.Service[services.SystemService](env.Core)},
		func() bool { return plat.Capabilities().Playback.Equalizer })
	env.PlayerControls.Add(controlOrder, e.button)
	return module.Module{
		ID: "equalizer",
		Routes: []module.Route{{
			Pattern: path,
			Title:   func(*ui.Route) string { return i18n.T("Equaliser") },
			Build:   e.page,
		}},
		Shortcuts: e.shortcuts(),
	}
}

func newEqualizer(env *module.Env, svc backend, supported func() bool) *equalizer {
	e := &equalizer{env: env, svc: svc, supported: supported, held: map[string]float64{}}
	e.preview.Send = func(eq config.Equalizer) { _ = svc.Preview(context.Background(), eq) } // errors are ignored
	e.saves.Send = e.send
	return e
}

// shortcuts is Ctrl+Shift+E, with v0.2.0's ID and keys (index.ts): only
// where playback has an equaliser.
func (e *equalizer) shortcuts() []module.Shortcut {
	return []module.Shortcut{{
		ID: "equalizer.open", Group: "pages", Keys: []string{"Ctrl+Shift+E"}, Icon: "sliders-vertical",
		Label:    func() string { return i18n.T("Open the equaliser") },
		Keywords: func() string { return i18n.T("eq bass treble sound") },
		When:     e.supported,
		Run:      func() { e.env.Nav.Push(path) },
	}}
}

// info is the equaliser's bands, limits and presets, fetched once and kept
// for the app's lifetime (staleTime and gcTime Infinity).
func (e *equalizer) info() store.Result[services.EqualizerInfo] {
	return store.Use(e.env.Queries, keyInfo, e.svc.Info, store.Options{Stale: store.Forever, Keep: store.Forever})
}

// settings is Go's settings, as v0.2.0's useSettings.
func (e *equalizer) settings() store.Result[config.Settings] {
	return store.Use(e.env.Queries, store.KeySettings, e.svc.Settings, store.Options{Stale: store.Forever})
}

// refetch asks again for what failed (the Retry button).
func (e *equalizer) refetch() {
	e.env.Queries.Invalidate(keyInfo, store.KeySettings)
}

// live shows eq at once and sends it to the playing audio: a fader is
// moving.
func (e *equalizer) live(eq config.Equalizer) {
	e.draft = &eq
	e.preview.Put(eq)
}

// save writes s with eq as the equaliser (v0.2.0's useUpdateSettings): the
// cached settings change at once, Go's answer replaces them, and a failure
// asks Go for the real ones again and says so.
func (e *equalizer) save(s config.Settings, eq config.Equalizer) {
	e.draft = nil
	s.Audio.Equalizer = eq
	store.Set(e.env.Queries, store.KeySettings, s)
	e.saves.Put(s)
}

// send runs on a goroutine: one save at a time, the newest of those that
// queued.
func (e *equalizer) send(s config.Settings) {
	saved, err := e.svc.Save(context.Background(), s)
	e.env.Post(func() {
		if err != nil {
			kit.ToastErrorf(i18n.T("Couldn't save the setting"), i18n.ErrorMessage(err))
			e.env.Queries.Invalidate(store.KeySettings)
			return
		}
		store.Set(e.env.Queries, store.KeySettings, saved)
	})
}
