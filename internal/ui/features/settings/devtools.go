package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// developerTools is Settings → Diagnostics → Developer tools: MyGo's
// inspector (the web inspector of v0.2.0 is WebKit's and does not exist).
// The Open button works where capabilities.DeveloperTools says the
// inspector is on for this run (a development build started with
// SONA_INSPECTOR=1); elsewhere the row says so.
func (s *settings) developerTools(c *ui.Context, enabled bool) {
	title := i18n.T("Open the inspector")
	s.group(c, "developer-tools", i18n.T("Developer tools"), "", func() {
		if !enabled {
			s.unavailableRow(c, "developer-tools", title,
				i18n.T("Available in development builds started with SONA_INSPECTOR=1."))
			return
		}
		s.row(c, rowOpts{
			ID: "developer-tools", Title: title,
			Desc: i18n.T("Shows the inspector of Sona's window: its elements and what each one draws, for debugging and for writing plugins."),
		}, func() ui.Element {
			b := kit.Button(c.Key("open"), i18n.T("Open"), kit.ButtonOpts{
				Variant: kit.Outline, Size: kit.SizeSM, Icon: "square-terminal", A11yLabel: title,
			})
			if b.Clicked() {
				s.openInspector()
			}
			return b
		})
	})
}

// openInspector shows the inspector; failure is a toast.
func (s *settings) openInspector() {
	api := s.diagSeams().desktop(s)
	go func() {
		err := api.OpenDeveloperTools()
		s.env.Post(func() {
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't open the inspector"), i18n.ErrorMessage(err))
			}
		})
	}()
}
