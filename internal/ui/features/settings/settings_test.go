package settings

import (
	"context"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// rig is the page over the full Env (a real services.Core on a temporary
// library, the settings store of the harness) and a router at /settings.
type rig struct {
	t      *testing.T
	h      *uitest.Harness
	router *ui.Router
	s      *settings
	tt     *uitest.Session
}

// newRig builds the page at a window size; opts are uitest options (Dark…).
// The Env lists the shortcuts of a few fake modules, as the shell would.
func newRig(t *testing.T, w, h int, shortcuts []module.BoundShortcut, opts ...uitest.Option) *rig {
	t.Helper()
	return newRigCaps(t, w, h, shortcuts, nil, opts...)
}

// newRigCaps is newRig on a platform with these capabilities (the cache is
// seeded before the first frame, so the real service never answers).
func newRigCaps(t *testing.T, w, h int, shortcuts []module.BoundShortcut, caps *services.Capabilities, opts ...uitest.Option) *rig {
	t.Helper()
	return newRigBeside(t, w, h, 0, shortcuts, caps, opts...)
}

// newRigBeside is newRigCaps with a sidebar sidebar DIP wide left of the
// page, as the shell's: the page has the window less that.
func newRigBeside(t *testing.T, w, h int, sidebar float32, shortcuts []module.BoundShortcut, caps *services.Capabilities, opts ...uitest.Option) *rig {
	t.Helper()
	hn := uitest.New(t, opts...)
	hn.Env.HasRoute = func(p string) bool { return p == "/library/albums" }
	if caps != nil {
		store.Set(hn.Env.Queries, store.KeyCapabilities, *caps)
	}
	r := &rig{t: t, h: hn, router: ui.NewRouter(settingsPath)}
	hn.Env.Nav = r.router
	hn.Env.Shortcuts = func() []module.BoundShortcut { return shortcuts }
	r.s = newSettings(hn.Env,
		services.Service[services.SystemService](hn.Core), services.Service[services.PlatformService](hn.Core),
		fakeScrobbler{}, services.Service[services.IntegrationsService](hn.Core),
		services.Service[services.CDMService](hn.Core), services.Service[services.UpdatesService](hn.Core))
	r.s.init()
	followShell(hn)
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		view := func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(settingsPath) {
					r.s.page(c, rt)
				}
			})
		}
		if sidebar == 0 {
			ui.Column(c).Fill().Children(view)
			return
		}
		ui.Row(c).Fill().Children(func() {
			ui.Box(c.Key("sidebar")).Width(sidebar).Shrink(0)
			ui.Column(c.Key("page")).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(view)
		})
	}, w, h)
	r.s.clock = r.tt.Now // the wash and the wait for a row run on the tester's clock
	return r
}

func TestPageListsTheSectionsAndOpensTheFirst(t *testing.T) {
	r := newRig(t, 1132, 900, nil)
	r.tt.Until("the page", r.tt.Shows("Settings", "General", "Playback", "Appearance", "Connections", "Shortcuts", "Storage", "Diagnostics"))
	if got := r.s.last; got != "general" {
		t.Errorf("the first visit opens %q", got)
	}
}

// fakeScrobbler is a scrobbler with nothing connected: the real
// ScrobbleService reads the machine's keyring, which a test must not.
type fakeScrobbler struct{}

func (fakeScrobbler) Status() services.ScrobbleStatus { return services.ScrobbleStatus{} }
func (fakeScrobbler) ConnectLastFM(context.Context) (services.ScrobbleStatus, error) {
	return services.ScrobbleStatus{}, nil
}
func (fakeScrobbler) CancelConnect() {}
func (fakeScrobbler) ConnectListenBrainz(context.Context, string) (services.ScrobbleStatus, error) {
	return services.ScrobbleStatus{}, nil
}

func (fakeScrobbler) Disconnect(string) (services.ScrobbleStatus, error) {
	return services.ScrobbleStatus{}, nil
}

// reveal scrolls a row into view and focuses its control (Find's landing),
// so a click finds it in a short window: a tall one costs every frame.
func (r *rig) reveal(id string) {
	r.t.Helper()
	r.s.focusSetting(id)
	r.tt.Drain()
	r.tt.Drain()
}

// toEnd scrolls the section to its end.
func (r *rig) toEnd() {
	r.t.Helper()
	r.s.pos.Y = math.MaxFloat32
	r.tt.Drain()
	r.tt.Drain()
}

// followShell does what the shell does on "settings:changed" (app.followSettings):
// the shared settings the features read follow Go's. The feature itself no
// longer writes them.
func followShell(h *uitest.Harness) {
	store.On(h.Bus, services.EventSettingsChanged, func(s config.Settings) {
		store.Set(h.Env.Queries, store.KeySettings, s)
		theme.SetTheme(s.ThemeID) // the shell applies the chosen theme
	})
}
