package playlist

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// coverStyles are the names the picker gives the presets, in order.
var coverStyles = []string{"Classic", "Aurora", "Dusk", "Horizon", "Moon", "Glow", "Duo"}

// showCover opens the cover picker of playlist p.a and waits for its options.
func (r *dialogsRig) showCover() {
	r.show(dialogState{kind: dialogCover, id: "p.a", name: "Road Trip"})
	r.shows("Playlist cover", "Automatic", "None", "Done")
}

// chosenCover is the label of the radio the tree says is checked ("" none).
func (r *dialogsRig) chosenCover(t *testing.T) string {
	t.Helper()
	tr := access.Read(t, r.tt.Tester)
	var got []string
	for _, n := range tr.Roles(ui.RoleRadio) {
		if n.Has(access.Checked) {
			got = append(got, n.Label)
		}
	}
	if len(got) > 1 {
		t.Fatalf("more than one cover is checked: %v", got)
	}
	if len(got) == 0 {
		return ""
	}
	return got[0]
}

func TestCoverDialogOffersEveryChoiceAndSaysWhereTheCoverLives(t *testing.T) {
	r := newDialogsRig(t)
	r.showCover()
	r.shows(append([]string{
		"Apple Music can't keep a playlist cover from here, so your choice stays on this computer and shows in Sona only.",
		"Choose an image…",
		"JPEG or PNG, up to 20 MB and 40 megapixels, at least 300 px wide and tall. Sona crops it to a square from the centre.",
	}, coverStyles...)...)
	if r.tt.HasText("Your image") || r.tt.HasText("Remove my image") {
		t.Error("an image is offered before one was chosen")
	}
	if got := r.chosenCover(t); got != "Automatic" {
		t.Errorf("checked cover = %q, want Automatic", got)
	}
	tr := access.Read(t, r.tt.Tester)
	if g, ok := tr.Find(ui.RoleRadioGroup, "Cover"); !ok || len(tr.Children(g)) != 9 {
		t.Errorf("radio group %+v of %d options, want nine", g, len(tr.Children(g)))
	}
	if n, ok := tr.Find(ui.RoleRadio, "None"); !ok || n.Description != "No picture" {
		t.Errorf("None = %+v", n)
	}
	if !r.asked("CoverOptions p.a") {
		t.Errorf("the picker did not ask for the options: %v", r.api.asked())
	}
}

func TestCoverDialogChoosingAppliesAtOnceAndKeepsTheDialogOpen(t *testing.T) {
	r := newDialogsRig(t)
	r.showCover()
	r.tt.Announcements()
	if err := r.tt.Click("Dusk"); err != nil {
		t.Fatal(err)
	}
	r.did("SetCover p.a preset:dusk:3a7bd5:d5b43a")
	r.tt.Until("Dusk is the chosen cover", func() bool { return r.chosenCover(t) == "Dusk" })
	if r.closed() {
		t.Fatal("choosing a cover closed the dialog")
	}
	if got := r.tt.Announcements(); !slices.Contains(got, "Cover: Dusk") {
		t.Errorf("announcements %q lack the new cover", got)
	}

	if err := r.tt.Click("None"); err != nil {
		t.Fatal(err)
	}
	r.did("SetCover p.a none")
	r.tt.Until("None is the chosen cover", func() bool { return r.chosenCover(t) == "None" })
	if err := r.tt.Click("Automatic"); err != nil {
		t.Fatal(err)
	}
	r.did("SetCover p.a auto")
	r.tt.Until("Automatic is the chosen cover", func() bool { return r.chosenCover(t) == "Automatic" })

	if err := r.tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("Done closes the dialog", r.closed)
}

// The choice is made before the service answers and undone, with the reason,
// when it fails.
func TestCoverDialogShowsTheChoiceAtOnceAndTakesItBackOnFailure(t *testing.T) {
	r := newDialogsRig(t)
	gate := make(chan struct{})
	r.api.setGate(gate)
	r.showCover()
	if err := r.tt.Click("Moon"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if got := r.chosenCover(t); got != "Moon" {
		t.Errorf("while the call is out the checked cover is %q, want Moon", got)
	}
	r.api.err = errOffline
	close(gate)
	r.shows(i18n.ErrorText(services.CodeOf(errOffline)))
	r.tt.Until("Automatic is back", func() bool { return r.chosenCover(t) == "Automatic" })
	if r.closed() {
		t.Error("a failed choice closed the dialog")
	}
}

func TestCoverDialogKeyboard(t *testing.T) {
	r := newDialogsRig(t)
	r.showCover()
	if !r.tt.Focused("Automatic") {
		t.Fatalf("the focus is not on the chosen cover: texts %q", r.tt.Texts())
	}
	for _, step := range []struct {
		key  ui.Key
		want string
	}{
		{ui.KeyRight, "None"},
		{ui.KeyRight, "Classic"},
		{ui.KeyDown, "Moon"}, // four columns: one row down from Classic is Moon
		{ui.KeyLeft, "Horizon"},
		{ui.KeyUp, "None"},
		{ui.KeyEnd, "Duo"},
		{ui.KeyHome, "Automatic"},
	} {
		r.tt.Key(0, step.key)
		r.tt.Frame()
		r.tt.Advance(50 * time.Millisecond)
		if !r.tt.Focused(step.want) {
			t.Fatalf("after %v the focus is not on %q", step.key, step.want)
		}
	}
	r.tt.Key(0, ui.KeyRight)
	r.tt.Frame()
	r.tt.Key(0, ui.KeyEnter)
	r.did("SetCover p.a none")
	r.tt.Until("None is chosen", func() bool { return r.chosenCover(t) == "None" })
}

func TestCoverDialogChooseAnImageAndRemoveIt(t *testing.T) {
	r := newDialogsRig(t)
	gate := make(chan struct{})
	r.showCover()
	r.api.setGate(gate)
	r.tt.Announcements()
	if err := r.tt.Click("Choose an image…"); err != nil {
		t.Fatal(err)
	}
	r.shows("Preparing your image…")
	if err := r.tt.Click("Aurora"); err == nil {
		r.tt.Advance(100 * time.Millisecond)
		if r.asked("SetCover") {
			t.Error("a cover was chosen while the image was being prepared")
		}
	}
	close(gate)
	r.did("UploadCover p.a")
	r.shows("Your image", "Remove my image")
	r.tt.Until("the image is the chosen cover", func() bool { return r.chosenCover(t) == "Your image" })
	if got := r.tt.Announcements(); !slices.Contains(got, "Your image is now the cover") {
		t.Errorf("announcements %q", got)
	}

	if err := r.tt.Click("Remove my image"); err != nil {
		t.Fatal(err)
	}
	r.did("RemoveCoverUpload p.a")
	r.tt.Until("the image is gone", func() bool { return !r.tt.HasText("Your image") && !r.tt.HasText("Remove my image") })
	if got := r.chosenCover(t); got != "Automatic" {
		t.Errorf("checked cover = %q, want Automatic", got)
	}
}

// What the service says about an image it can't use stays beside the button,
// where the limits are, and the cover does not change.
func TestCoverDialogUnusableImageSaysWhyBesideTheButton(t *testing.T) {
	r := newDialogsRig(t)
	r.showCover()
	r.api.err = artwork.ErrCoverImage
	r.tt.Announcements()
	if err := r.tt.Click("Choose an image…"); err != nil {
		t.Fatal(err)
	}
	r.did("UploadCover p.a")
	r.shows("Sona can't use that image. It must be a JPEG or PNG within the size limits.",
		"JPEG or PNG, up to 20 MB and 40 megapixels, at least 300 px wide and tall. Sona crops it to a square from the centre.")
	tr := access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleStatus, "Sona can't use that image. It must be a JPEG or PNG within the size limits."); !ok || !tr.Within(n, ui.RoleDialog) {
		t.Errorf("the failure is not a status in the dialog: %+v", n)
	}
	if got := r.chosenCover(t); got != "Automatic" {
		t.Errorf("checked cover = %q after a refused image", got)
	}
	if got := r.tt.Announcements(); !slices.ContainsFunc(got, func(s string) bool { return strings.HasPrefix(s, "Sona can't use that image") }) {
		t.Errorf("the failure was not announced: %q", got)
	}
	// Trying again clears it.
	r.api.err = nil
	if err := r.tt.Click("Choose an image…"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the old failure is gone", func() bool {
		return !r.tt.HasText("Sona can't use that image. It must be a JPEG or PNG within the size limits.")
	})
}

func TestCoverDialogWithoutAFileChooserExplainsInsteadOfShowingADeadButton(t *testing.T) {
	r := newDialogsRig(t)
	r.api.covers["p.a"] = testCovers(false, false)
	r.showCover()
	r.shows("Choosing an image needs a file chooser, and this system has none.")
	if r.tt.HasText("Choose an image…") {
		t.Error("a button that cannot work is shown")
	}
	r.shows(coverStyles...)
}

func TestCoverDialogLoadingAndFailedLoad(t *testing.T) {
	r := newDialogsRig(t)
	r.api.failReads = true
	r.show(dialogState{kind: dialogCover, id: "p.a", name: "Road Trip"})
	r.shows("Couldn't load the covers. "+i18n.ErrorText(services.CodeOf(errOffline)), "Try again")
	if r.tt.HasText("Automatic") {
		t.Error("options show with nothing loaded")
	}
	r.api.failReads = false
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.shows("Automatic", "None")
}

// A cover chosen in the picker stays chosen: another picker for the same
// playlist, and another playlist's, read it from the service.
func TestCoverDialogRemembersTheChoice(t *testing.T) {
	r := newDialogsRig(t)
	r.showCover()
	if err := r.tt.Click("Glow"); err != nil {
		t.Fatal(err)
	}
	r.did("SetCover p.a preset:glow:3a7bd5:d5b43a")
	r.tt.Until("Glow is chosen", func() bool { return r.chosenCover(t) == "Glow" })
	_ = r.tt.Click("Done")
	r.tt.Until("closed", r.closed)

	r.show(dialogState{kind: dialogCover, id: "p.a", name: "Road Trip"})
	r.shows("Playlist cover", "Glow")
	if got := r.chosenCover(t); got != "Glow" {
		t.Errorf("reopened with %q chosen, want Glow", got)
	}
	r.show(dialogState{kind: dialogCover, id: "p.b", name: "Gym"})
	r.shows("Playlist cover")
	r.tt.Until("another playlist starts with the automatic cover", func() bool { return r.chosenCover(t) == "Automatic" })
}

func TestCoverActionIsOfferedForLibraryPlaylistsOnly(t *testing.T) {
	f := newActionsFixture(t)
	d := f.def(t, "playlists.cover")
	catalogue := kit.PlaylistItem(services.Playlist{ID: "pl.123", Source: services.SourceCatalog, CanEdit: true})
	for _, c := range []struct {
		name string
		item kit.Item
		want bool
	}{
		{"own library playlist", libPlaylist("p.1", true, ""), true},
		// The cover is Sona's own: a playlist Apple keeps up to date has one to choose too.
		{"library playlist the user cannot edit", libPlaylist("p.2", false, ""), true},
		{"personal mix", libPlaylist("p.3", false, "pl.pm-1"), true},
		{"catalogue playlist", catalogue, false},
	} {
		if got := available(d, f.r.Context([]kit.Item{c.item}, nil, nil)); got != c.want {
			t.Errorf("%s: available = %v, want %v", c.name, got, c.want)
		}
	}
	if got := d.Label(f.r.Context([]kit.Item{libPlaylist("p.1", true, "")}, nil, nil)); got != "Change cover…" {
		t.Errorf("label = %q", got)
	}
	d.Run(f.r.Context([]kit.Item{libPlaylist("p.1", true, "")}, nil, nil))
	if s := f.p.dialogs.s; !f.p.dialogs.open || s.kind != dialogCover || s.id != "p.1" || s.name != "PL p.1" {
		t.Errorf("the action opened %+v (open %v)", s, f.p.dialogs.open)
	}
}

func TestEditModeHasACoverButton(t *testing.T) {
	r := newPageRig(t, "p.1", libraryDetail("p.1", "Road Trip", threeSongs()...))
	r.shows("Songs in Road Trip")
	if r.tt.HasText("Cover") {
		t.Error("the cover button shows outside edit mode, where the menu has the action")
	}
	r.click("Edit")
	r.shows("Done", "Name and description", "Cover")
	r.click("Cover")
	if s := r.p.dialogs.s; !r.p.dialogs.open || s.kind != dialogCover || s.id != "p.1" || s.name != "Road Trip" {
		t.Errorf("the cover button opened %+v (open %v)", s, r.p.dialogs.open)
	}
	r.shows("Playlist cover", "Automatic")
}

// The picker's names and states: a named dialog with a radio group of radios,
// the chosen one checked, nothing nested in a control, Tab inside.
func TestA11yCoverDialog(t *testing.T) {
	r := newDialogsRig(t)
	r.showCover()
	r.tt.Settle()
	tr := access.Read(t, r.tt.Tester)
	if _, ok := tr.Find(ui.RoleDialog, "Playlist cover"); !ok {
		t.Fatalf("no dialog:\n%s", tr.Dump())
	}
	if h, ok := tr.Find(ui.RoleHeading, "Playlist cover"); !ok || h.Level != 2 {
		t.Errorf("heading %+v", h)
	}
	if tr.Focus == nil || tr.Focus.Label != "Automatic" || tr.Focus.Role != ui.RoleRadio {
		t.Errorf("focus %+v, want the chosen radio", tr.Focus)
	}
	for _, name := range append([]string{"Automatic", "None"}, coverStyles...) {
		n, ok := tr.Find(ui.RoleRadio, name)
		if !ok || !n.Has(access.Focusable) || !n.Can(access.Press) || n.Description == "" {
			t.Errorf("radio %q: %+v (found %v)", name, n, ok)
		}
	}
	access.NoNestedControls(t, r.tt.Tester)
	stops := stopLabels(access.TabWalk(t, r.tt.Tester, 40))
	for _, want := range append([]string{"Automatic", "None", "Choose an image…", "Done", "Close"}, coverStyles...) {
		if !slices.Contains(stops, want) {
			t.Errorf("Tab walk %q lacks %q", stops, want)
		}
	}
	tr = access.Read(t, r.tt.Tester)
	for _, n := range tr.Nodes {
		if n.Has(access.Focusable) && !tr.Within(n, ui.RoleDialog) && n.Role != ui.RoleDialog {
			t.Errorf("%v is a stop outside the open dialog", n)
		}
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300 * time.Millisecond)
	r.tt.Drain()
	if !r.closed() {
		t.Error("Escape left the dialog open")
	}
}

// Replacing the kept image changes every surface and is said, though the
// chosen option stays "Your image".
func TestCoverDialogAnnouncesAReplacedImage(t *testing.T) {
	r := newDialogsRig(t)
	r.api.covers["p.a"] = func() services.PlaylistCovers {
		c := testCovers(true, true)
		c.Current = services.CoverUpload
		return c
	}()
	r.showCover()
	r.tt.Announcements()
	if err := r.tt.Click("Choose an image…"); err != nil {
		t.Fatal(err)
	}
	r.did("UploadCover p.a")
	r.tt.Until("it is announced", func() bool { return slices.Contains(r.tt.Announcements(), "Your image is now the cover") })
}
