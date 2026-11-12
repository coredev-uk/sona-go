package player

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
)

func TestMinutesLeftRoundsUp(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	for ms, want := range map[int64]int{
		-5_000: 0, 0: 0, 1: 1, 59_999: 1, 60_000: 1, 60_001: 2, 14*60_000 + 30_000: 15,
	} {
		if got := minutesLeft(now.UnixMilli()+ms, now); got != want {
			t.Errorf("%d ms left: %d minutes, want %d", ms, got, want)
		}
	}
}

// openSleepMenu seeds a playing song and opens the sleep timer's menu.
func openSleepMenu(t *testing.T, mutate ...func(*services.PlaybackState)) *rig {
	t.Helper()
	r := newRig(t, true)
	r.seed(mutate...)
	// The button is named "Sleep timer", or what the running timer says.
	for _, name := range r.tt.Texts() {
		if strings.HasPrefix(name, "Sleep timer") {
			r.tt.MustClick(name)
			break
		}
	}
	r.tt.Frame()
	return r
}

func TestSleepMenuChoicesSetTheTimer(t *testing.T) {
	for _, tc := range []struct{ choice, call string }{
		{"In 15 minutes", "sleep after 15"},
		{"In 30 minutes", "sleep after 30"},
		{"In 45 minutes", "sleep after 45"},
		{"In 1 hour", "sleep after 60"},
		{"In 2 hours", "sleep after 120"},
		{"At the end of this song", "sleep track 0"},
		{"At the end of the queue", "sleep queue 0"},
	} {
		r := openSleepMenu(t)
		if !r.tt.HasText("Stop playing") {
			t.Fatalf("the menu is not open: %q", r.tt.Texts())
		}
		if err := r.tt.Click(tc.choice); err != nil {
			t.Fatalf("%s: %v", tc.choice, err)
		}
		r.tt.Frame()
		r.tt.Frame()
		got := r.fake.Wait(t, 1)
		if last := got[len(got)-1]; last != tc.call {
			t.Errorf("%s sent %q, want %q", tc.choice, last, tc.call)
		}
	}
}

func TestSleepMenuWhileATimerRuns(t *testing.T) {
	r := openSleepMenu(t, func(s *services.PlaybackState) {
		s.Sleep = &services.SleepTimer{Mode: "track"}
	})
	for _, w := range []string{"Pauses when this song ends, fading out over the last 10 seconds.", "Turn off sleep timer"} {
		if !r.tt.HasText(w) {
			t.Errorf("menu lacks %q: %q", w, r.tt.Texts())
		}
	}
	// The menu is taller than the rig's window: End moves to its last row.
	r.tt.Key(0, ui.KeyEnd)
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Frame()
	r.tt.Frame()
	if got := r.fake.Wait(t, 1); got[len(got)-1] != "sleep  0" {
		t.Errorf("sent %q", got)
	}
}

func TestSleepButtonNamesTheTimerAndCountsDown(t *testing.T) {
	r := newRig(t, true)
	r.seed()
	if _, ok := r.tt.Find("Sleep timer"); !ok {
		t.Fatalf("no sleep timer button: %q", r.tt.Texts())
	}
	endsAt := r.h.Clock.Now().Add(150 * time.Second).UnixMilli()
	r.seed(func(s *services.PlaybackState) { s.Sleep = &services.SleepTimer{Mode: "after", EndsAt: endsAt} })
	if !r.tt.HasText("3m") {
		t.Fatalf("2:30 left should show 3m: %q", r.tt.Texts())
	}
	if _, ok := r.tt.Find("Sleep timer: 3m left, pauses at " + i18n.FormatClock(time.UnixMilli(endsAt))); !ok {
		t.Errorf("label: %q", r.tt.Texts())
	}
	r.h.Clock.Advance(90 * time.Second)
	r.tt.Advance(time.Minute)
	if !r.tt.HasText("1m") {
		t.Errorf("one minute on the countdown should show 1m: %q", r.tt.Texts())
	}

	r.seed(func(s *services.PlaybackState) { s.Sleep = &services.SleepTimer{Mode: "track"} })
	if _, ok := r.tt.Find("Sleep timer: pauses after this song"); !ok {
		t.Errorf("track label: %q", r.tt.Texts())
	}
	r.seed(func(s *services.PlaybackState) { s.Sleep = &services.SleepTimer{Mode: "queue"} })
	if _, ok := r.tt.Find("Sleep timer: pauses after the queue"); !ok {
		t.Errorf("queue label: %q", r.tt.Texts())
	}
}

func TestSleepButtonIsOffWithNothingPlaying(t *testing.T) {
	r := newRig(t, true)
	_ = r.tt.Click("Sleep timer")
	r.tt.Frame()
	if r.tt.HasText("Stop playing") {
		t.Error("the menu opened with nothing playing")
	}
	r.tt.Frame()
}
