package playlist

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
)

var abc = []string{"Song A", "Song B", "Song C"}

// editing opens the editor of a playlist of three songs and focuses the
// first row.
func editingRig(t *testing.T) *pageRig {
	t.Helper()
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Songs in Road Trip")
	r.click("Edit")
	r.shows("Done", "Edit the songs in Road Trip")
	r.click("Song A")
	r.tt.Frame()
	return r
}

func TestEditorRowsHaveHandleNumberArtistAlbumTimeAndCross(t *testing.T) {
	r := editingRig(t)
	r.shows("Drag Song A to reorder", "Remove Song A from Road Trip", "Artist A", "Album A", "3:20", "1", "2", "3")
	songs := threeSongs()
	songs[1].DurationMs = 0
	r = newPageRig(t, "p.1", libraryDetail("p.1", "Mix", songs...))
	r.shows("Songs in Mix")
	r.click("Edit")
	r.shows("—")
}

func TestEditorArrowsMoveFocusAndHomeEnd(t *testing.T) {
	r := editingRig(t)
	if !r.tt.Focused("Song A") {
		t.Fatal("clicking a row did not focus it")
	}
	r.tt.Key(0, ui.KeyDown)
	r.tt.Frame()
	if !r.tt.Focused("Song B") {
		t.Error("Down did not focus the next row")
	}
	r.tt.Key(0, ui.KeyEnd)
	r.tt.Frame()
	if !r.tt.Focused("Song C") {
		t.Error("End did not focus the last row")
	}
	r.tt.Key(0, ui.KeyUp)
	r.tt.Frame()
	if !r.tt.Focused("Song B") {
		t.Error("Up did not focus the previous row")
	}
	r.tt.Key(0, ui.KeyHome)
	r.tt.Frame()
	if !r.tt.Focused("Song A") {
		t.Error("Home did not focus the first row")
	}
	// Moving the focus changes nothing in the playlist.
	if got := r.calls("MoveTrack"); len(got) != 0 {
		t.Errorf("arrows moved a song: %v", got)
	}
}

func TestEditorAltArrowsMoveTheSongAndKeepTheFocus(t *testing.T) {
	r := editingRig(t)
	r.api.gate = make(chan struct{}) // keeps the edit in flight
	r.tt.Key(ui.Alt, ui.KeyDown)
	r.tt.Frame()
	// Shown at once, before Go answers.
	if got := r.order(abc...); !slices.Equal(got, []string{"Song B", "Song A", "Song C"}) {
		t.Errorf("optimistic order %v", got)
	}
	r.did("MoveTrack p.1 0 1 a")
	if got := r.tt.Announcements(); !slices.Contains(got, "Moved Song A to position 2 of 3") {
		t.Errorf("announcements %q", got)
	}
	if !r.tt.Focused("Song A") {
		t.Error("the focus left the moved song")
	}

	r.tt.Key(ui.Alt, ui.KeyUp)
	r.tt.Frame()
	if got := r.order(abc...); !slices.Equal(got, abc) {
		t.Errorf("order after moving back %v", got)
	}
	// The second edit waits for the first.
	for range 20 {
		r.tt.Drain()
	}
	if got := r.calls("MoveTrack"); len(got) != 1 {
		t.Errorf("both moves were sent at once: %v", got)
	}
	close(r.api.gate)
	r.did("MoveTrack p.1 1 0 a")
	if got := r.calls("MoveTrack"); !slices.Equal(got, []string{"MoveTrack p.1 0 1 a", "MoveTrack p.1 1 0 a"}) {
		t.Errorf("moves out of order: %v", got)
	}
	// The count drops before the last answer is posted to the main loop: wait
	// for the page to show it, then for the order to stay.
	r.tt.Until("the edits done", func() bool { return !r.p.edits.editing("p.1") })
	r.tt.Until("the final order", func() bool { return slices.Equal(r.order(abc...), abc) })
	r.tt.Drain()
	if got := r.order(abc...); !slices.Equal(got, abc) {
		t.Errorf("final order %v", got)
	}
}

func TestEditorAltArrowsDoNothingAtTheEnds(t *testing.T) {
	r := editingRig(t)
	r.tt.Key(ui.Alt, ui.KeyUp)
	r.tt.Frame()
	if got := r.calls("MoveTrack"); len(got) != 0 {
		t.Errorf("moved the first song up: %v", got)
	}
	r.tt.Key(0, ui.KeyEnd)
	r.tt.Frame()
	r.tt.Key(ui.Alt, ui.KeyDown)
	r.tt.Frame()
	if got := r.calls("MoveTrack"); len(got) != 0 {
		t.Errorf("moved the last song down: %v", got)
	}
}

func TestEditorMoveFailureRollsBackAndToasts(t *testing.T) {
	r := editingRig(t)
	r.api.err = errOffline
	r.tt.Key(ui.Alt, ui.KeyDown)
	r.shows("Couldn't move the song")
	r.tt.Until("the order is back", func() bool { return slices.Equal(r.order(abc...), abc) })
}

func TestEditorConflictToastsAndRefetches(t *testing.T) {
	r := editingRig(t)
	r.api.err = services.ErrConflict
	r.tt.Key(ui.Alt, ui.KeyDown)
	r.shows("This playlist changed on another device", "Here is its latest version. Make the change again.")
	r.tt.Until("the order is back", func() bool { return slices.Equal(r.order(abc...), abc) })
	// The refetch asked Go for the playlist again.
	if n := len(r.calls("Playlist p.1")); n < 2 {
		t.Errorf("the playlist was read %d times, want a refetch", n)
	}
}

func TestEditorDeleteRemovesTheFocusedSongAndFocusesTheNext(t *testing.T) {
	r := editingRig(t)
	r.click("Song B")
	r.api.gate = make(chan struct{})
	r.tt.Key(0, ui.KeyDelete)
	r.tt.Frame()
	if got := r.order(abc...); !slices.Equal(got, []string{"Song A", "Song C"}) {
		t.Errorf("order after Delete %v", got)
	}
	r.did("RemoveTracks p.1 [1] [b]")
	if got := r.tt.Announcements(); !slices.Contains(got, "Removed Song B") {
		t.Errorf("announcements %q", got)
	}
	r.tt.Frame()
	if !r.tt.Focused("Song C") {
		t.Error("the focus did not go to the next song")
	}
	close(r.api.gate)
	r.tt.Until("the edits done", func() bool { return !r.p.edits.editing("p.1") })
}

func TestEditorBackspaceRemovesAndTheLastRowFocusesThePrevious(t *testing.T) {
	r := editingRig(t)
	r.tt.Key(0, ui.KeyEnd)
	r.tt.Frame()
	r.tt.Key(0, ui.KeyBackspace)
	r.did("RemoveTracks p.1 [2] [c]")
	r.tt.Frame()
	if !r.tt.Focused("Song B") {
		t.Error("the focus did not go to the previous song")
	}
}

func TestEditorCrossRemovesThatRow(t *testing.T) {
	r := editingRig(t)
	r.click("Remove Song A from Road Trip")
	r.did("RemoveTracks p.1 [0] [a]")
	r.tt.Until("the row is gone", func() bool { return !r.tt.HasText("Remove Song A from Road Trip") })
}

func TestEditorRemoveFailureToastsAndRestores(t *testing.T) {
	r := editingRig(t)
	r.api.err = errOffline
	r.click("Remove Song B from Road Trip")
	r.shows("Couldn't remove “Song B”")
	r.tt.Until("the song is back", func() bool { return slices.Equal(r.order(abc...), abc) })
}

func TestEditorDragHandleReordersWithAnInsertionSlot(t *testing.T) {
	r := editingRig(t)
	handle, ok := r.tt.Find("Drag Song A to reorder")
	if !ok {
		t.Fatalf("no handle in %q", r.tt.Texts())
	}
	hx, hy := handle.X+handle.W/2, handle.Y+handle.H/2
	last, ok := r.tt.Find("Song C")
	if !ok {
		t.Fatal("no third row")
	}
	// Just above the bottom edge of the last row, in the list: the slot after it.
	ty := last.Y + last.H - 4
	r.tt.Press(hx, hy)
	r.tt.Move(hx, hy+4)
	r.tt.Move(hx, hy+10)
	r.tt.Move(hx+2, ty)
	r.tt.Move(hx+2, ty+1)
	r.tt.Release(hx+2, ty+1)
	r.did("MoveTrack p.1 0 2 a")
	r.tt.Until("the order", func() bool { return slices.Equal(r.order(abc...), []string{"Song B", "Song C", "Song A"}) })
}

func TestEditorDropAtTheSongsOwnPositionMovesNothing(t *testing.T) {
	r := editingRig(t)
	handle, _ := r.tt.Find("Drag Song B to reorder")
	hx, hy := handle.X+handle.W/2, handle.Y+handle.H/2
	r.tt.Press(hx, hy)
	r.tt.Move(hx, hy+4)
	r.tt.Move(hx, hy+10)
	r.tt.Move(hx, hy+12)
	r.tt.Release(hx, hy+12)
	r.tt.Frame()
	if got := r.calls("MoveTrack"); len(got) != 0 {
		t.Errorf("a drop on the song's own place moved it: %v", got)
	}
}

func TestEditorLongPlaylistsAreVirtualisedAndTheFocusScrollsIn(t *testing.T) {
	var songs []services.Song
	for i := range 300 {
		n := strconv.Itoa(i)
		songs = append(songs, pgSong("s"+n, "Song "+n, "Artist", "Album"))
	}
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", songs...))
	r.shows("Songs in Road Trip")
	r.click("Edit")
	r.shows("Done", "Remove Song 0 from Road Trip")
	built := 0
	for _, tx := range r.tt.Texts() {
		if strings.HasPrefix(tx, "Remove Song ") {
			built++
		}
	}
	if built == 0 || built >= 100 {
		t.Errorf("%d rows built of 300, want a window", built)
	}
	if r.tt.HasText("Remove Song 299 from Road Trip") {
		t.Error("the last row is built before it scrolled in")
	}
	r.click("Song 0")
	r.tt.Key(0, ui.KeyEnd)
	r.tt.Until("the last row focused", func() bool { return r.tt.Focused("Song 299") })
}
