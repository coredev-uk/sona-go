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

// InfoLine is a notice that sets an expectation: an info icon and one
// sentence on a quiet wash, a status read where it stands (not an alert: nothing
// has gone wrong). It sits at the top of a dialog or a form, where it is seen
// before the controls it is about.
func InfoLine(c *ui.Context, msg string) {
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Start).Padding(10, 12).Radius(theme.Current().Radius["md"]).
		Background(theme.C("hover")).Role(ui.RoleStatus).Label(msg).Children(func() {
		ui.Icon(c.Key("i"), theme.Icon("info")).FontSize(16).TextColor(theme.C("ink")).Margin(2, 0, 0, 0).Shrink(0)
		theme.Text(c, msg, "secondary").TextColor(theme.C("ink")).Shrink(1).MinWidth(0)
	})
}
