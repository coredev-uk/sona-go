package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// setupRow opens the first-run setup again over Settings.
func (s *settings) setupRow(c *ui.Context) {
	title := i18n.T("First-run setup")
	s.row(c, rowOpts{
		ID: "setup-again", Title: title,
		Desc: i18n.T("Sign in, then how Sona plays, connects, looks and starts, one step at a time, as on Sona's first start."),
	}, func() ui.Element {
		b := kit.Button(c.Key("setup"), i18n.T("Set up Sona again"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM})
		if b.Clicked() {
			s.env.Surfaces.OpenSetup(store.SetupWelcome)
		}
		return b
	})
}
