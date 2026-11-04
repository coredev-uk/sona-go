package settings

import (
	"context"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
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
	r.tt = hn.Tester(t, func(c *ui.Context) {
		c.DisableAnimations() // a frozen test clock would redraw 20 frames per input
		ui.Column(c).Fill().Children(func() {
			r.router.View(c, func(rt *ui.Route) {
				if rt.Match(settingsPath) {
					r.s.page(c, rt)
				}
			})
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
