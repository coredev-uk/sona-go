package account

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// text is a Text in one of the theme's type styles and colour tokens.
func text(c *ui.Context, s, style, color string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).
		LetterSpacing(t.Tracking).TextColor(theme.C(color))
}

// flowTitle is the sign-in flow's titles: h3, under the account dialog's (or
// the setup's) title.
func flowTitle(c *ui.Context, s string) ui.Element {
	return text(c, s, "heading", "ink").Role(ui.RoleHeading).Level(3)
}

// hairline is a 1px --line rule across its parent (divide-line).
func hairline(c *ui.Context) ui.Element {
	return ui.Box(c).Height(1).FillWidth().Shrink(0).Role(ui.RoleNone).Background(theme.C("line"))
}

// noticeOpts shapes a notice.
type noticeOpts struct {
	// Icon is a Lucide name, drawn in the IconColor token.
	Icon, IconColor string
	// Problem is the bordered raised look; neutral is the --hover wash.
	Problem bool
	// Spin shows a spinner in place of the icon.
	Spin bool
	// Announce, when set, is read out once as the notice appears: React's
	// role=alert, for a problem the user's last action caused. A notice
	// that does not announce is a plain status: a problem shown on arrival
	// or a neutral note.
	Announce string
}

// notice is Account.tsx's Notice: an icon, the text body and an optional
// action at the end.
func notice(c *ui.Context, o noticeOpts, body func(), action func()) {
	e := ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(12).Padding(12, 16).
		Radius(theme.Current().Radius["lg"]).Role(ui.RoleStatus)
	if o.Problem {
		e.Background(theme.C("raised")).Border(1, theme.C("control-line"))
	} else {
		e.Background(theme.C("hover"))
	}
	if o.Announce != "" {
		announced := ui.Local(e, "announced", func() bool { return false })
		if !*announced {
			*announced = true
			c.Announce(o.Announce)
		}
	}
	e.Children(func() {
		if o.Spin {
			ui.Spinner(c).Size(16, 16).Margin(2, 0, 0, 0).Shrink(0)
		} else {
			ui.Icon(c, theme.Icon(o.Icon)).FontSize(16).TextColor(theme.C(o.IconColor)).Margin(2, 0, 0, 0).Shrink(0)
		}
		ui.Column(c.Key("body")).Grow(1).MinWidth(0).Children(body)
		if action != nil {
			ui.Row(c.Key("action")).Shrink(0).AlignItems(ui.Center).Margin(-4, 0, -4, 0).Children(action)
		}
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

// placeholder is the 12rem busy surface shown while the status is unknown.
func placeholder(c *ui.Context, label string) ui.Element {
	return ui.Box(c).FillWidth().Height(192).Radius(theme.Current().Radius["lg"]).
		Background(theme.C("surface")).Role(ui.RoleStatus).Label(label)
}
