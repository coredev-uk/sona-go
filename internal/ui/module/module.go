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
	"time"

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
	// Surfaces is the open state of the account dialog and the first-run
	// setup: any "Sign in" action calls Surfaces.OpenAccount, Settings'
	// "Set up Sona again" Surfaces.OpenSetup.
	Surfaces *store.Surfaces
	// OpenURL opens a web page in the system browser.
	OpenURL func(url string) error
	// Copy puts text on the system clipboard and reports success.
	Copy func(text string) bool
	// Motion decides whether motion artwork plays and owns the decoders
	// (kit.MotionCover, kit.TileMotion take it); the shell tells it when the
	// window hides and when a layer, an overlay or a dialog covers the page.
	// Nil only in tests that build an Env by hand: a nil policy allows nothing.
	Motion *kit.MotionPolicy
	// Video is the music video view's shared state: whether it is open, the
	// window's full screen it toggles and the source of its pictures.
	// Features that start or show a music video call Video.Open and read
	// Video.IsOpen; the video feature draws the view. Never nil.
	Video *store.VideoView
	// Window is the main window: its keyboard focus (a view that animates
	// on its own, as browse's lead rotation, holds while the window is
	// unfocused) and its full screen (the music video, full-screen lyrics).
	// The shell binds it and redraws on each change. Never nil.
	Window *store.Window
	Nav    Navigator // the main window's router
	// Queue starts, shuffles and queues songs and videos through the
	// playback service: every feature that plays a collection or adds to the
	// queue goes through it, so replacing a long queue of the user's asks
	// first or offers Undo (v0.2.0's playSongs, queueItems and replaceQueue).
	// Nil in an Env built by hand without services.
	Queue *kit.QueueCommands
	// ItemStates is the one library state and rating layer of every item
	// (v0.2.0's shared item-state): menus, headers and the player's rating
	// buttons all read and change items through it, so a change shows
	// everywhere at once. It follows "library:item-state" itself. Main
	// thread. Nil in an Env built by hand without services.
	ItemStates *kit.ItemStates
	// HasRoute reports whether a registered module's route pattern matches
	// path (a query or fragment is ignored), so a feature links only to pages
	// that are ported: Pattern "/album/{id}" matches "/album/42". Call it
	// while building a view: the routes of modules built after the caller
	// are not known yet in its factory. Nil in a hand-built Env: treat as
	// false.
	HasRoute func(path string) bool
	// Players is the shared item-play registry (kit.ItemPlayers): the
	// features that own playback register how each kind plays (discover:
	// songs, albums, playlists, videos; radio: stations) and the shared item
	// components play through it. Built by NewPlayers; never nil.
	Players *kit.ItemPlayers
	// ConcertsShelf builds Home's concerts shelf (the next shows near the
	// chosen city); the concerts module sets it in its factory, so it is nil
	// until then and a page that wants it builds nothing without it.
	ConcertsShelf func(c *ui.Context, first bool)
	// Post runs fn on the main thread and redraws the windows: how a
	// goroutine (a service call) hands its outcome to the view.
	Post store.Poster
	// PlayerControls are the small controls of the player bar's right group
	// (private listening, the sleep timer, lyrics, the queue…), before mute
	// and the volume; PlayerNotice is the line under the song's title while
	// it waits for the Widevine CDM. A module adds to them as it is built
	// (the factories run in modules.go's order, a Point sorts by its own).
	PlayerControls Point
	PlayerNotice   Point
	// ArtistConcerts builds an artist page's "Upcoming concerts" (owned by
	// features/concerts, which sets it in its factory; nil while that feature
	// is not registered: build nothing). first marks the page's first shelf.
	ArtistConcerts func(c *ui.Context, artistID string, first bool)
	Log            *slog.Logger
	Version        string
	Dev            bool // a -tags dev build
	// Shortcuts lists every registered shortcut (every module's, the
	// navigation ones and the shell's) with the keys it answers to now:
	// the user's rebinding (config.Settings.Shortcuts, by ID) or its
	// defaults. Settings → Shortcuts and the help list them. Set by the
	// app before any module's Init; nil without a shell (tests).
	Shortcuts func() []BoundShortcut
}

// Point is a place in one feature's view other modules build into, the
// way the shell's Slots are places in the shell: a module Adds its piece
// when it is built, the owning feature Builds them in order. The zero
// value is ready; an Env is shared by pointer, so what a module adds is
// seen by the feature that builds the point.
type Point struct {
	items []pointItem
}

type pointItem struct {
	order int
	build func(c *ui.Context)
}

// Add puts build in the point, after the pieces of a lower or equal order
// (equal ones keep the order they were added in). Call it from a Factory,
// on the main thread: nothing locks.
func (p *Point) Add(order int, build func(c *ui.Context)) {
	i := len(p.items)
	for i > 0 && p.items[i-1].order > order {
		i--
	}
	p.items = append(p.items, pointItem{})
	copy(p.items[i+1:], p.items[i:])
	p.items[i] = pointItem{order, build}
}

// Build builds every piece, in order, each one building nothing when it has
// nothing to show.
func (p *Point) Build(c *ui.Context) { p.BuildRange(c, 0, 1<<30) }

// BuildRange builds the pieces whose order is from or more, and below to: a
// feature that builds its own controls among the added ones (the player
// bar's queue button, which Mini player and Equaliser follow).
func (p *Point) BuildRange(c *ui.Context, from, to int) {
	for _, it := range p.items {
		if it.order >= from && it.order < to {
			it.build(c)
		}
	}
}

// Empty reports whether nothing was added.
func (p *Point) Empty() bool { return len(p.items) == 0 }

// ControlsAfterQueue is the order from which PlayerControls pieces follow
// the bar's own sleep timer, lyrics and queue buttons (Mini player, then
// Equaliser); lower orders come before them (private listening).
const ControlsAfterQueue = 50

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

// KeysOf are the keys the shortcut with the given ID answers to now (the
// user's binding, else its defaults); nil for none, an unknown ID, or an Env
// with no shortcuts (a test's).
func (e *Env) KeysOf(id string) []string {
	if e.Shortcuts == nil {
		return nil
	}
	for _, s := range e.Shortcuts() {
		if s.ID == id {
			return s.Keys
		}
	}
	return nil
}

// Slots are places in the shell a module builds into. Every func builds
// nothing when it has nothing to show.
type Slots struct {
	Aside          func(c *ui.Context) // right of the page (the queue)
	Overlay        *Overlay            // covers the page column
	Bottom         func(c *ui.Context) // across the bottom (the player bar)
	Layer          func(c *ui.Context) // over the whole window (first-run setup); the window behind is inert while LayerOpen
	LayerOpen      func() bool
	SidebarFooter  func(c *ui.Context)
	SidebarSection func(c *ui.Context) // stacks in module order
	TopBarSearch   func(c *ui.Context) // the first one wins
	TopBarAccount  func(c *ui.Context) // the top bar's trailing end (the account menu); the first one wins
	// Mini is the mini player window's whole content (the first one wins).
	// The shell owns the window (fixed size, frameless, above other
	// windows) and builds Mini in it while it is shown; the content drags
	// the window by its background (DragWindow) and reaches the windows
	// through services.DesktopService (ShowMainWindow, CloseMiniPlayer).
	// The module that sets Mini offers the controls that open it; without
	// one, DesktopService.ToggleMiniPlayer keeps the main window.
	Mini func(c *ui.Context)
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

// NewPlayers is the Env's item-play registry: music videos play only where
// PlatformService says the platform plays video (read once, on first use; nil
// before it answers and without a Core). now is the clock a release is
// compared with (nil: time.Now).
func NewPlayers(core *services.Core, now func() time.Time) *kit.ItemPlayers {
	p := &kit.ItemPlayers{Now: now}
	if core != nil {
		plat := services.Service[services.PlatformService](core)
		var caps *services.Capabilities
		p.Capabilities = func() *services.Capabilities {
			if caps == nil {
				c := plat.Capabilities()
				caps = &c
			}
			return caps
		}
	}
	return p
}
