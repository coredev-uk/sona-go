package video

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// audit is the view's accessibility check: no control hides inside a leaf,
// and Tab comes back round with every stop on the tree. It returns the tree
// and the stops' names.
func audit(t *testing.T, tt *ui.Tester) (access.Tree, []string) {
	t.Helper()
	access.NoNestedControls(t, tt)
	var names []string
	for _, s := range access.TabWalk(t, tt, 60) {
		names = append(names, s.Node.Label)
	}
	return access.Read(t, tt), names
}

func TestViewKeyboardWalkAndTree(t *testing.T) {
	r := viewRig(t)
	r.open()
	tr, stops := audit(t, r.tt.Tester)
	access.InOrder(t, stops, "Sky Video", "Mia Rivers", "Full screen", "Close video", "Seek", "Previous", "Pause", "Next", "Mute", "Volume")

	view := tr.MustFind(t, ui.RoleGroup, "Music video")
	if view.Description == "" {
		t.Error("the view does not say its keys")
	}
	for _, l := range []string{"Sky Video", "Mia Rivers"} {
		tr.MustFind(t, ui.RoleLink, l)
	}
	for _, l := range []string{"Full screen", "Close video", "Previous", "Pause", "Next", "Mute"} {
		n := tr.MustFind(t, ui.RoleButton, l)
		if !n.Can(access.Press) || n.Has(access.Disabled) {
			t.Errorf("%q is not an enabled button: %+v", l, n)
		}
	}
	if got := tr.MustFind(t, ui.RoleSlider, "Seek").Description; got != "0:30 of 3:20" {
		t.Errorf("Seek reads %q", got)
	}
	if v := tr.MustFind(t, ui.RoleSlider, "Volume"); v.Description != "" || v.Now != 60 {
		t.Errorf("Volume reads %v and %q", v.Now, v.Description)
	}
	if _, ok := tr.Find(ui.RoleText, "2:50 left"); !ok {
		t.Errorf("the time left is not read as a duration:\n%s", tr.Dump())
	}
	// The spinner is the status's, not a node of its own.
	if n := len(tr.Roles(ui.RoleProgress)); n != 0 {
		t.Errorf("%d unnamed progress nodes", n)
	}
}

func TestFadedControlsStayOnTheTreeAndFocusBringsThemBack(t *testing.T) {
	r := viewRig(t)
	r.open()
	r.tt.Move(200, 200)
	r.pictures(500 * time.Millisecond) // the first picture: no loading status
	if r.v.view.alpha != 1 {
		t.Fatalf("controls not shown at first: %v", r.v.view.alpha)
	}
	r.pictures(4 * time.Second)
	if r.v.view.alpha != 0 {
		t.Fatalf("the controls did not fade after 4 s of nothing: %v", r.v.view.alpha)
	}
	// Unseen, they are still on the tree and can be pressed from there.
	tr := access.Read(t, r.tt.Tester)
	for _, l := range []string{"Pause", "Close video", "Full screen"} {
		if n := tr.MustFind(t, ui.RoleButton, l); !n.Can(access.Press) {
			t.Errorf("%q cannot be pressed by assistive technology while faded", l)
		}
	}
	tr.MustFind(t, ui.RoleSlider, "Seek")
	// Tab never lands on an unseen control: the focus brings them back.
	r.tt.SetFocused(true)
	r.tt.Key(0, ui.KeyTab)
	r.tt.Advance(2 * fadeFor)
	if r.v.view.alpha != 1 {
		t.Errorf("a control took the focus and the controls stayed faded: %v", r.v.view.alpha)
	}
	if f := access.Read(t, r.tt.Tester).Focus; f == nil {
		t.Error("the focused control is not on the tree")
	}
}

// With a screen reader on, MyGo v0.9.0 reads the status when its text
// changes, and not when it first appears: the view announces only the
// status it opens with, so every reading is once.
func TestViewTellsScreenReadersLoadingBufferingAndFailureOnce(t *testing.T) {
	r := viewRig(t)
	r.play(store.StateLoading)
	r.tt.UseScreenReader()
	r.tt.Announcements()
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Advance(fadeIn)
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Loading the video…"}) {
		t.Fatalf("opened loading, announced %q", got)
	}
	tr := access.Read(t, r.tt.Tester)
	tr.MustFind(t, ui.RoleStatus, "Loading the video…")
	r.tt.Frame()
	r.tt.Frame()
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("announced again: %q", got)
	}
	// A picture, then the pictures stop: buffering.
	r.play(store.StatePlaying)
	r.surface.Show(64, 36, 0, grey(64, 36, 235))
	r.tt.Until("the picture", r.v.feed.HasPicture)
	r.tt.Announcements()
	r.tt.Advance(1200 * time.Millisecond)
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Buffering…"}) {
		t.Errorf("a stalled video announced %q", got)
	}
	// Failing is an alert, with its why: MyGo reads it, so it is not announced as well.
	r.play(store.StateError, func(s *services.PlaybackState) { s.Error = services.CodeNetwork })
	r.tt.Frame()
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Fatalf("a failure announced %q besides being an alert", got)
	}
	tr = access.Read(t, r.tt.Tester)
	if alert := tr.MustFind(t, ui.RoleAlert, "Couldn't play this video"); len(tr.Children(alert)) != 2 {
		t.Errorf("the alert holds %v, want its title and its why", tr.Children(alert))
	}
}

func TestViewProblemOffersItsWaysOutToKeyboardAndScreenReader(t *testing.T) {
	r := viewRig(t)
	r.cat.vid = services.CatalogVideo{Video: services.MusicVideo{ID: "mv.1", URL: "https://music.apple.com/gb/music-video/x/1"}}
	r.play(store.StateError, func(s *services.PlaybackState) { s.Error = services.CodeNetwork })
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Until("the problem", r.tt.Shows("Couldn't play this video", "Try again", "Watch in Apple Music"))
	tr, stops := audit(t, r.tt.Tester)
	access.InOrder(t, stops, "Close video", "Watch in Apple Music", "Try again", "Seek", "Play")
	tr.MustFind(t, ui.RoleAlert, "Couldn't play this video")
	// The ways out are beside the message, not in it: an alert reads its content out.
	for _, l := range []string{"Watch in Apple Music", "Try again"} {
		if n := tr.MustFind(t, ui.RoleButton, l); tr.Within(n, ui.RoleAlert) {
			t.Errorf("%q is inside the alert:\n%s", l, tr.Dump())
		}
	}
	if _, ok := tr.Find(ui.RoleText, "SD"); ok {
		t.Error("the SD mark shows with a problem")
	}
}

func TestViewTakesTheFocusOnOpeningButTabNeverStopsAtIt(t *testing.T) {
	r := viewRig(t)
	r.open()
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if tr.Focus == nil || tr.Focus.Role != ui.RoleGroup || tr.Focus.Label != "Music video" {
		t.Fatalf("the view does not hold the focus on opening: %+v", tr.Focus)
	}
	for _, st := range access.TabWalk(t, r.tt.Tester, 40) {
		if st.Node.Role == ui.RoleGroup && st.Node.Label == "Music video" {
			t.Errorf("Tab stops at the view: %+v", st.Node)
		}
	}
}

func TestTitleIsAHeadingHoldingItsLink(t *testing.T) {
	r := viewRig(t)
	r.open()
	tr := access.Read(t, r.tt.Tester)
	link := tr.MustFind(t, ui.RoleLink, "Sky Video")
	var level int
	for _, a := range tr.Ancestors(link) {
		if a.Role == ui.RoleHeading {
			level = a.Level
		}
	}
	if level != 1 {
		t.Errorf("the title's link is in a heading of level %d, want 1:\n%s", level, tr.Dump())
	}
	if !link.Can(access.Press) {
		t.Errorf("the title's link cannot be pressed: %+v", link)
	}
	r.tt.MustClick("Sky Video")
	if r.nav.Last() != "/music-video/mv.1" {
		t.Errorf("the title opened %v", r.nav.Pushed)
	}
}
