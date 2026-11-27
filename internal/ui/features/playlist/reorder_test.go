package playlist

import (
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// reorderTree is three playlists of the user's own at the top (Road Trip,
// Commute, Zed), one joined collaborative playlist (Shared: not editable,
// in the user's Playlists), and the folder Running holding Tempo.
func reorderTree() services.PlaylistTree {
	shared := treePlaylist("p.shared", "Shared", "", 6, false)
	shared.Collaborative = true
	return services.PlaylistTree{
		Folders: []services.PlaylistFolder{folder("f.run", "Running", "", 0)},
		Playlists: []services.TreePlaylist{
			treePlaylist("p.own", "Road Trip", "", 3, true),
			treePlaylist("p.commute", "Commute", "", 5, true),
			shared,
			treePlaylist("p.zed", "Zed", "", 9, true),
			treePlaylist("p.run", "Tempo", "f.run", 0, true),
		},
	}
}

// pickUp presses on the sidebar row showing name and drags it off, so the
// row is the source of a drag.
func (e *sbEnv) pickUp(name string) {
	e.t.Helper()
	e.t.Cleanup(func() {
		e.tt.Release(e.atX, e.atY)
		e.tt.Frame()
		e.tt.Frame()
		if kit.DragActive(nil) {
			e.t.Error("a drag is still going at the end of the test")
		}
	})
	x, y := e.center(name)
	e.tt.Press(x, y)
	e.tt.Move(x+4, y)
	e.tt.Move(x+10, y)
	e.tt.Move(x+14, y)
	if !kit.DragActive(nil) {
		e.t.Fatalf("a drag did not start on %q", name)
	}
	e.atX, e.atY = x+14, y
	e.tt.Frame()
	e.tt.Frame()
}

// at moves the pointer to the row showing s, dy DIPs below its middle (a
// row is 32 high: -14 is its top edge, +14 its bottom edge).
func (e *sbEnv) at(s string, dy float32) {
	e.t.Helper()
	x, y := e.center(s)
	e.tt.Move(x, y+dy)
	e.tt.Move(x+1, y+dy)
	e.atX, e.atY = x+1, y+dy
	e.tt.Frame()
	e.tt.Frame()
}

// moves are the calls that moved or placed a playlist.
func (e *sbEnv) moves() []string {
	var out []string
	for _, c := range e.api.asked() {
		if strings.HasPrefix(c, "MoveToFolder ") || strings.HasPrefix(c, "ReorderPlaylist ") {
			out = append(out, c)
		}
	}
	return out
}

func TestZoneAt(t *testing.T) {
	for _, c := range []struct {
		name   string
		y      float32
		canAdd bool
		want   dropZone
	}{
		{"top edge of a row that takes songs", 3, true, zoneBefore},
		{"middle of a row that takes songs", 16, true, zoneOnto},
		{"bottom edge of a row that takes songs", 30, true, zoneAfter},
		{"upper half of a row that takes none", 15, false, zoneBefore},
		{"lower half of a row that takes none", 17, false, zoneAfter},
		{"a row not laid out yet", 5, true, zoneOnto},
	} {
		h := float32(32)
		if strings.HasPrefix(c.name, "a row not") {
			h = 0
		}
		if got := zoneAt(c.y, h, c.canAdd); got != c.want {
			t.Errorf("%s: zone %v, want %v", c.name, got, c.want)
		}
	}
}

func TestTheTreeListsThePlaylistsInTheUsersOrderThenApples(t *testing.T) {
	mk := func(id string, position, order int) services.TreePlaylist {
		tp := treePlaylist(id, id, "", position, true)
		tp.Order = order
		return tp
	}
	tree := services.PlaylistTree{Playlists: []services.TreePlaylist{
		mk("a", 0, 0), mk("b", 1, 2), mk("c", 2, 0), mk("d", 3, 1), mk("e", 4, 3),
	}}
	var got []string
	for _, n := range buildTree(tree, nil) {
		got = append(got, n.Item.Playlist.ID)
	}
	if want := []string{"d", "b", "e", "a", "c"}; !slices.Equal(got, want) {
		t.Fatalf("order %v, want the user's order (d, b, e) then Apple's (a, c): %v", got, want)
	}
}

func TestDraggingARowOntoAFolderMovesItIn(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Commute", "Running")
	e.pickUp("Commute")
	e.over("Running")
	if !e.tt.HasText("Move to “Running”") {
		t.Errorf("the ghost does not say the drop moves the playlist in: %q", e.tt.Texts())
	}
	e.drop()
	e.did(`MoveToFolder p.commute "f.run"`)
	if got := e.moves(); len(got) != 1 {
		t.Errorf("moves %v, want only the move into the folder", got)
	}

	// Inside the folder now: under Tempo, and no longer under Road Trip.
	e.p.prefs.SetFolderOpen("f.run", true)
	e.tt.Until("Commute in Running", func() bool {
		return e.tt.HasText("Tempo") && e.tt.HasText("Commute") && e.y("Commute") < e.y("Road Trip")
	})
	e.above("Tempo", "Commute")
}

func TestDraggingARowOntoItsOwnFolderMovesNothing(t *testing.T) {
	inBubble(t, func(t *testing.T) {
		e := sbSetup(t, reorderTree(), nil)
		e.p.prefs.SetFolderOpen("f.run", true)
		e.shows("Tempo", "Running")
		e.pickUp("Tempo")
		e.over("Running")
		if !e.tt.HasText("Already in “Running”") {
			t.Errorf("the ghost does not say it is already there: %q", e.tt.Texts())
		}
		e.drop()
		e.quiet()
		if got := e.moves(); len(got) != 0 {
			t.Errorf("moves %v", got)
		}
	})
}

func TestDraggingARowToTheListsSpaceMovesItOutOfItsFolder(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.p.prefs.SetFolderOpen("f.run", true)
	e.shows("Tempo", "Running")
	e.pickUp("Tempo")
	e.over("Playlists") // the section's heading: no row takes the drop
	if !e.tt.HasText("Move to the top level") {
		t.Errorf("the ghost does not say the drop moves it to the top level: %q", e.tt.Texts())
	}
	e.drop()
	e.did(`MoveToFolder p.run ""`)
	e.shows("Moved “Tempo” to the top level")
}

func TestDraggingARowBetweenRowsPlacesIt(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Road Trip", "Commute", "Zed")
	e.above("Road Trip", "Commute")

	// The top edge of Road Trip: before it.
	e.pickUp("Commute")
	e.at("Road Trip", -14)
	if !e.tt.HasText("Move before “Road Trip”") {
		t.Errorf("no caption for a place before: %q", e.tt.Texts())
	}
	e.drop()
	e.did(`ReorderPlaylist p.commute "p.own"`)
	e.tt.Until("Commute above Road Trip", func() bool { return e.tt.HasText("Commute") && e.y("Commute") < e.y("Road Trip") })
	if got := e.moves(); len(got) != 1 {
		t.Errorf("moves %v, want the one reorder", got)
	}
}

func TestDroppingOnTheBottomEdgePlacesItBeforeTheNextRow(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Road Trip", "Commute", "Zed")
	// Zed goes under Road Trip: before Commute, the playlist listed next.
	e.pickUp("Zed")
	e.at("Road Trip", 14)
	if !e.tt.HasText("Move after “Road Trip”") {
		t.Errorf("no caption for a place after: %q", e.tt.Texts())
	}
	e.drop()
	e.did(`ReorderPlaylist p.zed "p.commute"`)
	e.tt.Until("Zed under Road Trip", func() bool { return e.y("Zed") > e.y("Road Trip") && e.y("Zed") < e.y("Commute") })
}

func TestDroppingUnderTheLastRowPlacesItLast(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Road Trip", "Zed")
	e.pickUp("Road Trip")
	e.at("Zed", 14)
	e.drop()
	e.did(`ReorderPlaylist p.own ""`)
	e.tt.Until("Road Trip below Zed", func() bool { return e.y("Road Trip") > e.y("Zed") })
}

func TestDroppingARowWhereItIsMovesNothing(t *testing.T) {
	inBubble(t, func(t *testing.T) {
		e := sbSetup(t, reorderTree(), nil)
		e.shows("Road Trip", "Commute")
		e.pickUp("Commute")
		e.at("Road Trip", 14) // after Road Trip: where Commute already is
		e.drop()
		e.quiet()
		if got := e.moves(); len(got) != 0 {
			t.Errorf("moves %v", got)
		}
	})
}

// The middle of a playlist's row is still the drop that adds songs: a
// playlist dropped there adds its songs and does not move.
func TestDroppingARowOnAnotherRowsMiddleStillAddsItsSongs(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Road Trip", "Commute")
	e.pickUp("Commute")
	e.at("Road Trip", 0)
	if !e.tt.HasText("Add to “Road Trip”") {
		t.Errorf("the middle of the row does not offer the add: %q", e.tt.Texts())
	}
	e.drop()
	e.did("Add p.own [playlist:p.commute]")
	if got := e.moves(); len(got) != 0 {
		t.Errorf("the add also moved: %v", got)
	}
}

// A playlist the user cannot edit takes no songs, but it is a place like
// any other row: its whole row is an edge.
func TestARowThatTakesNoSongsIsOnlyAPlace(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Zed", "Shared")
	e.pickUp("Zed")
	e.at("Shared", 0)
	if e.tt.HasText("Add to “Shared”") {
		t.Errorf("a playlist that cannot be edited offers the add: %q", e.tt.Texts())
	}
	e.at("Shared", -3)
	if !e.tt.HasText("Move before “Shared”") {
		t.Errorf("the upper half is not a place before: %q", e.tt.Texts())
	}
	e.drop()
	e.did(`ReorderPlaylist p.zed "p.shared"`)
	if got := e.adds(); len(got) != 0 {
		t.Errorf("added: %v", got)
	}
}

func TestDroppingBetweenRowsOfAnotherFolderMovesItThereAndPlacesIt(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.p.prefs.SetFolderOpen("f.run", true)
	e.shows("Commute", "Tempo")
	e.pickUp("Commute")
	e.at("Tempo", -14)
	e.drop()
	e.did(`ReorderPlaylist p.commute "p.run"`)
	moves := e.moves()
	if want := []string{`MoveToFolder p.commute "f.run"`, `ReorderPlaylist p.commute "p.run"`}; !slices.Equal(moves, want) {
		t.Fatalf("moves %v, want %v", moves, want)
	}
	e.tt.Until("Commute above Tempo", func() bool { return e.tt.HasText("Commute") && e.y("Commute") < e.y("Tempo") })
}

func TestAFailedMoveToasts(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Commute", "Running")
	e.api.err = errOffline
	e.pickUp("Commute")
	e.over("Running")
	e.drop()
	e.shows("Couldn't move “Commute”")
}

// A tile's playlist, or one pinned, is not a sidebar row: dropping it on a
// folder moves nothing.
func TestAPlaylistDraggedFromElsewhereIsNotMovedByAFolder(t *testing.T) {
	inBubble(t, func(t *testing.T) {
		e := sbSetup(t, reorderTree(), nil)
		e.shows("Commute", "Running")
		own := services.Playlist{ID: "p.commute", Source: services.SourceLibrary, Name: "Commute", CanEdit: true}
		e.grab(&kit.DragPayload{Items: []kit.Item{kit.PlaylistItem(own)}, Label: "Commute"})
		e.over("Running")
		e.drop()
		e.quiet()
		if got := e.moves(); len(got) != 0 {
			t.Errorf("moves %v", got)
		}
	})
}

func TestMoveUpAndMoveDownInTheRowsMenu(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Road Trip", "Commute", "Zed")

	// The first row cannot go up, the last cannot go down.
	e.more("Road Trip", "More for Road Trip")
	e.shows("Move down")
	if e.has("Move up") {
		t.Error("the first playlist offers Move up")
	}
	e.tt.Key(0, ui.KeyEscape)
	e.settle()
	e.more("Zed", "More for Zed")
	e.shows("Move up")
	if e.has("Move down") {
		t.Error("the last playlist offers Move down")
	}
	e.tt.Key(0, ui.KeyEscape)
	e.settle()

	e.more("Commute", "More for Commute")
	e.shows("Move up", "Move down", "Move to folder…")
	e.choose("Move up")
	e.did(`ReorderPlaylist p.commute "p.own"`)
	e.tt.Until("Commute above Road Trip", func() bool { return e.y("Commute") < e.y("Road Trip") })
	if got := e.tt.Announcements(); !slices.Contains(got, "Moved “Commute” to position 1 of 4") {
		t.Errorf("announced %q", got)
	}

	// Down: before the playlist listed after the next one.
	e.more("Commute", "More for Commute")
	e.choose("Move down")
	e.did(`ReorderPlaylist p.commute "p.shared"`)
}

func TestAltArrowsMoveTheFocusedRow(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Road Trip", "Commute")
	focusRow := func(name string) {
		t.Helper()
		for range 80 {
			if tr := access.Read(t, e.tt.Tester); tr.Focus != nil && tr.Focus.Label == name {
				return
			}
			e.tt.Key(0, ui.KeyTab)
			e.tt.Frame()
		}
		t.Fatalf("Tab never reached %q", name)
	}
	focusRow("Commute")
	e.tt.Key(ui.Alt, ui.KeyUp)
	e.did(`ReorderPlaylist p.commute "p.own"`)
	e.tt.Until("Commute above Road Trip", func() bool { return e.y("Commute") < e.y("Road Trip") })
	if got := e.tt.Announcements(); !slices.Contains(got, "Moved “Commute” to position 1 of 4") {
		t.Errorf("announced %q", got)
	}
	if tr := access.Read(t, e.tt.Tester); tr.Focus == nil || tr.Focus.Label != "Commute" {
		t.Errorf("the focus left the moved row: %+v", tr.Focus)
	}
	e.tt.Key(ui.Alt, ui.KeyDown)
	e.did(`ReorderPlaylist p.commute "p.shared"`)
}

func TestAPlaylistInAnotherSectionHasNoMoveUpOrDown(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Quiet Hours")
	e.more("Quiet Hours", "More for Quiet Hours")
	e.shows("Pin playlist")
	if e.has("Move up") || e.has("Move down") || e.has("Move to folder…") {
		t.Errorf("an Apple playlist offers a move: %q", e.tt.Texts())
	}
}

// Moving a joined collaborative playlist is the user's own business: it sits
// among their folders, so the menu offers Move to folder.
func TestAJoinedCollaborativePlaylistCanBeMovedToAFolder(t *testing.T) {
	e := sbSetup(t, reorderTree(), nil)
	e.shows("Shared")
	e.more("Shared", "More for Shared")
	e.shows("Move to folder…")
	e.choose("Move to folder…")
	e.shows("Running")
	if err := e.tt.Click("Running"); err != nil {
		t.Fatal(err)
	}
	e.did(`MoveToFolder p.shared "f.run"`)
}
