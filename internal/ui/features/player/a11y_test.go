package player

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

const liveReason = "Not available while listening to a live station"

// stopLabels are the names of the Tab stops, in order.
func stopLabels(stops []access.Stop) []string {
	out := make([]string, len(stops))
	for i, s := range stops {
		out[i] = s.Node.Label
	}
	return out
}

// inOrder fails unless want is a subsequence of got.
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

// audit is deliverable A in one call: no control hides inside a leaf, and
// the Tab walk comes back (no trap) with every stop on the tree. It returns
// the tree and the stops' names.
func audit(t *testing.T, tt *ui.Tester) (access.Tree, []string) {
	t.Helper()
	access.NoNestedControls(t, tt)
	stops := access.TabWalk(t, tt, 60)
	return access.Read(t, tt), stopLabels(stops)
}

// tabTo presses Tab until the element named label has the focus.
func tabTo(t *testing.T, tt *ui.Tester, label string) access.Node {
	t.Helper()
	tt.SetFocused(true)
	for range 60 {
		tt.Key(0, ui.KeyTab)
		if f := access.Read(t, tt).Focus; f != nil && f.Label == label {
			return *f
		}
	}
	t.Fatalf("Tab never reached %q", label)
	return access.Node{}
}

func mustFind(t *testing.T, tr access.Tree, role ui.Role, label string) access.Node {
	t.Helper()
	n, ok := tr.Find(role, label)
	if !ok {
		t.Fatalf("no %v %q on the tree:\n%s", role, label, tr.Dump())
	}
	return n
}

func TestBarKeyboardWalkAndTree(t *testing.T) {
	playing := []string{"Open now playing", "Shuffle off", "Previous", "Pause", "Next", "Repeat off", "Seek", "Sleep timer", "Lyrics", "Queue", "Mute", "Volume"}
	for _, tc := range []struct {
		name  string
		seed  func(*rig)
		order []string
		check func(*testing.T, access.Tree)
	}{
		{
			name:  "nothing playing",
			seed:  func(*rig) {},
			order: []string{"Queue", "Mute", "Volume"},
			check: func(t *testing.T, tr access.Tree) {
				for _, n := range []struct {
					role  ui.Role
					label string
				}{
					{ui.RoleButton, "Nothing playing"},
					{ui.RoleButton, "Previous"},
					{ui.RoleButton, "Play"},
					{ui.RoleButton, "Next"},
					{ui.RoleToggleButton, "Shuffle off"},
					{ui.RoleToggleButton, "Repeat off"},
					{ui.RoleSlider, "Seek"},
				} {
					if !mustFind(t, tr, n.role, n.label).Has(access.Disabled) {
						t.Errorf("%q is not disabled with nothing playing", n.label)
					}
				}
				if d := mustFind(t, tr, ui.RoleSlider, "Seek").Description; d != "" {
					t.Errorf("an idle Seek reads %q", d)
				}
			},
		},
		{
			name:  "a song playing",
			seed:  func(r *rig) { r.seed() },
			order: playing,
			check: func(t *testing.T, tr access.Tree) {
				if g, ok := tr.Find(ui.RoleRegion, "Player"); !ok || !tr.Within(mustFind(t, tr, ui.RoleButton, "Pause"), ui.RoleRegion) {
					t.Errorf("the bar is not a region named Player: %v", g)
				}
				if got := mustFind(t, tr, ui.RoleSlider, "Seek").Description; got != "0:30 of 3:00" {
					t.Errorf("Seek reads %q", got)
				}
				// 0 to 100: the number and the percentage say it; a description would
				// make a screen reader say it a third time.
				if v := mustFind(t, tr, ui.RoleSlider, "Volume"); v.Description != "" || v.Now != 80 || v.Max != 100 {
					t.Errorf("Volume reads %v of %v and %q", v.Now, v.Max, v.Description)
				}
				// The name of a toggle is its state, and Checked is the state.
				for _, l := range []string{"Shuffle off", "Repeat off", "Lyrics", "Queue"} {
					if mustFind(t, tr, ui.RoleToggleButton, l).Has(access.Checked) {
						t.Errorf("%q is checked", l)
					}
				}
				for _, l := range []string{"Previous", "Pause", "Next", "Mute", "Sleep timer"} {
					n, ok := tr.Find(ui.RoleButton, l)
					if !ok {
						n, ok = tr.Find(ui.RoleMenuButton, l)
					}
					if !ok || n.Has(access.Disabled) || !n.Has(access.Focusable) {
						t.Errorf("%q is not an enabled, focusable button (found %v)", l, ok)
					}
				}
			},
		},
		{
			name: "toggles on",
			seed: func(r *rig) {
				r.p.ui.queueOpen = true
				r.seed(func(s *services.PlaybackState) { s.Shuffle, s.Repeat = true, "all" })
			},
			order: []string{"Open now playing", "Shuffle on", "Previous", "Pause", "Next", "Repeat all", "Seek"},
			check: func(t *testing.T, tr access.Tree) {
				for _, l := range []string{"Shuffle on", "Repeat all", "Queue"} {
					if !mustFind(t, tr, ui.RoleToggleButton, l).Has(access.Checked) {
						t.Errorf("%q is not checked", l)
					}
				}
			},
		},
		{
			name: "sign in again",
			seed: func(r *rig) {
				r.env.Surfaces = store.NewSurfaces()
				r.seed(func(s *services.PlaybackState) { s.AuthRequired = true })
			},
			order: []string{"Open now playing", "Sign in again to keep playing", "Shuffle off", "Previous", "Pause"},
			check: func(t *testing.T, tr access.Tree) {
				mustFind(t, tr, ui.RoleButton, "Sign in again to keep playing")
			},
		},
		{
			name: "a live station",
			seed: func(r *rig) {
				r.seed(func(s *services.PlaybackState) {
					s.Current.Live = true
					s.Controls = services.PlaybackControls{}
				})
			},
			// What a station cannot do keeps its place in the Tab order.
			order: []string{"Open now playing", "Shuffle off", "Previous", "Pause", "Next", "Repeat off", "Sleep timer"},
			check: func(t *testing.T, tr access.Tree) {
				for _, n := range []struct {
					role  ui.Role
					label string
				}{
					{ui.RoleToggleButton, "Shuffle off"}, {ui.RoleButton, "Previous"}, {ui.RoleButton, "Next"}, {ui.RoleToggleButton, "Repeat off"},
				} {
					b := mustFind(t, tr, n.role, n.label)
					if !b.Has(access.Disabled) || !b.Has(access.Focusable) || b.Can(access.Press) || b.Description != liveReason {
						t.Errorf("%q: disabled %v, focusable %v, description %q", n.label, b.Has(access.Disabled), b.Has(access.Focusable), b.Description)
					}
				}
				if _, ok := tr.Find(ui.RoleSlider, "Seek"); ok {
					t.Error("a live station has a Seek slider")
				}
			},
		},
		{
			name:  "loading",
			seed:  func(r *rig) { r.seed(func(s *services.PlaybackState) { s.State = store.StateLoading }) },
			order: []string{"Open now playing", "Shuffle off", "Previous", "Play", "Next"},
			check: func(t *testing.T, tr access.Tree) {
				mustFind(t, tr, ui.RoleStatus, "Loading")
			},
		},
		{
			name: "it failed to play",
			seed: func(r *rig) {
				r.seed(func(s *services.PlaybackState) { s.State, s.Error = store.StateError, services.CodeNetwork })
			},
			order: []string{"Open now playing", "Shuffle off", "Previous", "Play", "Next"},
			check: func(t *testing.T, tr access.Tree) {
				st := mustFind(t, tr, ui.RoleStatus, "Couldn't play this song. Press play to try again.")
				if st.Description == "" {
					t.Error("the failure's why (the tooltip) is not its description")
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, true)
			tc.seed(r)
			tr, got := audit(t, r.tt.Tester)
			inOrder(t, got, tc.order...)
			tc.check(t, tr)
		})
	}
}

func TestSignInAgainIsABarControlThatOpensTheAccountSurface(t *testing.T) {
	r := newRig(t, true)
	r.env.Surfaces = store.NewSurfaces()
	r.seed(func(s *services.PlaybackState) { s.AuthRequired = true })
	n := mustFind(t, access.Read(t, r.tt.Tester), ui.RoleButton, "Sign in again to keep playing")
	if !n.Has(access.Focusable) || !n.Can(access.Press) {
		t.Fatalf("the sign-in line is not a focusable, pressable button: %+v", n)
	}
	// It sits beside the title, not inside anything that hides it.
	access.NoNestedControls(t, r.tt.Tester)
	tabTo(t, r.tt.Tester, "Sign in again to keep playing")
	if r.env.Surfaces.AccountOpen() {
		t.Fatal("the account surface opened before Enter")
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Frame()
	if !r.env.Surfaces.AccountOpen() {
		t.Error("Enter on the sign-in line did not open the account surface")
	}
}

func TestUnavailableBarControlsKeepTheFocusExplainAndIgnoreThePress(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) {
		s.Current.Live = true
		s.Controls = services.PlaybackControls{}
	})
	for _, name := range []string{"Shuffle off", "Previous", "Next", "Repeat off"} {
		tabTo(t, r.tt.Tester, name)
		r.tt.Key(0, ui.KeyEnter)
		r.tt.Key(0, ui.KeySpace)
		r.tt.Frame()
		if !r.tt.Focused(name) {
			t.Errorf("%q lost the focus on a press", name)
		}
	}
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("pressing what a live station cannot do sent %v", got)
	}
	// A control that can be pressed still works from the same spot.
	tabTo(t, r.tt.Tester, "Pause")
	r.tt.Key(0, ui.KeyEnter)
	if got := r.fake.wait(t, 1); !slices.Equal(got, []string{"toggle"}) {
		t.Errorf("Pause sent %v", got)
	}
}

func TestTheBarSaysWhatItDoesNotPlayOnce(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.Current.ID = "a" })
	r.tt.Announcements()
	fail := func(code services.ErrorCode) {
		r.seed(func(s *services.PlaybackState) {
			s.Current.ID = "a"
			s.State, s.Error = store.StateError, code
		})
	}
	fail(services.CodeNetwork)
	want := []string{"Couldn't play this song. Press play to try again."}
	if got := r.tt.Announcements(); !slices.Equal(got, want) {
		t.Fatalf("announced %q, want %q", got, want)
	}
	r.tt.Frame()
	r.tt.Frame()
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Errorf("announced again: %q", got)
	}
	// Pressing play again and failing again is news.
	r.seed(func(s *services.PlaybackState) { s.Current.ID = "a"; s.State = store.StateLoading })
	r.tt.Announcements()
	fail(services.CodeNetwork)
	if got := r.tt.Announcements(); !slices.Equal(got, want) {
		t.Errorf("a second failure announced %q", got)
	}
	// A music video says so.
	r.seed(func(s *services.PlaybackState) {
		s.Current.ID, s.Current.Video = "v", true
		s.State, s.Error = store.StateError, services.CodeNetwork
	})
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Couldn't play this video."}) {
		t.Errorf("a video's failure announced %q", got)
	}
}

func TestTheTrackAnnouncerStaysQuietForWhatIsNotANewTrack(t *testing.T) {
	r := newRig(t, true)
	r.tt.Announcements()
	paused := func(s *services.PlaybackState) { s.State = store.StatePaused }
	// The track restored paused at launch says nothing…
	r.seed(paused)
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Fatalf("a track restored paused announced %q", got)
	}
	// …until it plays.
	r.seed()
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Now playing: Song Title by The Artist"}) {
		t.Fatalf("announced %q", got)
	}
	// Pause, resume, seek, loop the same track: quiet.
	for name, mut := range map[string]func(*services.PlaybackState){
		"pause":   paused,
		"resume":  func(*services.PlaybackState) {},
		"seek":    func(s *services.PlaybackState) { s.PositionMs = 90_000 },
		"loading": func(s *services.PlaybackState) { s.State = store.StateLoading },
		"repeat":  func(s *services.PlaybackState) { s.Repeat = "one" },
	} {
		r.seed(mut)
		if got := r.tt.Announcements(); len(got) != 0 {
			t.Errorf("%s announced %q", name, got)
		}
	}
	// A different track is announced once it plays, a preview as one.
	r.seed(func(s *services.PlaybackState) {
		s.Current = &services.QueueItem{ID: "i.2", Name: "Clocks", Artist: "Coldplay"}
		s.Preview = true
	})
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Now playing a preview: Clocks by Coldplay"}) {
		t.Errorf("announced %q", got)
	}
}

func TestARefusedCommandSaysWhyOncePerKeyHold(t *testing.T) {
	r := newRigWith(t, true, kit.Toaster)
	r.seed(func(s *services.PlaybackState) {
		s.Current.Live = true
		s.Controls = services.PlaybackControls{}
	})
	r.tt.Announcements()
	var heard []string
	for range 5 { // a key held down repeats
		if !r.p.refuse(ctlNext) {
			t.Fatal("Next was not refused on a live station")
		}
		r.tt.Frame()
		heard = append(heard, r.tt.Announcements()...)
	}
	r.tt.Frame()
	heard = append(heard, r.tt.Announcements()...)
	var n int
	for _, a := range heard {
		if strings.Contains(a, liveReason) {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the reason was announced %d times, want once: %q", n, heard)
	}
}

func TestSleepTimerIsAMenuButtonThatKeepsTheFocusAcrossTheMenu(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	b := tabTo(t, r.tt.Tester, "Sleep timer")
	if b.Role != ui.RoleMenuButton || b.Has(access.Expanded) {
		t.Fatalf("the sleep timer is %v, expanded %v", b.Role, b.Has(access.Expanded))
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Frame()
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if n := mustFind(t, tr, ui.RoleMenuButton, "Sleep timer"); !n.Has(access.Expanded) || n.HasPopup != ui.PopupMenu {
		t.Errorf("the open sleep menu does not show on its button, a menu button:\n%s", tr.Dump())
	}
	if _, ok := tr.Find(ui.RoleMenuItemRadio, "In 15 minutes"); !ok {
		t.Errorf("the menu's choices are not radio items:\n%s", tr.Dump())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Frame()
	r.tt.Frame()
	tr = access.Read(t, r.tt.Tester)
	b = mustFind(t, tr, ui.RoleMenuButton, "Sleep timer")
	if b.Has(access.Expanded) {
		t.Error("the sleep button still says it is expanded after Escape")
	}
	if tr.Focus == nil || tr.Focus.Label != "Sleep timer" {
		t.Errorf("the focus did not return to the sleep button: %v", tr.Focus)
	}
	access.NoNestedControls(t, r.tt.Tester)
}

// hasButton says whether the tree has a button of role named label inside
// the list item named item.
func inItem(t *testing.T, tr access.Tree, item string, role ui.Role, label string) access.Node {
	t.Helper()
	n := mustFind(t, tr, role, label)
	li := mustFind(t, tr, ui.RoleListItem, item)
	if !slices.ContainsFunc(tr.Ancestors(n), func(a access.Node) bool { return a.Index == li.Index }) {
		t.Errorf("%q is not inside the list item %q:\n%s", label, item, tr.Dump())
	}
	return n
}

func TestQueuePanelKeyboardWalkAndTree(t *testing.T) {
	rowActions := func(t *testing.T, tr access.Tree, stops []string, from, to int) {
		t.Helper()
		for i := from; i <= to; i++ {
			item := "Song " + string(rune('0'+i)) + ", Artist " + string(rune('0'+i))
			if i == from {
				item = "Up next: " + item
			}
			more := inItem(t, tr, item, ui.RoleMenuButton, "More for Song "+string(rune('0'+i)))
			rm := inItem(t, tr, item, ui.RoleButton, "Remove Song "+string(rune('0'+i))+" from the queue")
			for _, n := range []access.Node{more, rm} {
				if !n.Can(access.Press) || n.Has(access.Disabled) {
					t.Errorf("%q cannot be pressed by assistive technology: %+v", n.Label, n)
				}
				// The list is one Tab stop: the row's buttons are not stops.
				if slices.Contains(stops, n.Label) {
					t.Errorf("%q is a Tab stop", n.Label)
				}
			}
		}
	}
	for _, tc := range []struct {
		name   string
		inView bool
		seed   func(*queueRig)
		order  []string
		check  func(*testing.T, *queueRig, access.Tree, []string)
	}{
		{
			name:  "beside the page",
			seed:  func(r *queueRig) { r.seed(4, 0) },
			order: []string{"Toggle queue", "Clear", "AutoPlay", "Close queue", "Song 1, Artist 1"},
			check: func(t *testing.T, _ *queueRig, tr access.Tree, stops []string) {
				if h := mustFind(t, tr, ui.RoleHeading, "Queue"); h.Level != 2 {
					t.Errorf("the panel's heading is level %d", h.Level)
				}
				panel := mustFind(t, tr, ui.RoleGroup, "Queue")
				list := mustFind(t, tr, ui.RoleList, "Queue")
				if !tr.Within(list, ui.RoleGroup) || list.Index < panel.Index {
					t.Error("the list is not in the Queue panel")
				}
				// Every child of the list is a list item.
				for _, c := range tr.Children(list) {
					if c.Role != ui.RoleListItem {
						t.Errorf("the list holds a %v", c)
					}
				}
				now := mustFind(t, tr, ui.RoleListItem, "Song 1, Artist 1")
				if now.Current != ui.CurrentItem || now.Has(access.Selected) {
					t.Errorf("the playing row is not the current item: %+v", now)
				}
				if other := mustFind(t, tr, ui.RoleListItem, "Up next: Song 2, Artist 2"); other.Current != ui.CurrentNone {
					t.Errorf("a row that is not playing is the current item: %+v", other)
				}
				if _, ok := tr.Find(ui.RoleImage, "Now playing"); !ok {
					t.Error("the playing row does not say it is playing")
				}
				for _, l := range []string{"More for Song 1", "Remove Song 1 from the queue"} {
					if len(tr.Labelled(ui.RoleButton, l))+len(tr.Labelled(ui.RoleMenuButton, l)) != 0 {
						t.Errorf("the playing row has %q", l)
					}
				}
				rowActions(t, tr, stops, 2, 4)
				if !strings.Contains(mustFind(t, tr, ui.RoleListItem, "Up next: Song 2, Artist 2").Description, "Alt+Up and Alt+Down") {
					t.Error("a row does not say its keys")
				}
			},
		},
		{
			name:   "Up next in Now Playing",
			inView: true,
			seed:   func(r *queueRig) { r.seed(4, 0) },
			order:  []string{"Song 1, Artist 1"},
			check: func(t *testing.T, _ *queueRig, tr access.Tree, stops []string) {
				rowActions(t, tr, stops, 2, 4)
				d := mustFind(t, tr, ui.RoleListItem, "Up next: Song 2, Artist 2").Description
				if !strings.Contains(d, "Escape closes now playing") {
					t.Errorf("the row's help is %q", d)
				}
			},
		},
		{
			name:  "with AutoPlay's part",
			seed:  func(r *queueRig) { r.seed(6, 0, autoPlayFrom(3)) },
			order: []string{"Toggle queue", "Clear", "AutoPlay", "Close queue", "Song 1, Artist 1"},
			check: func(t *testing.T, _ *queueRig, tr access.Tree, _ []string) {
				if !mustFind(t, tr, ui.RoleToggleButton, "AutoPlay").Has(access.Checked) {
					t.Error("AutoPlay is on and its button is not checked")
				}
				if h := mustFind(t, tr, ui.RoleHeading, "AutoPlay"); h.Level != 3 {
					t.Errorf("AutoPlay's heading is level %d", h.Level)
				}
				list := mustFind(t, tr, ui.RoleList, "AutoPlay")
				for _, c := range tr.Children(list) {
					if c.Role != ui.RoleListItem {
						t.Errorf("AutoPlay's list holds a %v", c)
					}
				}
				mustFind(t, tr, ui.RoleList, "Queue")
			},
		},
		{
			name:  "empty",
			seed:  func(r *queueRig) { r.seed(0, -1) },
			order: []string{"Toggle queue", "AutoPlay", "Close queue"},
			check: func(t *testing.T, _ *queueRig, tr access.Tree, _ []string) {
				if _, ok := tr.Find(ui.RoleText, "The queue is empty"); !ok {
					t.Errorf("an empty queue does not say so:\n%s", tr.Dump())
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newQueueRig(t, tc.inView)
			tc.seed(r)
			tr, stops := audit(t, r.tt.Tester)
			inOrder(t, stops, tc.order...)
			tc.check(t, r, tr, stops)
		})
	}
}

// place is a list item's place in its list, as assistive technology reads it.
type place struct{ pos, size int }

func placeOf(t *testing.T, tr access.Tree, label string) place {
	t.Helper()
	n := mustFind(t, tr, ui.RoleListItem, label)
	return place{n.PosInSet, n.SetSize}
}

func TestQueueRowsSayTheirPlaceInTheirList(t *testing.T) {
	t.Run("the queue", func(t *testing.T) {
		r := newQueueRig(t, false)
		r.seed(4, 0)
		tr := access.Read(t, r.tt.Tester)
		for label, want := range map[string]place{
			"Song 1, Artist 1":          {1, 4},
			"Up next: Song 2, Artist 2": {2, 4},
			"Song 3, Artist 3":          {3, 4},
			"Song 4, Artist 4":          {4, 4},
		} {
			if got := placeOf(t, tr, label); got != want {
				t.Errorf("%q is %d of %d, want %d of %d", label, got.pos, got.size, want.pos, want.size)
			}
		}
	})
	t.Run("the station's part is a list of its own", func(t *testing.T) {
		r := newQueueRig(t, false)
		r.seed(6, 0, autoPlayFrom(3))
		tr := access.Read(t, r.tt.Tester)
		for label, want := range map[string]place{
			"Song 1, Artist 1":          {1, 3},
			"Up next: Song 2, Artist 2": {2, 3},
			"Song 3, Artist 3":          {3, 3},
			"Song 4, Artist 4":          {1, 3},
			"Song 6, Artist 6":          {3, 3},
		} {
			if got := placeOf(t, tr, label); got != want {
				t.Errorf("%q is %d of %d, want %d of %d", label, got.pos, got.size, want.pos, want.size)
			}
		}
	})
	t.Run("a long queue builds only the rows in view", func(t *testing.T) {
		r := newQueueRig(t, false)
		r.seed(500, 0)
		x, y := r.at(t, "Song 1, Artist 1")
		r.tt.Scroll(x, y, 0, 5200)
		r.tt.Frame()
		r.tt.Frame()
		top := int(r.p.queue.lists[0].pos.Y / queueRowH)
		if top < 50 {
			t.Fatalf("the list scrolled to %v", r.p.queue.lists[0].pos.Y)
		}
		n := top + 3
		label := fmt.Sprintf("Song %d, Artist %d", n, n)
		if got, want := placeOf(t, access.Read(t, r.tt.Tester), label), (place{n, 500}); got != want {
			t.Errorf("%q is %d of %d, want %d of %d", label, got.pos, got.size, want.pos, want.size)
		}
	})
}

func TestRowActionsAreTheSameOnTheTreeWhileTheRowIsHot(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(4, 0)
	rest := access.Read(t, r.tt.Tester)
	x, y := r.at(t, "Song 3, Artist 3")
	r.tt.Move(x, y)
	r.tt.Frame()
	r.tt.Frame()
	hot := access.Read(t, r.tt.Tester)
	for _, l := range []string{"More for Song 3", "More for Song 2", "More for Song 4"} {
		if n := len(hot.FindAll(ui.RoleMenuButton, l)); n != 1 {
			t.Errorf("%d %q while the row is hot, want 1", n, l)
		}
	}
	// A hot row shows its buttons where the duration was: only that text goes.
	for _, role := range []ui.Role{ui.RoleMenuButton, ui.RoleButton} {
		if a, b := len(rest.Roles(role)), len(hot.Roles(role)); a != b {
			t.Errorf("a hot row changes the %v nodes from %d to %d", role, a, b)
		}
	}
	access.NoNestedControls(t, r.tt.Tester)
	// The pointer's press on the hot button still opens the menu, and the
	// button says so.
	if err := r.tt.Click("More for Song 3"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if n := mustFind(t, tr, ui.RoleMenuButton, "More for Song 3"); !n.Has(access.Expanded) || n.HasPopup != ui.PopupMenu {
		t.Errorf("the open menu does not show on its button:\n%s", tr.Dump())
	}
	if len(tr.Labelled(ui.RoleMenuItem, "Play next")) == 0 {
		t.Errorf("the row menu has no Play next item:\n%s", tr.Dump())
	}
}

func TestRowMenuFromTheKeyboardClosesBackOnItsRow(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(4, 0)
	r.focusRow(t, "Song 3, Artist 3")
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Frame()
	r.tt.Frame()
	tr := access.Read(t, r.tt.Tester)
	if len(tr.Labelled(ui.RoleMenuItem, "Play")) == 0 {
		t.Fatalf("Shift+F10 did not open the row menu:\n%s", tr.Dump())
	}
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Frame()
	r.tt.Frame()
	if !r.p.ui.queueOpen {
		t.Error("Escape in the row menu closed the queue as well")
	}
	if f := access.Read(t, r.tt.Tester).Focus; f == nil || f.Label != "Song 3, Artist 3" {
		t.Errorf("the focus did not return to the row: %v", f)
	}
}

func TestQueueEditsAreAnnouncedWithTheSongsNewPlace(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(6, 0)
	x, y := r.at(t, "Song 4, Artist 4")
	r.tt.Move(x, y)
	r.tt.Frame()
	if err := r.tt.Click("More for Song 4"); err != nil {
		t.Fatal(err)
	}
	r.tt.Frame()
	if err := r.tt.Click("Play next"); err != nil {
		t.Fatal(err)
	}
	if got := r.fake.wait(t, 1); got[0] != "movenext 3" {
		t.Fatalf("calls %v", got)
	}
	r.tt.Announcements()
	// Go's snapshot with Song 4 right after the playing one.
	items := slices.Clone(r.h.Env.Play.Queue().Items)
	items = slices.Insert(slices.Delete(items, 3, 4), 1, r.h.Env.Play.Queue().Items[3])
	r.h.Env.Play.Seed(r.h.Env.Play.State(), services.QueueState{Index: 0, Items: items})
	var heard []string
	for range 4 {
		r.tt.Frame()
		heard = append(heard, r.tt.Announcements()...)
	}
	if !slices.Equal(heard, []string{"Song 4: moved to position 1 of 5"}) {
		t.Errorf("Play next announced %q", heard)
	}
	// A move that fails is forgotten: Go's refusal comes back to the main
	// thread (Drain runs it), so a later snapshot that happens to put the
	// song elsewhere is not announced as that move.
	r.fake.fail = errors.New("boom")
	r.focusRow(t, "Song 3, Artist 3")
	r.tt.Announcements()
	r.tt.Key(ui.Alt, ui.KeyDown)
	r.fake.wait(t, 1)
	if r.p.queue.moved == nil {
		t.Fatal("the move was not pending before Go answered")
	}
	// The failure is posted to the main thread after the call returns.
	for i := 0; r.p.queue.moved != nil; i++ {
		if i > 1_000_000 {
			t.Fatal("a move that failed is still waiting to be announced")
		}
		r.h.Main.Drain()
		runtime.Gosched()
	}
	r.tt.Frame()
	items = slices.Clone(r.h.Env.Play.Queue().Items)
	i3 := slices.IndexFunc(items, func(it services.QueueItem) bool { return it.Name == "Song 3" })
	items[i3], items[i3+1] = items[i3+1], items[i3] // as if another window moved it
	r.h.Env.Play.Seed(r.h.Env.Play.State(), services.QueueState{Index: 0, Items: items})
	for range 4 {
		r.tt.Frame()
	}
	for _, a := range r.tt.Announcements() {
		if strings.Contains(a, "moved to position") {
			t.Errorf("a failed move was announced: %q", a)
		}
	}
}

func TestAutoPlayWhereItDoesNotApplyKeepsTheFocusAndSaysWhy(t *testing.T) {
	r := newQueueRig(t, false)
	r.seed(3, 0, func(s *services.PlaybackState, q *services.QueueState) {
		s.Current.Live = true
		s.Controls.AutoPlay = false
	})
	n := tabTo(t, r.tt.Tester, "AutoPlay")
	if !n.Has(access.Disabled) || !n.Has(access.Focusable) || n.Description != liveReason || n.Has(access.Checked) {
		t.Errorf("AutoPlay on a live station: %+v", n)
	}
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Frame()
	if got := r.fake.snapshot(); len(got) != 0 {
		t.Errorf("a refused AutoPlay sent %v", got)
	}
	if !r.tt.Focused("AutoPlay") {
		t.Error("AutoPlay lost the focus")
	}
}

// A screen reader reads every change of the focused slider, and the position
// of the song moves four times a second: while Seek has the focus and nobody
// moves it, assistive technology keeps the position it had when the focus
// came, and the keys still move it.
func TestFocusedSeekHoldsTheValueAssistiveTechnologyReads(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	tabTo(t, r.tt.Tester, "Seek")
	held := mustFind(t, access.Read(t, r.tt.Tester), ui.RoleSlider, "Seek")
	if held.Now != 30 || held.Description != "0:30 of 3:00" {
		t.Fatalf("Seek reads %v and %q on arrival", held.Now, held.Description)
	}
	for range 3 {
		r.h.Clock.Advance(4 * time.Second)
		r.tt.Advance(4 * time.Second)
	}
	if got := mustFind(t, access.Read(t, r.tt.Tester), ui.RoleSlider, "Seek"); got.Now != 30 || got.Description != "0:30 of 3:00" {
		t.Errorf("a focused Seek reads %v and %q while the song plays on", got.Now, got.Description)
	}
	if !r.tt.HasText("0:42") {
		t.Errorf("the time beside the bar does not move: %q", r.tt.Texts())
	}
	// Leaving it, Seek reads the position again.
	r.tt.Key(0, ui.KeyTab)
	r.tt.Advance(time.Second)
	if got := mustFind(t, access.Read(t, r.tt.Tester), ui.RoleSlider, "Seek"); got.Now < 42 {
		t.Errorf("Seek still reads %v once the focus is gone", got.Now)
	}
}
