// Command uigallery shows every component of the native UI's kit in the
// look of the theme, light and dark, with knobs to try each one: the place
// to build and review a component without the app around it.
//
//	devenv shell -- gallery                 # watches the sources (cmd/devloop)
//	go run -tags dev,sqlite_fts5 ./cmd/uigallery
//
// Under cmd/devloop (the gallery script) a save restarts it on the section
// it was on, in the same mode and window, and edits to design tokens show
// without a restart.
//
// A kit component adds its section in a file of its own, cmd/uigallery/<area>.go:
//
//	func init() {
//		register(section{name: "Tabs", build: func(c *ui.Context, g *gallery) { … }})
//	}
//
// Sections list in the sidebar by name. g carries the mode (dark) and the
// artwork loader (art) that serves a generated cover for any src.
package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/devstate"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// section is a page of the gallery: the components of one kit area.
type section struct {
	name  string
	build func(c *ui.Context, g *gallery)
}

var sections []section

// register adds a section; call it from an init in the area's file.
func register(s section) {
	if s.name == "" || s.build == nil {
		panic("uigallery: a section needs a name and a build func")
	}
	if slices.ContainsFunc(sections, func(o section) bool { return o.name == s.name }) {
		panic("uigallery: two sections named " + s.name)
	}
	sections = append(sections, s)
}

// gallery is the state sections build against.
type gallery struct {
	// dark is the mode the toolbar's switch chose; the theme follows it.
	dark bool
	// art serves a generated cover for any src.
	art *kit.ArtLoader

	win      *mygo.Window
	log      *slog.Logger
	selected string
	scale    float64
	scrolls  map[string]*ui.ScrollState
	once     sync.Once
}

func main() {
	slices.SortFunc(sections, func(a, b section) int { return strings.Compare(a.name, b.name) })
	g := &gallery{
		log:      slog.New(slog.NewTextHandler(os.Stderr, nil)),
		selected: sections[0].name,
		scale:    1,
		scrolls:  map[string]*ui.ScrollState{},
	}
	mygo.App.SetName("Sona UI Gallery")
	mygo.App.WhenReady(g.open)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// open makes the window; it runs on the main thread once MyGo is ready.
func (g *gallery) open() {
	if err := theme.RegisterFonts(); err != nil {
		g.log.Error("interface font", "err", err)
	}
	g.win = mygo.NewWindow(mygo.WindowOptions{
		Title: "Sona UI Gallery", Width: 1100, Height: 800, MinWidth: 760, MinHeight: 520,
		Content: ui.View(g.view),
	})
	g.art = kit.NewArtLoaderFrom(coverSource{}, g.win.Update)
	g.dev()
}

// dev is what a development build does with the window: land where the last
// run was, and reload the tokens as their file changes (devstate).
func (g *gallery) dev() {
	devstate.SetName("uigallery") // its own restore file, not the app's
	devstate.SetLocation(func() string { return "/" + g.selected })
	devstate.Flag("gallery.dark", &g.dark)
	devstate.Live(g.win.Update, g.log)
	if st, ok := devstate.Load(); ok {
		if w := st.Window; w != nil && w.Width > 0 && w.Height > 0 {
			g.win.SetBounds(mygo.Rectangle{X: w.X, Y: w.Y, Width: w.Width, Height: w.Height})
		}
		if name := strings.TrimPrefix(st.Route, "/"); slices.ContainsFunc(sections, func(s section) bool { return s.name == name }) {
			g.selected = name
		}
		devstate.Restore(st)
	}
	save := func() {
		if g.win.IsDestroyed() {
			return
		}
		if b := g.win.Bounds(); b.Width > 0 && b.Height > 0 {
			devstate.Save("/"+g.selected, &devstate.Window{X: b.X, Y: b.Y, Width: b.Width, Height: b.Height})
		}
	}
	g.win.OnClose(func(*mygo.CloseEvent) { save() })
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { save() })
}

// view is the window: the toolbar over the sidebar and the section.
func (g *gallery) view(c *ui.Context) {
	if g.dark {
		theme.SetMode(theme.ModeDark)
	} else {
		theme.SetMode(theme.ModeLight)
	}
	theme.Apply(c)
	c.SetZoom(float32(g.scale))

	root := ui.Column(c).Fill().Background(theme.C("ground"))
	root.Draw(func(*ui.Painter, ui.Rect) {
		g.once.Do(func() { g.log.Info("startup: first paint") })
	})
	root.Children(func() {
		g.toolbar(c)
		ui.Box(c).Height(1).FillWidth().Background(theme.C("line"))
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			g.sidebar(c)
			ui.Box(c).Width(1).FillHeight().Background(theme.C("line"))
			g.content(c)
		})
	})
}

// toolbar holds the knobs of the gallery itself: the mode and the scale.
func (g *gallery) toolbar(c *ui.Context) {
	ui.Row(c).Height(52).FillWidth().Background(theme.C("surface")).Padding(0, 16).Gap(20).AlignItems(ui.Center).Children(func() {
		text(c, "Sona UI Gallery", "heading").TextColor(theme.C("ink"))
		ui.Box(c).Grow(1).Height(1)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			text(c, "Dark", "label").TextColor(theme.C("ink-muted"))
			ui.Switch(c, &g.dark).Label("Dark mode")
		})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			text(c, fmt.Sprintf("Scale %d%%", int(g.scale*100+0.5)), "label").TextColor(theme.C("ink-muted")).SingleLine().Width(88)
			ui.StepSlider(c, &g.scale, 0.5, 2, 0.25).Label("Scale").Width(160)
		})
	})
}

// sidebar lists the sections.
func (g *gallery) sidebar(c *ui.Context) {
	ui.Column(c).Width(theme.Current().Metric["sidebar"]).FillHeight().Background(theme.C("surface")).Padding(12).Gap(2).
		Role(ui.RoleGroup).Label("Sections").Children(func() {
		for _, s := range sections {
			active := s.name == g.selected
			if kit.Button(c.Key("section:"+s.name), s.name, kit.ButtonOpts{Variant: kit.Ghost, Size: kit.SizeSM, Pressed: &active}).FillWidth().Justify(ui.Start).Clicked() {
				g.selected = s.name
			}
		}
	})
}

// content builds the selected section in a scroll container of its own
// (each keeps its place).
func (g *gallery) content(c *ui.Context) {
	i := slices.IndexFunc(sections, func(s section) bool { return s.name == g.selected })
	if i < 0 {
		g.selected, i = sections[0].name, 0
	}
	s := sections[i]
	pos := g.scrolls[s.name]
	if pos == nil {
		pos = &ui.ScrollState{}
		g.scrolls[s.name] = pos
	}
	devstate.Scroll(c, pos)
	ui.Scroll(c).TrackScroll(pos).Grow(1).Basis(0).MinHeight(0).MinWidth(0).Children(func() {
		ui.Column(c).Padding(28, 32, 48).Gap(8).Children(func() {
			text(c, s.name, "title").TextColor(theme.C("ink"))
			s.build(c, g)
		})
	})
}
