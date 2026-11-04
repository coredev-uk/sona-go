package playlist

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// The sidebar's sections (PlaylistSidebar.test.tsx).

func TestSidebarListsTheSectionsInOrder(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Made for you", "Apple Music", "Playlists", "Road Trip")
	if e.tt.HasText("Pinned") {
		t.Error("Pinned shows with no pins")
	}
	e.above("Made for you", "Morning Mix")
	e.above("Morning Mix", "Discovery Mix")
	e.above("Discovery Mix", "Apple Music")
	e.above("Apple Music", "Quiet Hours")
	e.above("Quiet Hours", "Playlists")
	e.above("Playlists", "Road Trip")
	// The user's own playlists are the only ones under Playlists: a mix is
	// listed once.
	n := 0
	for _, s := range e.tt.Texts() {
		if s == "Morning Mix" {
			n++
		}
	}
	if n > 2 { // the row's name and the link it labels
		t.Errorf("Morning Mix is listed %d times", n)
	}
}

func TestSidebarOmitsEmptySections(t *testing.T) {
	tree := sbLibrary()
	tree.Playlists = []services.TreePlaylist{treePlaylist("p.own", "Road Trip", "", 0, true)}
	e := sbSetup(t, tree, nil)
	e.shows("Road Trip")
	for _, h := range []string{"Made for you", "Apple Music", "Pinned"} {
		if e.tt.HasText(h) {
			t.Errorf("%q shows with nothing in it", h)
		}
	}
}

func TestSidebarWaitsForThePinsBeforeListing(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPlaylistPins("p.mix1", "p.mix2", "p.ed"))
	// The first frame has no answer yet: no list, no empty message.
	e.tt.Frame()
	for _, s := range []string{"Morning Mix", "Made for you", "Your playlists appear here.", "Road Trip"} {
		if e.tt.HasText(s) {
			t.Fatalf("%q shows before the tree and pins arrived", s)
		}
	}
	e.shows("Road Trip", "Pinned")
}

func TestSidebarDoesNotFlashAPinnedPlaylistInItsSection(t *testing.T) {
	gate := make(chan struct{})
	e := sbSetupAPI(t, sbLibrary(), []services.Pin{{Kind: "playlist", ID: "p.mix1", Name: "Morning Mix", Action: "drillIn"}},
		func(api *fakeAPI) playlistsAPI { return &sbSlowPins{fakeAPI: api, gate: gate} })
	e.did("Tree")
	e.quiet()
	if e.tt.HasText("Morning Mix") || e.tt.HasText("Road Trip") {
		t.Fatalf("playlists listed while the pins are unknown: %q", e.tt.Texts())
	}
	close(gate)
	e.shows("Pinned", "Road Trip")
	e.above("Pinned", "Morning Mix")
	e.above("Morning Mix", "Made for you")
}

// sbSlowPins holds the pins back until gate is closed.
type sbSlowPins struct {
	*fakeAPI
	gate chan struct{}
}

func (s *sbSlowPins) Pins(ctx context.Context) ([]services.Pin, error) {
	<-s.gate
	return s.fakeAPI.Pins(ctx)
}

func TestSidebarShowsAPinnedPlaylistUnderPinnedOnly(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPlaylistPins("p.mix1", "p.mix2", "p.ed"))
	e.shows("Pinned", "Playlists", "Road Trip")
	// Every mix and the Apple Music playlist are pinned: their sections go.
	for _, h := range []string{"Made for you", "Apple Music"} {
		if e.tt.HasText(h) {
			t.Errorf("%q shows with all its playlists pinned", h)
		}
	}
	e.above("Pinned", "Playlists")
}

func TestSidebarKeepsTheUnpinnedOnesOfASection(t *testing.T) {
	e := sbSetup(t, sbLibrary(), []services.Pin{{Kind: "playlist", ID: "p.mix1", Name: "Morning Mix", Action: "drillIn"}})
	e.shows("Pinned", "Made for you", "Apple Music", "Playlists", "Discovery Mix")
	e.above("Pinned", "Morning Mix")
	e.above("Morning Mix", "Made for you")
	e.above("Made for you", "Discovery Mix")
}

func TestSidebarOwnSectionMessages(t *testing.T) {
	// Nothing at all.
	e := sbSetup(t, services.PlaylistTree{}, nil)
	e.shows("Your playlists appear here.")

	// Every playlist the user can edit is pinned.
	tree := services.PlaylistTree{Playlists: []services.TreePlaylist{treePlaylist("p.own", "Road Trip", "", 0, true)}}
	e = sbSetup(t, tree, []services.Pin{{Kind: "playlist", ID: "p.own", Name: "Road Trip", Action: "drillIn"}})
	e.shows("All your playlists are pinned above.")
	if e.tt.HasText("Your playlists appear here.") {
		t.Error("both messages show")
	}
}

func TestSidebarNewPlaylistOrFolderIsOnTheOwnSectionOnly(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Made for you", "Playlists", "Road Trip")
	// One button, level with the Playlists heading.
	r, ok := e.tt.Find("New playlist or folder")
	if !ok {
		t.Fatal("no New playlist or folder button")
	}
	if h := e.y("Playlists"); r.Y < h-10 || r.Y > h+10 {
		t.Errorf("the button is at y=%v, the Playlists heading at %v", r.Y, h)
	}
	if e.y("Made for you") > e.y("Playlists") {
		t.Fatal("sections out of order")
	}

	e.choosePlus("New playlist")
	if !e.p.dialogs.open || e.p.dialogs.s.kind != dialogCreate || e.p.dialogs.s.folderID != "" || len(e.p.dialogs.s.items) != 0 {
		t.Errorf("New playlist opened %+v", e.p.dialogs)
	}
	e.closeDialog()
	e.choosePlus("New folder…")
	if !e.p.dialogs.open || e.p.dialogs.s.kind != dialogFolder || e.p.dialogs.s.folder != nil {
		t.Errorf("New folder… opened %+v", e.p.dialogs)
	}
}

func (e *sbEnv) choosePlus(item string) {
	e.t.Helper()
	if err := e.tt.Click("New playlist or folder"); err != nil {
		e.t.Fatal(err)
	}
	if !e.tt.HasText("New folder…") {
		e.t.Fatalf("the menu did not open: %q", e.tt.Texts())
	}
	e.choose(item)
}

func TestSidebarRowsLinkToThePlaylistAndMarkThePage(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip", "Quiet Hours")
	if err := e.tt.Click("Quiet Hours"); err != nil {
		t.Fatal(err)
	}
	if got := e.router.Path(); got != "/playlist/p.ed" {
		t.Errorf("the row went to %q", got)
	}
}

// Folders.

func TestSidebarFolderOpensAndTheStateIsKept(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Running")
	if e.tt.HasText("Tempo") {
		t.Fatal("a folder starts closed")
	}
	if err := e.tt.Click("Running"); err != nil {
		t.Fatal(err)
	}
	e.shows("Tempo")
	if !e.p.prefs.FolderOpen("f.run") {
		t.Error("the open folder is not remembered")
	}
	// A new load of the file has it open.
	if !newPrefs(e.p.prefs.snapshot(), nil).FolderOpen("f.run") {
		t.Error("the open folder did not reach the file")
	}
	e.above("Running", "Tempo")
	e.above("Tempo", "Road Trip")

	if err := e.tt.Click("Running"); err != nil {
		t.Fatal(err)
	}
	e.tt.Frame()
	if e.tt.HasText("Tempo") {
		t.Error("the folder did not close")
	}
	if newPrefs(e.p.prefs.snapshot(), nil).FolderOpen("f.run") {
		t.Error("the closed folder is still remembered open")
	}
}

func TestSidebarStartsOnTheFoldersTheFileHasOpen(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.p.prefs.SetFolderOpen("f.run", true)
	e.shows("Tempo")
}

func TestSidebarEmptyFolderMessages(t *testing.T) {
	tree := services.PlaylistTree{
		Folders: []services.PlaylistFolder{folder("f.empty", "Nothing", "", 0), folder("f.one", "Mixes", "", 1), folder("f.two", "More mixes", "", 2)},
	}
	mix := func(id, folderID string) services.TreePlaylist {
		tp := treePlaylist(id, id, folderID, 0, false)
		tp.Playlist.CatalogID = "pl.pm-" + id
		return tp
	}
	tree.Playlists = []services.TreePlaylist{mix("a", "f.one"), mix("b", "f.two"), mix("c", "f.two")}
	e := sbSetup(t, tree, nil)
	e.shows("Nothing", "Mixes", "More mixes")
	for _, f := range []string{"Nothing", "Mixes", "More mixes"} {
		if err := e.tt.Click(f); err != nil {
			t.Fatal(err)
		}
	}
	e.shows("Empty folder", "Its playlist is listed in another section", "Its 2 playlists are listed in other sections")
}

func TestSidebarFolderMenuAndItsDisabledDelete(t *testing.T) {
	tree := sbLibrary()
	tree.Folders = append(tree.Folders, folder("f.empty", "Nothing", "", 1))
	e := sbSetup(t, tree, nil)
	e.shows("Running", "Nothing")

	// A folder that holds a playlist cannot be deleted.
	e.more("Running", "More for the folder Running")
	for _, s := range []string{"New playlist here", "Rename…", "Delete (move its playlists out first)"} {
		if !e.tt.HasText(s) {
			t.Fatalf("the folder menu lacks %q: %q", s, e.tt.Texts())
		}
	}
	if e.tt.HasText("Delete…") {
		t.Error("a folder with playlists offers Delete…")
	}
	_ = e.tt.Click("Delete (move its playlists out first)")
	e.settle()
	if e.tt.HasText("Delete the folder") {
		t.Error("the disabled Delete asked for confirmation")
	}
	e.tt.Key(0, ui.KeyEscape)
	e.settle()

	// New playlist here creates in the folder.
	e.more("Running", "More for the folder Running")
	e.choose("New playlist here")
	if s := e.p.dialogs.s; !e.p.dialogs.open || s.kind != dialogCreate || s.folderID != "f.run" {
		t.Errorf("New playlist here opened %+v", e.p.dialogs)
	}
	e.closeDialog()

	// Rename… opens the folder dialog on the folder.
	e.more("Running", "More for the folder Running")
	e.choose("Rename…")
	if s := e.p.dialogs.s; !e.p.dialogs.open || s.kind != dialogFolder || s.folder == nil || s.folder.ID != "f.run" {
		t.Errorf("Rename… opened %+v", e.p.dialogs)
	}
	e.closeDialog()

	// An empty folder can go, after the question.
	e.more("Nothing", "More for the folder Nothing")
	e.choose("Delete…")
	e.shows("Delete the folder “Nothing”?")
	if err := e.tt.Click("Delete folder"); err != nil {
		t.Fatal(err)
	}
	e.did("DeleteFolder f.empty")
}

// Section folding.

func TestSidebarFoldsAReadOnlySectionAndRemembersIt(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Apple Music", "Quiet Hours", "Morning Mix")
	if err := e.tt.Click("Apple Music"); err != nil {
		t.Fatal(err)
	}
	e.tt.Frame()
	if e.tt.HasText("Quiet Hours") {
		t.Error("the folded list is still built")
	}
	if !e.tt.HasText("Morning Mix") {
		t.Error("folding Apple Music folded Made for you")
	}
	if !e.p.prefs.Collapsed("apple-music") || e.p.prefs.Collapsed("made-for-you") {
		t.Error("the fold state is not kept")
	}

	// A new run: the same file, a new feature state.
	again := newPlaylists(e.h.Env, e.api, newPrefs(e.p.prefs.snapshot(), nil))
	e2 := &sbEnv{t: t, p: again, api: e.api, play: e.play, h: e.h, router: e.router}
	e2.tt = e.h.Tester(t, e2.view, 520, 900)
	e2.shows("Apple Music", "Morning Mix")
	if e2.tt.HasText("Quiet Hours") {
		t.Error("the section did not start folded")
	}

	// Unfolding shows the list again.
	if err := e2.tt.Click("Apple Music"); err != nil {
		t.Fatal(err)
	}
	e2.shows("Quiet Hours")
	if again.prefs.Collapsed("apple-music") {
		t.Error("the unfolded section is still remembered folded")
	}
}

// Pins.

func sbPins() []services.Pin {
	return []services.Pin{
		{Kind: "playlist", ID: "p.a", Name: "Alpha", Action: "drillIn"},
		{Kind: "album", ID: "l.b", Name: "Beta", Action: "play"},
		{Kind: "song", ID: "i.c", Name: "Gamma", Action: "shuffle", AlbumID: "l.cc"},
		{Kind: "artist", ID: "r.d", Name: "Delta", Action: "play"},
	}
}

func TestPinsOpenOrPlayPerTheirAction(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPins())
	e.api.pinTracks["l.b"] = []services.Song{song("s1", "One", "A"), song("s2", "Two", "A")}
	e.api.pinTracks["i.c"] = []services.Song{song("s3", "Three", "B")}
	e.shows("Pinned", "Alpha", "Beta, plays", "Gamma, shuffles")

	// Open: a playlist pin and, whatever its action, an artist pin.
	if err := e.tt.Click("Alpha"); err != nil {
		t.Fatal(err)
	}
	if got := e.router.Path(); got != "/playlist/p.a" {
		t.Errorf("Alpha went to %q", got)
	}
	if err := e.tt.Click("Delta"); err != nil {
		t.Fatal(err)
	}
	if got := e.router.Path(); got != "/artist/r.d" {
		t.Errorf("Delta went to %q", got)
	}

	// Play: an album from its first song.
	if err := e.tt.Click("Beta, plays"); err != nil {
		t.Fatal(err)
	}
	e.tt.Until("Beta to play", func() bool { return len(e.play.played()) == 1 })
	if got := e.play.played()[0]; got != "[s1 s2] start=0 from=album:l.b" {
		t.Errorf("Beta played %q", got)
	}

	// Shuffle: shuffle on.
	if err := e.tt.Click("Gamma, shuffles"); err != nil {
		t.Fatal(err)
	}
	e.tt.Until("Gamma to play", func() bool { return len(e.play.played()) == 2 })
	e.play.mu.Lock()
	shuffled := e.play.shuffle
	e.play.mu.Unlock()
	if !shuffled {
		t.Error("a shuffle pin did not turn shuffle on")
	}
}

func TestAPinWithNoRouteForItsPageIsAButton(t *testing.T) {
	e := sbSetup(t, sbLibrary(), []services.Pin{{Kind: "album", ID: "l.b", Name: "Beta", Action: "drillIn"}})
	e.h.Env.HasRoute = func(path string) bool { return !strings.HasPrefix(path, "/album/") }
	e.api.pinTracks["l.b"] = []services.Song{song("s1", "One", "A")}
	e.shows("Beta, plays")
	if err := e.tt.Click("Beta, plays"); err != nil {
		t.Fatal(err)
	}
	e.did("PinTracks l.b")
	if got := e.router.Path(); got != "/library/albums" {
		t.Errorf("a pin whose page is not ported navigated to %q", got)
	}
}

func TestPinMenuChoosesWhatAPinDoes(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPins())
	e.shows("Beta, plays")
	e.more("Beta, plays", "More for Beta")
	for _, s := range []string{"When chosen", "Play", "Shuffle", "Open", "Move up", "Unpin"} {
		if !e.tt.HasText(s) {
			t.Fatalf("the pin menu lacks %q: %q", s, e.tt.Texts())
		}
	}
	e.choose("Shuffle")
	e.did("SetPinAction l.b shuffle")

	// The current choice does nothing.
	e.more("Beta, plays", "More for Beta")
	e.choose("Play")
	e.quiet()
	if e.api.did("SetPinAction l.b play") {
		t.Error("choosing the current action called SetPinAction")
	}
}

func TestPinMenuGoToAlbumForASong(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPins())
	e.shows("Gamma, shuffles")
	e.more("Gamma, shuffles", "More for Gamma")
	if !e.tt.HasText("Go to album") {
		t.Fatalf("a song pin offers Go to album: %q", e.tt.Texts())
	}
	e.choose("Go to album")
	if got := e.router.Path(); got != "/album/l.cc" {
		t.Errorf("Go to album went to %q", got)
	}
}

func TestPinMenuMoveUpAndUnpin(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPins())
	e.shows("Alpha")
	// The first pin has no Move up.
	e.more("Alpha", "More for Alpha")
	if e.tt.HasText("Move up") {
		t.Error("the first pin offers Move up")
	}
	e.tt.Key(0, ui.KeyEscape)
	e.settle()

	// The second goes to the top, the third after the pin two above it.
	e.more("Beta, plays", "More for Beta")
	e.choose("Move up")
	e.did(`MovePin l.b after ""`)
	e.more("Gamma, shuffles", "More for Gamma")
	e.choose("Move up")
	e.did(`MovePin i.c after "p.a"`)

	e.more("Beta, plays", "More for Beta")
	e.choose("Unpin")
	e.did("Unpin l.b")
}

func TestPinMenuFailuresAreToasted(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPins())
	e.shows("Beta, plays")
	e.api.err = errOffline
	e.more("Beta, plays", "More for Beta")
	e.choose("Shuffle")
	e.shows("Couldn't change the pin")
	e.more("Beta, plays", "More for Beta")
	e.choose("Move up")
	e.shows("Couldn't move the pin")
	e.more("Beta, plays", "More for Beta")
	e.choose("Unpin")
	e.shows("Couldn't unpin “Beta”")
}

func TestPinsWithoutAnAnswerFromPlayAreToasted(t *testing.T) {
	e := sbSetup(t, sbLibrary(), sbPins())
	e.shows("Beta, plays")
	if err := e.tt.Click("Beta, plays"); err != nil { // PinTracks answers nothing
		t.Fatal(err)
	}
	e.shows("“Beta” has no songs to play")
}

// Drops.

func TestDroppingASongOnAnEditablePlaylistAddsIt(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip")
	e.grab(sbSongPayload(song("s1", "Airbag", "Radiohead")))
	e.over("Road Trip")
	if !e.tt.HasText("Add to “Road Trip”") {
		t.Errorf("the ghost does not say what the drop does: %q", e.tt.Texts())
	}
	e.drop()
	e.did("Add p.own [song:s1]")
	e.shows("Added “Airbag” to “Road Trip”", "Open")
	if err := e.tt.Click("Open"); err != nil {
		t.Fatal(err)
	}
	if got := e.router.Path(); got != "/playlist/p.own" {
		t.Errorf("the toast's Open went to %q", got)
	}
}

func TestDroppingAQueueRowOnAPlaylistAddsItsSong(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip")
	e.grab(&kit.DragPayload{
		Items:  []kit.Item{kit.SongItem(song("s2", "Paranoid Android", "Radiohead"))},
		Origin: &kit.DragOrigin{List: "queue", Index: 2},
		Label:  "Paranoid Android",
	})
	e.over("Road Trip")
	e.drop()
	e.did("Add p.own [song:s2]")
	e.shows("Added “Paranoid Android” to “Road Trip”")
}

// A queue row of a music video (the queue's payload carries it as a video
// item) is nothing a playlist takes.
func TestDroppingAQueueVideoRowOnAPlaylistAddsNothing(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip")
	e.grab(&kit.DragPayload{
		Items:  []kit.Item{kit.MusicVideoItem(services.MusicVideo{ID: "v1", Source: services.SourceCatalog, Name: "Clip"})},
		Origin: &kit.DragOrigin{List: "queue", Index: 1},
		Label:  "Clip",
	})
	e.over("Road Trip")
	e.drop()
	e.quiet()
	if got := e.adds(); len(got) != 0 {
		t.Errorf("a video row was added: %v", got)
	}
}

func TestDroppingAnAlbumAndSeveralItems(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip")
	album := services.Album{ID: "l.9", Source: services.SourceLibrary, Name: "OK Computer"}
	e.grab(&kit.DragPayload{Items: []kit.Item{kit.AlbumItem(album), kit.SongItem(song("s1", "Airbag", "R"))}, Label: "2 items"})
	e.over("Road Trip")
	e.drop()
	e.did("Add p.own [album:l.9 song:s1]")
	e.shows("Added 2 items to “Road Trip”")
}

func TestAPlaylistThatCannotBeEditedTakesNoDrop(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Quiet Hours")
	e.grab(sbSongPayload(song("s1", "Airbag", "Radiohead")))
	e.over("Quiet Hours")
	if e.tt.HasText("Add to “Quiet Hours”") {
		t.Error("a playlist the user cannot edit offers a drop")
	}
	e.drop()
	e.quiet()
	if got := e.adds(); len(got) != 0 {
		t.Errorf("a drop on a read-only playlist added: %v", got)
	}
}

func TestAPlaylistDoesNotTakeItself(t *testing.T) {
	tree := sbLibrary()
	tree.Playlists = append(tree.Playlists, treePlaylist("p.two", "Commute", "", 5, true))
	e := sbSetup(t, tree, nil)
	e.shows("Road Trip", "Commute")
	self := services.Playlist{ID: "p.own", Source: services.SourceLibrary, Name: "Road Trip", CanEdit: true}
	e.grab(&kit.DragPayload{Items: []kit.Item{kit.PlaylistItem(self)}, Label: "Road Trip"})
	e.over("Road Trip")
	if e.tt.HasText("Add to “Road Trip”") {
		t.Error("a playlist offers to take itself")
	}
	e.drop()
	e.quiet()
	if got := e.adds(); len(got) != 0 {
		t.Fatalf("a playlist was added to itself: %v", got)
	}

	// Another editable playlist takes it.
	e.grab(&kit.DragPayload{Items: []kit.Item{kit.PlaylistItem(self)}, Label: "Road Trip"})
	e.over("Commute")
	e.drop()
	e.did("Add p.two [playlist:p.own]")
}

func TestAPayloadWithNothingAddableIsDeclined(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip")
	station := services.Station{ID: "ra.1", Name: "Station"}
	e.grab(&kit.DragPayload{Items: []kit.Item{kit.StationItem(station)}, Label: "Station"})
	e.over("Road Trip")
	if e.tt.HasText("Add to “Road Trip”") || e.has("New playlist") {
		t.Errorf("a station is offered a playlist: %q", e.tt.Texts())
	}
	e.drop()
	e.quiet()
	if got := e.adds(); len(got) != 0 {
		t.Errorf("a station was added: %v", got)
	}
}

func TestAFailedDropToastsTheError(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip")
	e.api.err = errOffline
	e.grab(sbSongPayload(song("s1", "Airbag", "Radiohead")))
	e.over("Road Trip")
	e.drop()
	e.shows("Couldn't add to “Road Trip”")
}

func TestAPinnedEditablePlaylistTakesDrops(t *testing.T) {
	e := sbSetup(t, sbLibrary(), []services.Pin{{Kind: "playlist", ID: "p.own", Name: "Road Trip", Action: "drillIn"}})
	e.shows("Pinned", "Road Trip")
	e.grab(sbSongPayload(song("s1", "Airbag", "Radiohead")))
	e.over("Road Trip")
	if !e.tt.HasText("Add to “Road Trip”") {
		t.Errorf("a pinned playlist does not offer the drop: %q", e.tt.Texts())
	}
	e.drop()
	e.did("Add p.own [song:s1]")
}

func TestTheNewPlaylistRowShowsWhileAddableItemsAreDragged(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip")
	if e.has("New playlist") {
		t.Fatal("the New playlist drop row shows at rest")
	}
	e.grab(sbSongPayload(song("s1", "Airbag", "Radiohead")))
	if !e.has("New playlist") {
		t.Fatalf("no New playlist drop row during the drag: %q", e.tt.Texts())
	}
	e.over("New playlist")
	e.drop()
	s := e.p.dialogs.s
	if !e.p.dialogs.open || s.kind != dialogCreate || s.folderID != "" || len(s.items) != 1 || s.items[0].ID != "s1" || s.label != "“Airbag”" {
		t.Errorf("the drop opened %+v", e.p.dialogs)
	}
	e.tt.Frame()
	if e.tt.HasText("Add to “Road Trip”") {
		t.Error("the ghost stayed after the drop")
	}
}

func TestThePinnedZoneTakesLibraryItemsInOrder(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Road Trip")
	if e.has("Pinned") {
		t.Fatal("Pinned shows at rest with no pins")
	}
	e.grab(&kit.DragPayload{
		Items: []kit.Item{kit.SongItem(song("s1", "Airbag", "R")), kit.SongItem(song("s2", "Lucky", "R"))},
		Label: "2 songs",
	})
	if !e.has("Pinned") || !e.has("Pin here") {
		t.Fatalf("no Pin here zone during the drag: %q", e.tt.Texts())
	}
	e.over("Pin here")
	if !e.tt.HasText("Pin to sidebar") {
		t.Errorf("the ghost does not say what the drop does: %q", e.tt.Texts())
	}
	e.drop()
	e.did("Pin s2")
	calls := e.api.asked()
	i1, i2 := slices.Index(calls, "Pin s1"), slices.Index(calls, "Pin s2")
	if i1 < 0 || i1 > i2 {
		t.Errorf("pins out of order: %q", calls)
	}
	e.shows("Pinned “Airbag”")
}

func TestThePinnedZoneDeclinesWhatCannotBePinned(t *testing.T) {
	catalogue := services.Song{ID: "1440", Source: services.SourceCatalog, Name: "Catalogue song"}
	video := services.MusicVideo{ID: "i.v", Source: services.SourceLibrary, Name: "Video"}
	for name, payload := range map[string]*kit.DragPayload{
		"a catalogue song": {Items: []kit.Item{kit.SongItem(catalogue)}},
		"a music video":    {Items: []kit.Item{kit.MusicVideoItem(video)}},
		"a library item and a catalogue one": {Items: []kit.Item{
			kit.SongItem(song("s1", "Airbag", "R")), kit.SongItem(catalogue),
		}},
	} {
		t.Run(name, func(t *testing.T) {
			e := sbSetup(t, sbLibrary(), nil)
			e.shows("Road Trip")
			e.grab(payload)
			if e.has("Pin here") {
				t.Errorf("Pin here shows for %s", name)
			}
		})
	}

	t.Run("an item already pinned", func(t *testing.T) {
		e := sbSetup(t, sbLibrary(), []services.Pin{{Kind: "song", ID: "s1", Name: "Airbag", Action: "play"}})
		e.shows("Pinned")
		e.grab(sbSongPayload(song("s1", "Airbag", "R")))
		if e.has("Pin here") {
			t.Error("Pin here shows for a song that is pinned")
		}
	})

	t.Run("more than six pins", func(t *testing.T) {
		var pins []services.Pin
		for _, id := range []string{"a", "b", "c", "d", "e", "f"} {
			pins = append(pins, services.Pin{Kind: "song", ID: "i." + id, Name: "Pin " + id, Action: "play"})
		}
		e := sbSetup(t, sbLibrary(), pins)
		e.shows("Pinned")
		e.grab(sbSongPayload(song("s1", "Airbag", "R")))
		if e.has("Pin here") {
			t.Error("Pin here shows with six pins")
		}
	})
}

func TestThePinnedZoneIsDashedBelowThePinsWhenThereAreSome(t *testing.T) {
	e := sbSetup(t, sbLibrary(), []services.Pin{{Kind: "playlist", ID: "p.a", Name: "Alpha", Action: "drillIn"}})
	e.shows("Alpha")
	e.grab(sbSongPayload(song("s1", "Airbag", "R")))
	e.above("Alpha", "Pin here")
	e.above("Pin here", "Playlists")
}

func TestADragRestingOnAClosedFolderOpensIt(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Running")
	e.grab(sbSongPayload(song("s1", "Airbag", "R")))
	e.over("Running")
	e.tt.Advance(500 * time.Millisecond)
	if e.tt.HasText("Tempo") || e.p.prefs.FolderOpen("f.run") {
		t.Fatal("the folder opened before 600 ms")
	}
	e.tt.Advance(200 * time.Millisecond)
	e.tt.Frame()
	if !e.tt.HasText("Tempo") || !e.p.prefs.FolderOpen("f.run") {
		t.Fatalf("the folder did not open under the drag: %q", e.tt.Texts())
	}
	// The playlist inside takes the drop.
	e.over("Tempo")
	e.drop()
	e.did("Add p.run [song:s1]")
}

func TestLeavingAFolderCancelsItsOpening(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Running")
	e.grab(sbSongPayload(song("s1", "Airbag", "R")))
	e.over("Running")
	e.tt.Advance(400 * time.Millisecond)
	e.over("Road Trip")
	e.tt.Advance(time.Second)
	e.tt.Frame()
	if e.tt.HasText("Tempo") || e.p.prefs.FolderOpen("f.run") {
		t.Error("a folder opened though the drag left it")
	}
	e.drop()
}

func TestAFolderIsNotADropTarget(t *testing.T) {
	e := sbSetup(t, sbLibrary(), nil)
	e.shows("Running")
	e.grab(sbSongPayload(song("s1", "Airbag", "R")))
	e.over("Running")
	if e.tt.HasText("Add to “Running”") {
		t.Error("a folder offers a drop")
	}
	e.drop()
	e.quiet()
	if got := e.adds(); len(got) != 0 {
		t.Errorf("a drop on a folder added: %v", got)
	}
}

// A pin set to Open whose page is not ported plays when chosen, and its menu
// says so: Open is off and Play is the checked row.
func TestPinMenuDoesNotOfferOpenForAPinWithoutAPage(t *testing.T) {
	p, _, _, _, _ := testPlaylists(t, "/")
	pin := services.Pin{Kind: "album", ID: "l.1", Name: "Album", Action: "drillIn"}
	row := func(items []kit.MenuItem, key string) kit.MenuItem {
		for _, it := range items {
			if it.Key == key {
				return it
			}
		}
		t.Fatalf("no row %q", key)
		return kit.MenuItem{}
	}
	items := p.pinMenu(pin, "", false, "", false)
	if open := row(items, "drillIn"); open.Checked || !open.Disabled {
		t.Errorf("Open: checked=%v disabled=%v without a page", open.Checked, open.Disabled)
	}
	if !row(items, "play").Checked {
		t.Error("Play is not the checked row of a pin that plays")
	}
	items = p.pinMenu(pin, "/album/l.1", true, "", false)
	if open := row(items, "drillIn"); !open.Checked || open.Disabled {
		t.Errorf("Open: checked=%v disabled=%v with a page", open.Checked, open.Disabled)
	}
}
