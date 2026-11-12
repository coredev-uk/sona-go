// Package playlist is the playlist feature: the playlist page (library and
// catalogue, with editing and collaboration), the sidebar's pins and folder
// tree with its drop targets, the add-to-playlist picker and the other
// playlist dialogs, and the pin, add, edit, move and delete item actions
// (v0.2.0's features/playlist).
package playlist

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// playlistPath is the playlist page's route pattern.
const playlistPath = "/playlist/{id}"

// playlistsPath is the library's Playlists page, where deleting the open
// playlist leaves to.
const playlistsPath = "/library/playlists"

// playlistsAPI is what the feature asks of PlaylistsService.
type playlistsAPI interface {
	Playlist(ctx context.Context, id string) (services.PlaylistDetail, error)
	Collaboration(ctx context.Context, id string) (services.PlaylistCollaboration, error)
	Suggestions(ctx context.Context, id string) ([]services.Song, error)
	Tree(ctx context.Context) (services.PlaylistTree, error)
	RecentTargets(ctx context.Context) ([]string, error)
	Create(ctx context.Context, in services.PlaylistInput, items []services.PlaylistItem, folderID string) (services.Playlist, error)
	Update(ctx context.Context, id string, in services.PlaylistInput) error
	CoverOptions(ctx context.Context, id string) (services.PlaylistCovers, error)
	SetCover(ctx context.Context, id, option string) (services.PlaylistCovers, error)
	UploadCover(ctx context.Context, id string) (services.PlaylistCovers, error)
	RemoveCoverUpload(ctx context.Context, id string) (services.PlaylistCovers, error)
	Delete(ctx context.Context, id string) error
	Add(ctx context.Context, id string, items []services.PlaylistItem) (services.PlaylistDetail, error)
	MoveTrack(ctx context.Context, id string, from, to int, trackID string) (services.PlaylistDetail, error)
	RemoveTracks(ctx context.Context, id string, positions []int, trackIDs []string) (services.PlaylistDetail, error)
	MoveToFolder(ctx context.Context, id, folderID string) error
	CreateFolder(ctx context.Context, name string) (services.PlaylistFolder, error)
	RenameFolder(ctx context.Context, id, name string) error
	DeleteFolder(ctx context.Context, id string) error
	Pins(ctx context.Context) ([]services.Pin, error)
	Pin(ctx context.Context, id string) error
	Unpin(ctx context.Context, id string) error
	MovePin(ctx context.Context, id, afterID string) error
	SetPinAction(ctx context.Context, id, action string) error
	PinTracks(ctx context.Context, id string) ([]services.Song, error)
}

// playlists is the feature's state.
type playlists struct {
	env *module.Env
	api playlistsAPI

	// prefs is the sidebar's remembered open folders and folded sections.
	prefs *prefs
	// edits serialises a playlist's track edits (edit.go).
	edits edits

	dialogs dialogs // dialogs.go
	side    sidebar // sidebar.go
	page    page    // page.go

	// now is the page's clock where a test sets one; nil reads the frame's
	// (c.Now).
	now func() time.Time
}

// New is the playlist module.
func New(env *module.Env) module.Module {
	p := newPlaylists(env,
		services.Service[services.PlaylistsService](env.Core),
		settingsPrefs(env.Queries, env.Core))
	return module.Module{
		ID: "playlist",
		Routes: []module.Route{{
			Pattern: playlistPath,
			Title:   func(*ui.Route) string { return i18n.T("Playlist") },
			Build:   p.playlistPage,
		}},
		Actions: p.actions(),
		Shortcuts: []module.Shortcut{{
			ID: "playlist.new", Group: "general", Keys: []string{"Ctrl+N"}, Icon: "list-plus",
			// translators: Command in the command palette, the shortcut help and the File menu: opens the form that makes a playlist.
			Label:    func() string { return i18n.T("New playlist") },
			Keywords: func() string { return i18n.T("create playlist add") },
			Run:      func() { p.dialogs.show(dialogState{kind: dialogCreate}) },
		}},
		Slots: module.Slots{
			SidebarSection: p.sidebarSection,
			Layer:          p.dialogHost, // dialogs only: no LayerOpen, so the layer takes no input itself
		},
		Init: p.init,
	}
}

func newPlaylists(env *module.Env, api playlistsAPI, pr *prefs) *playlists {
	return &playlists{env: env, api: api, prefs: pr}
}

// Query keys: they sit under "library" so a library sync invalidates them
// with everything else it stored (queries.ts).
var (
	keyTree    = store.Key{"library", "playlist-tree"}
	keyPins    = store.Key{"library", "pins"}
	keyRecents = store.Key{"library", "playlist-recent-targets"}
)

func keySuggestions(id string) store.Key { return store.Key{"library", "playlist-suggestions", id} }

// isLibraryID: library playlists carry Apple's "p." prefix, everything
// else is catalogue.
func isLibraryID(id string) bool { return strings.HasPrefix(id, "p.") }

// keyDetail is a playlist page's key: library or catalogue by id prefix.
func keyDetail(id string) store.Key {
	if isLibraryID(id) {
		return store.Key{"library", "playlist", id}
	}
	return store.Key{"catalog", "playlist", id}
}

// keyCollab is who works on a shared playlist; it sits under the
// playlist's own key, so whatever refreshes the playlist refreshes it too.
func keyCollab(id string) store.Key { return append(keyDetail(id), "collaboration") }

// init ports the module's eventHandlers: playlists:changed invalidates the
// changed playlists' details (except one being edited here, which keeps its
// optimistic state: the edit's own answer replaces it), the tree, counts
// and library playlists when the tree changed, and the pins.
func (p *playlists) init() {
	store.On(p.env.Bus, services.EventPlaylistsChanged, func(ev services.PlaylistsChanged) {
		var stale []store.Key
		for _, id := range ev.IDs {
			if !p.edits.editing(id) {
				stale = append(stale, keyDetail(id))
			}
			// The covers follow the playlist's tracks, edit or not.
			stale = append(stale, keyCovers(id))
		}
		if ev.Tree {
			stale = append(stale, keyTree, store.KeyLibraryCounts, store.KeyLibraryPlaylistsAll)
		}
		if ev.Pins {
			stale = append(stale, keyPins)
		}
		if len(stale) > 0 {
			p.env.Queries.Invalidate(stale...)
		}
	})
}

// Queries (queries.ts). Every view calls them each frame.

// detail is a playlist page.
func (p *playlists) detail(id string) store.Result[services.PlaylistDetail] {
	return store.Use(p.env.Queries, keyDetail(id), func(ctx context.Context) (services.PlaylistDetail, error) {
		return p.api.Playlist(ctx, id)
	}, store.Options{Stale: store.Forever})
}

// collaboration is who works on a shared user playlist (network, 5 min, no
// retry: a failure is kept until the page is mounted again).
func (p *playlists) collaboration(id string) store.Result[services.PlaylistCollaboration] {
	return store.Use(p.env.Queries, keyCollab(id), func(ctx context.Context) (services.PlaylistCollaboration, error) {
		return p.api.Collaboration(ctx, id)
	}, store.Options{Stale: 5 * time.Minute, NoRetry: true})
}

// tree is the folders and library playlists, from SQLite.
func (p *playlists) tree() store.Result[services.PlaylistTree] {
	return store.Use(p.env.Queries, keyTree, p.api.Tree, store.Options{Stale: store.Forever})
}

// pins are the sidebar's pins.
func (p *playlists) pins() store.Result[[]services.Pin] {
	return store.Use(p.env.Queries, keyPins, p.api.Pins, store.Options{Stale: store.Forever})
}

// suggestions are the songs Apple suggests for a library playlist (network,
// cached 10 min).
func (p *playlists) suggestions(id string) store.Result[[]services.Song] {
	return store.Use(p.env.Queries, keySuggestions(id), func(ctx context.Context) ([]services.Song, error) {
		return p.api.Suggestions(ctx, id)
	}, store.Options{Stale: 10 * time.Minute, NoRetry: true})
}

// recentTargets are Apple's recently used playlists for "Add to playlist"
// (library IDs, newest first; cached 1 min).
func (p *playlists) recentTargets() store.Result[[]string] {
	return store.Use(p.env.Queries, keyRecents, p.api.RecentTargets, store.Options{Stale: time.Minute, NoRetry: true})
}
