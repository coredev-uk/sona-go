package playlist

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// inOrder reports whether want is a subsequence of got.
func inOrder(got, want []string) bool {
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	return i == len(want)
}

func stopLabels(stops []access.Stop) []string {
	out := make([]string, len(stops))
	for i, s := range stops {
		out[i] = s.Node.Label
	}
	return out
}

// walk checks what every playlist surface promises assistive technology: no
// control inside a leaf, every Tab stop on the tree, the walk not a trap, and
// want among the stops in reading order. It returns the stops and the tree.
func walk(t *testing.T, tt *uitest.Session, want ...string) ([]string, access.Tree) {
	t.Helper()
	tt.Settle()
	access.NoNestedControls(t, tt.Tester)
	stops := stopLabels(access.TabWalk(t, tt.Tester, 120))
	if !inOrder(stops, want) {
		t.Errorf("Tab walk %q, want in order %q", stops, want)
	}
	return stops, access.Read(t, tt.Tester)
}

// childRoles are the roles of the nodes directly below n.
func childRoles(tr access.Tree, n access.Node) []ui.Role {
	var out []ui.Role
	for _, c := range tr.Children(n) {
		out = append(out, c.Role)
	}
	return out
}

// The sidebar: every section is a heading over a named list of items, an
// item holds the row's opener (link, button or disclosure) and, beside it and
// not inside it, its More button; the section folds and the folders are
// disclosures that say whether they are open; the page's own row says it is
// the current page.
func TestA11ySidebar(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPins())
	e.router.Replace("/playlist/p.own")
	e.shows("Made for you", "Apple Music", "Playlists", "Road Trip", "Alpha")
	stops, tr := walk(t, e.tt,
		"Alpha", "More for Alpha", "Beta, plays", "More for Beta", "Gamma, shuffles", "More for Gamma", "Delta", "More for Delta",
		"Made for you", "Morning Mix", "Discovery Mix", "Apple Music", "Quiet Hours",
		"New playlist or folder", "Running", "More for the folder Running", "Road Trip",
	)
	_ = stops

	for _, h := range []string{"Pinned", "Made for you", "Apple Music", "Playlists"} {
		if n, ok := tr.Find(ui.RoleHeading, h); !ok || n.Level != 2 {
			t.Errorf("no h2 %q: %+v", h, n)
		}
		list, ok := tr.Find(ui.RoleList, h)
		if !ok {
			t.Errorf("no list named %q", h)
			continue
		}
		for _, it := range tr.Children(list) {
			if it.Role != ui.RoleListItem {
				t.Errorf("list %q holds %v", h, it)
			}
		}
	}
	// A row is its opener then its More button: siblings in one item.
	for _, row := range []struct {
		item   string
		opener ui.Role
		name   string
		more   string
	}{
		{"Pinned", ui.RoleLink, "Alpha", "More for Alpha"},
		{"Pinned", ui.RoleButton, "Beta, plays", "More for Beta"},
		{"Made for you", ui.RoleLink, "Morning Mix", "More for Morning Mix"},
		{"Playlists", ui.RoleDisclosure, "Running", "More for the folder Running"},
		{"Playlists", ui.RoleLink, "Road Trip", "More for Road Trip"},
	} {
		list, _ := tr.Find(ui.RoleList, row.item)
		var found bool
		for _, it := range tr.Children(list) {
			var names []string
			for _, c := range tr.Children(it) {
				names = append(names, c.Label)
			}
			if slices.Contains(names, row.name) {
				found = true
				roles := childRoles(tr, it)
				if len(roles) < 2 || roles[0] != row.opener || roles[1] != ui.RoleMenuButton || !slices.Contains(names, row.more) {
					t.Errorf("item of %q holds %v %q, want %v then a menu button %q", row.name, roles, names, row.opener, row.more)
				}
			}
		}
		if !found {
			t.Errorf("no item of %q in the list %q", row.name, row.item)
		}
	}
	// The folders and the foldable sections say whether they are open.
	for name, open := range map[string]bool{"Made for you": true, "Apple Music": true, "Running": false} {
		n, ok := tr.Find(ui.RoleDisclosure, name)
		if !ok || n.Has(access.Expanded) != open {
			t.Errorf("disclosure %q: %+v, want expanded %v", name, n, open)
		}
	}
	// A foldable section's heading controls the list it folds, while it shows.
	for _, name := range []string{"Made for you", "Apple Music"} {
		n, _ := tr.Find(ui.RoleDisclosure, name)
		if got := tr.ControlledBy(n); len(got) != 1 || got[0].Role != ui.RoleList || got[0].Label != name {
			t.Errorf("disclosure %q controls %v, want the list %q", name, got, name)
		}
	}
	// Only the page's own row is the current page.
	for _, n := range tr.Nodes {
		if cur := n.Current == ui.CurrentPage; cur != (n.Role == ui.RoleLink && n.Label == "Road Trip") {
			t.Errorf("%v current %v, description %q", n, n.Current, n.Description)
		}
	}
	// A heading is not a button, and no heading holds one.
	for _, n := range tr.Roles(ui.RoleHeading) {
		if len(tr.Children(n)) != 0 {
			t.Errorf("heading %v holds %v", n, tr.Children(n))
		}
	}
}

// Every item of the sidebar's lists says which of how many it is, for the
// list it is in: a pin, a playlist of a section, and a folder's own
// playlists, which are a list of their own inside the folder's item.
func TestA11ySidebarItemsSayTheirPlaceInTheirList(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPins())
	e.p.prefs.SetFolderOpen("f.run", true)
	e.shows("Alpha", "Morning Mix", "Quiet Hours", "Running", "Road Trip", "Tempo")
	e.tt.Settle()
	tr := access.Read(t, e.tt.Tester)
	// The item is the opener's parent: the opener and its More button are its children.
	for _, tc := range []struct {
		opener ui.Role
		name   string
		pos    int
		size   int
	}{
		{ui.RoleLink, "Alpha", 1, 4},
		{ui.RoleButton, "Beta, plays", 2, 4},
		{ui.RoleButton, "Gamma, shuffles", 3, 4},
		{ui.RoleLink, "Delta", 4, 4},
		{ui.RoleLink, "Morning Mix", 1, 2},
		{ui.RoleLink, "Discovery Mix", 2, 2},
		{ui.RoleLink, "Quiet Hours", 1, 1},
		{ui.RoleDisclosure, "Running", 1, 2},
		{ui.RoleLink, "Road Trip", 2, 2},
		{ui.RoleLink, "Tempo", 1, 1},
	} {
		op, ok := tr.Find(tc.opener, tc.name)
		if !ok {
			t.Errorf("no %v %q:\n%s", tc.opener, tc.name, tr.Dump())
			continue
		}
		item := tr.Nodes[op.Parent]
		if item.Role != ui.RoleListItem || item.PosInSet != tc.pos || item.SetSize != tc.size {
			t.Errorf("the item of %q is a %v at %d of %d, want a list item at %d of %d", tc.name, item.Role, item.PosInSet, item.SetSize, tc.pos, tc.size)
		}
	}
}

// The folds and folders work from the keyboard and say so: Enter on a
// disclosure flips its state on the tree.
func TestA11ySidebarDisclosuresFromTheKeyboard(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Apple Music", "Quiet Hours", "Running")
	focus := func(label string) {
		t.Helper()
		for range 30 {
			e.tt.Key(0, ui.KeyTab)
			e.tt.Frame()
			if tr := access.Read(t, e.tt.Tester); tr.Focus != nil && tr.Focus.Role == ui.RoleDisclosure && tr.Focus.Label == label {
				return
			}
		}
		t.Fatalf("Tab never reached the disclosure %q", label)
	}
	state := func(label string) bool {
		n, ok := access.Read(t, e.tt.Tester).Find(ui.RoleDisclosure, label)
		if !ok {
			t.Fatalf("no disclosure %q", label)
		}
		return n.Has(access.Expanded)
	}
	focus("Apple Music")
	e.tt.Key(0, ui.KeyEnter)
	e.tt.Frame()
	if state("Apple Music") || e.tt.HasText("Quiet Hours") {
		t.Error("Enter did not fold the section")
	}
	e.tt.Key(0, ui.KeyEnter)
	e.tt.Frame()
	if !state("Apple Music") || !e.tt.HasText("Quiet Hours") {
		t.Error("Enter did not unfold the section")
	}
	focus("Running")
	e.tt.Key(0, ui.KeyEnter)
	e.tt.Frame()
	tr := access.Read(t, e.tt.Tester)
	if n, ok := tr.Find(ui.RoleDisclosure, "Running"); !ok || !n.Has(access.Expanded) {
		t.Fatalf("Enter did not open the folder: %+v", n)
	}
	// The folder's playlists are a list inside its item.
	folder, _ := tr.Find(ui.RoleDisclosure, "Running")
	item := tr.Nodes[folder.Parent]
	var inner []access.Node
	for _, c := range tr.Children(item) {
		if c.Role == ui.RoleList {
			inner = tr.Children(c)
		}
	}
	if item.Role != ui.RoleListItem || len(inner) != 1 || inner[0].Role != ui.RoleListItem {
		t.Errorf("the folder's item %v holds %v", item, inner)
	}
}

// A row's More menu opens and closes from the keyboard, its button says
// whether it is open, and the focus returns to it on Escape.
func TestA11ySidebarMoreMenuReturnsTheFocus(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPins())
	e.shows("Alpha")
	if err := e.tt.Click("More for Alpha"); err != nil {
		t.Fatal(err)
	}
	e.tt.Until("the menu", e.tt.Shows("Unpin"))
	if n, ok := access.Read(t, e.tt.Tester).Find(ui.RoleMenuButton, "More for Alpha"); !ok || !n.Has(access.Expanded) {
		t.Errorf("the open menu's button: %+v", n)
	}
	e.tt.Key(0, ui.KeyEscape)
	e.tt.Advance(300 * time.Millisecond)
	e.tt.Drain()
	if e.tt.HasText("Unpin") {
		t.Fatal("Escape left the menu open")
	}
	if !e.tt.Focused("More for Alpha") {
		t.Error("the focus did not return to the More button")
	}
}

// The page of a library playlist: the name is the h1, the controls follow in
// reading order, the songs are a named table and the suggestions a named list
// of items; adding a suggestion is announced and keeps the focus in its row.
func TestA11yPlaylistPage(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.api.suggestions["p.1"] = []services.Song{
		pgSong("s1", "Sugg One", "Singer", "Album S"),
		pgSong("s2", "Sugg Two", "Singer", "Album S"),
	}
	r.shows("Suggested songs", "Sugg One")
	_, tr := walk(t, r.tt,
		"Play Road Trip", "Shuffle", "Favourite Road Trip", "Suggest less like Road Trip", "Edit", "More for Road Trip",
		"Song A", "Play Sugg One", "Add Sugg One to Road Trip", "Play Sugg Two", "Add Sugg Two to Road Trip",
	)
	if h, ok := tr.Find(ui.RoleHeading, "Road Trip"); !ok || h.Level != 1 {
		t.Errorf("the name is not the h1: %+v", h)
	}
	if _, ok := tr.Find(ui.RoleTable, "Songs in Road Trip"); !ok {
		t.Errorf("no named song table:\n%s", tr.Dump())
	}
	if more, ok := tr.Find(ui.RoleMenuButton, "More for Song B"); !ok || more.Has(access.Focusable) {
		t.Errorf("More for Song B: %+v (on the tree at rest, not a stop)", more)
	}
	list, ok := tr.Find(ui.RoleList, "Suggested songs")
	if !ok {
		t.Fatalf("no suggestions list:\n%s", tr.Dump())
	}
	var items int
	for _, n := range tr.Nodes {
		if n.Role == ui.RoleListItem && slices.ContainsFunc(tr.Ancestors(n), func(a access.Node) bool { return a.Index == list.Index }) {
			items++
		}
	}
	if items != 2 {
		t.Errorf("the suggestions are %d items, want 2", items)
	}

	// Adding: the button goes, "Added" shows, a screen reader hears it and the
	// focus stays in the row.
	r.tt.Drain()
	r.tt.Announcements()
	r.click("Add Sugg One to Road Trip")
	if got := r.tt.Announcements(); !slices.Contains(got, "Added “Sugg One” to “Road Trip”") {
		t.Errorf("announcements %q", got)
	}
	if !r.tt.Focused("Play Sugg One") {
		t.Error("the focus did not stay in the suggestion's row")
	}
	r.tt.Until("the edits done", func() bool { return !r.p.edits.editing("p.1") })
}

// A playlist Apple made, or a mix, has no edit controls; one with no songs
// says so.
func TestA11yReadOnlyAndEmptyPlaylists(t *testing.T) {
	r := newPageRig(t, "pl.apple", catalogDetail("pl.apple", "Quiet Hours", threeSongs()...))
	r.shows("Songs in Quiet Hours")
	_, tr := walk(t, r.tt, "Play Quiet Hours", "Shuffle", "Add", "Favourite Quiet Hours", "More for Quiet Hours", "Song A")
	if _, ok := tr.Find(ui.RoleButton, "Edit"); ok {
		t.Error("a playlist nobody can edit offers Edit")
	}
	e := newPageRig(t, "p.2", libraryDetail("p.2", "Empty"))
	e.shows("This playlist is empty")
	_, tr = walk(t, e.tt, "Favourite Empty", "Edit")
	if play, ok := tr.Find(ui.RoleButton, "Play Empty"); !ok || !play.Has(access.Disabled) {
		t.Errorf("Play on an empty playlist: %+v", play)
	}
}

// The editor: the rows are cells of a named table whose drag handle and
// remove button are siblings of the song's cells, Edit and Done hand the focus
// to each other, and every move or removal is announced in a live region.
func TestA11yPlaylistEditor(t *testing.T) {
	r := editingRig(t)
	tr := access.Read(t, r.tt.Tester)
	if !r.tt.Focused("Song A") {
		t.Fatal("the first row has no focus")
	}
	table, ok := tr.Find(ui.RoleTable, "Edit the songs in Road Trip")
	if !ok || table.Description == "" {
		t.Fatalf("the editor is not a described table: %+v\n%s", table, tr.Dump())
	}
	rows := tr.Children(table)
	if len(rows) != 3 {
		t.Fatalf("the editor holds %v, want three rows", rows)
	}
	for _, row := range rows {
		if row.Role != ui.RoleRow {
			t.Errorf("%v is not a row", row)
		}
		for _, c := range tr.Children(row) {
			if c.Role != ui.RoleCell {
				t.Errorf("row %v holds %v: a row holds cells", row, c)
			}
		}
	}
	first := tr.Children(rows[0])
	if h := tr.Children(first[0]); len(h) != 1 || h[0].Role != ui.RoleButton || h[0].Label != "Drag Song A to reorder" {
		t.Errorf("the first cell is %v", h)
	}
	if rm := tr.Children(first[len(first)-1]); len(rm) != 1 || rm[0].Role != ui.RoleButton || rm[0].Label != "Remove Song A from Road Trip" {
		t.Errorf("the last cell is %v", rm)
	}

	// Moves and removals are announced.
	r.tt.Announcements()
	r.tt.Key(ui.Alt, ui.KeyDown)
	r.tt.Frame()
	if got := r.tt.Announcements(); !slices.Contains(got, "Moved Song A to position 2 of 3") {
		t.Errorf("Alt+Down announced %q", got)
	}
	r.tt.Key(ui.Alt, ui.KeyUp)
	r.tt.Frame()
	if got := r.tt.Announcements(); !slices.Contains(got, "Moved Song A to position 1 of 3") {
		t.Errorf("Alt+Up announced %q", got)
	}
	r.tt.Key(0, ui.KeyDelete)
	r.tt.Frame()
	if got := r.tt.Announcements(); !slices.Contains(got, "Removed Song A") {
		t.Errorf("Delete announced %q", got)
	}
	r.click("Remove Song B from Road Trip")
	if got := r.tt.Announcements(); !slices.Contains(got, "Removed Song B") {
		t.Errorf("the cross announced %q", got)
	}
	r.tt.Until("the edits done", func() bool { return !r.p.edits.editing("p.1") })

	// Done hands the focus back to Edit.
	r.click("Done")
	r.tt.Until("Edit has the focus", func() bool { return r.tt.Focused("Edit") })
}

// Entering the editor focuses Done, and the walk through it reaches the
// header's buttons then the rows.
func TestA11yEditorWalk(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Songs in Road Trip")
	r.click("Edit")
	r.shows("Done", "Edit the songs in Road Trip")
	r.tt.Until("Done has the focus", func() bool { return r.tt.Focused("Done") })
	walk(t, r.tt, "Name and description", "Song A", "Done")
	// The buttons that open dialogs say so.
	tr := access.Read(t, r.tt.Tester)
	for _, name := range []string{"Name and description", "Cover"} {
		if n, ok := tr.Find(ui.RoleButton, name); !ok || n.HasPopup != ui.PopupDialog {
			t.Errorf("%q opens a dialog and says so: %+v", name, n)
		}
	}
}

// A shared playlist's collaborators: a button that opens a dialog, named by
// its words and described once, a dialog that lists everyone as links named by all the row
// shows, and Escape returning the focus to the button.
func TestA11yCollaborators(t *testing.T) {
	songs := threeSongs()
	for i := range songs {
		songs[i].CatalogID = "c" + songs[i].ID
	}
	d := catalogDetail("pl.u-9", "Shared mix", songs...)
	d.Kind = "user-shared"
	r := newPageRig(t, "pl.u-9", d)
	r.api.collab["pl.u-9"] = services.PlaylistCollaboration{
		Collaborative: true,
		OwnerID:       "sp.1",
		Collaborators: []services.SocialProfile{
			{ID: "sp.1", Name: "Ada", Handle: "ada"},
			{ID: "sp.2", Name: "Bo", Handle: "bo"},
			{ID: "sp.3", Name: "Cy", Handle: "cy"},
		},
		Former:  []services.SocialProfile{{ID: "sp.4", Name: "Dee", Handle: "dee"}},
		Viewer:  services.RoleListener,
		AddedBy: []services.TrackAddedBy{{SongID: "cb", ProfileID: "sp.2"}},
	}
	r.shows("Collaborative playlist · 3 songs, 10 min", "Ada & 2 others", "Added by")
	stops, tr := walk(t, r.tt, "Ada & 2 others", "Play Shared mix", "Shuffle")
	if stops[0] != "Ada & 2 others" {
		t.Errorf("the byline is not the first stop: %q", stops)
	}
	btn, ok := tr.Find(ui.RoleButton, "Ada & 2 others")
	if !ok || btn.Description != "View playlist collaborators" || btn.HasPopup != ui.PopupDialog {
		t.Fatalf("collaborators button %+v:\n%s", btn, tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleColumnHeader, "Added by"); !ok {
		t.Error("no Added by column header")
	}

	// Open it from the keyboard.
	for range 20 {
		r.tt.Key(0, ui.KeyTab)
		r.tt.Frame()
		if r.tt.Focused("Ada & 2 others") {
			break
		}
	}
	r.tt.Key(0, ui.KeyEnter)
	r.shows("Collaborators", "Former collaborators")
	r.tt.Advance(300 * time.Millisecond)
	r.tt.Settle()
	access.NoNestedControls(t, r.tt.Tester)
	tr = access.Read(t, r.tt.Tester)
	dlg, ok := tr.Find(ui.RoleDialog, "Collaborators")
	if !ok || dlg.Description != "3 people can add songs to this playlist." {
		t.Fatalf("the dialog %+v:\n%s", dlg, tr.Dump())
	}
	for _, name := range []string{"Ada @ada Owner", "Bo @bo", "Cy @cy", "Dee @dee"} {
		n, ok := tr.Find(ui.RoleLink, name)
		if !ok || !tr.Within(n, ui.RoleDialog) || !tr.Within(n, ui.RoleListItem) || !tr.Within(n, ui.RoleList) {
			t.Errorf("person %q: %+v", name, n)
		}
	}
	for _, list := range []string{"Collaborators", "Former collaborators"} {
		if _, ok := tr.Find(ui.RoleList, list); !ok {
			t.Errorf("no list %q", list)
		}
	}
	stops = stopLabels(access.TabWalk(t, r.tt.Tester, 30))
	for _, s := range stops {
		if !slices.Contains([]string{"Ada @ada Owner", "Bo @bo", "Cy @cy", "Dee @dee", "Close"}, s) {
			t.Errorf("the focus left the dialog for %q (%q)", s, stops)
		}
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300 * time.Millisecond)
	r.tt.Drain()
	if r.p.page.collab.open {
		t.Fatal("Escape left the dialog open")
	}
	if !r.tt.Focused("Ada & 2 others") {
		t.Error("the focus did not return to the collaborators button")
	}
}

// The playlist dialogs: each opens from a button, takes the focus, keeps Tab
// inside, names its controls, and gives the focus back to the button on Escape.
func TestA11yDialogs(t *testing.T) {
	f := folder("pf.1", "Mixes", "", 0)
	for _, c := range []struct {
		name  string
		state dialogState
		title string
		first string   // what has the focus once it is open
		stops []string // among the Tab stops, in order
	}{
		{"create", dialogState{kind: dialogCreate}, "New playlist", "Name", []string{"Name", "Description", "Show on my profile", "Cancel", "Close"}},
		{"details", dialogState{kind: dialogDetails, id: "p.a", name: "Road Trip", description: "Songs", isPublic: true}, "Edit playlist", "Name", []string{"Name", "Description", "Show on my profile", "Cancel", "Save", "Close"}},
		{"new folder", dialogState{kind: dialogFolder}, "New folder", "Name", []string{"Name", "Cancel", "Close"}},
		{"rename folder", dialogState{kind: dialogFolder, folder: &f}, "Rename folder", "Name", []string{"Name", "Cancel", "Rename", "Close"}},
		{"add picker", dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel}, "Add to playlist", "Find a playlist", []string{"Find a playlist", "New playlist", "Road Trip", "Close"}},
		{"move", dialogState{kind: dialogMove, id: "p.b", name: "Gym", folderID: "pf.1"}, "Move to folder", "Top level", []string{"Top level", "Mixes", "Close"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			theme.SetMode(theme.ModeLight)
			t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
			t.Cleanup(kit.CancelConfirm)
			p, api, _, h, _ := testPlaylists(t, "/library/albums")
			api.tree = services.PlaylistTree{
				Folders: []services.PlaylistFolder{folder("pf.1", "Mixes", "", 0)},
				Playlists: []services.TreePlaylist{
					treePlaylist("p.a", "Road Trip", "", 0, true),
					treePlaylist("p.b", "Gym", "pf.1", 0, true),
				},
			}
			tt := h.Tester(t, func(ctx *ui.Context) {
				ctx.DisableAnimations()
				ui.Column(ctx).Padding(20).AlignItems(ui.Start).Children(func() {
					if kit.Button(ctx.Key("opener"), "Open dialog", kit.ButtonOpts{}).Clicked() {
						p.dialogs.show(c.state)
					}
				})
				p.dialogHost(ctx)
				kit.OverlayLayer(ctx)
			}, 900, 700)
			tt.Key(0, ui.KeyTab)
			if !tt.Focused("Open dialog") {
				t.Fatal("the opener is not the first stop")
			}
			tt.Key(0, ui.KeyEnter)
			tt.Advance(300 * time.Millisecond)
			tt.Settle()
			tt.Advance(300 * time.Millisecond)
			tt.Drain()

			tr := access.Read(t, tt.Tester)
			dlg, ok := tr.Find(ui.RoleDialog, c.title)
			if !ok {
				t.Fatalf("no dialog %q:\n%s", c.title, tr.Dump())
			}
			if h, ok := tr.Find(ui.RoleHeading, c.title); !ok || h.Level != 2 || !tr.Within(h, ui.RoleDialog) {
				t.Errorf("the dialog's heading: %+v", h)
			}
			if tr.Focus == nil || !tr.Within(*tr.Focus, ui.RoleDialog) || tr.Focus.Label != c.first {
				t.Errorf("focus %+v, want %q in the dialog", tr.Focus, c.first)
			}
			access.NoNestedControls(t, tt.Tester)
			stops := stopLabels(access.TabWalk(t, tt.Tester, 40))
			if !inOrder(stops, c.stops) {
				// The walk starts after the first field: compare as a set.
				for _, w := range c.stops {
					if !slices.Contains(stops, w) {
						t.Errorf("Tab walk %q lacks %q", stops, w)
					}
				}
			}
			tr = access.Read(t, tt.Tester)
			for _, n := range tr.Nodes {
				if n.Has(access.Focusable) && !tr.Within(n, ui.RoleDialog) && n.Index != dlg.Index {
					t.Errorf("%v is a stop outside the open dialog", n)
				}
			}

			tt.Key(0, ui.KeyEscape)
			tt.Advance(300 * time.Millisecond)
			tt.Drain()
			if p.dialogs.open {
				t.Fatal("Escape left the dialog open")
			}
			if !tt.Focused("Open dialog") {
				t.Error("the focus did not return to the opener")
			}
		})
	}
}

// The picker's empty and filtered states are statuses, announced once, and the
// current folder of the Move dialog is on the tree.
func TestA11yPickerStatusAndCurrentFolder(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.show(dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel})
	r.tt.Announcements()
	r.typed("zzz")
	r.shows("No playlist you can edit is called “zzz”.")
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleStatus, "No playlist you can edit is called “zzz”."); !ok {
		t.Errorf("the empty filter is not a status:\n%s", tr.Dump())
	}
	r.tt.Drain()
	var said int
	for _, a := range tr.Announcements {
		if a == "No playlist you can edit is called “zzz”." {
			said++
		}
	}
	got := r.tt.Announcements()
	for _, a := range got {
		if a == "No playlist you can edit is called “zzz”." {
			said++
		}
	}
	if said != 1 {
		t.Errorf("the empty filter was announced %d times (%q)", said, got)
	}

	m := newDialogsRig(t)
	m.seedTree()
	m.show(dialogState{kind: dialogMove, id: "p.a", name: "Road Trip", folderID: "pf.2"})
	m.shows("Move to folder", "Mixes / Old")
	tr = access.Read(t, m.tt.Tester)
	for _, f := range []struct {
		name    string
		current bool
	}{{"Top level", false}, {"Mixes", false}, {"Mixes / Old", true}} {
		n, ok := tr.Find(ui.RoleButton, f.name)
		if !ok || (n.Description == "Current") != f.current {
			t.Errorf("folder %q: %+v, current %v", f.name, n, f.current)
		}
	}
}

// The songs' find field is a search landmark holding a named text field, in
// the Tab order before the sort buttons of the headers (each inside its column
// header) and the songs; the order the songs are in is said by the header
// that sorts them, and a third click leaves none.
func TestA11yPlaylistFindAndSort(t *testing.T) {
	r := viewRig(t)
	_, tr := walk(t, r.tt, "Edit", "More for Road Trip", "Find in playlist", "Title", "Artist", "Album", "Duration", "Charlie")

	land, ok := tr.Find(ui.RoleSearch, "Find songs in this playlist")
	if !ok {
		t.Fatalf("no search landmark:\n%s", tr.Dump())
	}
	field, ok := tr.Find(ui.RoleTextField, "Find in playlist")
	if !ok || !slices.ContainsFunc(tr.Ancestors(field), func(a access.Node) bool { return a.Index == land.Index }) {
		t.Errorf("the field is not inside the landmark: %+v\n%s", field, tr.Dump())
	}
	for _, title := range []string{"Title", "Artist", "Album", "Duration"} {
		h, ok := tr.Find(ui.RoleColumnHeader, title)
		b, bok := tr.Find(ui.RoleButton, title)
		if !ok || !bok || b.Parent != h.Index || !b.Can(access.Press) {
			t.Errorf("%s: header %+v holds no sort button %+v", title, h, b)
		}
		if got := sortOfHeader(t, tr, title); got != "" {
			t.Errorf("%s is sorted %q before any click", title, got)
		}
	}

	for _, step := range []struct{ title, want string }{
		{"Artist", "ascending"}, {"Artist", "descending"}, {"Artist", ""}, {"Duration", "ascending"},
	} {
		r.click(step.title)
		r.tt.Settle()
		tr = access.Read(t, r.tt.Tester)
		for _, title := range []string{"Title", "Artist", "Album", "Duration"} {
			want := ""
			if title == step.title {
				want = step.want
			}
			if got := sortOfHeader(t, tr, title); got != want {
				t.Errorf("after a click on %s, %s is sorted %q, want %q", step.title, title, got, want)
			}
		}
	}

	// With a query the table has the songs found, and says how many.
	r.find("salt")
	r.tt.Until("the count", r.tt.Shows("2 of 4 songs"))
	tr = access.Read(t, r.tt.Tester)
	tbl, ok := tr.Find(ui.RoleTable, "Songs in Road Trip")
	if !ok || tbl.RowCount != 3 {
		t.Errorf("the table of the songs found: %+v, want 3 rows with the header", tbl)
	}
	if _, ok := tr.Find(ui.RoleButton, "Clear search"); !ok {
		t.Error("no way to clear the query for assistive technology")
	}
}
