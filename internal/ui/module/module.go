// Package module is the contract between the shell (internal/ui/app) and
// the features (internal/ui/features/<name>): types only. A feature builds
// a Module over the shared Env and the app collects the modules into its
// routes, sidebar, shortcuts and shell slots (docs/ui-architecture.md, "The
// module contract"). Nothing here opens a window or holds state.
//
// Features never import each other: shared pieces live in kit (UI) and
// store (data), and a feature reaches another's state through the store
// (the playback mirror) or an Env hook.
package module

import (
	"log/slog"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// Env is what the shell hands every feature. It is built once in app.New;
// its fields never change after.
type Env struct {
	Core    *services.Core // typed lookup: services.Service[services.LibraryService](env.Core)
	Bus     *store.Bus     // Go events (services.Emitter); store.On subscribes
	Queries *store.Client  // cached queries; Invalidate(prefixes...)
	Play    *store.Playback
	Art     *kit.ArtLoader // exact-size artwork decode off the main thread
	// Actions is the item action registry (every module's Module.Actions),
	// read by kit.ActionMenus.
	Actions *kit.ActionRegistry
	// Motion decides whether motion artwork plays and owns the decoders
	// (kit.MotionCover, kit.TileMotion take it); the shell tells it when the
	// window hides and when a layer, an overlay or a dialog covers the page.
	// Nil only in tests that build an Env by hand: a nil policy allows nothing.
	Motion *kit.MotionPolicy
	Nav    Navigator // the main window's router
	// HasRoute reports whether a registered module's route pattern matches
	// path (a query or fragment is ignored), so a feature links only to pages
	// that are ported: Pattern "/album/{id}" matches "/album/42". Call it
	// while building a view: the routes of modules built after the caller
	// are not known yet in its factory. Nil in a hand-built Env: treat as
	// false.
	HasRoute func(path string) bool
	// Post runs fn on the main thread and redraws the windows: how a
	// goroutine (a service call) hands its outcome to the view.
	Post    store.Poster
	Log     *slog.Logger
	Version string
	Dev     bool // a -tags dev build
	// Shortcuts lists every registered shortcut (every module's, the
	// navigation ones and the shell's) with the keys it answers to now:
	// the user's rebinding (config.Settings.Shortcuts, by ID) or its
	// defaults. Settings → Shortcuts and the help list them. Set by the
	// app before any module's Init; nil without a shell (tests).
	Shortcuts func() []BoundShortcut
}

// Navigator is the main window's router.
type Navigator interface {
	Push(path string) // "/library/albums?sort=title"
	Replace(path string)
	Back()
	Forward()
	Path() string     // without the query
	Location() string // with the query
}

// NavSection is the part of the sidebar a NavItem sits in.
type NavSection int

const (
	NavPrimary NavSection = iota
	NavLibrary
	NavFooter
)

// NavItem is a sidebar entry.
type NavItem struct {
	Section NavSection
	Label   func() string // func() string { return i18n.T("Albums") }
	Icon    string        // Lucide name, theme.Icon(name)
	To      string        // one of the module's own route patterns (no params)
	Order   int           // ascending within the section
	Exact   bool          // active only on this exact path
	Keys    []string      // default chords of its go-to shortcut; nil: Ctrl+1…9 / Alt+1…9 by position
}

// Route is a page.
type Route struct {
	Pattern string // MyGo router pattern: "/album/{id}", "/settings/{section...}"
	Title   func(r *ui.Route) string
	Build   func(c *ui.Context, r *ui.Route)
}

// ShortcutGroup orders shortcuts in the help: "playback", "view", "pages",
// "navigation", "general", in that order.
type ShortcutGroup string

// Shortcut is a key binding and a command-palette entry.
type Shortcut struct {
	ID       string // "<module>.<action>", stable: settings store bindings by it (the IDs of v0.2.0)
	Label    func() string
	Group    ShortcutGroup
	Keys     []string // "Space", "Ctrl+K", "Shift+ArrowRight" (v0.2.0's spelling); nil for none
	Scope    string   // "" = page, "global", or a context name with When
	Keywords func() string
	Icon     string
	When     func() bool
	// Unavailable, when it returns text, says why the key does nothing:
	// the key is taken, a toast shows the reason and the palette hides it.
	Unavailable func() string
	Repeat      bool
	NoPalette   bool
	Run         func()
}

// BoundShortcut is a registered shortcut with the keys it answers to now
// (Shortcut.Keys is its default).
type BoundShortcut struct {
	Shortcut
	Keys []string // effective: the user's binding, else the defaults; nil for none
}

// Slots are places in the shell a module builds into. Every func builds
// nothing when it has nothing to show.
type Slots struct {
	Aside          func(c *ui.Context) // right of the page (the queue)
	Overlay        *Overlay            // covers the page column
	Bottom         func(c *ui.Context) // across the bottom (the player bar)
	Layer          func(c *ui.Context) // over the whole window (first-run setup); the window behind is inert while LayerOpen, and the layer takes clicks only then (a layer that builds dialogs only, which are overlays, needs no LayerOpen)
	LayerOpen      func() bool
	SidebarFooter  func(c *ui.Context)
	SidebarSection func(c *ui.Context) // stacks in module order
	TopBarSearch   func(c *ui.Context) // the first one wins
}

// Overlay covers the page column while Open.
type Overlay struct {
	Open  func() bool
	Build func(c *ui.Context)
}

// Module is a feature as the shell sees it.
type Module struct {
	ID        string
	Routes    []Route
	Nav       []NavItem
	Shortcuts []Shortcut
	// Actions are the module's item actions (single and bulk): the app
	// collects every module's into Env.Actions, which menus list and
	// shortcuts and the palette share by ID.
	Actions []kit.ActionDef
	Slots   Slots
	// Init runs once after the shell connects, on the main thread:
	// subscriptions and the first snapshots.
	Init func()
}

// Factory builds a feature's module over the shared Env; the feature
// keeps its state in its own struct.
type Factory func(env *Env) Module
