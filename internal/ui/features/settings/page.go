package settings

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// The page's geometry (Settings.tsx): a 240 DIP index column and content up
// to 800 DIP, below 960 DIP of window width the index wraps above the
// content, and the index is held in view only where the window is also
// 700 DIP tall.
const (
	indexWidth   = 240
	contentWidth = 800
	indexGap     = 48
	narrowWidth  = 960
	stickyHeight = 700
	padTop       = 28
	padBottom    = 48
)

// page is SettingsPage: the index (title, Find a setting, the section list)
// and the section, in a scroll container. Choosing a section replaces the
// history entry, so Back leaves Settings instead of stepping through the
// sections. A /settings with no section (Ctrl+,, the sidebar, the command
// palette) opens the section shown last in this run.
func (s *settings) page(c *ui.Context, r *ui.Route) {
	section := r.Query("section")
	if section == "account" {
		// The retired Account section: the account lives in the top bar's
		// menu now; an old link opens its dialog over Settings.
		s.env.Post(func() {
			s.env.Surfaces.OpenAccount()
			s.env.Nav.Replace(settingsPath)
		})
		section = s.last
	}
	if !slices.Contains(sections, section) {
		section = s.last
	}
	if section != s.last {
		s.pos = ui.ScrollState{} // another section: from its top
	}
	s.last = section

	// A Find landing waits for its row for three seconds, then gives up.
	if p := s.pending; p != nil {
		if left := p.until.Sub(s.now()); left <= 0 {
			s.pending = nil
		} else {
			c.After(left)
		}
	}

	w, h := c.Size()
	narrow := w <= narrowWidth
	sticky := !narrow && h >= stickyHeight
	tk := theme.Current()
	gutter := tk.Metric["gutter"]
	if narrow {
		gutter = tk.Metric["gutter-narrow"]
	}
	scrollbar := tk.Metric["scrollbar"]

	index := func() { s.index(c, section, narrow) }
	content := func() { s.content(c, section, narrow) }

	devstate.Scroll(c, &s.pos)
	switch {
	case sticky:
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).MinWidth(0).Padding(padTop, 0, 0, gutter).Gap(indexGap).Children(func() {
			// The index scrolls on its own: Find's results are longer than
			// the column at 700-810 DIP of height.
			ui.Scroll(c.Key("index")).Width(indexWidth).Shrink(0).MinHeight(0).FillHeight().Children(func() {
				ui.Column(c).FillWidth().Padding(0, 0, padBottom, 0).Children(index)
			})
			ui.Scroll(c.Key("section")).TrackScroll(&s.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0).FillHeight().Children(func() {
				ui.Column(c).FillWidth().MaxWidth(contentWidth+gutter+scrollbar).Padding(0, gutter+scrollbar, padBottom, 0).Children(content)
			})
		})
	case narrow:
		ui.Scroll(c).TrackScroll(&s.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0).Children(func() {
			ui.Column(c).Padding(padTop, gutter+scrollbar, padBottom, gutter).Gap(24).Children(func() {
				index()
				content()
			})
		})
	default:
		ui.Scroll(c).TrackScroll(&s.pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0).Children(func() {
			ui.Row(c).Padding(padTop, gutter+scrollbar, padBottom, gutter).Gap(indexGap).AlignItems(ui.Start).Children(func() {
				ui.Column(c.Key("index")).Width(indexWidth).Shrink(0).Children(index)
				ui.Column(c.Key("section")).Grow(1).Shrink(1).MinWidth(0).MaxWidth(contentWidth).Children(content)
			})
		})
	}
}

// content is the section: a region named by its h2 (the name only a screen
// reader hears below 960 DIP, where the wrapped list names it).
func (s *settings) content(c *ui.Context, section string, narrow bool) {
	title := sectionTitle(section)
	ui.Column(c.Key("content:" + section)).FillWidth().Role(ui.RoleGroup).Label(title).Children(func() {
		if narrow {
			text(c, title, "page").Size(0, 0).Opacity(0).Role(ui.RoleHeading).Level(2) // sr-only
		} else {
			text(c, title, "page").TextColor(theme.C("ink")).Margin(0, 0, 24, 0).Role(ui.RoleHeading).Level(2)
		}
		switch section {
		case secGeneral:
			s.general(c)
		case secPlayback:
			s.playback(c)
		case secAppearance:
			s.appearance(c)
		case secConnections:
			s.connections(c)
		case secShortcuts:
			s.shortcutsSection(c)
		case secStorage:
			s.storage(c)
		case secDiagnostics:
			s.diagnostics(c)
		}
	})
}

// index is the page's index column: its title, Find a setting and the
// section list (or, while the field holds a query, what it found).
func (s *settings) index(c *ui.Context, section string, narrow bool) {
	s.findPanel(c, narrow, func() { s.sectionList(c, section, narrow) })
}

// sectionList is the nav of sections: each section's name over its state in
// one line (summaries.go). Below 960 DIP it wraps horizontally and the
// summary is for assistive technology only.
func (s *settings) sectionList(c *ui.Context, section string, narrow bool) {
	sums := s.summaries()
	var list ui.Element
	if narrow {
		list = ui.Row(c.Key("sections")).FillWidth().Gap(2).Wrap()
	} else {
		list = ui.Column(c.Key("sections")).FillWidth().Gap(2)
	}
	list.Role(ui.RoleList).Label(i18n.T("Settings sections"))
	list.Children(func() {
		for _, id := range sections {
			on := id == section
			title, sum := sectionTitle(id), sums[id]
			b := ui.ButtonBase(c.Key("nav:"+id)).Role(ui.RoleLink).Label(title).Radius(theme.Current().Radius["md"]).
				MinHeight(32).Padding(6, 12).Justify(ui.Start).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if narrow {
				b.PaddingX(10)
			} else {
				b.FillWidth()
			}
			if sum != "" {
				b.Description(sum)
			}
			fg := theme.C("ink-muted")
			switch {
			case on:
				b.Background(theme.C("active"))
				fg = theme.C("ink")
			case b.Hovered():
				b.Background(theme.C("hover"))
				fg = theme.C("ink")
			}
			if b.Clicked() && !on {
				s.env.Nav.Replace(settingsPath + "?section=" + id)
			}
			b.Children(func() {
				ui.Column(c.Key(id)).Shrink(1).MinWidth(0).Children(func() {
					text(c, title, "label").TextColor(fg)
					if sum != "" && !narrow {
						text(c, sum, "meta").TextColor(theme.C("ink-muted")).MaxLines(2)
					}
				})
			})
		}
	})
}
