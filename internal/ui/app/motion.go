package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// reportSettle is how long after the first frame the shell waits before it
// starts keeping MotionService's report current: building the report asks the
// policy whether motion is allowed, which starts the decoder probe
// (GStreamer's start-up), and that must not compete with the first paint.
const reportSettle = 3 * time.Second

// newMotion builds the motion policy over what the app has: the motion
// cache's files, the Motion artwork setting (kept current by followSettings),
// the power source, and the art loader's scale.
func (a *App) newMotion(post func(func())) *kit.MotionPolicy {
	o := kit.MotionOptions{
		Files: a.opts.Motion,
		Art:   a.env.Art,
		Log:   func(msg string, args ...any) { a.log.Debug(msg, args...) },
	}
	if a.opts.Settings != nil {
		a.motionOn = a.opts.Settings.Get().MotionArtwork.Enabled
		o.Setting = func() bool { return a.motionOn }
	}
	if a.opts.Core != nil {
		svc := services.Service[services.MotionService](a.opts.Core)
		o.Power = svc.Power
		a.sendReport = svc.Report
	}
	return kit.NewMotionPolicy(a.opts.Bus, post, o)
}

// hasRoute is Env.HasRoute: some module's route pattern matches path.
func (a *App) hasRoute(path string) bool {
	for _, r := range a.routes {
		if routeMatches(r.Pattern, path) {
			return true
		}
	}
	return false
}

// routeMatches is MyGo's Route.Match for a path that is not the router's:
// the parts between slashes are the same, {name} takes any one part and
// {name...}, at the end, the rest of the path, which may be empty. A query
// or fragment and a slash at the end make no difference.
func routeMatches(pattern, path string) bool {
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	parts, segs := splitPath(pattern), splitPath(path)
	for i, p := range parts {
		if len(p) < 2 || p[0] != '{' || p[len(p)-1] != '}' {
			if i >= len(segs) || segs[i] != p {
				return false
			}
			continue
		}
		if strings.HasSuffix(p, "...}") {
			return i == len(parts)-1
		}
		if i >= len(segs) || segs[i] == "" {
			return false
		}
	}
	return len(parts) == len(segs)
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// overlayOpen reports whether a module's overlay covers the page column.
func (a *App) overlayOpen() bool {
	for _, m := range a.mods {
		if o := m.Slots.Overlay; o != nil && o.Open != nil && o.Open() {
			return true
		}
	}
	return false
}

// layerOpen reports whether a module's layer is over the window.
func (a *App) layerOpen() bool {
	for _, m := range a.mods {
		if m.Slots.Layer != nil && m.Slots.LayerOpen != nil && m.Slots.LayerOpen() {
			return true
		}
	}
	return false
}

// afterFrame runs at the end of every frame the shell builds: it tells the
// motion policy whether a layer, an overlay or a modal dialog covers the page
// (the playing videos pause under them, as React paused when the page went
// inert), and keeps MotionService's report current for the bug-report bundle.
func (a *App) afterFrame(c *ui.Context, layerOpen, overlayOpen bool) {
	covered := layerOpen || overlayOpen || kit.TakeDialogsBuilt() > 0
	if covered != (a.uncover != nil) {
		if covered {
			a.uncover = a.env.Motion.Cover()
		} else {
			a.uncover()
			a.uncover = nil
		}
		// What was built this frame saw the old state.
		c.Invalidate()
	}
	a.reportMotion()
}

// reportMotion sends MotionService the native side of motion artwork
// (v0.2.0's Diagnostics sent it before it built a bundle; there is no such
// moment here yet, so the shell keeps the service's copy current: it sends
// when the report changed). It waits for reportSettle after the first frame.
func (a *App) reportMotion() {
	if a.sendReport == nil {
		return
	}
	now := a.now()
	if a.reportAt.IsZero() {
		a.reportAt = now.Add(reportSettle)
	}
	if now.Before(a.reportAt) {
		return
	}
	r := a.env.Motion.Report()
	if a.reported && r == a.lastReport {
		return
	}
	a.reported, a.lastReport = true, r
	a.sendReport(r)
}

func (a *App) now() time.Time {
	if a.opts.Now != nil {
		return a.opts.Now()
	}
	return time.Now()
}

// followVisibility tells the motion policy when the window hides or shows
// (minimised, hidden to the tray): playing videos pause meanwhile.
func (a *App) followVisibility(win *mygo.Window) {
	set := func(v bool) func() {
		return func() {
			a.env.Motion.SetVisible(v)
			a.post(func() {})
		}
	}
	win.OnShow(set(true))
	win.OnRestore(set(true))
	win.OnHide(set(false))
	win.OnMinimize(set(false))
}
