package kit

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// ItemKind is what an item action acts on (v0.2.0's ActionTargetKind).
type ItemKind string

// The kinds with actions. KindSelection is an ActionDef's Target for a
// bulk action: the `targets[]` form of an action.
const (
	KindSong       ItemKind = "song"
	KindAlbum      ItemKind = "album"
	KindPlaylist   ItemKind = "playlist"
	KindArtist     ItemKind = "artist"
	KindStation    ItemKind = "station"
	KindMusicVideo ItemKind = "music-video"
	KindSelection  ItemKind = "selection"
)

var itemKinds = [...]ItemKind{KindSong, KindAlbum, KindPlaylist, KindArtist, KindStation, KindMusicVideo}

// Item is one item of any kind: what a selection holds and a drag carries.
// Target is the DTO (services.Song, services.Album…), by value.
type Item struct {
	Kind   ItemKind
	Target any
}

// SongItem and its siblings wrap a DTO as an Item.
func SongItem(s services.Song) Item             { return Item{KindSong, s} }
func AlbumItem(a services.Album) Item           { return Item{KindAlbum, a} }
func PlaylistItem(p services.Playlist) Item     { return Item{KindPlaylist, p} }
func ArtistItem(a services.Artist) Item         { return Item{KindArtist, a} }
func StationItem(s services.Station) Item       { return Item{KindStation, s} }
func MusicVideoItem(v services.MusicVideo) Item { return Item{KindMusicVideo, v} }

// Song returns the target of a song item (the zero Song for another kind).
func (i Item) Song() services.Song { s, _ := i.Target.(services.Song); return s }

// Album returns the target of an album item.
func (i Item) Album() services.Album { a, _ := i.Target.(services.Album); return a }

// Playlist returns the target of a playlist item.
func (i Item) Playlist() services.Playlist { p, _ := i.Target.(services.Playlist); return p }

// Artist returns the target of an artist item.
func (i Item) Artist() services.Artist { a, _ := i.Target.(services.Artist); return a }

// Station returns the target of a station item.
func (i Item) Station() services.Station { s, _ := i.Target.(services.Station); return s }

// MusicVideo returns the target of a music-video item.
func (i Item) MusicVideo() services.MusicVideo { v, _ := i.Target.(services.MusicVideo); return v }

// Name is the item's name, whatever its kind.
func (i Item) Name() string {
	switch t := i.Target.(type) {
	case services.Song:
		return t.Name
	case services.Album:
		return t.Name
	case services.Playlist:
		return t.Name
	case services.Artist:
		return t.Name
	case services.Station:
		return t.Name
	case services.MusicVideo:
		return t.Name
	}
	return ""
}

// ID is the item's ID in its own source.
func (i Item) ID() string {
	switch t := i.Target.(type) {
	case services.Song:
		return t.ID
	case services.Album:
		return t.ID
	case services.Playlist:
		return t.ID
	case services.Artist:
		return t.ID
	case services.Station:
		return t.ID
	case services.MusicVideo:
		return t.ID
	}
	return ""
}

// ActionGroup is a menu section, in display order: a separator divides
// neighbouring non-empty sections.
type ActionGroup int

const (
	GroupQueue ActionGroup = iota
	GroupItem
	GroupRating
	groupCount
)

// ActionContainer is the collection a menu's item is listed in, passed
// down by the list that renders the menu (a playlist's track list), so
// actions can act on the item's place in it ("Remove from this playlist").
type ActionContainer struct {
	// Kind is what the collection is: "playlist", "album"…
	Kind string
	// ID is the collection's ID.
	ID string
	// Index is the item's position in the collection as listed (single
	// actions).
	Index int
	// Indices are the positions of every selected item, in list order
	// (bulk actions).
	Indices []int
	// Editable: the user may change the collection's items.
	Editable bool
}

// MenuData is data an action reads only when its menu opens, not for every
// row (a song's rating). Actions sharing a MenuData (by ID) share one load.
// Load runs on a goroutine with the items the menu acts on (one for a
// single action); it should not fail (map failures to a fallback).
type MenuData struct {
	ID   string
	Load func(ctx context.Context, items []Item) any
}

// ActionContext is what an action reads and acts with: the item or items
// (a single action has exactly one), the menu data that has loaded and
// where the item is listed.
type ActionContext struct {
	// Items are the targets: one for a single action, the selection in list
	// order for a bulk one.
	Items []Item
	// Container is the collection the items are listed in, when the list
	// passes one.
	Container *ActionContainer
	// Navigate opens a page ("/album/{id}").
	Navigate func(path string)

	loaded map[string]any
}

// Item is the single target (the first of Items).
func (a *ActionContext) Item() Item {
	if len(a.Items) == 0 {
		return Item{}
	}
	return a.Items[0]
}

// Data returns the value d loaded when the menu opened, nil until it
// resolves.
func (a *ActionContext) Data(d MenuData) any { return a.loaded[d.ID] }

// DataOf is Data typed: ok is false until the data has loaded (or when it
// is of another type).
func DataOf[V any](a *ActionContext, d MenuData) (v V, ok bool) {
	v, ok = a.loaded[d.ID].(V)
	return v, ok
}

// WithData returns a copy of a whose Data answers from values (tests and
// previews: a menu fills them as its loads resolve).
func (a *ActionContext) WithData(values map[string]any) *ActionContext {
	cp := *a
	cp.loaded = values
	return &cp
}

// ActionDef is one action, single (Target is a kind) or bulk (Target is
// KindSelection, Kinds the kinds it accepts). IDs are unique across the
// registry and stable ("player.play-next", "library.favorite.song").
type ActionDef struct {
	ID string
	// Target is the kind the action acts on, or KindSelection for the
	// `targets[]` form: one run receives every selected item, in list
	// order, and it is listed only when every selected item is one of Kinds.
	Target ItemKind
	Kinds  []ItemKind
	// Label is translated when the menu renders: func(a) { return i18n.T("Play next") }.
	Label func(a *ActionContext) string
	// Icon is a Lucide name (theme.Icon).
	Icon string
	// Filled draws the icon filled (a loved heart); icons are outlined
	// otherwise.
	Filled func(a *ActionContext) bool
	Group  ActionGroup
	// Order positions the action inside its group, ascending.
	Order int
	// Available: listed for this target at all (default: always).
	Available func(a *ActionContext) bool
	// Enabled: listed but greyed out when false (default: enabled).
	Enabled func(a *ActionContext) bool
	// DisabledReason greys the row out with this line under its label
	// saying why (a live station has no queue to add to); "" for none.
	DisabledReason func(a *ActionContext) string
	// Data is read through a.Data, loaded when the menu opens.
	Data []MenuData
	Run  func(a *ActionContext)
}

func (d *ActionDef) bulk() bool { return d.Target == KindSelection }

func (d *ActionDef) rank() int { return int(d.Group)*1000 + d.Order }

// ActionRegistry holds the actions of every module: per-kind lists for
// song, album, playlist, artist, station and music-video targets plus one
// bulk (selection) list. The shell builds one from Module.Actions and hands
// it to features as Env.Actions.
type ActionRegistry struct {
	mu       sync.RWMutex
	byKind   map[ItemKind][]*ActionDef
	bulk     []*ActionDef
	ids      map[string]bool
	navigate func(string)
	post     store.Poster
}

// NewActionRegistry returns a registry holding defs; it panics (a
// programming error caught at start-up) on a duplicate ID.
func NewActionRegistry(defs ...ActionDef) *ActionRegistry {
	r := &ActionRegistry{byKind: map[ItemKind][]*ActionDef{}, ids: map[string]bool{}}
	r.Register(defs...)
	return r
}

// SetNavigator sets what ActionContext.Navigate does (the shell's router).
func (r *ActionRegistry) SetNavigator(fn func(path string)) {
	r.mu.Lock()
	r.navigate = fn
	r.mu.Unlock()
}

// SetPoster sets how menu data loaded on a goroutine gets back to the main
// thread (Env.Post).
func (r *ActionRegistry) SetPoster(p store.Poster) {
	r.mu.Lock()
	r.post = p
	r.mu.Unlock()
}

func (r *ActionRegistry) poster() store.Poster {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.post == nil {
		return func(fn func()) { fn() }
	}
	return r.post
}

// Navigate opens a page through the navigator, when one is set.
func (r *ActionRegistry) Navigate(path string) {
	r.mu.RLock()
	fn := r.navigate
	r.mu.RUnlock()
	if fn != nil {
		fn(path)
	}
}

// Register adds actions; menus list them by group, then order. It panics
// with "action <id> registered twice" for a duplicate ID across single and
// bulk actions, and for a definition that cannot work.
func (r *ActionRegistry) Register(defs ...ActionDef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range defs {
		d := defs[i]
		if d.ID == "" || d.Label == nil || d.Run == nil {
			panic(fmt.Sprintf("action %q needs an ID, a Label and a Run", d.ID))
		}
		if r.ids[d.ID] {
			panic(fmt.Sprintf("action %s registered twice", d.ID))
		}
		if d.bulk() {
			if len(d.Kinds) == 0 {
				panic(fmt.Sprintf("bulk action %s lists no kinds", d.ID))
			}
		} else if !slices.Contains(itemKinds[:], d.Target) {
			panic(fmt.Sprintf("action %s has unknown target %q", d.ID, d.Target))
		}
		r.ids[d.ID] = true
		list := &r.bulk
		var kind []*ActionDef
		if !d.bulk() {
			kind = r.byKind[d.Target]
			list = &kind
		}
		*list = append(*list, &d)
		slices.SortStableFunc(*list, func(a, b *ActionDef) int { return cmp.Compare(a.rank(), b.rank()) })
		if !d.bulk() {
			r.byKind[d.Target] = kind
		}
	}
}

// For returns every registered action for a kind of target, in menu order;
// none for a kind with nothing registered.
func (r *ActionRegistry) For(kind ItemKind) []*ActionDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return slices.Clone(r.byKind[kind])
}

// BulkFor returns the bulk actions that accept every item of items, in
// menu order (songs and albums show only the actions both accept).
func (r *ActionRegistry) BulkFor(items []Item) []*ActionDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []*ActionDef
	for _, a := range r.bulk {
		if !slices.ContainsFunc(items, func(i Item) bool { return !slices.Contains(a.Kinds, i.Kind) }) {
			out = append(out, a)
		}
	}
	return out
}

// Has reports whether anything is registered for the kind (ItemMenu shows
// a "more" menu only for those).
func (r *ActionRegistry) Has(kind ItemKind) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byKind[kind]) > 0
}

// Context builds the ActionContext of items with the loaded menu data.
func (r *ActionRegistry) Context(items []Item, container *ActionContainer, loaded map[string]any) *ActionContext {
	r.mu.RLock()
	nav := r.navigate
	r.mu.RUnlock()
	return &ActionContext{Items: items, Container: container, Navigate: nav, loaded: loaded}
}

// ActionSection is the non-empty actions of one group.
type ActionSection struct {
	Group   ActionGroup
	Actions []*ActionDef
}

// Sections splits ordered actions into their non-empty groups, in group
// order.
func Sections(actions []*ActionDef) []ActionSection {
	var out []ActionSection
	for g := GroupQueue; g < groupCount; g++ {
		var in []*ActionDef
		for _, a := range actions {
			if a.Group == g {
				in = append(in, a)
			}
		}
		if len(in) > 0 {
			out = append(out, ActionSection{g, in})
		}
	}
	return out
}

// DescribeItems says what items are, for menus and toasts: “Airbag”,
// "12 songs", "3 albums", "5 items".
func DescribeItems(items []Item) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		return i18n.T("“{name}”", "name", items[0].Name())
	}
	kind := items[0].Kind
	for _, i := range items {
		if i.Kind != kind {
			// translators: {0} is the number of items.
			return i18n.T("{0, plural, one {# item} other {# items}}", "0", len(items))
		}
	}
	n := len(items)
	switch kind {
	case KindSong:
		return i18n.T("{n, plural, one {# song} other {# songs}}", "n", n)
	case KindAlbum:
		return i18n.T("{n, plural, one {# album} other {# albums}}", "n", n)
	case KindPlaylist:
		return i18n.T("{n, plural, one {# playlist} other {# playlists}}", "n", n)
	case KindArtist:
		return i18n.T("{n, plural, one {# artist} other {# artists}}", "n", n)
	case KindStation:
		return i18n.T("{n, plural, one {# station} other {# stations}}", "n", n)
	default:
		return i18n.T("{n, plural, one {# music video} other {# music videos}}", "n", n)
	}
}

// TracksOf resolves a song to itself, an album to its tracks (the library
// or the catalogue by its source), a playlist to its tracks and other kinds
// to none. It calls services: run it off the main thread.
func TracksOf(ctx context.Context, core *services.Core, item Item) ([]services.Song, error) {
	switch item.Kind {
	case KindSong:
		return []services.Song{item.Song()}, nil
	case KindAlbum:
		a := item.Album()
		if a.Source == services.SourceLibrary {
			d, err := services.Service[services.LibraryService](core).Album(ctx, a.ID)
			return d.Tracks, err
		}
		d, err := services.Service[services.CatalogService](core).Album(ctx, a.ID)
		return d.Tracks, err
	case KindPlaylist:
		d, err := services.Service[services.PlaylistsService](core).Playlist(ctx, item.Playlist().ID)
		return d.Tracks, err
	}
	return nil, nil
}
