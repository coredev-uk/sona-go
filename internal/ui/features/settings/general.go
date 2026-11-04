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
			s.setupRow(c)
		})
		s.group(c, "general-window", i18n.T("Window"), "", func() {
			s.closeActionRow(c, cfg, cp)
			if cp.WindowControls.Native == "frame" {
				s.titleBarRow(c)
			}
			s.windowSizeRow(c, cfg)
		})
		s.group(c, "general-playing", i18n.T("While music plays"), "", func() { s.playingRows(c, cfg, cp) })
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

// openOn is the page Sona opens on, as the shell resolves it: the saved
// start page if it is registered in this build, else the first registered
// one (App.startPath's fallback). "The page you were last on" waits for the
// shell's route memory (phase 3), so it is neither offered nor shown.
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
	var items []kit.SelectItem
	for _, p := range startPages() {
		if s.hasRoute(p.Route) {
			items = append(items, kit.SelectOption(p.Route, p.Label))
		}
	}
	current, _ := s.openOn(cfg.Desktop)
	s.selectRow(c, rowOpts{
		ID: "start-page", Title: i18n.T("Open on"),
		Desc: i18n.T("The page Sona shows when it starts. Your queue is always kept."),
	}, current, items, false, func(v string) {
		s.saveDesktop(func(d *config.Desktop) { d.StartPage, d.ResumeLastRoute, d.LastRoute = v, false, "" })
	})
}
