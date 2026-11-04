package settings

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// maxResults is how many results Find shows at once; a longer query
// narrows the rest.
const maxResults = 12

// findState is Find a setting's field and what it announced last.
type findState struct {
	query string
	// focus asks the next frame to focus a result (0..n-1) or the field
	// (focusField); focusNone is no request.
	focus  int
	status string
}

const (
	focusNone  = -1
	focusField = -2
)

// findPanel is FindSetting: the page's title and the field, then the list
// (the section list, shown while the field is empty) or the settings the
// query finds (titles, groups, sections and a few other words for each);
// choosing one opens its section and puts the focus on its control. Down
// goes from the field into the results, Up and Down, Home and End move
// between them, Enter in the field takes the first, Escape clears the
// query. The count is announced.
func (s *settings) findPanel(c *ui.Context, narrow bool, list func()) {
	f := &s.find
	q := f.query
	var found []settingEntry
	if trimmed := trim(q); trimmed != "" {
		st, caps := s.cfg(), s.caps()
		ctx := findContext{languageRow: s.languageRow()}
		if st.HasData {
			ctx.settings = &st.Data
		}
		if caps.HasData {
			ctx.caps = &caps.Data
		}
		found = findSettings(settingIndex(s.shortcuts()), trimmed, ctx, i18n.Locale())
	}
	shown := found
	if len(shown) > maxResults {
		shown = shown[:maxResults]
	}
	status := ""
	switch {
	case trim(q) == "":
	case len(found) > 0:
		status = i18n.T("{count, plural, one {# setting found} other {# settings found}}", "count", len(found))
	default:
		status = i18n.T("No setting matches “{q}”.", "q", trim(q))
	}
	if status != f.status {
		f.status = status
		if status != "" {
			c.Announce(status)
		}
	}

	pick := func(e settingEntry) {
		f.query = ""
		f.focus = focusNone
		if e.Section != s.last {
			s.last = e.Section
			s.env.Nav.Replace(settingsPath + "?section=" + e.Section)
		}
		s.focusSetting(e.ID)
	}

	ui.Column(c.Key("find")).FillWidth().Gap(12).Children(func() {
		var head ui.Element
		if narrow {
			head = ui.Row(c.Key("head")).FillWidth().Gap(20).AlignItems(ui.Center).Justify(ui.SpaceBetween)
		} else {
			head = ui.Column(c.Key("head")).FillWidth().Gap(20).Margin(0, 0, 8, 0)
		}
		var field ui.Element
		head.Children(func() {
			text(c, i18n.T("Settings"), "page").TextColor(theme.C("ink")).Role(ui.RoleHeading).Level(1)
			w := float32(0)
			if narrow {
				w = 288
			}
			field = kit.SearchField(c, &f.query, kit.SearchFieldOpts{
				Placeholder: i18n.T("Find a setting"), A11yLabel: i18n.T("Find a setting"), Width: w,
			})
		})
		if f.focus == focusField {
			field.Focus()
			if field.Focused() {
				f.focus = focusNone
			}
		}
		if len(shown) > 0 {
			switch {
			case field.Shortcut(0, ui.KeyDown):
				f.focus = 0
			case field.Submitted():
				pick(shown[0])
			}
		}

		switch {
		case trim(q) == "":
			list()
		case len(shown) == 0:
			text(c, status, "secondary").TextColor(theme.C("ink-muted")).Padding(6, 12)
		default:
			s.results(c, shown, pick)
		}
	})
}

// results is the list of settings found.
func (s *settings) results(c *ui.Context, shown []settingEntry, pick func(settingEntry)) {
	f := &s.find
	ui.Column(c.Key("results")).FillWidth().Role(ui.RoleList).Label(i18n.T("Settings found")).Children(func() {
		for i, e := range shown {
			section := sectionTitle(e.Section)
			where := section
			if e.Group != section {
				where = section + " · " + e.Group
			}
			b := ui.ButtonBase(c.Key("r:"+e.ID)).Label(e.Title).Description(where).FillWidth().
				Radius(theme.Current().Radius["md"]).Padding(6, 12).AlignItems(ui.Start).Cursor(ui.CursorPointer)
			if f.focus == i {
				b.Focus()
				if b.Focused() {
					f.focus = focusNone
				}
			}
			if b.Hovered() || b.Focused() {
				b.Background(theme.C("hover"))
			}
			switch {
			case b.Clicked():
				pick(e)
			case b.Shortcut(0, ui.KeyDown):
				f.focus = min(i+1, len(shown)-1)
			case b.Shortcut(0, ui.KeyUp):
				f.focus = i - 1
				if i == 0 {
					f.focus = focusField
				}
			case b.Shortcut(0, ui.KeyHome):
				f.focus = 0
			case b.Shortcut(0, ui.KeyEnd):
				f.focus = len(shown) - 1
			case b.Shortcut(0, ui.KeyEscape):
				f.query = ""
				f.focus = focusField
			}
			b.Children(func() {
				ui.Column(c.Key("c:" + e.ID)).Shrink(1).MinWidth(0).Children(func() {
					text(c, e.Title, "body").TextColor(theme.C("ink"))
					text(c, where, "meta").TextColor(theme.C("ink-muted"))
				})
			})
		}
	})
}
