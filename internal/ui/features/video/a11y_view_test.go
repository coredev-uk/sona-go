package video

import (
	"slices"
	"strings"
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

func inOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	at := 0
	for _, g := range got {
		if at < len(want) && g == want[at] {
			at++
		}
	}
	if at < len(want) {
		t.Errorf("Tab reached %q, which lacks %q after %q", got, want[at], want[:at])
	}
}

func mustFind(t *testing.T, tr access.Tree, role ui.Role, label string) access.Node {
	t.Helper()
	n, ok := tr.Find(role, label)
	if !ok {
		t.Fatalf("no %v %q on the tree:\n%s", role, label, tr.Dump())
	}
	return n
}

func TestViewKeyboardWalkAndTree(t *testing.T) {
	r := viewRig(t)
	r.open()
	tr, stops := audit(t, r.tt.Tester)
	inOrder(t, stops, "Sky Video", "Mia Rivers", "Full screen", "Close video", "Seek", "Previous", "Pause", "Next", "Mute", "Volume")

	view := mustFind(t, tr, ui.RoleGroup, "Music video")
	if view.Description == "" {
		t.Error("the view does not say its keys")
	}
	for _, l := range []string{"Sky Video", "Mia Rivers"} {
		mustFind(t, tr, ui.RoleLink, l)
	}
	for _, l := range []string{"Full screen", "Close video", "Previous", "Pause", "Next", "Mute"} {
		n := mustFind(t, tr, ui.RoleButton, l)
		if !n.Can(access.Press) || n.Has(access.Disabled) {
			t.Errorf("%q is not an enabled button: %+v", l, n)
		}
	}
	if got := mustFind(t, tr, ui.RoleSlider, "Seek").Description; got != "0:30 of 3:20" {
		t.Errorf("Seek reads %q", got)
	}
	if got := mustFind(t, tr, ui.RoleSlider, "Volume").Description; got != "60%" {
		t.Errorf("Volume reads %q", got)
	}
	if _, ok := tr.Find(ui.RoleText, "2:50 left"); !ok {
		t.Errorf("the time left is not read as a duration:\n%s", tr.Dump())
	}
	// The spinner is the status's, not a node of its own.
	if n := len(tr.Roles(ui.RoleProgress)); n != 0 {
		t.Errorf("%d unnamed progress nodes", n)
	}
}

func TestFadedControlsStayOnTheTree(t *testing.T) {
	r := viewRig(t)
	r.open()
	r.pictures(500 * time.Millisecond)
	r.tt.Advance(idleAfter + 2*fadeFor)
	r.pictures(500 * time.Millisecond)
	tr := access.Read(t, r.tt.Tester)
	for _, l := range []string{"Pause", "Close video", "Full screen"} {
		if n := mustFind(t, tr, ui.RoleButton, l); !n.Can(access.Press) {
			t.Errorf("%q cannot be pressed by assistive technology while faded", l)
		}
	}
	mustFind(t, tr, ui.RoleSlider, "Seek")
}

func TestViewTellsScreenReadersLoadingBufferingAndFailureOnce(t *testing.T) {
	r := viewRig(t)
	r.play(store.StateLoading)
	r.tt.Announcements()
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Advance(fadeIn)
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Loading the video…"}) {
		t.Fatalf("opened loading, announced %q", got)
	}
	tr := access.Read(t, r.tt.Tester)
	mustFind(t, tr, ui.RoleStatus, "Loading the video…")
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
	// Failing is announced once, with its why.
	r.play(store.StateError, func(s *services.PlaybackState) { s.Error = services.CodeNetwork })
	got := r.tt.Announcements()
	if len(got) != 1 || !strings.HasPrefix(got[0], "Couldn't play this video ") {
		t.Fatalf("a failure announced %q, want its title then its why", got)
	}
	r.tt.Frame()
	r.tt.Frame()
	if again := r.tt.Announcements(); len(again) != 0 {
		t.Errorf("the failure was announced again: %q", again)
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
	inOrder(t, stops, "Close video", "Watch in Apple Music", "Try again", "Seek", "Play")
	status := mustFind(t, tr, ui.RoleStatus, "Couldn't play this video")
	for _, l := range []string{"Watch in Apple Music", "Try again"} {
		n := mustFind(t, tr, ui.RoleButton, l)
		if !tr.Within(n, ui.RoleStatus) || !slices.ContainsFunc(tr.Ancestors(n), func(a access.Node) bool { return a.Index == status.Index }) {
			t.Errorf("%q is not in the problem's status:\n%s", l, tr.Dump())
		}
	}
	if _, ok := tr.Find(ui.RoleText, "SD"); ok {
		t.Error("the SD mark shows with a problem")
	}
}
