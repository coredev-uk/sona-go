package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// AlertLine is a sentence with a danger icon, an alert read as it appears.
func AlertLine(c *ui.Context, msg string) {
	ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Start).Role(ui.RoleAlert).Label(msg).Children(func() {
		ui.Icon(c.Key("i"), theme.Icon("circle-alert")).FontSize(16).TextColor(theme.C("danger")).Margin(2, 0, 0, 0).Shrink(0)
		theme.Text(c, msg, "secondary").TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
	})
}
