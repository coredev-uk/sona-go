package player

import (
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The sung line is the current item of the lines.
func TestTheSungLyricLineIsTheCurrentItem(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(6, false)}, nil)
	r.play(5000) // inside the second line
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 1 sung words"))
	tr := access.Read(t, r.tt.Tester)
	for i := range 6 {
		label := "Line " + itoa(i) + " sung words"
		n, ok := tr.Find(ui.RoleButton, label)
		if !ok {
			t.Fatalf("no button %q", label)
		}
		want := ui.CurrentNone
		if i == 1 {
			want = ui.CurrentItem
		}
		if n.Current != want || n.Description != "" {
			t.Errorf("%q is current %v and described %q, want current %v and no description", label, n.Current, n.Description, want)
		}
	}
}

// A nudge is said when it changes, whoever has the focus, and not when the
// song's readout is first shown. With a screen reader on it is said once:
// the readout is a button, not a status MyGo v0.9.0 would read as well.
func TestTheLyricsTimingOffsetIsAnnouncedWhenItChanges(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(10, false)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("tools", r.tt.Shows("Lyrics on time"))
	r.tt.UseScreenReader()
	r.tt.Announcements()
	r.tt.Frame()
	if got := r.tt.Announcements(); len(got) != 0 {
		t.Fatalf("announced without a change: %q", got)
	}
	click := func(s string) {
		t.Helper()
		if err := r.tt.Click(s); err != nil {
			t.Fatal(err)
		}
		r.tt.Drain()
	}
	click("Show lyrics 0.5 s later")
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Lyrics 0.5 s later"}) {
		t.Errorf("announced %q after a nudge", got)
	}
	click("Lyrics 0.5 s later")
	if got := r.tt.Announcements(); !slices.Equal(got, []string{"Lyrics on time"}) {
		t.Errorf("announced %q after the reset", got)
	}
}

// With a tab focused, a pane changed from outside (the Queue button) takes
// the focus to its tab.
func TestTabFocusFollowsAPaneChangedFromOutside(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(6, false)}, nil)
	r.play(2000)
	r.open(paneLyrics)
	r.tt.Until("tabs", r.tt.Shows("Up next"))
	r.tt.SetFocused(true)
	for range 60 {
		if f := access.Read(t, r.tt.Tester).Focus; f != nil && f.Label == "Lyrics" {
			break
		}
		r.tt.Key(0, ui.KeyTab)
	}
	if f := access.Read(t, r.tt.Tester).Focus; f == nil || f.Label != "Lyrics" {
		t.Fatalf("focus %v, want the Lyrics tab", f)
	}
	r.p.toggleQueueView()
	r.tt.Drain()
	if f := access.Read(t, r.tt.Tester).Focus; f == nil || f.Label != "Up next" {
		t.Errorf("focus %v after the pane changed to Up next, want its tab", f)
	}
	r.p.ui.setPane(paneLyrics)
	r.tt.Drain()
	if f := access.Read(t, r.tt.Tester).Focus; f == nil || f.Label != "Lyrics" {
		t.Errorf("focus %v after the pane changed back, want the Lyrics tab", f)
	}
}

// Now Playing is a region named for what it is, which holds its tabs and
// panes, and is not nested in another landmark of the same name.
func TestNowPlayingIsARegion(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(3, false)}, nil)
	r.play(1000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 1 sung words"))
	tr := access.Read(t, r.tt.Tester)
	region, ok := tr.Find(ui.RoleRegion, "Now playing")
	if !ok {
		t.Fatalf("no region named Now playing:\n%s", tr.Dump())
	}
	if tab, ok := tr.Find(ui.RoleTab, "Lyrics"); !ok || !tr.Within(tab, ui.RoleRegion) {
		t.Errorf("the Lyrics tab is not in the region: %+v", tab)
	}
	for _, n := range tr.Roles(ui.RoleRegion) {
		if n.Label == region.Label && n.Index != region.Index {
			t.Errorf("a second region named %q", n.Label)
		}
	}
}
