package account

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// text is a Text in one of the theme's type styles and colour tokens.
func text(c *ui.Context, s, style, color string) ui.Element {
	return theme.Text(c, s, style).TextColor(theme.C(color))
}

// flowTitle is the sign-in flow's titles: h3, under the account dialog's (or
// the setup's) title.
func flowTitle(c *ui.Context, s string) ui.Element {
	return text(c, s, "heading", "ink").Role(ui.RoleHeading).Level(3)
}

// hairline is a 1px --line rule across its parent (divide-line).
func hairline(c *ui.Context) ui.Element {
	return theme.Hairline(c).Shrink(0).Role(ui.RoleNone)
}

// noticeOpts shapes a notice.
type noticeOpts struct {
	// Icon is a Lucide name, drawn in the IconColor token.
	Icon, IconColor string
	// Problem is the bordered raised look; neutral is the --hover wash.
	Problem bool
	// Spin shows a spinner in place of the icon.
	Spin bool
}

// notice is Account.tsx's Notice: an icon, the text body and an optional
// action under it. It is a plain status: a problem shown on arrival or a
// neutral note. A problem the user's last action caused puts its words in
// an alert (errorNotice).
func notice(c *ui.Context, o noticeOpts, body func(), action func()) {
	e := ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(12).Padding(12, 16).
		Radius(theme.Current().Radius["lg"]).Role(ui.RoleStatus)
	if o.Problem {
		e.Background(theme.C("raised")).Border(1, theme.C("control-line"))
	} else {
		e.Background(theme.C("hover"))
	}
	e.Children(func() {
		if o.Spin {
			ui.Spinner(c).Size(16, 16).Margin(2, 0, 0, 0).Shrink(0).Role(ui.RoleNone) // the notice's text says what is busy
		} else {
			ui.Icon(c, theme.Icon(o.Icon)).FontSize(16).TextColor(theme.C(o.IconColor)).Margin(2, 0, 0, 0).Shrink(0)
		}
		// The action sits under the message, so the text keeps the notice's
		// whole measure.
		ui.Column(c.Key("body")).Grow(1).MinWidth(0).Children(func() {
			body()
			if action != nil {
				ui.Row(c.Key("action")).AlignItems(ui.Center).Margin(8, 0, -4, 0).Children(action)
			}
		})
	})
}

// noticeTitle is a notice's first line (font-medium).
func noticeTitle(c *ui.Context, s string) ui.Element {
	return text(c, s, "secondary", "ink").FontWeight(500)
}

// noticeDetail is the explanation under it (mt-0.5, muted).
func noticeDetail(c *ui.Context, s string) ui.Element {
	return text(c, s, "secondary", "ink-muted").Margin(2, 0, 0, 0)
}

// placeholder is the 12rem busy surface shown while the status is unknown: a
// busy status labelled label.
func placeholder(c *ui.Context, label string) ui.Element {
	return ui.Box(c).FillWidth().Height(192).Radius(theme.Current().Radius["lg"]).
		Background(theme.C("surface")).Role(ui.RoleStatus).Busy(true).Label(label)
}
