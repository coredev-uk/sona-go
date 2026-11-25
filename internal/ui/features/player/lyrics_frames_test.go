package player

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// asked runs one second of a 250 Hz display's steps (4 ms) and returns what
// the drawings asked MyGo for in the frames painted (kit.FrameProbe).
func (r *npRig) asked(probe *kit.FrameProbe) []float32 {
	probe.Asked = nil
	for range 250 {
		r.tick(4 * time.Millisecond)
	}
	return probe.Asked
}

func TestTheDotsAskForAtMost60FramesASecondInTheStageAndTheSheet(t *testing.T) {
	check := func(t *testing.T, got []float32) {
		t.Helper()
		if len(got) == 0 {
			t.Fatal("the dots asked for no frames")
		}
		if len(got) > 61 {
			t.Errorf("the dots were painted %d times in a second, want at most about 60", len(got))
		}
		for _, fps := range got {
			if fps <= 0 || fps > 60 {
				t.Fatalf("the dots asked for %v frames a second, want at most 60 (0 is the display's rate)", fps)
			}
		}
	}
	t.Run("full-screen lyrics", func(t *testing.T) {
		probe, stop := kit.ProbeFrames(true)
		defer stop()
		r := stageRig(t, gapLines(), 500, nil)
		check(t, r.asked(probe))
	})
	t.Run("the sheet", func(t *testing.T) {
		probe, stop := kit.ProbeFrames(true)
		defer stop()
		r := newNPRig(t)
		lines := timedLines(3, false)
		for i := range lines {
			lines[i].BeginMs += 9000
			lines[i].EndMs += 9000
		}
		r.lyr.set(services.Lyrics{Timing: "Line", Lines: lines}, nil)
		r.play(1000)
		r.open("")
		r.tt.Until("lyrics", r.tt.Shows("Line 0 sung words"))
		r.settle()
		check(t, r.asked(probe))
	})
}

// The sweep of a held word is what the eye follows: it keeps the display's
// rate while the dots, in the same window, ask for 60.
func TestTheSweepAsksForEveryFrameOfTheDisplay(t *testing.T) {
	probe, stop := kit.ProbeFrames(true)
	defer stop()
	r := newNPRigWith(t, nil, func(r *npRig) {
		r.width, r.height = 1920, 1080
		r.animate = true
	})
	lines := duetLines()
	r.lyr.set(services.Lyrics{Timing: "Word", Lines: lines}, nil)
	r.play(lines[1].BeginMs + 300)
	r.p.ui.openRoom()
	r.tt.Until("the room and its lines", r.tt.Shows("Full-Screen Lyrics", "Out past the breakwater"))
	r.settle()
	got := r.asked(probe)
	display := 0
	for _, fps := range got {
		if fps == 0 {
			display++
		}
	}
	if display == 0 || display < len(got)/2 {
		t.Errorf("%d of %d frame requests were for the display's rate: the sweep must ask for it", display, len(got))
	}
}
