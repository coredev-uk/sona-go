package auth

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/account"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

const (
	// dialogWidth is the panel at rest: the lead (leadWidth), its 1 px edge
	// and the account's panel (399 wide, 327 inside its 36 padding).
	dialogWidth = 680
	// leadWidth is the leading panel's width beside the account.
	leadWidth = 280
	// stackBelow is the panel width under which the lead sits above the
	// account instead of beside it: the account keeps at least 287 inside
	// its padding. The window is never narrower than 940, so this is for
	// a window the shell lets shrink.
	stackBelow = 640
	// windowMargin is the room kit.Dialog keeps beside and above the panel:
	// the window's 2 rem margins.
	windowMargin = 32
	// minBody keeps the panel in sight in a very short window.
	minBody = 120
)

// leadLines are what signing in adds, one glyph and one short line each.
// The words are the lead's, whatever the account's state.
func leadLines() []struct{ icon, text string } {
	return []struct{ icon, text string }{
		{"list-music", i18n.T("Your library, in sync")},
		{"sparkles", i18n.T("Recommendations for you")},
		{"volume-2", i18n.T("Whole songs, not previews")},
	}
}

// dialog is AccountDialog: the account section in a dialog, open while
// Surfaces says so. Signing in or the account's settings (account.Section),
// opened from the avatar's menu, the palette and every "Sign in" action. It
// stays open after a sign-in, which then shows the account it reached. The
// panel is two parts, the lead (the mark, a heading and what signing in
// adds, on the surface tone) and the account's (the sign-in form or the
// signed-in view); a narrow window stacks them. The whole panel scrolls
// inside the window, so the lead's tone runs as long as the form; when it
// opens, the sign-in form's Apple Account field takes the focus.
func (f *feature) dialog(c *ui.Context) {
	open := f.env.Surfaces.AccountOpen()
	if open && !f.shown {
		f.section.FocusAccount()
	}
	f.shown = open
	w, h := c.Size()
	stacked := min(dialogWidth, w-windowMargin) < stackBelow
	kit.Dialog(c, &open, kit.DialogOpts{Title: i18n.T("Account"), HideHeader: true, Bare: true, MaxWidth: dialogWidth}, func() {
		ui.Scroll(c.Key("auth:body")).FillWidth().MaxHeight(max(h-windowMargin, minBody)).Children(func() {
			var layout ui.Element
			if stacked {
				layout = ui.Column(c.Key("auth:panels"))
			} else {
				layout = ui.Row(c.Key("auth:panels")).AlignItems(ui.Stretch)
			}
			layout.FillWidth().Children(func() {
				f.lead(c.Key("auth:lead"), stacked)
				edge := ui.Box(c.Key("auth:edge")).Shrink(0).Background(theme.C("line")).Role(ui.RoleNone)
				if stacked {
					edge.Height(1).FillWidth()
				} else {
					edge.Width(1)
				}
				f.account(c.Key("auth:account"), stacked)
			})
		})
	})
	if !open && f.shown {
		f.shown = false
		f.section.Reset()
		f.env.Surfaces.CloseAccount()
	}
}

// lead is the dialog's leading panel: the Sona mark, and under it the one
// heading and the three lines on what signing in adds. Beside the account it
// is as tall as the account's panel, the mark at the top and the words at the
// bottom; stacked it is a short band above it.
func (f *feature) lead(c *ui.Context, stacked bool) {
	col := ui.Column(c).Background(theme.C("surface")).Gap(24)
	if stacked {
		col.FillWidth().Padding(36)
	} else {
		col.Width(leadWidth).Shrink(0).Padding(36)
	}
	col.Children(func() {
		ui.Row(c.Key("brand")).AlignItems(ui.Center).Gap(8).Children(func() {
			// The Sona mark: a record label seen from above, amber with a
			// spindle hole, as the sidebar draws it.
			ui.Row(c.Key("mark")).Size(22, 22).Shrink(0).Radius(11).Background(theme.C("amber")).Center().Children(func() {
				ui.Box(c.Key("hole")).Size(5, 5).Radius(2.5).Background(theme.C("surface"))
			})
			leadText(c.Key("name"), i18n.T("Sona"), "heading", "ink").FontWeight(700).LetterSpacing(-0.4)
		})
		if !stacked {
			ui.Box(c.Key("space")).Grow(1).MinHeight(0)
		}
		ui.Column(c.Key("words")).FillWidth().Gap(20).Children(func() {
			leadText(c.Key("heading"), i18n.T("Your Apple Music, on your desktop"), "title", "ink").Role(ui.RoleHeading).Level(2)
			ui.Column(c.Key("lines")).FillWidth().Gap(12).Children(func() {
				for _, l := range leadLines() {
					ui.Row(c.Key("line:" + l.icon)).FillWidth().AlignItems(ui.Start).Gap(12).Children(func() {
						theme.Glyph(c.Key("icon"), l.icon, 16, theme.GlyphOpts{Color: theme.C("ink-muted")}).Margin(2, 0, 0, 0)
						leadText(c.Key("text"), l.text, "body", "ink").Grow(1).MinWidth(0)
					})
				}
			})
		})
	})
}

// account is the trailing panel: the account section. Signed in it has no
// title of its own, so the panel gives it one; the sign-in flow titles
// itself.
func (f *feature) account(c *ui.Context, stacked bool) {
	res := account.UseStatus(f.d)
	col := ui.Column(c).MinWidth(0).Padding(36)
	if stacked {
		col.FillWidth()
	} else {
		col.Grow(1).Basis(0)
	}
	col.Children(func() {
		if res.HasData && res.Err == nil && account.HasAccount(res.Data.State) {
			leadText(c.Key("title"), i18n.T("Account"), "heading", "ink").Role(ui.RoleHeading).Level(3).Margin(0, 0, 20, 0)
		}
		ui.Column(c.Key("section")).FillWidth().Children(func() {
			f.section.Build(c, false)
		})
	})
}

// leadText is a Text in one of the theme's type styles and colour tokens.
func leadText(c *ui.Context, s, style, color string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).
		LetterSpacing(t.Tracking).TextColor(theme.C(color))
}
