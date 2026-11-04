package playlist

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

func TestCreateDialogBlankIsDisabledAndEnterSubmits(t *testing.T) {
	r := newDialogsRig(t)
	r.show(dialogState{kind: dialogCreate})
	r.shows("New playlist", "Add songs from any album, playlist or search result.", "Name", "Description",
		"Show on my profile", "People who follow you can find and play it.", "Create playlist", "Cancel")

	_ = r.tt.Click("Create playlist")
	r.typed("   ")
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Advance(100 * time.Millisecond)
	if r.asked("Create ") {
		t.Fatalf("a blank name was submitted: %v", r.api.asked())
	}

	r.replaced("Road Trip")
	r.tt.Key(0, ui.KeyEnter)
	r.did(`Create "Road Trip" desc="" public=false items=[] folder=""`)
	r.tt.Until("the new playlist opens", func() bool { return r.router.Path() == "/playlist/p.created" })
	if !r.closed() {
		t.Error("the dialog stayed open after the playlist was made")
	}
}

func TestCreateDialogSendsDescriptionVisibilityAndFolder(t *testing.T) {
	r := newDialogsRig(t)
	r.show(dialogState{kind: dialogCreate, folderID: "pf.1"})
	r.typed("Chill")
	if err := r.tt.Click("Description"); err != nil {
		t.Fatal(err)
	}
	r.typed("Slow ones")
	if err := r.tt.Click("Show on my profile"); err != nil {
		t.Fatal(err)
	}
	if !r.p.dialogs.public {
		t.Fatal("the switch did not turn on")
	}
	if err := r.tt.Click("Create playlist"); err != nil {
		t.Fatal(err)
	}
	r.did(`Create "Chill" desc="Slow ones" public=true items=[] folder="pf.1"`)
	r.tt.Until("the dialog closes", r.closed)
}

func TestCreateDialogWithItemsToastsAndDoesNotNavigate(t *testing.T) {
	r := newDialogsRig(t)
	r.show(dialogState{kind: dialogCreate, items: rigItems(), label: rigLabel})
	r.shows("With “Hundred”.")
	if r.tt.HasText("Add songs from any album") {
		t.Error("the plain description shows with items carried")
	}
	r.typed("Mix")
	r.tt.Key(0, ui.KeyEnter)
	r.did(`Create "Mix" desc="" public=false items=[song:s1] folder=""`)
	r.shows("Added “Hundred” to “Mix”", "Open")
	if !r.closed() {
		t.Error("the dialog stayed open")
	}
	if got := r.router.Path(); got != "/library/albums" {
		t.Fatalf("creating with items navigated to %q", got)
	}
	if err := r.tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/playlist/p.created" {
		t.Errorf("Open went to %q", got)
	}
}

func TestCreateDialogFailureKeepsTheFormOpen(t *testing.T) {
	r := newDialogsRig(t)
	r.api.err = errOffline
	r.show(dialogState{kind: dialogCreate})
	r.typed("Road Trip")
	r.tt.Key(0, ui.KeyEnter)
	r.shows("Couldn't create the playlist", i18n.ErrorText(services.CodeOf(errOffline)))
	if r.closed() {
		t.Fatal("a failed create closed the dialog")
	}
	if !r.tt.HasText("Create playlist") || r.p.dialogs.busy {
		t.Error("the form is not usable again")
	}
	if got := r.router.Path(); got != "/library/albums" {
		t.Errorf("a failed create navigated to %q", got)
	}

	r.api.err = nil
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the retry opens the playlist", func() bool { return r.router.Path() == "/playlist/p.created" })
	if got := r.count(`Create "Road Trip" desc="" public=false items=[] folder=""`); got != 2 {
		t.Errorf("Create was asked %d times, want 2", got)
	}
}

func TestCreateDialogIsBusyWhileTheRequestIsOut(t *testing.T) {
	r := newDialogsRig(t)
	release := r.holdCreate(t)
	r.show(dialogState{kind: dialogCreate})
	r.typed("Road Trip")
	r.tt.Key(0, ui.KeyEnter)
	r.shows("Creating…")
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Advance(100 * time.Millisecond)
	release()
	r.tt.Until("the dialog closes", r.closed)
	if got := r.count(`Create "Road Trip" desc="" public=false items=[] folder=""`); got != 1 {
		t.Errorf("Create was asked %d times while busy, want 1", got)
	}
}

func TestCreateDialogCancelAndLimitsAndReset(t *testing.T) {
	r := newDialogsRig(t)
	r.show(dialogState{kind: dialogCreate})
	r.typed(strings.Repeat("a", maxNameRunes+5))
	if got := len([]rune(r.p.dialogs.name)); got != maxNameRunes {
		t.Errorf("the name holds %d characters, want %d", got, maxNameRunes)
	}
	if err := r.tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if !r.closed() || r.asked("Create ") {
		t.Fatalf("Cancel left the dialog open or created: %v", r.api.asked())
	}
	r.tt.Advance(300 * time.Millisecond)
	r.show(dialogState{kind: dialogCreate})
	if d := r.p.dialogs; d.name != "" || d.description != "" || d.public || d.busy || d.query != "" {
		t.Errorf("a new dialog kept the last form: %+v", d)
	}
}

func TestDetailsDialogSavesAndRefreshesTheDetail(t *testing.T) {
	r := newDialogsRig(t)
	r.api.details["p.a"] = services.PlaylistDetail{Playlist: services.Playlist{ID: "p.a", Name: "Road Trip", CanEdit: true}}
	r.p.detail("p.a")
	r.tt.Until("the first read", func() bool { return r.count("Playlist p.a") == 1 })

	r.show(dialogState{kind: dialogDetails, id: "p.a", name: "Road Trip", description: "Songs", isPublic: true})
	r.shows("Edit playlist", "Changes appear in Apple Music on all your devices.", "Save")
	if d := r.p.dialogs; d.name != "Road Trip" || d.description != "Songs" || !d.public {
		t.Fatalf("the form is not prefilled: %q %q %v", d.name, d.description, d.public)
	}
	r.replaced("Night Drive")
	if err := r.tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	r.did(`Update p.a "Night Drive" desc="Songs" public=true`)
	r.tt.Until("the dialog closes", r.closed)
	r.tt.Until("the detail is read again", func() bool { return r.count("Playlist p.a") == 2 })
}

func TestDetailsDialogFailureToastsAndStaysOpen(t *testing.T) {
	r := newDialogsRig(t)
	r.api.err = errOffline
	r.show(dialogState{kind: dialogDetails, id: "p.a", name: "Road Trip"})
	if err := r.tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	r.shows("Couldn't save the playlist")
	if r.closed() {
		t.Error("a failed save closed the dialog")
	}
	if !r.tt.HasText("Save") || r.p.dialogs.busy {
		t.Error("the form is not usable again")
	}
}

func TestFolderDialogCreates(t *testing.T) {
	r := newDialogsRig(t)
	r.show(dialogState{kind: dialogFolder})
	r.shows("New folder", "Folders group playlists in the sidebar on all your devices.", "Create folder")
	_ = r.tt.Click("Create folder")
	if r.asked("CreateFolder") {
		t.Fatal("a blank name created a folder")
	}
	r.typed("Mixes")
	if err := r.tt.Click("Create folder"); err != nil {
		t.Fatal(err)
	}
	r.did(`CreateFolder "Mixes"`)
	r.tt.Until("the dialog closes", r.closed)
}

func TestFolderDialogCreateFailureStaysOpen(t *testing.T) {
	r := newDialogsRig(t)
	r.api.err = errOffline
	r.show(dialogState{kind: dialogFolder})
	r.typed("Mixes")
	r.tt.Key(0, ui.KeyEnter)
	r.shows("Couldn't create the folder")
	if r.closed() {
		t.Error("a failed create closed the dialog")
	}
}

func TestFolderDialogRenames(t *testing.T) {
	r := newDialogsRig(t)
	f := folder("pf.1", "Old name", "", 0)
	r.show(dialogState{kind: dialogFolder, folder: &f})
	r.shows("Rename folder", "Rename")
	if r.p.dialogs.name != "Old name" {
		t.Fatalf("the name is %q, want the folder's", r.p.dialogs.name)
	}
	r.replaced("New name")
	r.tt.Key(0, ui.KeyEnter)
	r.did(`RenameFolder pf.1 "New name"`)
	r.tt.Until("the dialog closes", r.closed)
}

func TestFolderDialogRenameFailureStaysOpen(t *testing.T) {
	r := newDialogsRig(t)
	r.api.err = errOffline
	f := folder("pf.1", "Old name", "", 0)
	r.show(dialogState{kind: dialogFolder, folder: &f})
	r.replaced("New name")
	if err := r.tt.Click("Rename"); err != nil {
		t.Fatal(err)
	}
	r.shows("Couldn't rename the folder")
	if r.closed() {
		t.Error("a failed rename closed the dialog")
	}
}

func TestMoveDialogMarksTheCurrentFolderAndChoosingItDoesNothing(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.show(dialogState{kind: dialogMove, id: "p.a", name: "Road Trip", folderID: "pf.2"})
	r.shows("Move to folder", "Road Trip", "Top level", "Mixes", "Mixes / Old")
	cur, ok := r.tt.Find("Current")
	if !ok {
		t.Fatal("no Current mark")
	}
	mid := func(s string) float32 {
		b, ok := r.tt.Find(s)
		if !ok {
			t.Fatalf("%q is not shown", s)
		}
		return b.Y + b.H/2
	}
	at := cur.Y + cur.H/2
	if d := at - mid("Mixes / Old"); d < -22 || d > 22 {
		t.Errorf("Current is %v off the current folder's row", d)
	}
	if d := at - mid("Top level"); d > -22 && d < 22 {
		t.Error("Current sits on the Top level row")
	}

	if err := r.tt.Click("Mixes / Old"); err != nil {
		t.Fatal(err)
	}
	r.tt.Advance(100 * time.Millisecond)
	if !r.closed() {
		t.Error("choosing the current folder left the dialog open")
	}
	if r.asked("MoveToFolder") {
		t.Errorf("choosing the current folder moved: %v", r.api.asked())
	}
}

func TestMoveDialogMovesAndToasts(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.show(dialogState{kind: dialogMove, id: "p.a", name: "Road Trip", folderID: "pf.2"})
	r.shows("Top level")
	if err := r.tt.Click("Top level"); err != nil {
		t.Fatal(err)
	}
	r.did(`MoveToFolder p.a ""`)
	r.shows("Moved “Road Trip” to the top level")
	if !r.closed() {
		t.Error("the dialog stayed open")
	}

	// From the top level, the keyboard: the first row has the focus, Down and
	// Enter choose the next.
	r.show(dialogState{kind: dialogMove, id: "p.a", name: "Road Trip", folderID: ""})
	r.shows("Mixes")
	if !r.tt.Focused("Top level") {
		t.Fatal("the first row does not have the focus")
	}
	r.tt.Key(0, ui.KeyDown)
	r.tt.Frame()
	if got := r.p.dialogs.nav.Focus(); got != 1 {
		t.Fatalf("Down moved the focus to row %d, want 1", got)
	}
	r.tt.Key(0, ui.KeyEnter)
	r.did(`MoveToFolder p.a "pf.1"`)
	r.shows("Moved “Road Trip” to “Mixes”")
}

func TestMoveDialogFailureToasts(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.api.err = errOffline
	r.show(dialogState{kind: dialogMove, id: "p.a", name: "Road Trip", folderID: "pf.2"})
	r.shows("Top level")
	if err := r.tt.Click("Top level"); err != nil {
		t.Fatal(err)
	}
	r.shows("Couldn't move “Road Trip”")
	if r.tt.HasText("Moved “Road Trip”") {
		t.Error("a failed move said it moved")
	}
}

func TestMoveDialogWithoutFoldersSaysSo(t *testing.T) {
	r := newDialogsRig(t)
	r.seedNoFolders()
	r.show(dialogState{kind: dialogMove, id: "p.a", name: "Road Trip"})
	r.shows("Top level", "You have no folders yet. Create one from the sidebar’s Playlists menu.")
}

func TestAddPickerListsRecentFirstThenFolderGroups(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.show(dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel})
	r.shows("Add to playlist", rigLabel, "New playlist", "Recent", "Playlists", "Mixes", "Road Trip", "Roadie", "Gym", "3 songs")
	if r.tt.HasText("Apple Curated") {
		t.Error("a playlist the user cannot edit is listed")
	}
	recent, top, mixes := r.index("Recent"), r.index("Playlists"), r.index("Mixes")
	if recent < 0 || top < 0 || mixes < 0 || recent >= top || top >= mixes {
		t.Errorf("sections out of order: Recent %d, Playlists %d, Mixes %d", recent, top, mixes)
	}
	if gym := r.index("Gym"); gym < recent || gym > top {
		t.Errorf("Gym (%d) is not in the Recent section (%d..%d)", gym, recent, top)
	}
}

func TestAddPickerFilters(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.show(dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel})
	r.shows("Road Trip", "Gym")

	r.typed("road")
	r.tt.Until("filtered to the matches", func() bool {
		return r.tt.HasText("Roadie") && r.tt.HasText("Road Trip") && !r.tt.HasText("Gym") && !r.tt.HasText("Recent")
	})
	if !r.tt.HasText("New playlist") {
		t.Error("the New playlist row went while filtering")
	}

	r.typed("zzz")
	r.shows("No playlist you can edit is called “roadzzz”.")
	if r.tt.HasText("Roadie") {
		t.Error("a playlist that does not match is listed")
	}
}

func TestAddPickerEmptyAndLoadingStates(t *testing.T) {
	r := newDialogsRig(t)
	release := r.holdTree(t)
	r.show(dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel})
	r.shows("Loading your playlists…")
	r.api.tree = services.PlaylistTree{Playlists: []services.TreePlaylist{treePlaylist("p.c", "Apple Curated", "", 0, false)}}
	release()
	r.shows("You have no playlists you can edit yet.")
	if r.tt.HasText("Loading your playlists…") {
		t.Error("still loading")
	}
}

func TestAddPickerKeyboard(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.show(dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel})
	r.shows("Road Trip", "Gym")
	nav := &r.p.dialogs.nav

	// The options are New playlist, Gym (recent), Road Trip, Roadie, Gym.
	r.tt.Key(0, ui.KeyDown)
	r.tt.Frame()
	if !r.tt.Focused("New playlist") || nav.Focus() != 0 {
		t.Fatalf("Down in the field did not focus the first option (option %d)", nav.Focus())
	}
	for i, k := range []struct {
		key  ui.Key
		want int
	}{
		{ui.KeyDown, 1}, {ui.KeyDown, 2}, {ui.KeyEnd, 4}, {ui.KeyDown, 4}, {ui.KeyUp, 3}, {ui.KeyHome, 0}, {ui.KeyUp, 0}, {ui.KeyDown, 1}, {ui.KeyDown, 2},
	} {
		r.tt.Key(0, k.key)
		r.tt.Frame()
		if got := nav.Focus(); got != k.want {
			t.Fatalf("step %d: the focus is on option %d, want %d", i, got, k.want)
		}
	}
	r.tt.Key(0, ui.KeyEnter)
	r.did("Add p.a [song:s1]")
	r.shows("Added “Hundred” to “Road Trip”")
	if !r.closed() {
		t.Error("choosing left the picker open")
	}
}

func TestAddPickerChoosingARecentAddsAndOpenGoesThere(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.show(dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel})
	r.shows("Recent", "Gym")
	if err := r.tt.Click("Gym"); err != nil { // the first one listed: the recent
		t.Fatal(err)
	}
	r.did("Add p.b [song:s1]")
	r.shows("Added “Hundred” to “Gym”", "Open")
	if !r.closed() {
		t.Error("choosing left the picker open")
	}
	if err := r.tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Path(); got != "/playlist/p.b" {
		t.Errorf("Open went to %q", got)
	}
}

func TestAddPickerFailureToasts(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.api.err = errOffline
	r.show(dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel})
	r.shows("Road Trip")
	if err := r.tt.Click("Roadie"); err != nil {
		t.Fatal(err)
	}
	r.shows("Couldn't add to “Roadie”")
}

func TestAddPickerNewPlaylistHoldsTheItems(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.show(dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel})
	r.shows("Road Trip")
	if err := r.tt.Click("New playlist"); err != nil {
		t.Fatal(err)
	}
	r.shows("With “Hundred”.", "Create playlist")
	if d := r.p.dialogs; d.s.kind != dialogCreate || len(d.s.items) != 1 || d.s.label != rigLabel || d.s.folderID != "" {
		t.Fatalf("the create dialog is %+v", d.s)
	}
	if r.tt.HasText("Find a playlist") {
		t.Error("the picker is still shown")
	}
	r.tt.Advance(300 * time.Millisecond)
	r.typed("Mix")
	r.tt.Key(0, ui.KeyEnter)
	r.did(`Create "Mix" desc="" public=false items=[song:s1] folder=""`)
	r.shows("Added “Hundred” to “Mix”")
}

func TestOneDialogAtATimeAndEscapeClosesAndReturnsTheFocus(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.tt.Key(0, ui.KeyTab)
	if !r.tt.Focused("Open picker") {
		t.Fatal("the opener is not focused")
	}
	r.tt.Key(0, ui.KeyEnter)
	r.shows("Find a playlist")
	if r.opened != 1 {
		t.Fatalf("the opener ran %d times", r.opened)
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300 * time.Millisecond)
	if !r.closed() {
		t.Fatal("Escape did not close the picker")
	}
	if !r.tt.Focused("Open picker") {
		t.Error("the focus did not return to the opener")
	}
	if r.tt.HasText("Find a playlist") {
		t.Error("the picker is still drawn")
	}
}

func TestShowingADialogReplacesTheOpenOne(t *testing.T) {
	r := newDialogsRig(t)
	r.seedTree()
	r.show(dialogState{kind: dialogAdd, items: rigItems(), label: rigLabel})
	r.shows("Find a playlist")
	r.show(dialogState{kind: dialogFolder})
	r.shows("New folder")
	if r.tt.HasText("Find a playlist") || r.tt.HasText("Add to playlist") {
		t.Error("two dialogs are drawn")
	}
	r.show(dialogState{kind: dialogMove, id: "p.a", name: "Road Trip"})
	r.shows("Move to folder")
	if r.tt.HasText("New folder") {
		t.Error("two dialogs are drawn")
	}
	if n := kit.TakeDialogsBuilt(); n == 0 {
		t.Error("no dialog was built")
	}
	kit.TakeDialogsBuilt()
	r.tt.Frame()
	if n := kit.TakeDialogsBuilt(); n != 1 {
		t.Errorf("%d dialogs built in a frame, want 1", n)
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300 * time.Millisecond)
	if !r.closed() || r.tt.HasText("Move to folder") {
		t.Error("Escape did not close the dialog")
	}
}

func TestDialogCloseButtonAndScrimClose(t *testing.T) {
	r := newDialogsRig(t)
	r.show(dialogState{kind: dialogFolder})
	if err := r.tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	r.tt.Advance(300 * time.Millisecond)
	if !r.closed() {
		t.Error("the X did not close the dialog")
	}
	r.show(dialogState{kind: dialogFolder})
	r.tt.ClickAt(5, 5)
	r.tt.Advance(300 * time.Millisecond)
	if !r.closed() {
		t.Error("a click on the scrim did not close the dialog")
	}
}

func TestClosedHostBuildsNothing(t *testing.T) {
	r := newDialogsRig(t)
	kit.TakeDialogsBuilt()
	r.tt.Frame()
	if n := kit.TakeDialogsBuilt(); n != 0 {
		t.Errorf("a closed host built %d dialogs", n)
	}
}

func TestDeletePlaylistConfirm(t *testing.T) {
	r := newDialogsRig(t)
	var answers []bool
	done := func(ok bool) { answers = append(answers, ok) }

	r.p.deletePlaylist("p.a", "Road Trip", done)
	r.tt.Frame()
	r.shows("Delete “Road Trip”?", "It's removed from your library on all your devices. The songs in it stay in your library.", "Delete playlist", "Cancel")
	if err := r.tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if r.asked("Delete ") || len(answers) != 1 || answers[0] {
		t.Fatalf("Cancel deleted or misreported: calls %v, answers %v", r.api.asked(), answers)
	}
	if r.tt.HasText("Delete “Road Trip”?") {
		t.Error("the question stayed after Cancel")
	}

	r.p.deletePlaylist("p.a", "Road Trip", done)
	r.tt.Frame()
	if err := r.tt.Click("Delete playlist"); err != nil {
		t.Fatal(err)
	}
	r.did("Delete p.a")
	r.shows("Deleted “Road Trip”")
	r.tt.Until("the answer", func() bool { return len(answers) == 2 })
	if !answers[1] {
		t.Errorf("answers %v, want the second true", answers)
	}
}

func TestDeletePlaylistFailureToasts(t *testing.T) {
	r := newDialogsRig(t)
	r.api.err = errOffline
	var answers []bool
	r.p.deletePlaylist("p.a", "Road Trip", func(ok bool) { answers = append(answers, ok) })
	r.tt.Frame()
	if err := r.tt.Click("Delete playlist"); err != nil {
		t.Fatal(err)
	}
	r.shows("Couldn't delete “Road Trip”")
	r.tt.Until("the answer", func() bool { return len(answers) == 1 })
	if answers[0] || r.tt.HasText("Deleted “Road Trip”") {
		t.Errorf("a failed delete reported success: %v", answers)
	}
}

func TestDeleteFolderConfirm(t *testing.T) {
	r := newDialogsRig(t)
	r.p.deleteFolder("pf.1", "Mixes")
	r.tt.Frame()
	r.shows("Delete the folder “Mixes”?", "The empty folder is removed on all your devices.", "Delete folder", "Cancel")
	if err := r.tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if r.asked("DeleteFolder") {
		t.Fatalf("Cancel deleted: %v", r.api.asked())
	}

	r.p.deleteFolder("pf.1", "Mixes")
	r.tt.Frame()
	if err := r.tt.Click("Delete folder"); err != nil {
		t.Fatal(err)
	}
	r.did("DeleteFolder pf.1")
	r.shows("Deleted the folder “Mixes”")
}

func TestDeleteFolderFailureToasts(t *testing.T) {
	r := newDialogsRig(t)
	r.api.err = errOffline
	r.p.deleteFolder("pf.1", "Mixes")
	r.tt.Frame()
	if err := r.tt.Click("Delete folder"); err != nil {
		t.Fatal(err)
	}
	r.shows("Couldn't delete the folder “Mixes”")
}

func TestLimitRunesAndSameTree(t *testing.T) {
	if got := limitRunes("héllo", 3); got != "hél" {
		t.Errorf("limitRunes cut to %q", got)
	}
	if got := limitRunes("hé", 3); got != "hé" {
		t.Errorf("limitRunes changed a short string: %q", got)
	}
	a := services.PlaylistTree{Playlists: []services.TreePlaylist{treePlaylist("p.a", "A", "", 0, true)}}
	b := services.PlaylistTree{Playlists: []services.TreePlaylist{treePlaylist("p.a", "A", "", 0, true)}}
	if !sameTree(a, a) || sameTree(a, b) || !sameTree(services.PlaylistTree{}, services.PlaylistTree{}) {
		t.Error("sameTree does not tell the same data from a copy")
	}
}
