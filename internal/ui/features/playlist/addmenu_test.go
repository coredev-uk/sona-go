package playlist

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// addTree is a library with a recent playlist in a folder, a folder in it,
// two playlists at the top and one the user cannot edit.
func addTree() services.PlaylistTree {
	return services.PlaylistTree{
		Folders: []services.PlaylistFolder{folder("pf.1", "Mixes", "", 0), folder("pf.2", "Old", "pf.1", 0)},
		Playlists: []services.TreePlaylist{
			treePlaylist("p.w", "Weekend", "", 0, true),
			treePlaylist("p.r", "Roadie", "", 1, true),
			treePlaylist("p.g", "Gym", "pf.1", 0, true),
			treePlaylist("p.d", "Dusty", "pf.2", 0, true),
			treePlaylist("p.c", "Apple Curated", "", 2, false),
		},
	}
}

// addMenuRig is the playlist page of viewSongs over a library of tree, whose
// recent targets are recents.
func addMenuRig(t *testing.T, tree services.PlaylistTree, recents ...string) *pageRig {
	t.Helper()
	r := newPageRigWith(t, "p.1", func(api *fakeAPI) { api.tree, api.recents = tree, recents },
		libraryDetail("p.1", "Road Trip", viewSongs()...))
	r.shows("Charlie", "Find in playlist")
	return r
}

// openAddMenu opens the menu of the song "alpha" and waits for its playlists
// to load, so "Add to playlist" has become a submenu.
func (r *pageRig) openAddMenu() {
	r.t.Helper()
	if err := r.tt.RightClick("alpha"); err != nil {
		r.t.Fatal(err)
	}
	r.shows("Add to playlist")
	r.did("Tree")
	r.did("RecentTargets")
	r.tt.Settle()
}

// hover moves the pointer onto the row showing text, as a real move does.
func (r *pageRig) hover(text string) {
	r.t.Helper()
	rc, ok := r.tt.Find(text)
	if !ok {
		r.t.Fatalf("no %q: %q", text, r.tt.Texts())
	}
	r.tt.Move(rc.X-1, rc.Y-1)
	r.tt.Move(rc.X+rc.W/2, rc.Y+rc.H/2)
	r.tt.Move(rc.X+rc.W/2+1, rc.Y+rc.H/2+1)
	r.tt.Advance(50 * time.Millisecond)
}

// indexes returns where each text first shows in the frame, -1 for none.
func (r *pageRig) indexes(texts ...string) []int {
	out := make([]int, len(texts))
	all := r.tt.Texts()
	for i, s := range texts {
		out[i] = slices.Index(all, s)
	}
	return out
}

func TestAddToPlaylistIsASubmenuOfNewPlaylistAndThePlaylists(t *testing.T) {
	r := addMenuRig(t, addTree(), "p.g", "p.c")
	r.openAddMenu()
	r.hover("Add to playlist")
	r.shows("New playlist…", "Gym", "Mixes", "Roadie", "Weekend")
	if r.tt.HasText("Apple Curated") {
		t.Error("a playlist the user cannot edit is offered")
	}
	if r.tt.HasText("Find a playlist") {
		t.Error("the picker opened for the hover")
	}
	// New playlist… first, then the recent one, then the tree in the
	// sidebar's order: folders, then playlists.
	idx := r.indexes("New playlist…", "Gym", "Mixes", "Weekend", "Roadie")
	if !slices.IsSorted(idx) || slices.Contains(idx, -1) {
		t.Errorf("order of New playlist…, Gym, Mixes, Weekend, Roadie: %v", idx)
	}

	// The folder is a submenu of its own, with the folder in it.
	r.hover("Mixes")
	r.shows("Old")
	r.hover("Old")
	r.shows("Dusty")
}

func TestChoosingAPlaylistAddsTheItemAndSaysSo(t *testing.T) {
	r := addMenuRig(t, addTree(), "p.g")
	r.tt.Announcements()
	r.openAddMenu()
	r.hover("Add to playlist")
	r.shows("Weekend")
	r.click("Weekend")
	r.did("Add p.w [song:b]")
	r.shows("Added “alpha” to “Weekend”", "Open")
	if r.tt.HasText("Find a playlist") {
		t.Error("the picker opened too")
	}
}

func TestChoosingAPlaylistInAFolderAddsToIt(t *testing.T) {
	r := addMenuRig(t, addTree())
	r.openAddMenu()
	r.hover("Add to playlist")
	r.hover("Mixes")
	r.hover("Old")
	r.shows("Dusty")
	r.click("Dusty")
	r.did("Add p.d [song:b]")
	r.shows("Added “alpha” to “Dusty”")
}

func TestAddSubmenuKeys(t *testing.T) {
	r := addMenuRig(t, addTree())
	r.openAddMenu()
	r.tt.Key(0, ui.KeyDown) // the menu's first row: Add to playlist
	r.tt.Key(0, ui.KeyRight)
	r.shows("New playlist…", "Weekend")
	r.tt.Key(0, ui.KeyLeft)
	r.tt.Advance(50 * time.Millisecond)
	if r.tt.HasText("New playlist…") {
		t.Fatal("Left did not close the submenu")
	}
	if !r.tt.HasText("Add to playlist") {
		t.Fatal("Left closed the menu")
	}
	r.tt.Key(0, ui.KeyRight)
	r.shows("New playlist…")
	// First row is New playlist…; Mixes is the folder at the third row
	// (rows: New playlist…, Mixes, Roadie, Weekend).
	r.tt.Key(0, ui.KeyDown)
	r.tt.Key(0, ui.KeyRight)
	r.shows("Old")
	r.tt.Key(0, ui.KeyDown) // Gym
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Advance(300 * time.Millisecond)
	r.did("Add p.g [song:b]")
}

func TestNewPlaylistInTheSubmenuMakesOneWithTheItem(t *testing.T) {
	r := addMenuRig(t, addTree())
	r.openAddMenu()
	r.hover("Add to playlist")
	r.click("New playlist…")
	r.tt.Until("the form", r.tt.Shows("Create playlist", "With “alpha”."))
	if r.tt.HasText("Find a playlist") {
		t.Error("the picker opened")
	}
	r.tt.Advance(300 * time.Millisecond)
	r.tt.Type("Mix")
	r.tt.Key(0, ui.KeyEnter)
	r.did(`Create "Mix" desc="" public=false items=[song:b] folder=""`)
	r.shows("Added “alpha” to “Mix”")
}

// Before the playlists load, and with none to list, the row is the plain one
// that opens the picker, which loads them itself and says when there are
// none.
func TestAddToPlaylistWithNoPlaylistsOpensThePicker(t *testing.T) {
	r := addMenuRig(t, services.PlaylistTree{Playlists: []services.TreePlaylist{treePlaylist("p.c", "Apple Curated", "", 0, false)}})
	r.openAddMenu()
	r.click("Add to playlist")
	r.tt.Until("the picker", r.tt.Shows("Find a playlist", "New playlist", "You have no playlists you can edit yet."))
}

func TestAddSubmenuOffersMorePlaylistsBelowTwoFolders(t *testing.T) {
	tree := services.PlaylistTree{
		Folders: []services.PlaylistFolder{
			folder("pf.1", "Trips", "", 0), folder("pf.2", "Summer", "pf.1", 0), folder("pf.3", "Beach", "pf.2", 0),
		},
		Playlists: []services.TreePlaylist{
			treePlaylist("p.1", "Camping", "pf.2", 0, true),
			treePlaylist("p.2", "Sandcastles", "pf.3", 0, true),
		},
	}
	r := addMenuRig(t, tree)
	r.openAddMenu()
	r.hover("Add to playlist")
	r.hover("Trips")
	r.shows("Summer")
	r.hover("Summer")
	r.shows("Camping", "More playlists…")
	if r.tt.HasText("Beach") || r.tt.HasText("Sandcastles") {
		t.Errorf("the third folder is in the menu: %q", r.tt.Texts())
	}
	r.click("More playlists…")
	r.tt.Until("the picker", r.tt.Shows("Find a playlist", "Sandcastles", "Camping"))
}

func TestAddSubmenuOffersMorePlaylistsPastFifteen(t *testing.T) {
	var tree services.PlaylistTree
	for i := range 20 {
		tree.Playlists = append(tree.Playlists, treePlaylist(fmt.Sprintf("p.%02d", i), fmt.Sprintf("List %02d", i+1), "", i, true))
	}
	r := addMenuRig(t, tree)
	r.openAddMenu()
	r.hover("Add to playlist")
	r.shows("New playlist…", "List 01", "List 15", "More playlists…")
	if r.tt.HasText("List 16") {
		t.Error("a 16th playlist is in the menu")
	}
	r.click("More playlists…")
	r.tt.Until("the picker", r.tt.Shows("Find a playlist", "List 20"))
}

func addTargetsOf(t *testing.T, f *actionsFixture, items []kit.Item, id string) (*kit.ActionDef, *kit.ActionContext) {
	t.Helper()
	def := f.def(t, id)
	ctx := f.r.Context(items, nil, nil)
	v := def.Data[0].Load(context.Background(), items)
	return def, ctx.WithData(map[string]any{def.Data[0].ID: v})
}

func choiceKeys(choices []kit.ActionChoice) []string {
	out := make([]string, len(choices))
	for i, c := range choices {
		out[i] = c.Key
	}
	return out
}

func TestAddToPlaylistForASelectionAddsAllOfIt(t *testing.T) {
	f := newActionsFixture(t)
	f.api.tree = addTree()
	items := []kit.Item{libSong("s.1"), kit.AlbumItem(services.Album{ID: "l.9", Name: "Low Tide", Source: services.SourceLibrary})}
	def, ctx := addTargetsOf(t, f, items, "playlists.add-to.many")
	choices := def.Choices(ctx)
	if got := choiceKeys(choices); !slices.Equal(got, []string{"new", "folder:pf.1", "in:p.w", "in:p.r"}) {
		t.Fatalf("choices %v", got)
	}
	choices[2].Run(ctx)
	waitDid(t, f.api, "Add p.w [song:s.1 album:l.9]")

	// New playlist… carries the whole selection into the form.
	choices[0].Run(ctx)
	if s := f.p.dialogs.s; s.kind != dialogCreate || len(s.items) != 2 {
		t.Errorf("New playlist… opened %+v", s)
	}
}

func TestAddToPlaylistRowIsPlainUntilThePlaylistsLoad(t *testing.T) {
	f := newActionsFixture(t)
	f.api.tree = addTree()
	items := []kit.Item{libSong("s.1")}
	def := f.def(t, "playlists.add-to.song")
	ctx := f.r.Context(items, nil, nil)
	if got := def.Choices(ctx); got != nil {
		t.Errorf("choices before the data loaded: %v", choiceKeys(got))
	}
	def.Run(ctx)
	if s := f.p.dialogs.s; !f.p.dialogs.open || s.kind != dialogAdd {
		t.Errorf("the plain row opened %+v", s)
	}
}

func TestAddToPlaylistRowIsPlainWhenTheTreeFailsToLoad(t *testing.T) {
	f := newActionsFixture(t)
	f.api.failReads = true
	def, ctx := addTargetsOf(t, f, []kit.Item{libSong("s.1")}, "playlists.add-to.song")
	if got := def.Choices(ctx); got != nil {
		t.Errorf("choices over a failed read: %v", choiceKeys(got))
	}
}

func entryNames(entries []addEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		switch {
		case e.more:
			out[i] = "…"
		case e.folder():
			out[i] = e.name + "/"
		default:
			out[i] = e.name
		}
	}
	return out
}

func TestBuildAddMenuLeadsWithRecentThenFoldersThenPlaylists(t *testing.T) {
	m := buildAddMenu(addTree(), []string{"p.r", "p.c", "p.g", "p.unknown"})
	// Recents are the ones the user can edit, in Apple's order; the top level
	// does not list one twice; Gym stays in its folder too.
	want := []string{"Roadie", "Gym", "Mixes/", "Weekend"}
	if got := entryNames(m.entries); !slices.Equal(got, want) {
		t.Fatalf("top level %v, want %v", got, want)
	}
	mixes := m.entries[2]
	if got := entryNames(mixes.sub); !slices.Equal(got, []string{"Old/", "Gym"}) {
		t.Errorf("Mixes: %v", got)
	}
	if !m.entries[0].recent || !m.entries[1].recent || m.entries[3].recent {
		t.Errorf("recent marks: %+v", m.entries)
	}
}

func TestBuildAddMenuLeavesOutWhatTheUserCannotEdit(t *testing.T) {
	tree := services.PlaylistTree{
		Folders:   []services.PlaylistFolder{folder("pf.1", "Apple", "", 0), folder("pf.2", "Empty", "", 1)},
		Playlists: []services.TreePlaylist{treePlaylist("p.1", "Curated", "pf.1", 0, false), treePlaylist("p.2", "Mine", "", 0, true)},
	}
	if got := entryNames(buildAddMenu(tree, nil).entries); !slices.Equal(got, []string{"Mine"}) {
		t.Errorf("entries %v: folders with nothing to add to are listed", got)
	}
	if got := buildAddMenu(services.PlaylistTree{}, nil).entries; len(got) != 0 {
		t.Errorf("an empty library lists %v", got)
	}
}

func TestBuildAddMenuLimits(t *testing.T) {
	var tree services.PlaylistTree
	for i := range 16 {
		tree.Playlists = append(tree.Playlists, treePlaylist(fmt.Sprintf("p.%02d", i), fmt.Sprintf("L%02d", i), "", i, true))
	}
	got := entryNames(buildAddMenu(tree, nil).entries)
	if len(got) != addMenuLevel+1 || got[addMenuLevel] != "…" || got[addMenuLevel-1] != "L14" {
		t.Errorf("16 playlists: %v", got)
	}
	// Exactly fifteen fit.
	tree.Playlists = tree.Playlists[:15]
	if got := entryNames(buildAddMenu(tree, nil).entries); len(got) != 15 || got[14] != "L14" {
		t.Errorf("15 playlists: %v", got)
	}

	// Folders: two deep are submenus, a third is the picker's.
	deep := services.PlaylistTree{
		Folders: []services.PlaylistFolder{
			folder("a", "A", "", 0), folder("b", "B", "a", 0), folder("c", "C", "b", 0), folder("d", "D", "c", 0),
		},
		Playlists: []services.TreePlaylist{
			treePlaylist("p.b", "InB", "b", 0, true), treePlaylist("p.c", "InC", "c", 0, true), treePlaylist("p.d", "InD", "d", 0, true),
		},
	}
	a := buildAddMenu(deep, nil).entries
	if len(a) != 1 || a[0].name != "A" || !a[0].folder() {
		t.Fatalf("top level %v", entryNames(a))
	}
	b := a[0].sub
	if got := entryNames(b); !slices.Equal(got, []string{"B/"}) {
		t.Fatalf("A: %v", got)
	}
	if got := entryNames(b[0].sub); !slices.Equal(got, []string{"InB", "…"}) {
		t.Errorf("B: %v (C and below are the picker's)", got)
	}

	// A third folder with nothing to add to costs no "More playlists…".
	deep.Playlists = deep.Playlists[:1]
	b = buildAddMenu(deep, nil).entries[0].sub
	if got := entryNames(b[0].sub); !slices.Equal(got, []string{"InB"}) {
		t.Errorf("B with an empty C: %v", got)
	}
}

func TestAddChoicesDrawRulesBetweenTheirParts(t *testing.T) {
	f := newActionsFixture(t)
	f.api.tree, f.api.recents = addTree(), []string{"p.g"}
	def, ctx := addTargetsOf(t, f, []kit.Item{libSong("s.1")}, "playlists.add-to.song")
	choices := def.Choices(ctx)
	// After New playlist…, and between the recent playlist and the rest.
	if !choices[1].Rule || choices[1].Key != "recent:p.g" || !choices[2].Rule {
		t.Errorf("rules: %+v", choices)
	}
	if choices[0].Icon != "plus" || choices[2].Icon != "folder-closed" {
		t.Errorf("icons: %q %q", choices[0].Icon, choices[2].Icon)
	}
}
