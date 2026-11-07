package song

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
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

// walk checks what every song page promises assistive technology: no control
// inside a leaf, every Tab stop on the tree and named, the walk not a trap,
// and want among the stops in reading order.
func (r *rig) walk(want ...string) access.Tree {
	r.t.Helper()
	r.tt.Settle()
	tt := r.tt.Tester
	access.NoNestedControls(r.t, tt)
	stops := stopLabels(access.TabWalk(r.t, tt, 100))
	if !inOrder(stops, want) {
		r.t.Errorf("Tab walk %q, want in order %q", stops, want)
	}
	tr := access.Read(r.t, tt)
	for _, n := range tr.Nodes {
		if n.Has(access.Focusable) && n.Label == "" && n.Role != ui.RoleGroup && n.Role != ui.RoleScroll {
			r.t.Errorf("nameless control %v\n%s", n, tr.Dump())
		}
	}
	return tr
}

// The song page: the title is the h1, the artist and album are links, the
// actions follow in reading order, the shelves are headings over their
// links, and the credits are groups named by their h3.
func TestA11ySongPage(t *testing.T) {
	r := newRig(t, 1132, 1400, airbag())
	r.open()
	tr := r.walk(
		"Radiohead", "OK Computer", "Play Airbag", "Add", "Favourite Airbag", "Suggest less like Airbag", "More for Airbag",
		"View full lyrics", "Airbag (Live)", "See all", "Kid A", "Amnesiac",
	)
	if h, ok := tr.Find(ui.RoleHeading, "Airbag"); !ok || h.Level != 1 {
		t.Errorf("the title is not the h1: %+v", h)
	}
	for _, name := range []string{"Radiohead", "OK Computer", "Airbag (Live)", "Kid A"} {
		if _, ok := tr.Find(ui.RoleLink, name); !ok {
			t.Errorf("%q is not a link", name)
		}
	}
	for _, h2 := range []string{"Lyrics", "Credits", "Music videos", "More by Radiohead"} {
		if h, ok := tr.Find(ui.RoleHeading, h2); !ok || h.Level != 2 {
			t.Errorf("no h2 %q: %+v", h2, h)
		}
	}
	for _, h3 := range []string{"Performers", "Mixing and mastering"} {
		h, ok := tr.Find(ui.RoleHeading, h3)
		if !ok || h.Level != 3 || !tr.Within(h, ui.RoleGroup) {
			t.Errorf("credit group %q: %+v", h3, h)
		}
	}
	// The lyrics toggle says what it does and whether the lyrics are open.
	toggle, ok := tr.Find(ui.RoleDisclosure, "View full lyrics")
	if !ok || toggle.Has(access.Expanded) {
		t.Errorf("lyrics toggle %+v", toggle)
	}
	if n, ok := tr.Find(ui.RoleMenuButton, "More for Airbag"); !ok {
		t.Errorf("header menu %+v", n)
	}
}

// The lyrics: opening them keeps the focus on the toggle, which now says
// Show less and is expanded; the wait is a status announced once, and the
// failure a status with Try again.
func TestA11yLyricsStates(t *testing.T) {
	t.Run("loading then lyrics", func(t *testing.T) {
		r := newRig(t, 1132, 1000, airbag())
		release := r.lyrics.hold()
		r.open()
		r.click("View full lyrics")
		r.tt.Until("the wait", r.tt.Shows("Loading the lyrics…"))
		tr := access.Read(t, r.tt.Tester)
		if _, ok := tr.Find(ui.RoleStatus, "Loading the lyrics…"); !ok {
			t.Errorf("the wait is not a status:\n%s", tr.Dump())
		}
		if got := r.tt.Announcements(); !slices.Contains(got, "Loading the lyrics…") {
			t.Errorf("announcements %q", got)
		}
		if toggle, ok := tr.Find(ui.RoleDisclosure, "Show less"); !ok || !toggle.Has(access.Expanded) {
			t.Errorf("toggle %+v", toggle)
		}
		if !r.tt.Focused("Show less") {
			t.Error("the focus left the toggle")
		}
		// Still waiting: no second announcement.
		r.tt.Drain()
		if got := r.tt.Announcements(); slices.Contains(got, "Loading the lyrics…") {
			t.Errorf("the wait is announced again: %q", got)
		}
		release()
		r.tt.Until("the lyrics", r.tt.Shows("In a blaze of light"))
		if _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleStatus, "Loading the lyrics…"); ok {
			t.Error("the wait stays after the lyrics landed")
		}
	})
	t.Run("failure", func(t *testing.T) {
		r := newRig(t, 1132, 1000, airbag())
		r.lyrics.fail(errors.New("down"))
		r.open()
		r.click("View full lyrics")
		r.tt.Until("the failure", r.tt.Shows("Couldn't load the lyrics", "Try again"))
		tr := r.walk("Try again")
		if _, ok := tr.Find(ui.RoleAlert, "Couldn't load the lyrics"); !ok {
			t.Errorf("the failure is not an alert:\n%s", tr.Dump())
		}
	})
}

// Removing from the library asks first; Escape closes the question and the
// focus returns to the button that opened it.
func TestA11yRemoveQuestionReturnsTheFocus(t *testing.T) {
	r := newRig(t, 1132, 1000, airbag())
	r.library.state = services.ItemState{InLibrary: true, LibraryID: "i.1"}
	r.open()
	r.tt.Until("In library", r.tt.Shows("In library"))
	r.click("In library")
	r.tt.Until("the question", r.tt.Shows("Delete “Airbag” from your library?"))
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Advance(300 * time.Millisecond)
	r.tt.Drain()
	if r.tt.HasText("Delete “Airbag” from your library?") {
		t.Fatal("Escape left the question open")
	}
	if r.library.has("remove") {
		t.Error("Escape removed the song")
	}
	if !r.tt.Focused("In library") {
		t.Error("the focus did not return to the library button")
	}
}

// A song with nothing to read or hear says so, and its controls are reachable.
func TestA11yBareSong(t *testing.T) {
	r := newRig(t, 1132, 1000, bare())
	r.open()
	tr := r.walk("Radiohead", "Play Airbag", "Add", "Favourite Airbag")
	if _, ok := tr.Find(ui.RoleText, "Apple Music has no credits or lyrics for this song."); !ok {
		t.Errorf("the empty note is not on the tree:\n%s", tr.Dump())
	}
}

// Loading is a status with nothing to focus; an unknown song is a heading
// with its way out, and a failure a status with Try again.
func TestA11yLoadingAndErrorStates(t *testing.T) {
	t.Run("loading", func(t *testing.T) {
		r := newRigWith(t, 1132, 1000, func(r *rig) { r.catalog.gate = make(chan struct{}) }, airbag())
		r.tt.Drain()
		r.tt.Settle()
		tr := access.Read(t, r.tt.Tester)
		if _, ok := tr.Find(ui.RoleStatus, "Loading"); !ok {
			t.Fatalf("no Loading status:\n%s", tr.Dump())
		}
		for _, n := range tr.Nodes {
			if n.Has(access.Focusable) && n.Role != ui.RoleGroup {
				t.Errorf("the skeleton has a stop: %v", n)
			}
		}
		access.NoNestedControls(t, r.tt.Tester)
		close(r.catalog.gate)
	})
	t.Run("not found", func(t *testing.T) {
		r := newRig(t, 1132, 1000)
		r.tt.Until("the message", r.tt.Shows("This song isn't available", "Go to Songs"))
		tr := r.walk("Go to Songs")
		if _, ok := tr.Find(ui.RoleHeading, "This song isn't available"); !ok {
			t.Errorf("no heading:\n%s", tr.Dump())
		}
	})
	t.Run("failure", func(t *testing.T) {
		r := newRigWith(t, 1132, 1000, func(r *rig) { r.catalog.set(errors.New("boom")) }, airbag())
		r.tt.Until("the failure", r.tt.Shows("Couldn't load this song", "Try again"))
		tr := r.walk("Try again")
		if _, ok := tr.Find(ui.RoleAlert, "Couldn't load this song"); !ok {
			t.Errorf("the failure is not an alert:\n%s", tr.Dump())
		}
	})
}
