package app

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// view is AppShell: the sidebar and the top bar on the surface tone, the
// routed page on the ground with the modules' asides beside it and their
// overlays over it, the modules' bars across the bottom, and their layers
// over the whole window. Only the page area scrolls (each page owns its
// scroll container, which its virtualised lists need).
func (a *App) view(c *ui.Context) {
	theme.Apply(c)
	a.keys.dispatch(c)

	layerOpen, overlayOpen := a.layerOpen(), a.overlayOpen()

	root := ui.Column(c).Fill().Background(theme.C("ground")).IgnoreInput(layerOpen)
	root.Draw(func(*ui.Painter, ui.Rect) { a.paintOnce.Do(a.painted) })
	root.Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if a.sidebar {
				a.sidebarView(c)
				ui.Box(c).Width(1).FillHeight().Background(theme.C("line"))
			}
			a.mainColumn(c)
		})
		for _, m := range a.mods {
			if m.Slots.Bottom != nil {
				m.Slots.Bottom(c)
			}
		}
	})
	for _, m := range a.mods {
		if m.Slots.Layer != nil {
			ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Children(func() { m.Slots.Layer(c) })
		}
	}
	kit.OverlayLayer(c)
	a.afterFrame(c, layerOpen, overlayOpen)
}

// painted is the first frame drawn: the network opens (SystemService
// logs "startup: first paint").
func (a *App) painted() {
	if a.firstPaint != nil {
		a.firstPaint()
	}
}

// mainColumn is the top bar over the page area.
func (a *App) mainColumn(c *ui.Context) {
	ui.Column(c).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(func() {
		a.topBar(c)
		ui.Box(c).Height(1).FillWidth().Background(theme.C("line"))
		overlayOpen := a.overlayOpen()
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			// An open overlay covers the page, which keeps its place (and
			// its scroll) while it is hidden.
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Background(theme.C("ground")).Hidden(overlayOpen).Children(func() {
				a.page(c)
			})
			for _, m := range a.mods {
				if m.Slots.Aside != nil {
					m.Slots.Aside(c)
				}
			}
			for _, m := range a.mods {
				if o := m.Slots.Overlay; o != nil && o.Open != nil && o.Open() {
					ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(theme.C("ground")).Children(func() { o.Build(c) })
				}
			}
		})
	})
}

// page builds the route the router shows.
func (a *App) page(c *ui.Context) {
	a.router.View(c, func(r *ui.Route) {
		for _, rt := range a.routes {
			if !r.Match(rt.Pattern) {
				continue
			}
			if rt.Title != nil {
				r.Title(rt.Title(r))
			}
			rt.Build(c, r)
			return
		}
		// A path no module registered (a saved or linked page that is not
		// ported): the start page, not an empty window.
		if a.router.Path() != a.start {
			a.router.Replace(a.start)
		}
	})
}

// topBar is the top bar: history, the search a module provides, room
// to grab the window by, and the account menu a module provides.
func (a *App) topBar(c *ui.Context) {
	var search func(*ui.Context)
	for _, m := range a.mods {
		if m.Slots.TopBarSearch != nil {
			search = m.Slots.TopBarSearch
			break
		}
	}
	var account func(*ui.Context)
	for _, m := range a.mods {
		if m.Slots.TopBarAccount != nil {
			account = m.Slots.TopBarAccount
			break
		}
	}
	ui.Row(c).Height(theme.Current().Metric["top-bar"]).FillWidth().Background(theme.C("surface")).
		Padding(0, 16, 0, 12).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Gap(2).AlignItems(ui.Center).Children(func() {
			// MyGo's labels are English only.
			ui.BackButton(c, a.router).Label(i18n.T("Back")).Tooltip(i18n.T("Back"))
			ui.ForwardButton(c, a.router).Label(i18n.T("Forward")).Tooltip(i18n.T("Forward"))
		})
		if search != nil {
			ui.Row(c).MinWidth(240).Basis(520).Shrink(1).Grow(0).Children(func() { search(c) })
		}
		ui.Box(c).Grow(1).MinWidth(64).Height(1)
		if account != nil {
			account(c)
		}
	})
}

// sidebarView is the sidebar: the modules' links by section, their
// sections in the scrolling middle, their footers and footer links.
func (a *App) sidebarView(c *ui.Context) {
	tk := theme.Current()
	ui.Column(c).Width(tk.Metric["sidebar"]).FillHeight().Background(theme.C("surface")).
		Padding(0, 12, 12).Clip().Label(i18n.T("Main")).Role(ui.RoleGroup).Children(func() {
		a.brand(c)
		ui.Column(c).Gap(2).Margin(8, 0, 0, 0).Children(func() {
			for _, n := range a.nav[module.NavPrimary] {
				a.navLink(c, n)
			}
		})
		ui.Scroll(c).Grow(1).Basis(0).MinHeight(0).Children(func() {
			ui.Column(c).Padding(0, 0, 16).Children(func() {
				if len(a.nav[module.NavLibrary]) > 0 {
					text(c, i18n.T("Library"), "meta").TextColor(theme.C("ink-faint")).Padding(24, 10, 6)
					ui.Column(c).Gap(2).Children(func() {
						for _, n := range a.nav[module.NavLibrary] {
							a.navLink(c, n)
						}
					})
				}
				for _, m := range a.mods {
					if m.Slots.SidebarSection != nil {
						m.Slots.SidebarSection(c)
					}
				}
			})
		})
		ui.Column(c).Children(func() {
			for _, m := range a.mods {
				if m.Slots.SidebarFooter != nil {
					m.Slots.SidebarFooter(c)
				}
			}
			ui.Column(c).Gap(2).Children(func() {
				for _, n := range a.nav[module.NavFooter] {
					a.navLink(c, n)
				}
			})
		})
	})
}

// brand is the sidebar's header: the mark (a record label seen from above,
// amber with a spindle hole) and the wordmark.
func (a *App) brand(c *ui.Context) {
	ui.Row(c).Height(theme.Current().Metric["top-bar"]).Padding(0, 10).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Size(20, 20).Radius(10).Background(theme.C("amber")).Center().Children(func() {
			ui.Box(c).Size(4, 4).Radius(2).Background(theme.C("surface"))
		})
		text(c, i18n.T("Sona"), "heading").FontWeight(700).LetterSpacing(-0.4).TextColor(theme.C("ink"))
	})
}

// navLink is one sidebar link: active on its page (and below it unless
// Exact), with the icon at 16 and the label.
func (a *App) navLink(c *ui.Context, n module.NavItem) {
	label := n.Label()
	on := navActive(a.router.Path(), n)
	row := ui.ButtonBase(c.Key("nav:" + n.To)).Role(ui.RoleLink).Label(label).Height(theme.Current().Metric["nav-row"]).
		PaddingX(10).Gap(10).Justify(ui.Start).AlignItems(ui.Center).Radius(theme.Current().Radius["md"]).Cursor(ui.CursorPointer)
	fg := theme.C("ink-muted")
	switch {
	case on:
		row.Background(theme.C("active"))
		fg = theme.C("ink")
	case row.Hovered():
		row.Background(theme.C("hover"))
		fg = theme.C("ink")
	}
	if row.Clicked() {
		a.router.Push(n.To)
	}
	row.Children(func() {
		if n.Icon != "" {
			ui.Icon(c, theme.Icon(n.Icon)).FontSize(theme.Current().Metric["nav-icon"]).TextColor(fg)
		}
		t := text(c, label, "body").TextColor(fg).SingleLine().Shrink(1).MinWidth(0)
		if on {
			t.FontWeight(500)
		}
	})
}

// navActive is TanStack's active link: the page itself, or below it
// (a whole path segment) unless the item is Exact.
func navActive(path string, n module.NavItem) bool {
	if path == n.To {
		return true
	}
	return !n.Exact && n.To != "/" && strings.HasPrefix(path, strings.TrimSuffix(n.To, "/")+"/")
}

// text is a Text in one of the theme's type styles ("body", "meta"…).
func text(c *ui.Context, s, style string) ui.Element {
	t := theme.Current().Type[style]
	return ui.Text(c, s).FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}
