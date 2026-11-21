package player

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// stripRig is Full-Screen Lyrics over a long line, with a fake playback
// service behind the player's commands, and the strip showing.
func stripRig(t *testing.T, mutate ...func(*services.PlaybackState)) (*npRig, *fakeTransport) {
	t.Helper()
	r := stageRig(t, heldLines(60000), 5000, nil)
	fake := newFake()
	r.p.tr = kit.Transport{Svc: fake, Play: r.h.Env.Play, Fail: r.p.fail}
	if len(mutate) > 0 {
		r.play(5000, mutate...)
	}
	r.tt.SetFocused(true)
	r.wake()
	r.tt.Until("the strip", r.tt.Shows("Exit full screen"))
	return r, fake
}

func TestTheStripHasPreviousPlayPauseAndNextThatDoWhatTheBarsDo(t *testing.T) {
	r, fake := stripRig(t)
	for _, c := range []struct{ label, call string }{{"Next", "next"}, {"Previous", "prev"}, {"Pause", "toggle"}} {
		r.tt.MustClick(c.label)
		if got := fake.Wait(t, 1); got[len(got)-1] != c.call {
			t.Errorf("%s sent %v, want %s", c.label, got, c.call)
		}
	}
	// The strip stays: pressing it is a pointer on the view.
	if !r.tt.HasText("Exit full screen") {
		t.Error("the strip went when a control was pressed")
	}
	// Paused, the disc says Play, and the strip holds.
	r.play(5000, func(s *services.PlaybackState) { s.State = store.StatePaused })
	r.settle()
	r.tt.Want("Play")
	r.tt.Lacks("Pause")
	r.tt.MustClick("Play")
	if got := fake.Wait(t, 1); got[len(got)-1] != "toggle" {
		t.Errorf("Play sent %v", got)
	}
}

func TestTheStripsControlsAreOnlyBuiltWhileItShows(t *testing.T) {
	r := stageRig(t, heldLines(60000), 5000, nil)
	r.tt.SetFocused(true)
	for _, l := range []string{"Previous", "Pause", "Next"} {
		if r.tt.HasText(l) {
			t.Errorf("%q is built with the strip hidden: Tab would land on it unseen", l)
		}
	}
	r.wake()
	r.tt.Until("the strip", r.tt.Shows("Exit full screen"))
	r.tt.Want("Previous", "Pause", "Next")
	tr := access.Read(t, r.tt.Tester)
	g, ok := tr.Find(ui.RoleGroup, "Playback controls")
	if !ok {
		t.Fatalf("the controls are not a labelled group:\n%s", tr.Dump())
	}
	for _, l := range []string{"Previous", "Pause", "Next"} {
		if n, ok := tr.Find(ui.RoleButton, l); !ok || !tr.Within(n, ui.RoleGroup) || n.Parent != g.Index {
			t.Errorf("%q is not a button in the controls group", l)
		}
	}
}

// Tab goes into the strip once it shows, previous, play and next first (the
// bar's order), then the timing nudge, then Exit.
func TestTabWalksThePlayerControlsBeforeTheTimingNudge(t *testing.T) {
	r, _ := stripRig(t)
	stops := access.TabWalk(t, r.tt.Tester, 12)
	access.InOrder(t, access.StopLabels(stops), "Previous", "Pause", "Next", "Seek", "Show lyrics 0.5 s earlier", "Show lyrics 0.5 s later", "Exit full screen")
	access.NoNestedControls(t, r.tt.Tester)
}

// What a station or the end of the queue cannot do is as unavailable here
// as in the bar: disabled at the end of the queue, explained and ignored for
// a station the user started.
func TestTheStripsPreviousAndNextFollowTheBarsRules(t *testing.T) {
	r, _ := stripRig(t, func(s *services.PlaybackState) {
		s.Controls = services.PlaybackControls{Seek: true, Shuffle: true, Repeat: true}
	})
	tr := access.Read(t, r.tt.Tester)
	for _, l := range []string{"Previous", "Next"} {
		if n, ok := tr.Find(ui.RoleButton, l); !ok || n.States&access.Disabled == 0 {
			t.Errorf("%q is not disabled with nothing to go to: %+v", l, n)
		}
	}
	if n, ok := tr.Find(ui.RoleButton, "Pause"); !ok || n.States&access.Disabled != 0 {
		t.Errorf("Pause is disabled: %+v", n)
	}

	r, fake := stripRig(t, func(s *services.PlaybackState) {
		s.Controls = services.PlaybackControls{Seek: true, Shuffle: true, Repeat: true}
		s.Station = &services.QueueStation{ID: "ra.1", Name: "A Station"}
	})
	for _, l := range []string{"Previous", "Next"} {
		tabTo(t, r.tt.Tester, l)
		r.tt.Key(0, ui.KeyEnter)
		r.tt.Frame()
		if !r.tt.Focused(l) {
			t.Errorf("%q lost the focus on a press", l)
		}
	}
	fake.Quiet(t, "pressing what a station cannot do")
	if got := fake.Snapshot(); len(got) != 0 {
		t.Errorf("a station's strip sent %v", got)
	}
	tabTo(t, r.tt.Tester, "Pause")
	r.tt.Key(0, ui.KeyEnter)
	if got := fake.Wait(t, 1); !slices.Equal(got, []string{"toggle"}) {
		t.Errorf("Pause sent %v", got)
	}
}

// In the narrowest window the controls stay on the screen: the transport and
// Exit full screen are not pushed off the edge by a long title.
func TestTheStripFitsTheNarrowestWindowWithALongTitle(t *testing.T) {
	for _, w := range []int{960, 1024, 1366} {
		r := stageRig(t, heldLines(60000), 5000, nil, func(r *npRig) { r.width, r.height = w, 700 })
		r.play(5000, func(s *services.PlaybackState) {
			s.Current.Name = "A Very Long Song Title That Goes On And On"
			s.Current.Artist = "An Artist With A Long Name, And A Featured Guest"
		})
		r.wake()
		r.tt.Until("the strip", r.tt.Shows("Exit full screen"))
		r.settle()
		for _, l := range []string{"Previous", "Pause", "Next", "Show lyrics 0.5 s later", "Exit full screen"} {
			b, ok := r.tt.Find(l)
			if !ok {
				t.Errorf("%dpx: %q is not in the strip", w, l)
				continue
			}
			if left, right := float32(w)*stageLeft-1, float32(w)*(1-stageLeft)+1; b.X < left || b.X+b.W > right {
				t.Errorf("%dpx: %q is at %v..%v, outside the strip's %v..%v", w, l, b.X, b.X+b.W, left, right)
			}
		}
	}
}

// The strip's scrubber is the bar's seek slider: a click jumps, a drag shows
// where it will go and seeks once when the pointer lets go, the arrows seek.
func TestTheStripsScrubberSeeksOnAClickADragAndTheArrows(t *testing.T) {
	r, fake := stripRig(t)
	b, ok := r.tt.Find("Seek")
	if !ok {
		t.Fatal("the strip has no scrubber")
	}
	y := b.Y + b.H/2
	r.tt.ClickAt(b.X+b.W/2, y)
	fake.Wait(t, 1)
	seeks := fake.Seeks()
	if len(seeks) != 1 || seeks[0] < 110_000 || seeks[0] > 130_000 {
		t.Fatalf("a click half way along a 4:00 song sought %v ms", seeks)
	}

	r.tt.Press(b.X+b.W/4, y)
	for i := 1; i <= 4; i++ {
		r.tt.Move(b.X+b.W/4+float32(i)*b.W/16, y)
		r.tt.Frame()
	}
	if got := fake.Seeks(); len(got) != 1 {
		t.Fatalf("seeks while the pointer is held: %v", got)
	}
	r.tt.Release(b.X+b.W/2, y)
	r.tt.Frame()
	r.tt.Frame()
	fake.Wait(t, 1)
	seeks = fake.Seeks()
	if len(seeks) != 2 || seeks[1] < 110_000 || seeks[1] > 130_000 {
		t.Fatalf("a drag released half way sought %v ms, want one more seek there", seeks)
	}

	tabTo(t, r.tt.Tester, "Seek")
	r.tt.Key(0, ui.KeyRight)
	r.tt.Frame()
	fake.Wait(t, 1)
	if got := fake.Seeks(); len(got) != 3 {
		t.Errorf("an arrow sought %v, want one more seek", got)
	}
}

// What cannot seek, as a live station's controls say (Full-Screen Lyrics
// itself closes for a station), has its scrubber there and disabled.
func TestTheStripsScrubberIsDisabledWhereTheSongCannotSeek(t *testing.T) {
	r, fake := stripRig(t, func(s *services.PlaybackState) {
		s.Controls = services.PlaybackControls{}
	})
	n, ok := access.Read(t, r.tt.Tester).Find(ui.RoleSlider, "Seek")
	if !ok || n.States&access.Disabled == 0 {
		t.Fatalf("the scrubber is not disabled for a live item: %+v (found %v)", n, ok)
	}
	b, _ := r.tt.Find("Seek")
	r.tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
	fake.Quiet(t, "clicking a live item's scrubber")
	if got := fake.Seeks(); len(got) != 0 {
		t.Errorf("a live item's scrubber sought %v", got)
	}
}
