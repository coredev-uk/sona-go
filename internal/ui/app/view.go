package app

import (
	"strings"

	mygoi18n "github.com/egoist/mygo/i18n"
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
	dir := a.direction()
	c.Root().Direction(dir)
	a.swipe.RightToLeft = dir == ui.RightToLeft
	theme.Apply(c)
	glassReport(c, a.log)
	a.rememberRoute(c)

	overlayOpen := a.overlayGivesWay()
	layerOpen := a.layerOpen()
	if !layerOpen {
		a.keys.dispatch(c)
	}

	// The glass spike (kit/glass_darwin.go): the window's material shows
	// where the view draws no background (the root, the sidebar); the player
	// bar floats over the page, which scrolls under it.
	vibrant := c.Vibrancy() && kit.GlassOn("window")
	root := ui.Column(c).Fill().IgnoreInput(layerOpen).HandleInput(a.swipe.Input)
	if vibrant {
		c.Root().Background(ui.Transparent)
	} else {
		root.Background(theme.C("ground"))
	}
	endPass := a.env.Art.Pass(c)
	root.Draw(func(p *ui.Painter, _ ui.Rect) {
		a.paintOnce.Do(a.painted)
		endPass(p)
	})
	floatBar := kit.GlassOn("bars")
	root.Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if a.sidebar {
				a.sidebarView(c, vibrant)
				ui.Box(c).Width(1).FillHeight().Background(theme.C("line"))
			}
			a.mainColumn(c, vibrant)
		})
		for _, m := range a.mods {
			if m.Slots.Bottom == nil {
				continue
			}
			if !floatBar {
				m.Slots.Bottom(c)
				continue
			}
			// Floating over the page and the foot of the sidebar, inset from
			// the window's edges; the page and the sidebar leave room for it.
			edge := kit.GlassBarMargin()
			ui.Column(c.Key("glass-bottom:" + m.ID)).Absolute().Bottom(edge).Left(edge).Right(edge).Children(func() {
				m.Slots.Bottom(c)
			})
		}
	})
	dispatched := !layerOpen
	for _, m := range a.mods {
		if m.Slots.Layer == nil {
			continue
		}
		if m.Slots.LayerOpen == nil || !m.Slots.LayerOpen() {
			// A closed layer's wrapper takes no input: an input-taking box
			// over the page would swallow every pointer press.
			ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).IgnoreInput(true).Children(func() { m.Slots.Layer(c) })
			continue
		}
		// An open layer is a modal overlay: assistive technology sees it and
		// what is above it alone (the page behind is inert to it), and the
		// focus stays in it. The window's shortcuts wait outside a modal, so
		// they are read from inside it, before the layer's own keys. The
		// page is still built and drawn behind it, so the first paint is
		// reported as for any frame.
		ui.Overlay(c, func() {
			ui.Column(c.Key("layer:" + m.ID)).Absolute().Top(0).Left(0).Right(0).Bottom(0).Modal().Children(func() {
				if !dispatched {
					dispatched = true
					a.keys.dispatch(c)
				}
				m.Slots.Layer(c)
			})
		})
	}
	kit.OverlayLayer(c)
	if a.menubar != nil {
		a.menubar.frame()
	}
	a.afterFrame(c, layerOpen, overlayOpen)
}

// direction is the window's direction: right to left for a language written
// so (Arabic, Hebrew…, as the language shown, i18n.Locale, says), else left
// to right. Set on the root before anything is built, so every element, the
// layers and the popups inherit it.
// The answer is kept per language: parsing the tag every frame, for both
// windows, is work a frame can skip.
func (a *App) direction() ui.Direction {
	if loc := i18n.Locale(); a.dirLocale != loc {
		a.dirLocale, a.dir = loc, ui.LeftToRight
		if mygoi18n.Direction(loc) == mygoi18n.RightToLeft {
			a.dir = ui.RightToLeft
		}
	}
	return a.dir
}

// painted is the first frame drawn: the network opens (SystemService
// logs "startup: first paint").
func (a *App) painted() {
	if a.firstPaint != nil {
		a.firstPaint()
	}
}

// mainColumn is the top bar over the page area. With the glass spike's bars
// (kit/glass_darwin.go) the top bar floats over the page instead, which
// scrolls under it on a scroll edge.
func (a *App) mainColumn(c *ui.Context, vibrant bool) {
	floats := kit.GlassOn("bars")
	col := ui.Column(c).Grow(1).Basis(0).MinWidth(0).FillHeight()
	if vibrant {
		// The window's glass shows behind the sidebar alone.
		col.Background(theme.C("ground"))
	}
	col.Children(func() {
		if !floats {
			a.topBar(c)
			ui.Box(c).Height(1).FillWidth().Background(theme.C("line"))
			a.statusBanner(c)
		}
		overlayOpen := a.overlayOpen()
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			// An open overlay covers the page, which keeps its place (and
			// its scroll) while it is hidden.
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Background(theme.C("ground")).Hidden(overlayOpen).Children(func() {
				a.page(c)
			})
			for _, m := range a.mods {
				if m.Slots.Aside == nil {
					continue
				}
				if !floats {
					m.Slots.Aside(c)
					continue
				}
				top, bottom := kit.GlassInsets()
				ui.Column(c.Key("glass-aside:"+m.ID)).Shrink(0).FillHeight().Padding(top, 0, bottom).Background(theme.C("ground")).Children(func() {
					m.Slots.Aside(c)
				})
			}
			for _, m := range a.mods {
				if o := m.Slots.Overlay; o != nil && o.Open != nil && o.Open() {
					ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Background(theme.C("ground")).Children(func() { o.Build(c) })
				}
			}
		})
		if floats {
			kit.GlassTopEdge(c, theme.Current().Metric["top-bar"])
			ui.Column(c.Key("glass-top")).Absolute().Top(0).Left(0).Right(0).Children(func() {
				a.topBar(c)
				a.statusBanner(c)
			})
		}
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
	// A hidden title bar: the top bar is the window's, dragging it, and
	// keeps clear of the window buttons; the sidebar's header takes the
	// leading ones while the sidebar shows (macOS's, Linux's on the left).
	lead, trail := titleBarRoom(c)
	start := float32(12)
	if !a.sidebar {
		start = max(start, lead+8)
	}
	row := ui.Row(c).Height(theme.Current().Metric["top-bar"]).FillWidth().
		PaddingStart(start).PaddingEnd(max(16, trail+8)).Gap(12).AlignItems(ui.Center)
	switch {
	case !kit.GlassOn("bars"):
		row.Background(theme.C("surface"))
	case kit.GlassTopStrip():
		kit.GlassPane(row, "bars", 0)
	}
	if a.drawnTitleBar {
		row.DragWindow()
	}
	row.Children(func() {
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
func (a *App) sidebarView(c *ui.Context, vibrant bool) {
	tk := theme.Current()
	side := ui.Column(c).Width(tk.Metric["sidebar"]).FillHeight()
	if !vibrant {
		side.Background(theme.C("surface"))
	}
	_, barRoom := kit.GlassInsets() // the floating player bar covers the sidebar's foot
	side.Padding(0, 12, 12+barRoom).Clip().Label(i18n.T("Main")).Role(ui.RoleGroup).Children(func() {
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

// titleBarRoom is the room the window buttons take at the start and at the
// end of the window's top edge: the title bar's physical left and right,
// swapped in a right-to-left window.
func titleBarRoom(c *ui.Context) (lead, trail float32) {
	bar := c.TitleBar()
	if c.Direction() == ui.RightToLeft {
		return bar.Right, bar.Left
	}
	return bar.Left, bar.Right
}

// brand is the sidebar's header: the mark (a record label seen from above,
// amber with a spindle hole) and the wordmark.
func (a *App) brand(c *ui.Context) {
	lead, _ := titleBarRoom(c)
	row := ui.Row(c).Height(theme.Current().Metric["top-bar"]).PaddingStart(max(10, lead-4)).PaddingEnd(10).Gap(8).AlignItems(ui.Center)
	if a.drawnTitleBar {
		row.DragWindow()
	}
	row.Children(func() {
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
		row.Current(ui.CurrentPage)
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
			theme.Glyph(c, n.Icon, theme.Current().Metric["nav-icon"], theme.GlyphOpts{Color: fg})
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
