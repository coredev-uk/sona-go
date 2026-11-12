package settings

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The gallery's cards: one theme each, as a picture of Sona's window drawn
// from the theme's own tokens, its name and a line under it.
const (
	themeCardWidth = 168
	themeCardGap   = 12
	// themeProblemsShown is how many of a refused file's problems the card
	// lists; the rest are counted.
	themeProblemsShown = 2
)

// themeAPI is what the Theme group asks of the themes service: where the
// files go, and opening that folder. Tests replace it (themeSeam).
type themeAPI interface {
	Folder() string
	OpenFolder() error
}

type themeSeamState struct{ api themeAPI }

// themeService is the themes service, or the seam a test installed.
func (s *settings) themeService() themeAPI {
	if st := stateOf[themeSeamState](s, "seam:themes"); st.api != nil {
		return st.api
	}
	return services.Service[services.ThemesService](s.env.Core)
}

// themeName is a theme's name in the user's language where Sona wrote the
// theme (the built-ins are named in the catalogue), else as its file says.
func themeName(t *theme.Theme) string {
	if !t.Builtin {
		return t.Name
	}
	switch t.ID {
	case "sona":
		return i18n.T("Sona")
	case "graphite":
		return i18n.T("Graphite")
	case "oled":
		return i18n.T("OLED")
	case "contrast":
		return i18n.T("High contrast")
	case "paper":
		return i18n.T("Paper")
	}
	return t.Name // the community palettes' own names
}

// sonaThemes are the built-ins Sona made: the board is a neutral grey and the
// cover is the colour. The other built-ins are community palettes, which keep
// their own hues.
var sonaThemes = map[string]bool{"sona": true, "graphite": true, "paper": true, "contrast": true, "oled": true}

// themeHint is the line under a theme's name, what it looks like, and, when
// it has one look only, the line that says so.
func themeHint(t *theme.Theme) (what, only string) {
	if t.Builtin {
		switch t.ID {
		case "sona":
			what = i18n.T("Soft grey, amber")
		case "graphite":
			what = i18n.T("Cool grey, sky blue")
		case "oled":
			what = i18n.T("True black, lime")
		case "contrast":
			what = i18n.T("Text at 7:1, strong lines")
		case "paper":
			what = i18n.T("Warm grey, teal")
		case "catppuccin":
			what = i18n.T("Latte and Mocha, peach")
		case "nord":
			what = i18n.T("Snow Storm and Polar Night, frost")
		case "gruvbox":
			what = i18n.T("Retro groove, yellow")
		case "solarized":
			what = i18n.T("Precision colours, yellow")
		case "dracula":
			what = i18n.T("Purple night, pink")
		case "rose-pine":
			what = i18n.T("Dawn and night, gold")
		case "tokyo-night":
			what = i18n.T("Day and Night, orange")
		}
	} else if t.Author != "" {
		what = i18n.T("By {author}", "author", t.Author)
	} else {
		what = i18n.T("Your theme")
	}
	if dark, pinned := t.Pinned(); pinned {
		only = i18n.T("Light only")
		if dark {
			only = i18n.T("Dark only")
		}
	}
	return what, only
}

// problemText words one problem of a refused theme file.
func problemText(p theme.Problem) string {
	var text string
	switch p.Kind {
	case theme.ProblemSyntax:
		text = i18n.T("Not a valid theme file: {detail}", "detail", p.Detail)
	case theme.ProblemFormat:
		text = i18n.T("Format {format} is not one Sona reads", "format", p.Detail)
	case theme.ProblemID:
		text = i18n.T("The id has to be 1 to 40 of a to z, 0 to 9 and -")
	case theme.ProblemIDTaken:
		text = i18n.T("Another theme already has this id")
	case theme.ProblemName:
		text = i18n.T("The name has to be 1 to 40 characters")
	case theme.ProblemNoVariant:
		text = i18n.T("It needs a light variant, a dark variant or both")
	case theme.ProblemTooLarge:
		text = i18n.T("The file is larger than {size} KiB", "size", services.MaxThemeFileBytes>>10)
	case theme.ProblemUnreadable:
		text = i18n.T("The file can't be read")
	case theme.ProblemUnknownToken:
		text = i18n.T("{token} is not a {group} token", "token", p.Token, "group", p.Detail)
	case theme.ProblemBadValue:
		if p.Token == "" {
			text = p.Detail
		} else {
			text = i18n.T("{token}: {detail}", "token", p.Token, "detail", p.Detail)
		}
	case theme.ProblemContrast:
		text = i18n.T("{token} on {other} has {got}:1, it needs {want}:1",
			"token", p.Token, "other", p.Other, "got", fmt.Sprintf("%.2f", p.Got), "want", fmt.Sprintf("%.1f", p.Want))
	case theme.ProblemRange:
		text = i18n.T("{token} is {got}, it needs to be at least {want}",
			"token", p.Token, "got", fmt.Sprintf("%.2f", p.Got), "want", fmt.Sprintf("%.2f", p.Want))
	case theme.ProblemPlayRole:
		text = i18n.T("{token} has to follow the spot colour", "token", p.Token)
	case theme.ProblemSpotRole:
		text = i18n.T("{token} can't be the spot colour: it only marks what plays", "token", p.Token)
	}
	switch p.Mode {
	case "light":
		return i18n.T("Light: {text}", "text", text)
	case "dark":
		return i18n.T("Dark: {text}", "text", text)
	}
	return text
}

// problemsText is a refused file's first problems in one paragraph, the rest
// counted.
func problemsText(ps []theme.Problem) string {
	var parts []string
	for _, p := range ps[:min(len(ps), themeProblemsShown)] {
		parts = append(parts, problemText(p))
	}
	text := strings.Join(parts, ". ")
	if more := len(ps) - themeProblemsShown; more > 0 {
		text += " " + i18n.T("and {count} more", "count", more)
	}
	return text
}

// themeGallery is the choice of theme as cards, all in view at once: radio
// buttons, so Tab reaches the chosen one and the arrow keys choose (each
// choice saves at once). The built-ins come first, then the user's files:
// a valid file as a card of its own, a refused one as a disabled card that
// says why, a refused edit of a good file as the good version's card with a
// note. It returns the group, which Find focuses.
func (s *settings) themeGallery(c *ui.Context, disabled bool) ui.Element {
	active := theme.Active()
	all := theme.Themes()
	entries := theme.UserEntries()
	byFile := map[string]theme.UserEntry{}
	for _, e := range entries {
		byFile[e.File] = e
	}
	// A pinned theme draws its own look; the others the appearance now.
	dark := theme.Dark()
	sel, changed := active.ID, false
	card := func(t *theme.Theme, hint, only, description string) {
		if s.themeCard(c, &sel, t, hint, only, description, dark, disabled) {
			changed = true
		}
	}
	group := ui.RadioGroup(c.Key("themes"), func() {
		heading := func(key, text string, first bool) {
			top := float32(12)
			if first {
				top = 0
			}
			ui.Column(c.Key(key)).FillWidth().Padding(top, 0, 0, 0).Children(func() {
				theme.Text(c, text, "label").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(4)
			})
		}
		heading("sona", i18n.T("Sona's own"), true)
		for _, t := range all {
			if t.Builtin && sonaThemes[t.ID] {
				hint, only := themeHint(t)
				card(t, hint, only, "")
			}
		}
		heading("community", i18n.T("Community palettes"), false)
		for _, t := range all {
			if t.Builtin && !sonaThemes[t.ID] {
				hint, only := themeHint(t)
				card(t, hint, only, "")
			}
		}
		if len(entries) > 0 {
			heading("yours", i18n.T("Your themes"), false)
		}
		for _, t := range all {
			if t.Builtin {
				continue
			}
			hint, only := themeHint(t)
			description := ""
			if e := byFile[t.File]; e.Kept {
				hint = i18n.T("Your last edit was refused. This earlier version stays.")
				description = problemsText(e.Problems)
			}
			card(t, hint, only, description)
		}
		for _, e := range entries {
			if e.Theme == nil {
				s.refusedCard(c, e)
			}
		}
	}).Row().Wrap().AlignItems(ui.Stretch).FillWidth().Gap(themeCardGap).Label(i18n.T("Theme"))
	if changed && sel != active.ID {
		next := sel
		s.save(func(n *config.Settings) { n.ThemeID = next })
	}
	return group
}

// themeCard is one theme: its picture, its name (with a check when it is the
// chosen one, which the ring says too: the ring alone would leave the choice
// to colour) and its line. It reports a click or key that chose it.
func (s *settings) themeCard(c *ui.Context, sel *string, t *theme.Theme, hint, only, description string, dark, disabled bool) bool {
	tk := theme.Current()
	checked := *sel == t.ID
	name := themeName(t)
	spoken := i18n.Meta(hint, only)
	if description != "" {
		spoken += ". " + description
	}
	r := ui.RadioBase(c.Key("card:"+t.ID), sel, t.ID).Width(themeCardWidth).Padding(8).Column().Gap(8).AlignItems(ui.Stretch).
		Radius(tk.Radius["lg"]).Background(theme.C("surface")).Label(name).Description(spoken).Cursor(ui.CursorPointer)
	switch {
	case disabled:
		r.Disabled(true).Opacity(0.5)
	case r.Hovered():
		r.Background(theme.C("hover"))
	}
	if checked {
		r.Shadow(0, 0, 0, 2, theme.C("ink"))
	}
	r.Children(func() {
		look := dark
		if pd, pinned := t.Pinned(); pinned {
			look = pd
		}
		kit.ThemePreview(c.Key("picture"), t.Tokens(look))
		ui.Column(c.Key("words")).FillWidth().Padding(0, 2, 2, 2).Gap(2).Children(func() {
			ui.Row(c.Key("name")).Gap(4).AlignItems(ui.Center).Children(func() {
				if checked {
					ui.Icon(c.Key("check"), theme.Icon("check")).FontSize(14).TextColor(theme.C("ink")).Shrink(0)
				}
				weight := 500
				if checked {
					weight = 600
				}
				theme.Text(c, name, "ui").FontWeight(weight).TextColor(theme.C("ink")).SingleLine().Shrink(1).MinWidth(0)
			})
			theme.Text(c.Key("hint"), hint, "secondary").TextColor(theme.C("ink-muted"))
			if only != "" {
				theme.Text(c.Key("only"), only, "secondary").TextColor(theme.C("ink-muted"))
			}
			if description != "" {
				theme.Text(c.Key("problems"), description, "secondary").TextColor(theme.C("ink-muted"))
			}
		})
	})
	return r.Changed()
}

// refusedCard is a theme file that was refused and has no earlier good
// version: a disabled card with the file's name and why it can't be used.
func (s *settings) refusedCard(c *ui.Context, e theme.UserEntry) {
	text := problemsText(e.Problems)
	none := ""
	r := ui.RadioBase(c.Key("refused:"+e.File), &none, "refused:"+e.File).FillWidth().Padding(12).Gap(10).AlignItems(ui.Start).
		Radius(theme.Current().Radius["lg"]).Background(theme.C("danger-wash")).Border(1, theme.C("danger-line")).
		Label(e.File).Description(text).Disabled(true)
	r.Children(func() {
		ui.Icon(c.Key("icon"), theme.Icon("triangle-alert")).FontSize(16).TextColor(theme.C("danger")).Shrink(0).Margin(2, 0, 0, 0)
		ui.Column(c.Key("words")).Grow(1).Shrink(1).MinWidth(0).Children(func() {
			theme.Text(c, e.File, "ui").FontWeight(500).TextColor(theme.C("ink")).SingleLine()
			theme.Text(c.Key("problems"), text, "secondary").TextColor(theme.C("ink"))
		})
	})
}

// themeFallbackNote says that the chosen theme is not there: its file is gone
// or refused, and Sona's own shows until it is back.
func themeFallbackNote(c *ui.Context) {
	if want := theme.Requested(); want != theme.Active().ID {
		ui.Row(c.Key("fallback")).Gap(6).AlignItems(ui.Start).Margin(6, 0, 0, 0).MaxWidth(rowTextWidth).Role(ui.RoleStatus).Children(func() {
			ui.Icon(c.Key("icon"), theme.Icon("circle-alert")).FontSize(14).TextColor(theme.C("ink-muted")).Margin(2, 0, 0, 0).Shrink(0)
			theme.Text(c, i18n.T("The theme “{id}” isn't available, so Sona's own is showing. It comes back with its file.", "id", want), "secondary").
				TextColor(theme.C("ink-muted")).Shrink(1).MinWidth(0)
		})
	}
}

// themesFolderRow says where theme files go and opens the folder.
func (s *settings) themesFolderRow(c *ui.Context) {
	folder := s.themeService().Folder()
	s.row(c, rowOpts{
		ID: "themes-folder", Title: i18n.T("Your own themes"),
		Desc: i18n.T("Put a theme file, a .json, in {folder}. It shows here, and an edit shows as you save it.", "folder", folder), Ink: folder,
	}, func() ui.Element {
		b := kit.Button(c.Key("open"), i18n.T("Open themes folder"), kit.ButtonOpts{Variant: kit.Outline, Size: kit.SizeSM, Icon: "folder-open"})
		if b.Clicked() {
			s.openThemesFolder()
		}
		return b
	})
}

// openThemesFolder opens the themes folder, which the service creates when it
// is not there yet; failure is a toast.
func (s *settings) openThemesFolder() {
	api := s.themeService()
	go func() {
		err := api.OpenFolder()
		s.env.Post(func() {
			if err != nil {
				kit.ToastErrorf(i18n.T("Couldn't open the themes folder"), i18n.ErrorMessage(err))
			}
		})
	}()
}
