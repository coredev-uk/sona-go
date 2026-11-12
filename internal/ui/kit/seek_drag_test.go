package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestSeekDragShowsTheDraggedPositionAndCommitsOnceOnRelease(t *testing.T) {
	var d SeekDrag
	v := 20.0
	var commits []time.Duration
	live := 10 * time.Second
	var shown time.Duration
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(40).Children(func() {
			shown = d.Pos(live)
			sl := Slider(c.Key("s"), &v, 0, 100, SliderOpts{A11yLabel: "Seek", Width: 200})
			if to, ok := d.Update(sl, v); ok {
				commits = append(commits, to)
			}
		})
	}, 400, 300)
	tt.Frame()
	if shown != live || d.Active() {
		t.Fatalf("idle: shown %v active %v, want the live position", shown, d.Active())
	}
	tt.Press(60, 48)
	tt.Move(120, 48)
	tt.Frame()
	if !d.Active() || shown == live {
		t.Errorf("while dragging: active %v shown %v, want the dragged position", d.Active(), shown)
	}
	if len(commits) != 0 {
		t.Fatalf("%d commits while the pointer is held", len(commits))
	}
	tt.Release(120, 48)
	tt.Frame()
	tt.Frame()
	if len(commits) != 1 {
		t.Fatalf("%d commits after the release, want 1", len(commits))
	}
	if want := time.Duration(v * float64(time.Second)); commits[0] != want {
		t.Errorf("committed %v, want the slider's %v", commits[0], want)
	}
	if d.Active() {
		t.Error("still dragging after the commit")
	}
}

func TestSeekDragCommitsAKeyboardMoveOnce(t *testing.T) {
	var d SeekDrag
	v := 20.0
	var commits []time.Duration
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(40).Children(func() {
			sl := Slider(c.Key("s"), &v, 0, 100, SliderOpts{A11yLabel: "Seek", Step: 5, Width: 200})
			if to, ok := d.Update(sl, v); ok {
				commits = append(commits, to)
			}
		})
	}, 400, 300)
	tt.Frame()
	tt.SetFocused(true)
	tt.Key(0, ui.KeyTab) // onto the slider
	tt.Key(0, ui.KeyRight)
	tt.Frame()
	tt.Frame()
	if len(commits) != 1 {
		t.Fatalf("%d commits after one arrow key, want 1", len(commits))
	}
	if want := time.Duration(v * float64(time.Second)); v <= 20 || commits[0] != want {
		t.Errorf("committed %v with the slider at %v, want the moved value", commits[0], v)
	}
	tt.Frame()
	if len(commits) != 1 || d.Active() {
		t.Errorf("the commit repeated or the drag stayed active: %d commits, active %v", len(commits), d.Active())
	}
}
