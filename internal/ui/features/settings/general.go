package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// general is Settings → General: the language (once there is a choice),
// starting Sona, its window, what it does while music plays, and updates.
// Rows the platform can't back keep their place and say so.
func (s *settings) general(c *ui.Context) {
	st, caps := s.cfg(), s.caps()
	if err := firstErr(st.HasData, st.Err, caps.HasData, caps.Err); err != nil {
		kit.ErrorBlock(c.Key("err"), i18n.T("Couldn't read Sona's settings"), err, kit.ErrorBlockOpts{
			OnRetry: func() { s.env.Queries.Invalidate(store.KeySettings, store.KeyCapabilities) },
		})
		return
	}
	cfg, cp, ok := s.ready()
	if !ok {
		s.loading(c, 6)
		return
	}
	s.stack(c, func() {
		s.languagePanel(c, cfg)
		s.group(c, "general-startup", i18n.T("Starting Sona"), "", func() {
			s.startupRows(c, cfg, cp)
			s.startPageRow(c, cfg)
			s.greetingRow(c, cfg)
			s.useNameRow(c, cfg)
			s.linkHandlerRow(c, cp)
			s.setupRow(c)
		})
		s.group(c, "general-window", i18n.T("Window"), "", func() {
			s.closeActionRow(c, cfg, cp)
			if cp.WindowControls.Native == "frame" {
				s.titleBarRow(c, cfg, cp)
			}
			s.windowSizeRow(c, cfg)
		})
		s.group(c, "general-playing", i18n.T("While Music Plays"), "", func() { s.playingRows(c, cfg, cp) })
		s.group(c, "general-releases", i18n.T("New Releases"), "", func() { s.releaseRows(c, cfg, cp) })
		s.updatesGroup(c, cfg)
	})
}

// firstErr is the error of a query that has no data (a failed read), nil
// while both have theirs.
func firstErr(hasA bool, errA error, hasB bool, errB error) error {
	switch {
	case !hasA && errA != nil:
		return errA
	case !hasB && errB != nil:
		return errB
	}
	return nil
}

// saveDesktop saves a change of the desktop settings.
func (s *settings) saveDesktop(change func(*config.Desktop)) {
	s.save(func(st *config.Settings) { change(&st.Desktop) })
}

// lastPage is the "Open on" value standing for the page Sona was last on
// (a select needs a non-empty value; no route is spelled so).
const lastPage = "last-page"

// openOn is the page Sona opens on, as the shell resolves it: the saved
// start page if it is registered in this build, else the first registered
// one (App.startPath's fallback). Callers check d.ResumeLastRoute first:
// then the shell opens on the remembered route, no fixed page.
func (s *settings) openOn(d config.Desktop) (route, label string) {
	for _, p := range startPages() {
		if !s.hasRoute(p.Route) {
			continue
		}
		if route == "" || p.Route == d.StartPage {
			route, label = p.Route, p.Label
		}
		if p.Route == d.StartPage {
			break
		}
	}
	return route, label
}

// startPageRow is "Open on": the pages Sona can open on that exist in this
// build.
func (s *settings) startPageRow(c *ui.Context, cfg config.Settings) {
	items := []kit.SelectItem{kit.SelectOption(lastPage, i18n.T("The page you were last on"))}
	for _, p := range startPages() {
		if s.hasRoute(p.Route) {
			items = append(items, kit.SelectOption(p.Route, p.Label))
		}
	}
	current := lastPage
	if !cfg.Desktop.ResumeLastRoute {
		current, _ = s.openOn(cfg.Desktop)
	}
	s.selectRow(c, rowOpts{
		ID: "start-page", Title: i18n.T("Open on"),
		Desc: i18n.T("The page Sona shows when it starts. Your queue is always kept."),
	}, current, items, false, func(v string) {
		if v == lastPage {
			s.saveDesktop(func(d *config.Desktop) { d.ResumeLastRoute = true })
			return
		}
		s.saveDesktop(func(d *config.Desktop) { d.StartPage, d.ResumeLastRoute, d.LastRoute = v, false, "" })
	})
}

// greetingRow turns the greeting that opens For You on or off.
func (s *settings) greetingRow(c *ui.Context, cfg config.Settings) {
	s.switchRow(c, rowOpts{
		ID: "home-greeting", Title: i18n.T("Greeting on For You"),
		Desc: i18n.T("Opens the page with good morning, good afternoon or good evening, by the time on this computer."),
	}, cfg.Home.Greeting, false, func(on bool) {
		s.save(func(n *config.Settings) { n.Home.Greeting = on })
	})
}

// useNameRow adds the first name from the person's Apple Music profile to
// the greeting. It has nothing to act on while the greeting is off.
func (s *settings) useNameRow(c *ui.Context, cfg config.Settings) {
	s.switchRow(c, rowOpts{
		ID: "home-use-name", Title: i18n.T("Use my name"),
		Desc: i18n.T("Adds the first name on your Apple Music profile to the greeting, when you have one. It stays on this computer."),
	}, cfg.Home.UseName, !cfg.Home.Greeting, func(on bool) {
		s.save(func(n *config.Settings) { n.Home.UseName = on })
	})
}
