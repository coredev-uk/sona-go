package player

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// bind gives the rig's Env the keys a shortcut answers to now.
func bind(r *rig, keys map[string][]string) {
	r.env.Shortcuts = func() []module.BoundShortcut {
		var out []module.BoundShortcut
		for id, k := range keys {
			out = append(out, module.BoundShortcut{Shortcut: module.Shortcut{ID: id}, Keys: k})
		}
		return out
	}
}

// hover rests the pointer on the named button until its tip shows.
func hover(t *testing.T, r *rig, name string) {
	t.Helper()
	n, ok := r.tt.Find(name)
	if !ok {
		t.Fatalf("no %q: %q", name, r.tt.Texts())
	}
	r.tt.Move(n.X+n.W/2, n.Y+n.H/2)
	r.tt.Advance(time.Second)
}

func TestBarTooltipsShowTheBoundKeys(t *testing.T) {
	r := newRig(t, true)
	bind(r, map[string][]string{
		"player.toggle": {"Space"}, "player.next": {"Ctrl+ArrowRight"}, "player.queue": {"Q"},
		"player.lyrics": {}, "player.mute": {"Ctrl+M"},
	})
	r.seed()

	hover(t, r, "Pause")
	if !r.tt.HasText("Pause") || !r.tt.HasText("Space") {
		t.Errorf("the play button's tip lacks its key: %q", r.tt.Texts())
	}
	tr := access.Read(t, r.tt.Tester)
	if n := mustFind(t, tr, ui.RoleButton, "Pause"); n.Description != "Space" {
		t.Errorf("the play button's description is %q", n.Description)
	}

	hover(t, r, "Next")
	if !r.tt.HasText("Ctrl") || !r.tt.HasText("→") {
		t.Errorf("Next's tip lacks its chord: %q", r.tt.Texts())
	}
	hover(t, r, "Queue")
	if !r.tt.HasText("Q") {
		t.Errorf("Queue's tip lacks its key: %q", r.tt.Texts())
	}
	// A shortcut the user unbound has no keys to show, and its tip is the name.
	hover(t, r, "Lyrics")
	tr = access.Read(t, r.tt.Tester)
	if n := mustFind(t, tr, ui.RoleToggleButton, "Lyrics"); n.Description != "" {
		t.Errorf("an unbound button has the description %q", n.Description)
	}
}

func TestBarTooltipFollowsARebinding(t *testing.T) {
	r := newRig(t, true)
	bind(r, map[string][]string{"player.toggle": {"Space"}})
	r.seed()
	bind(r, map[string][]string{"player.toggle": {"Ctrl+Shift+P"}})
	hover(t, r, "Pause")
	if !r.tt.HasText("Shift") || !r.tt.HasText("P") || r.tt.HasText("Space") {
		t.Errorf("the tip kept the old key: %q", r.tt.Texts())
	}
}

// bindNP gives an npRig's Env the keys of the shortcuts the lyrics tools name.
func bindNP(r *npRig, keys map[string][]string) {
	r.h.Env.Shortcuts = func() []module.BoundShortcut {
		var out []module.BoundShortcut
		for id, k := range keys {
			out = append(out, module.BoundShortcut{Shortcut: module.Shortcut{ID: id}, Keys: k})
		}
		return out
	}
}

func hoverNP(t *testing.T, r *npRig, name string) {
	t.Helper()
	n, ok := r.tt.Find(name)
	if !ok {
		t.Fatalf("no %q: %q", name, r.tt.Texts())
	}
	r.tt.Move(n.X+n.W/2, n.Y+n.H/2)
	r.tt.Advance(time.Second)
}

func TestLyricsToolsShowTheirKeysInTheirTips(t *testing.T) {
	r := newNPRig(t)
	bindNP(r, map[string][]string{
		"player.lyrics-earlier": {"["}, "player.lyrics-later": {"]"}, "player.lyrics-reset": {"\\"},
		"player.lyrics-room": {"Shift+L"}, "player.close-overlay": {"Escape"},
	})
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(10, false)}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("tools", r.tt.Shows("Lyrics on time"))

	hoverNP(t, r, "Show lyrics 0.5 s earlier")
	if !r.tt.HasText("[") {
		t.Errorf("Earlier's tip lacks its key: %q", r.tt.Texts())
	}
	hoverNP(t, r, "Show lyrics 0.5 s later")
	if !r.tt.HasText("]") {
		t.Errorf("Later's tip lacks its key: %q", r.tt.Texts())
	}
	// The readout has a tip, with its key, only once the timing is off.
	if err := r.tt.Click("Show lyrics 0.5 s later"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	hoverNP(t, r, "Lyrics 0.5 s later")
	if !r.tt.HasText("Back to the original timing") || !r.tt.HasText("\\") {
		t.Errorf("the readout's tip lacks its key: %q", r.tt.Texts())
	}
	hoverNP(t, r, "Full screen")
	if !r.tt.HasText("Shift") {
		t.Errorf("Full screen's tip lacks its chord: %q", r.tt.Texts())
	}
	if err := r.tt.Click("Full screen"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the room", r.tt.Shows("Exit full screen"))
	hoverNP(t, r, "Exit full screen")
	if !r.tt.HasText("Esc") {
		t.Errorf("Exit full screen's tip lacks Esc: %q", r.tt.Texts())
	}
}

// --- Sleep timer commands, the live line, the order of the controls --------

func TestSleepTimerShortcutsAreCommandsForThePalette(t *testing.T) {
	r := newRig(t, true)
	byID := map[string]module.Shortcut{}
	for _, s := range r.p.shortcuts() {
		byID[s.ID] = s
	}
	want := map[string]struct {
		label, call string
	}{
		"player.sleep-30":    {"Sleep timer: stop in 30 minutes", "sleep after 30"},
		"player.sleep-60":    {"Sleep timer: stop in 1 hour", "sleep after 60"},
		"player.sleep-track": {"Sleep timer: stop after this song", "sleep track 0"},
		"player.sleep-queue": {"Sleep timer: stop after the queue", "sleep queue 0"},
		"player.sleep-off":   {"Turn off the sleep timer", "sleep  0"},
	}
	for id, w := range want {
		s, ok := byID[id]
		if !ok {
			t.Fatalf("no %s", id)
		}
		if s.Label() != w.label || s.Group != "playback" || len(s.Keys) != 0 || s.NoPalette || s.Run == nil || s.Keywords() == "" {
			t.Errorf("%s: %q, group %q, keys %v", id, s.Label(), s.Group, s.Keys)
		}
		if s.When() {
			t.Errorf("%s is offered with nothing playing", id)
		}
	}
	r.seed()
	for _, id := range []string{"player.sleep-30", "player.sleep-60", "player.sleep-track", "player.sleep-queue"} {
		if !byID[id].When() {
			t.Errorf("%s is not offered while a song plays", id)
		}
	}
	if byID["player.sleep-off"].When() {
		t.Error("turning the timer off is offered with no timer")
	}
	r.seed(func(s *services.PlaybackState) { s.Sleep = &services.SleepTimer{Mode: "track"} })
	if !byID["player.sleep-off"].When() {
		t.Error("turning the timer off is not offered while a timer runs")
	}
	var calls []string
	for id, w := range want {
		byID[id].Run()
		calls = append(calls, w.call)
	}
	got := r.fake.wait(t, len(calls))
	for _, c := range calls {
		if !slices.Contains(got, c) {
			t.Errorf("no %q call in %v", c, got)
		}
	}
}

func TestTheLiveLineFallsBackToTheProgrammeOnAir(t *testing.T) {
	r := newRig(t, true)
	now := r.h.Clock.Now()
	end := now.Add(40 * time.Minute)
	r.p.station = func(_ context.Context, id string) (services.StationDetail, error) {
		return services.StationDetail{Schedule: []services.StationEvent{
			{ID: "e0", Title: "Earlier", StartMs: now.Add(-2 * time.Hour).UnixMilli(), EndMs: now.Add(-20 * time.Minute).UnixMilli()},
			{ID: "e1", Title: "Morning Show", StartMs: now.Add(-20 * time.Minute).UnixMilli(), EndMs: end.UnixMilli()},
		}}, nil
	}
	r.seed(func(s *services.PlaybackState) { s.Current = &services.QueueItem{ID: "ra.1", Name: "Station", Live: true} })
	want := "Morning Show · " + i18n.T("until {until}", "until", i18n.FormatClock(end))
	r.tt.Until("the programme", func() bool { r.tt.Frame(); return r.tt.HasText(want) })
	if !r.tt.HasText("Live") {
		t.Errorf("no Live marker: %q", r.tt.Texts())
	}
	// What the station says is on air wins over the schedule.
	r.seed(func(s *services.PlaybackState) {
		s.Current = &services.QueueItem{ID: "ra.1", Name: "Station", Live: true}
		s.OnAir = &services.OnAir{Title: "A Song", Artist: "An Artist"}
	})
	if !r.tt.HasText("An Artist – A Song") || r.tt.HasText(want) {
		t.Errorf("the song on air does not win: %q", r.tt.Texts())
	}
}

func TestMiniPlayerAndEqualiserFollowTheQueueButton(t *testing.T) {
	r := newRig(t, true)
	r.env.PlayerControls.Add(10, func(c *ui.Context) { ui.Button(c, "Private") })
	r.env.PlayerControls.Add(module.ControlsAfterQueue+20, func(c *ui.Context) { ui.Button(c, "Equaliser") })
	r.env.PlayerControls.Add(module.ControlsAfterQueue+10, func(c *ui.Context) { ui.Button(c, "Mini player") })
	r.seed()
	var xs []float32
	for _, name := range []string{"Private", "Sleep timer", "Lyrics", "Queue", "Mini player", "Equaliser", "Mute"} {
		n, ok := r.tt.Find(name)
		if !ok {
			t.Fatalf("no %q: %q", name, r.tt.Texts())
		}
		xs = append(xs, n.X)
	}
	if !slices.IsSorted(xs) {
		t.Errorf("the right group is out of order (Private, Sleep, Lyrics, Queue, Mini player, Equaliser, Mute): %v", xs)
	}
}
