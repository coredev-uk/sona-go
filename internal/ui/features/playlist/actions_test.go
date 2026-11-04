package playlist

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// actionsFixture is p's actions in a registry, as the shell registers them.
type actionsFixture struct {
	p   *playlists
	r   *kit.ActionRegistry
	api *fakeAPI
}

func newActionsFixture(t *testing.T) *actionsFixture {
	t.Helper()
	p, api, _, _, router := testPlaylists(t, "/")
	r := kit.NewActionRegistry(p.actions()...)
	r.SetNavigator(func(path string) { router.Push(path) })
	return &actionsFixture{p: p, r: r, api: api}
}

func (f *actionsFixture) def(t *testing.T, id string) *kit.ActionDef {
	t.Helper()
	for _, d := range f.p.actions() {
		if d.ID == id {
			return &d
		}
	}
	t.Fatalf("no action %q", id)
	return nil
}

func available(d *kit.ActionDef, a *kit.ActionContext) bool {
	return d.Available == nil || d.Available(a)
}
func enabled(d *kit.ActionContext, def *kit.ActionDef) bool {
	return def.Enabled == nil || def.Enabled(d)
}

func listIDs(list []*kit.ActionDef) []string {
	out := make([]string, len(list))
	for i, d := range list {
		out[i] = d.ID
	}
	return out
}

func libSong(id string) kit.Item {
	return kit.SongItem(services.Song{ID: id, Name: "S " + id, Source: services.SourceLibrary})
}

func libPlaylist(id string, canEdit bool, catalogID string) kit.Item {
	return kit.PlaylistItem(services.Playlist{ID: id, Name: "PL " + id, Source: services.SourceLibrary, CanEdit: canEdit, CatalogID: catalogID})
}

func TestActionsRegisterByKind(t *testing.T) {
	f := newActionsFixture(t)
	for _, c := range []struct {
		kind kit.ItemKind
		want []string
	}{
		{kit.KindSong, []string{"playlists.add-to.song", "playlists.remove-from", "playlists.pin.song"}},
		{kit.KindAlbum, []string{"playlists.add-to.album", "playlists.pin.album"}},
		{kit.KindArtist, []string{"playlists.pin.artist"}},
		{kit.KindPlaylist, []string{"playlists.add-to.playlist", "playlists.pin.playlist", "playlists.edit", "playlists.move", "playlists.delete"}},
	} {
		got := listIDs(f.r.For(c.kind))
		slices.Sort(got)
		want := slices.Clone(c.want)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%v: got %v, want %v", c.kind, got, want)
		}
	}
}

func TestActionsForASelection(t *testing.T) {
	f := newActionsFixture(t)
	songs := []kit.Item{libSong("s.1"), libSong("s.2")}
	got := listIDs(f.r.BulkFor(songs))
	slices.Sort(got)
	if want := []string{"playlists.add-to.many", "playlists.remove-from.many"}; !slices.Equal(got, want) {
		t.Errorf("songs: got %v, want %v", got, want)
	}
	mixed := []kit.Item{libSong("s.1"), kit.AlbumItem(services.Album{ID: "l.1"})}
	if got := listIDs(f.r.BulkFor(mixed)); !slices.Equal(got, []string{"playlists.add-to.many"}) {
		t.Errorf("songs and albums: got %v (only adding fits both)", got)
	}
	withArtist := []kit.Item{libSong("s.1"), kit.ArtistItem(services.Artist{ID: "r.1"})}
	if got := f.r.BulkFor(withArtist); len(got) != 0 {
		t.Errorf("an artist in the selection: got %v", listIDs(got))
	}
}

func TestRemoveFromNeedsAnEditablePlaylistContainer(t *testing.T) {
	f := newActionsFixture(t)
	single, bulk := f.def(t, "playlists.remove-from"), f.def(t, "playlists.remove-from.many")
	items := []kit.Item{libSong("s.1")}
	for _, c := range []struct {
		name string
		cont *kit.ActionContainer
		want bool
	}{
		{"no container", nil, false},
		{"editable playlist", &kit.ActionContainer{Kind: "playlist", ID: "p.1", Editable: true}, true},
		{"read-only playlist", &kit.ActionContainer{Kind: "playlist", ID: "p.1"}, false},
		{"editable album", &kit.ActionContainer{Kind: "album", ID: "l.1", Editable: true}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := f.r.Context(items, c.cont, nil)
			if got := available(single, ctx); got != c.want {
				t.Errorf("single available = %v", got)
			}
			if got := available(bulk, ctx); got != c.want {
				t.Errorf("bulk available = %v", got)
			}
		})
	}
}

func TestAddToIsAlwaysAvailable(t *testing.T) {
	f := newActionsFixture(t)
	for id, it := range map[string]kit.Item{
		"playlists.add-to.song":     libSong("s.1"),
		"playlists.add-to.album":    kit.AlbumItem(services.Album{ID: "a.1", Source: services.SourceCatalog}),
		"playlists.add-to.playlist": libPlaylist("p.1", false, ""),
	} {
		d := f.def(t, id)
		ctx := f.r.Context([]kit.Item{it}, nil, nil)
		if !available(d, ctx) || !enabled(ctx, d) {
			t.Errorf("%s: available %v enabled %v", id, available(d, ctx), enabled(ctx, d))
		}
		if got := d.Label(ctx); got != "Add to playlist" {
			t.Errorf("%s label %q", id, got)
		}
	}
}

func TestPinActionFollowsTheLoadedPins(t *testing.T) {
	f := newActionsFixture(t)
	pinned := map[string]any{"playlists.pinned": map[string]bool{"s.1": true}}
	for _, c := range []struct {
		name     string
		id       string
		item     kit.Item
		loaded   map[string]any
		wantAvl  bool
		wantText string
	}{
		{"song not pinned (nothing loaded yet)", "playlists.pin.song", libSong("s.1"), nil, true, "Pin song"},
		{"song pinned", "playlists.pin.song", libSong("s.1"), pinned, true, "Unpin song"},
		{"another song", "playlists.pin.song", libSong("s.2"), pinned, true, "Pin song"},
		{"catalogue song is not in the library", "playlists.pin.song",
			kit.SongItem(services.Song{ID: "1234", Source: services.SourceCatalog}), pinned, false, "Pin song"},
		{"catalogue song with a library copy", "playlists.pin.song",
			kit.SongItem(services.Song{ID: "1234", Source: services.SourceCatalog, LibraryID: "s.1"}), pinned, true, "Unpin song"},
		{"album", "playlists.pin.album", kit.AlbumItem(services.Album{ID: "l.1", Source: services.SourceLibrary}), nil, true, "Pin album"},
		{"artist pinned", "playlists.pin.artist", kit.ArtistItem(services.Artist{ID: "r.1", Source: services.SourceLibrary}),
			map[string]any{"playlists.pinned": map[string]bool{"r.1": true}}, true, "Unpin artist"},
		{"playlist", "playlists.pin.playlist", libPlaylist("p.1", true, ""), nil, true, "Pin playlist"},
		{"playlist pinned", "playlists.pin.playlist", libPlaylist("p.1", true, ""),
			map[string]any{"playlists.pinned": map[string]bool{"p.1": true}}, true, "Unpin playlist"},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := f.def(t, c.id)
			ctx := f.r.Context([]kit.Item{c.item}, nil, map[string]any{})
			if c.loaded != nil {
				ctx = ctx.WithData(c.loaded)
			}
			if got := available(d, ctx); got != c.wantAvl {
				t.Errorf("available = %v", got)
			}
			if got := d.Label(ctx); got != c.wantText {
				t.Errorf("label %q, want %q", got, c.wantText)
			}
		})
	}
}

func TestPinActionRunPinsAndUnpins(t *testing.T) {
	f := newActionsFixture(t)
	d := f.def(t, "playlists.pin.song")
	item := []kit.Item{libSong("s.1")}
	d.Run(f.r.Context(item, nil, map[string]any{}).WithData(map[string]any{"playlists.pinned": map[string]bool{}}))
	waitDid(t, f.api, "Pin s.1")
	d.Run(f.r.Context(item, nil, map[string]any{}).WithData(map[string]any{"playlists.pinned": map[string]bool{"s.1": true}}))
	waitDid(t, f.api, "Unpin s.1")
}

func TestPinnedAndTreeDataLoadFromTheAPI(t *testing.T) {
	f := newActionsFixture(t)
	f.api.pins = []services.Pin{{Kind: "song", ID: "s.1"}, {Kind: "album", ID: "l.2"}}
	f.api.tree = services.PlaylistTree{Playlists: []services.TreePlaylist{ownPL("p.1", "Mine", "f.a", 0)}}
	pins, _ := f.p.pinnedData().Load(t.Context(), nil).(map[string]bool)
	if !pins["s.1"] || !pins["l.2"] || len(pins) != 2 {
		t.Errorf("pins %v", pins)
	}
	byID, _ := f.p.treeData().Load(t.Context(), nil).(map[string]services.TreePlaylist)
	if byID["p.1"].FolderID != "f.a" || len(byID) != 1 {
		t.Errorf("tree %v", byID)
	}
	f.api.failReads = true
	if pins, _ := f.p.pinnedData().Load(t.Context(), nil).(map[string]bool); len(pins) != 0 {
		t.Errorf("pins after a failed read: %v", pins)
	}
	if byID, _ := f.p.treeData().Load(t.Context(), nil).(map[string]services.TreePlaylist); len(byID) != 0 {
		t.Errorf("tree after a failed read: %v", byID)
	}
}

func TestEditAndMoveOnlyForEditableLibraryPlaylists(t *testing.T) {
	f := newActionsFixture(t)
	catalogue := kit.PlaylistItem(services.Playlist{ID: "pl.123", Source: services.SourceCatalog, CanEdit: true})
	for _, c := range []struct {
		name string
		item kit.Item
		want bool
	}{
		{"own library playlist", libPlaylist("p.1", true, ""), true},
		{"library playlist the user cannot edit", libPlaylist("p.2", false, ""), false},
		{"personal mix", libPlaylist("p.3", false, "pl.pm-1"), false},
		{"catalogue playlist", catalogue, false},
	} {
		for _, id := range []string{"playlists.edit", "playlists.move"} {
			d := f.def(t, id)
			if got := available(d, f.r.Context([]kit.Item{c.item}, nil, nil)); got != c.want {
				t.Errorf("%s: %s available = %v, want %v", c.name, id, got, c.want)
			}
		}
	}
}

func TestDeleteRules(t *testing.T) {
	f := newActionsFixture(t)
	d := f.def(t, "playlists.delete")
	tree := map[string]any{"playlists.tree": map[string]services.TreePlaylist{
		"p.own":  {Playlist: services.Playlist{ID: "p.own"}, CanDelete: true},
		"p.lock": {Playlist: services.Playlist{ID: "p.lock"}, CanDelete: false},
		"p.mix":  {Playlist: services.Playlist{ID: "p.mix"}, CanDelete: true},
	}}
	catalogue := kit.PlaylistItem(services.Playlist{ID: "pl.123", Source: services.SourceCatalog})
	for _, c := range []struct {
		name        string
		item        kit.Item
		loaded      map[string]any
		wantAvl     bool
		wantEnabled bool
	}{
		{"own, may delete", libPlaylist("p.own", true, ""), tree, true, true},
		{"library playlist that cannot be deleted", libPlaylist("p.lock", false, ""), tree, true, false},
		{"not loaded yet", libPlaylist("p.own", true, ""), nil, true, false},
		{"personal mix is never offered", libPlaylist("p.mix", false, "pl.pm-9"), tree, false, true},
		{"catalogue playlist is never offered", catalogue, tree, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ctx := f.r.Context([]kit.Item{c.item}, nil, map[string]any{})
			if c.loaded != nil {
				ctx = ctx.WithData(c.loaded)
			}
			if got := available(d, ctx); got != c.wantAvl {
				t.Errorf("available = %v", got)
			}
			if c.wantAvl {
				if got := enabled(ctx, d); got != c.wantEnabled {
					t.Errorf("enabled = %v", got)
				}
			}
		})
	}
}

func TestActionIDsAreUniqueAndLabelled(t *testing.T) {
	f := newActionsFixture(t)
	seen := map[string]bool{}
	for _, d := range f.p.actions() {
		if seen[d.ID] {
			t.Errorf("duplicate ID %s", d.ID)
		}
		seen[d.ID] = true
		ctx := f.r.Context([]kit.Item{libPlaylist("p.1", true, "")}, nil, map[string]any{})
		if d.Label(ctx) == "" || d.Icon == "" {
			t.Errorf("%s has no label or icon", d.ID)
		}
	}
}
