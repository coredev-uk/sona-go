package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// lastPage is the "Open on" value standing for the page Sona was last on.
const lastPage = "last-page"

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
				s.titleBarRow(c, cfg, cp)
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

// startPageRow is "Open on".
func (s *settings) startPageRow(c *ui.Context, cfg config.Settings) {
	d := cfg.Desktop
	items := []kit.SelectItem{kit.SelectOption(lastPage, i18n.T("The page you were last on"))}
	for _, p := range startPages() {
		items = append(items, kit.SelectOption(p.Route, p.Label))
	}
	value := d.StartPage
	if d.ResumeLastRoute {
		value = lastPage
	}
	s.selectRow(c, rowOpts{
		ID: "start-page", Title: i18n.T("Open on"),
		Desc: i18n.T("The page Sona shows when it starts. Your queue is always kept."),
	}, value, items, false, func(v string) {
		if v == lastPage {
			s.saveDesktop(func(d *config.Desktop) { d.ResumeLastRoute = true })
			return
		}
		s.saveDesktop(func(d *config.Desktop) { d.StartPage, d.ResumeLastRoute, d.LastRoute = v, false, "" })
	})
}
