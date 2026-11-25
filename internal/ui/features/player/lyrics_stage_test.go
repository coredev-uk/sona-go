package player

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/module"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// heldLines are lines sung one after another, 600 ms apart, each held for the
// given time: "Held 0", "Held 1", …
func heldLines(durs ...int64) []services.LyricLine {
	out := make([]services.LyricLine, len(durs))
	at := int64(1000)
	for i, d := range durs {
		out[i] = services.LyricLine{BeginMs: at, EndMs: at + d, Text: fmt.Sprintf("Held %d", i), Agent: "v1", AgentType: "person"}
		at += d + 600
	}
	return out
}

// stageRig is Full-Screen Lyrics open over a song with lines, at 1920×1080
// unless a mutate says otherwise. It returns once the lyrics have answered
// and the stage has built its first column: the view shows its own label
// (with flat bars for the lines) before that, and the tests read the stage's
// lyrics, steps and column.
func stageRig(t *testing.T, lines []services.LyricLine, posMs int64, opts []uitest.Option, mutate ...func(*npRig)) *npRig {
	t.Helper()
	r := newNPRigWith(t, opts, append([]func(*npRig){func(r *npRig) { r.width, r.height = 1920, 1080 }}, mutate...)...)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: lines}, nil)
	r.play(posMs)
	r.p.ui.openRoom()
	r.tt.Until("the first column", func() bool {
		st := r.p.np.stage
		return r.tt.HasText("Full-Screen Lyrics") && st != nil && st.lyr != nil && st.anchor >= 0
	})
	r.settle()
	return r
}

func TestEveryTimedLineIsSetInOneSizeThatScalesWithTheWindow(t *testing.T) {
	for _, c := range []struct {
		w, h int
		want float32
	}{
		{1920, 1080, 72},
		{3840, 2160, 144}, // scales with the window's height
		{1280, 540, 36},
		{640, 270, stageFloorDIP}, // and has a floor in a small window
	} {
		t.Run(fmt.Sprintf("%dx%d", c.w, c.h), func(t *testing.T) {
			// Lines held for very different times: how long a line is held
			// does not set its size.
			lines := heldLines(1200, 1800, 2400, 3000, 4200, 5600, 7000, 9000, 12000, 14000)
			r := stageRig(t, lines, 1000+1200+600+1800+600+2400+600+500, nil, func(r *npRig) { r.width, r.height = c.w, c.h })
			st := r.p.np.stage
			for i := st.anchor - stageReach; i <= st.anchor+stageReach; i++ {
				if i < 0 || i >= len(lines) {
					continue
				}
				if b := st.block(i); b.size != c.want || b.step != 0 {
					t.Errorf("line %d (held %d ms) is %v at step %d, want %v at step 0", i, lines[i].EndMs-lines[i].BeginMs, b.size, b.step, c.want)
				}
			}
		})
	}
}

func TestALongLineWrapsToTwoLinesAtMostBeforeItsSizeStepsDown(t *testing.T) {
	lines := heldLines(1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 20000, 1500)
	lines[9].Text = strings.Repeat("the tide came back slower than we thought ", 3)
	r := stageRig(t, lines, lines[9].BeginMs+500, nil)
	st := r.p.np.stage
	b := st.block(9)
	if b.step == 0 {
		t.Errorf("a line that wraps to %d lines at the full size kept it", b.lines())
	}
	if b.lines() > stageMaxWrap && b.step < len(stageLadder)-1 {
		t.Errorf("wrapped to %d lines at step %d with sizes left to step down to", b.lines(), b.step)
	}
	if short := st.block(8); short.size <= b.size || short.step != 0 {
		t.Errorf("a short line is %v at step %d beside the long one's %v", short.size, short.step, b.size)
	}
}

func TestPlainLyricsAreSetInOneSize(t *testing.T) {
	r := newNPRigWith(t, nil, func(r *npRig) { r.width, r.height = 1920, 1080 })
	lines := make([]services.LyricLine, 8)
	for i := range lines {
		lines[i] = services.LyricLine{Text: fmt.Sprintf("Plain %d %s", i, strings.Repeat("x", i*3))}
	}
	r.lyr.set(services.Lyrics{Lines: lines}, nil)
	r.play(2000)
	r.p.ui.openRoom()
	r.tt.Until("the room", r.tt.Shows("Full-Screen Lyrics", "Plain 0 "))
	st := r.p.np.stage
	if st.synced {
		t.Fatal("plain lyrics came out synced")
	}
	for i := range 4 {
		if got := st.block(i).size; got != stagePlainDIP {
			t.Errorf("line %d is %v, want %v", i, got, float32(stagePlainDIP))
		}
	}
}

// openFrom: Shift+L, and the entry points the view had.
func TestFullScreenLyricsOpensFromShiftLAndTheOtherEntryPoints(t *testing.T) {
	r := newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(10, false)}, nil)
	r.play(2000)
	byID := map[string]module.Shortcut{}
	for _, s := range r.p.nowPlayingShortcuts() {
		byID[s.ID] = s
	}
	sc := byID["player.lyrics-room"]
	if sc.Label() != "Full-Screen Lyrics" || len(sc.Keys) != 1 || sc.Keys[0] != "Shift+L" {
		t.Errorf("the shortcut is %q on %v", sc.Label(), sc.Keys)
	}
	// Shift+L from the page.
	sc.Run()
	r.tt.Until("the view", r.tt.Shows("Full-Screen Lyrics", "Line 0 sung words"))
	if !r.p.roomOpen() {
		t.Error("Shift+L did not open it")
	}
	// And again to leave.
	sc.Run()
	if r.p.ui.overlay == overlayRoom {
		t.Error("Shift+L did not leave")
	}
	// Now Playing's button (its entry point stays; the tip carries the name).
	r.open("")
	r.tt.Until("tools", r.tt.Shows("Full screen"))
	r.tt.Hover("Full screen")
	r.tt.Until("the tip", r.tt.Shows("Full-Screen Lyrics"))
	r.tt.MustClick("Full screen")
	r.tt.Until("the view", r.tt.Shows("Full-Screen Lyrics", "Line 0 sung words"))
	if r.p.ui.overlay != overlayRoom {
		t.Errorf("the button opened %v", r.p.ui.overlay)
	}
}

// mid is where line label's middle is on the stage now.
func (r *npRig) mid(t *testing.T, label string) float32 {
	t.Helper()
	box, ok := r.tt.Find(label)
	if !ok {
		t.Fatalf("%q is not on screen: %q", label, r.tt.Texts())
	}
	return box.Y + box.H/2
}

// Each line glides to its place on its own: the sung line goes first and the
// line after it follows a step later (cascadeStep), then the ones beyond.
func TestEachLineClimbsOnItsOwnWhenTheSungLineAdvances(t *testing.T) {
	r := stageRig(t, heldLines(2500, 2500, 2500, 2500, 2500, 2500, 2500), 1500, nil, func(r *npRig) { r.animate = true })
	r.settle()
	r.advance(time.Second)
	focus := float32(r.height) * stageFocus
	if got := r.mid(t, "Held 0"); math.Abs(float64(got-focus)) > 6 {
		t.Fatalf("the sung line is at %v, want the optical centre %v", got, focus)
	}
	was1, was2 := r.mid(t, "Held 1"), r.mid(t, "Held 2") // coming
	// The next line begins: the lines climb to it over 600 ms, the second
	// one after the first.
	r.play(1000 + 2500 + 600 + 400)
	r.tt.Drain() // the view builds and the climb starts
	r.advance(30 * time.Millisecond)
	early1, early2 := r.mid(t, "Held 1"), r.mid(t, "Held 2")
	if !(early1 < was1 && early1 > focus+6) {
		t.Errorf("30 ms into the climb Held 1 is at %v: not between where it was (%v) and the centre (%v)", early1, was1, focus)
	}
	if math.Abs(float64(early2-was2)) > 0.5 {
		t.Errorf("30 ms into the climb Held 2 has moved from %v to %v: it follows a step (%v) after the sung line", was2, early2, cascadeStep)
	}
	r.advance(cascadeStep) // 90 ms: Held 2's turn has come
	mid1, mid2 := r.mid(t, "Held 1"), r.mid(t, "Held 2")
	if !(mid2 < was2-0.5 && mid2 > was2-(was1-mid1)) {
		t.Errorf("90 ms in, Held 2 is at %v (was %v) and Held 1 at %v (was %v): the follower did not start behind the leader", mid2, was2, mid1, was1)
	}
	r.advance(150 * time.Millisecond)
	if later := r.mid(t, "Held 1"); !(later < mid1 && later > focus) {
		t.Errorf("it did not keep climbing: %v then %v", mid1, later)
	}
	r.advance(time.Second)
	if got := r.mid(t, "Held 1"); math.Abs(float64(got-focus)) > 6 {
		t.Errorf("after the climb Held 1 is at %v, want the centre %v", got, focus)
	}
	// Every line is where the column puts it, spaced as it was.
	if got, want := r.mid(t, "Held 2")-r.mid(t, "Held 1"), was2-was1; math.Abs(float64(got-want)) > 1 {
		t.Errorf("after the climb Held 2 is %v under Held 1, want %v", got, want)
	}
}

// On a seek back one line the lines go down, mirrored: the lines above the
// sung one trail it, the ones below go with it.
func TestTheLinesGoDownOnASeekBackAndTheOnesAboveTrail(t *testing.T) {
	r := stageRig(t, heldLines(2500, 2500, 2500, 2500, 2500, 2500, 2500), 1000+2*(2500+600)+400, nil, func(r *npRig) { r.animate = true })
	r.settle()
	r.advance(time.Second)
	if got := r.p.np.stage.anchor; got != 2 {
		t.Fatalf("the sung line is %d, want 2", got)
	}
	was0, was1, was2 := r.mid(t, "Held 0"), r.mid(t, "Held 1"), r.mid(t, "Held 2")
	r.play(1000 + 2500 + 600 + 400) // back to line 1
	r.tt.Drain()
	r.advance(30 * time.Millisecond)
	if got := r.mid(t, "Held 1"); !(got > was1+0.5) {
		t.Errorf("30 ms into the seek back the new sung line is at %v (was %v): it did not go down", got, was1)
	}
	if got := r.mid(t, "Held 2"); !(got > was2+0.5) {
		t.Errorf("30 ms in, the line below is at %v (was %v): it did not go down with the sung one", got, was2)
	}
	if got := r.mid(t, "Held 0"); math.Abs(float64(got-was0)) > 0.5 {
		t.Errorf("30 ms in, the line above has moved from %v to %v: it trails the sung line by a step (%v)", was0, got, cascadeStep)
	}
	r.advance(cascadeStep)
	if got := r.mid(t, "Held 0"); !(got > was0+0.5) {
		t.Errorf("90 ms in, the line above is still at %v (was %v)", got, was0)
	}
	r.advance(time.Second)
	focus := float32(r.height) * stageFocus
	if got := r.mid(t, "Held 1"); math.Abs(float64(got-focus)) > 6 {
		t.Errorf("after the seek back Held 1 is at %v, want the centre %v", got, focus)
	}
}

// A seek back from a song opened in the middle goes to lines before the first
// one the column laid out: they are added in front of it, and the column
// glides instead of cutting.
func TestASeekBackBeforeTheFirstLineLaidOutStillGlides(t *testing.T) {
	r := stageRig(t, heldLines(2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500), 1000+9*(2500+600)+400, nil, func(r *npRig) { r.animate = true })
	r.settle()
	r.advance(time.Second)
	st := r.p.np.stage
	if st.anchor != 9 || st.origin != 9-stageBuild {
		t.Fatalf("anchor %d origin %d: the song was not opened at line 9", st.anchor, st.origin)
	}
	focus := float32(r.height) * stageFocus
	was := r.mid(t, "Held 7") // two places above the sung line
	r.play(1000 + 7*(2500+600) + 400)
	r.tt.Drain()
	r.advance(30 * time.Millisecond)
	if st.anchor != 7 || st.origin != 7-stageBuild {
		t.Fatalf("anchor %d origin %d after the seek: want 7 and %d (the column grew in front)", st.anchor, st.origin, 7-stageBuild)
	}
	if got := r.mid(t, "Held 7"); !(got > was+0.5 && got < focus-6) {
		t.Errorf("30 ms into the seek back Held 7 is at %v: not between where it was (%v) and the centre (%v): it cut", got, was, focus)
	}
	r.advance(time.Second)
	if got := r.mid(t, "Held 7"); math.Abs(float64(got-focus)) > 6 {
		t.Errorf("after the seek back Held 7 is at %v, want the centre %v", got, focus)
	}
	// Lines laid out in front of the first one stand where a column laid out
	// from there would have them.
	if got, want := st.top(8)-st.top(7), st.top(9)-st.top(8); math.Abs(float64(got-want)) > 0.5 {
		t.Errorf("lines 7 and 8 are %v apart, lines 8 and 9 %v", got, want)
	}
	if got, want := st.top(2)-st.top(1), st.top(3)-st.top(2); math.Abs(float64(got-want)) > 0.5 {
		t.Errorf("lines 1 and 2 (laid in front) are %v apart, lines 2 and 3 %v", got, want)
	}
}

// A seek of more than a few lines cuts: every line is in place the frame the
// sung line changes, none of them gliding the way of the cascade.
func TestASeekOfSeveralLinesCutsTheColumnInsteadOfGliding(t *testing.T) {
	r := stageRig(t, heldLines(2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500, 2500), 1500, nil, func(r *npRig) { r.animate = true })
	r.settle()
	r.advance(time.Second)
	r.play(1000 + 6*(2500+600) + 400) // line 6: six places on
	r.tt.Drain()
	r.advance(20 * time.Millisecond)
	st := r.p.np.stage
	if st.anchor != 6 {
		t.Fatalf("anchor %d after a seek of six lines, want 6", st.anchor)
	}
	focus := float32(r.height) * stageFocus
	was := r.mid(t, "Held 7")
	if got := r.mid(t, "Held 6"); math.Abs(float64(got-focus)) > 6 {
		t.Errorf("a frame after the seek Held 6 is at %v, want the centre %v", got, focus)
	}
	r.advance(time.Second)
	if got := r.mid(t, "Held 7"); math.Abs(float64(got-was)) > 0.5 {
		t.Errorf("Held 7 was at %v a frame after the seek and %v at rest: it glided", was, got)
	}
}

// A line that begins while the lines are still gliding sets them on from where
// they are: no line jumps from one frame to the next by more than the glide
// moves it.
func TestALineThatBeginsMidClimbRetargetsWithoutAJump(t *testing.T) {
	lines := make([]services.LyricLine, 12)
	for i := range lines { // a fast song: a line every 200 ms
		begin := int64(1000 + i*200)
		lines[i] = services.LyricLine{BeginMs: begin, EndMs: begin + 180, Text: fmt.Sprintf("Held %d", i), Agent: "v1", AgentType: "person"}
	}
	r := stageRig(t, lines, 1000+2*200+100, nil, func(r *npRig) { r.animate = true })
	st := r.p.np.stage
	pitch := st.top(5) - st.top(4)
	started := st.anchor
	prev := map[int]float32{}
	worst := float32(0)
	for range 70 { // over a second of frames, over five lines
		r.advance(16 * time.Millisecond)
		for _, i := range []int{6, 7} {
			box, ok := r.tt.Find(fmt.Sprintf("Held %d", i))
			if !ok {
				delete(prev, i)
				continue
			}
			y := box.Y + box.H/2
			if was, seen := prev[i]; seen {
				worst = max(worst, float32(math.Abs(float64(y-was))))
			}
			prev[i] = y
		}
	}
	if st.anchor < started+4 {
		t.Fatalf("the sung line went from %d to %d: the lines were not fast", started, st.anchor)
	}
	if worst == 0 {
		t.Fatal("no line moved: the test saw nothing")
	}
	if worst > pitch/2 {
		t.Errorf("a line moved %v in one frame, more than half of a line (%v): it jumped", worst, pitch)
	}
}

func TestUnderReducedMotionTheColumnJumpsAndTheWashDoesNotDrift(t *testing.T) {
	theme.SetReducedMotion(true)
	t.Cleanup(func() { theme.SetReducedMotion(false) })
	r := stageRig(t, heldLines(2500, 2500, 2500, 2500, 2500, 2500, 2500), 1500, nil, func(r *npRig) {
		r.animate = true
		pol := kit.NewTestMotionPolicy(r.h.Main.Post, (&motionOpens{}).open)
		t.Cleanup(pol.Close)
		r.h.Env.Motion = pol
	})
	r.settle()
	r.play(1000 + 2500 + 600 + 400)
	r.tt.Drain()
	r.advance(20 * time.Millisecond)
	box, ok := r.tt.Find("Held 1")
	if !ok {
		t.Fatalf("Held 1 is not on screen: %q", r.tt.Texts())
	}
	if got, want := box.Y+box.H/2, float32(r.height)*stageFocus; math.Abs(float64(got-want)) > 6 {
		t.Errorf("one frame after the line advanced Held 1 is at %v, want the centre %v: a jump", got, want)
	}
	// Every line jumped with it, the ones that would trail too: a frame on
	// they are where they will be a second on.
	next, later := r.mid(t, "Held 2"), r.mid(t, "Held 0")
	r.advance(time.Second)
	if got := r.mid(t, "Held 2"); math.Abs(float64(got-next)) > 0.5 {
		t.Errorf("a frame after the jump Held 2 is at %v but comes to rest at %v: it trailed", next, got)
	}
	if got := r.mid(t, "Held 0"); math.Abs(float64(got-later)) > 0.5 {
		t.Errorf("a frame after the jump Held 0 is at %v but comes to rest at %v: it trailed", later, got)
	}
	if r.p.np.stage.drifted {
		t.Error("the wash drifts under reduced motion")
	}
}

func TestTheWashDriftsOnlyWhereMotionMayRun(t *testing.T) {
	policy := func(r *npRig) {
		pol := kit.NewTestMotionPolicy(r.h.Main.Post, (&motionOpens{}).open)
		t.Cleanup(pol.Close)
		r.h.Env.Motion = pol
	}
	r := stageRig(t, heldLines(2500, 2500, 2500), 1500, nil, policy)
	r.settle()
	if !r.p.np.stage.drifted {
		t.Error("the wash does not drift where motion may run")
	}
	// No policy (a hand-built Env): nothing may move on its own.
	r = stageRig(t, heldLines(2500, 2500, 2500), 1500, nil)
	r.settle()
	if r.p.np.stage.drifted {
		t.Error("the wash drifts with no motion policy")
	}
}

func TestTheStripHidesAfterThreeSecondsAndComesBackOnAKeyOrAMove(t *testing.T) {
	r := stageRig(t, heldLines(60000), 5000, nil)
	r.tt.SetFocused(true)
	strip := func() bool { return r.tt.HasText("Exit full screen") }
	if strip() {
		t.Fatal("the strip is up when the view opens")
	}
	r.wake()
	r.tt.Until("the strip", r.tt.Shows("Exit full screen", "Song Title", "The Artist"))
	for _, want := range []string{"Lyrics on time", "0:05", "\u22123:55"} {
		if !r.tt.HasText(want) {
			t.Errorf("the strip lacks %q: %q", want, r.tt.Texts())
		}
	}
	if r.tt.Cursor() == ui.CursorNone {
		t.Error("the pointer is hidden while the strip shows")
	}
	r.advance(2900 * time.Millisecond)
	if !strip() {
		t.Error("the strip went before 3 s")
	}
	r.advance(200 * time.Millisecond)
	r.settle()
	if strip() {
		t.Errorf("the strip stays after 3 s idle: %q", r.tt.Texts())
	}
	if r.tt.Cursor() != ui.CursorNone {
		t.Errorf("the pointer shows with the strip gone: %v", r.tt.Cursor())
	}
	// A key brings it back, and the pointer too.
	r.tt.Key(0, ui.KeyHome)
	r.tt.Until("the strip on a key", r.tt.Shows("Exit full screen"))
	r.advance(3100 * time.Millisecond)
	r.settle()
	if strip() {
		t.Error("the strip did not go again")
	}
	r.tt.Move(10, 10)
	r.tt.Move(40, 30)
	r.tt.Until("the strip on a move", r.tt.Shows("Exit full screen"))
	// Paused: it holds.
	r.play(5000, func(s *services.PlaybackState) { s.State = store.StatePaused })
	r.advance(10 * time.Second)
	r.settle()
	if !strip() {
		t.Error("the strip went while paused")
	}
}

func TestFullScreenLyricsKeysSeekAndStepLines(t *testing.T) {
	lines := heldLines(2500, 2500, 2500, 2500, 2500)
	r := stageRig(t, lines, 1000+2500+600+800, nil) // in line 1
	byID := map[string]module.Shortcut{}
	for _, s := range r.p.nowPlayingShortcuts() {
		byID[s.ID] = s
	}
	pos := func() int64 { return r.h.Env.Play.State().PositionMs }
	for _, id := range []string{"player.lyrics-line-previous", "player.lyrics-line-next", "player.lyrics-seek-back", "player.lyrics-seek-forward", "player.lyrics-start"} {
		if sc, ok := byID[id]; !ok || sc.Scope != stageScope || !sc.When() {
			t.Fatalf("%s: %+v", id, sc)
		}
	}
	for _, c := range []struct {
		id   string
		want int64
	}{
		{"player.lyrics-line-next", lines[2].BeginMs},     // down to the next line
		{"player.lyrics-line-previous", lines[1].BeginMs}, // up from line 2 to line 1
		{"player.lyrics-seek-forward", lines[1].BeginMs + 5000},
		{"player.lyrics-seek-back", lines[1].BeginMs},
		{"player.lyrics-start", 0},
	} {
		r.tt.Drain()
		byID[c.id].Run()
		r.tt.Drain()
		if got := pos(); got != c.want {
			t.Errorf("%s: position %d, want %d", c.id, got, c.want)
		}
	}
	// Space is the player's own (it plays and pauses wherever the focus is).
	if sc := byID["player.toggle"]; sc.ID != "" {
		t.Error("the view registered Space again")
	}
	// The keys are the view's only while it is open.
	r.p.ui.closeOverlay()
	if byID["player.lyrics-start"].When() {
		t.Error("Home seeks with the view closed")
	}
}

func TestInABreakUpAndDownStepFromTheLineJustSung(t *testing.T) {
	lines := heldLines(2000, 2000, 2000, 2000)
	lines[2].BeginMs += 8000 // a break before line 2
	lines[2].EndMs += 8000
	lines[3].BeginMs += 8000
	lines[3].EndMs += 8000
	r := stageRig(t, lines, lines[1].EndMs+2000, nil) // resting after line 1
	r.settle()
	st := r.p.np.stage
	if st.active != -1 || st.anchor != 2 {
		t.Fatalf("active %d anchor %d: the voice is not resting before line 2", st.active, st.anchor)
	}
	r.p.stageStep(-1)
	if got := r.h.Env.Play.State().PositionMs; got != lines[0].BeginMs {
		t.Errorf("Up in a break went to %d, want the line before the one just sung (%d)", got, lines[0].BeginMs)
	}
}

func TestPlainLyricsUpAndDownMoveTheReadingLine(t *testing.T) {
	r := newNPRigWith(t, nil, func(r *npRig) { r.width, r.height = 1920, 1080 })
	lines := make([]services.LyricLine, 8)
	for i := range lines {
		lines[i] = services.LyricLine{Text: fmt.Sprintf("Plain %d", i)}
	}
	r.lyr.set(services.Lyrics{Lines: lines}, nil)
	r.play(2000)
	r.p.ui.openRoom()
	r.tt.Until("the room", r.tt.Shows("Full-Screen Lyrics", "Plain 0"))
	r.p.stageStep(1)
	r.p.stageStep(1)
	r.tt.Drain()
	if st := r.p.np.stage; st.anchor != 2 {
		t.Errorf("the reading line is %d after two Downs, want 2", st.anchor)
	}
	r.p.stageStep(-1)
	r.tt.Drain()
	if st := r.p.np.stage; st.anchor != 1 {
		t.Errorf("the reading line is %d after an Up, want 1", st.anchor)
	}
}

func TestNoLyricsShowsTheCoverTheTitleAndAQuietWord(t *testing.T) {
	r := newNPRigWith(t, nil, func(r *npRig) { r.width, r.height = 1280, 720 })
	r.lyr.set(services.Lyrics{}, lyrics.ErrNotFound)
	r.play(2000)
	r.p.ui.openRoom()
	r.tt.Until("the view", r.tt.Shows("Full-Screen Lyrics", "No lyrics for this song", "Song Title", "The Artist"))
	if !r.p.roomOpen() {
		t.Fatal("the view closed over a song without lyrics")
	}
	// The quiet word is muted ink, not the ink of a lyric.
	box, ok := r.tt.Find("No lyrics for this song")
	if !ok {
		t.Fatal("no word")
	}
	if box.X < float32(r.width)*stageLeft {
		t.Errorf("the word starts at %v, left of the column (%v)", box.X, float32(r.width)*stageLeft)
	}
}

func TestLoadingKeepsTheLayoutWithFlatBars(t *testing.T) {
	r := newNPRigWith(t, nil, func(r *npRig) { r.width, r.height = 1280, 720 })
	gate := make(chan struct{})
	t.Cleanup(func() { close(gate) })
	r.p.np.lyrics = gatedLyrics{r.lyr, gate}
	r.lyr.set(services.Lyrics{}, errors.New("slow"))
	r.play(2000)
	r.p.ui.openRoom()
	r.tt.Until("the view", r.tt.Shows("Full-Screen Lyrics", "Loading lyrics"))
	if r.tt.HasText("No lyrics for this song") {
		t.Error("loading looks like no lyrics")
	}
}

func TestAFailedLoadIsTheUsualBlockWithRetry(t *testing.T) {
	r := newNPRigWith(t, nil, func(r *npRig) { r.width, r.height = 1280, 720 })
	r.lyr.set(services.Lyrics{}, &url.Error{Op: "Get", URL: "https://lyrics.example", Err: errors.New("connection refused")})
	r.play(2000)
	r.p.ui.openRoom()
	r.tt.Until("the view", r.tt.Shows("Full-Screen Lyrics", "Couldn't load the lyrics", "Try again"))
	calls := r.lyr.count()
	r.tt.MustClick("Try again")
	r.tt.Until("a second ask", func() bool { return r.lyr.count() > calls })
}

func TestDuetsKeepTheirIndentTheirTogetherHeadingAndTheirBackgroundVocals(t *testing.T) {
	r := newNPRigWith(t, nil, func(r *npRig) { r.width, r.height = 1920, 1080 })
	lines := duetLines()
	r.lyr.set(services.Lyrics{Timing: "Word", Lines: lines}, nil)
	r.play(lines[1].BeginMs + 500)
	r.p.ui.openRoom()
	r.tt.Until("the view", r.tt.Shows("Full-Screen Lyrics", "Together", "for the tide"))
	st := r.p.np.stage
	first, second, together := st.block(0), st.block(1), st.block(2)
	if first.other || first.indent != 0 {
		t.Errorf("the first voice is indented: %+v", first)
	}
	if !second.other || second.indent != round32(second.size*stageIndent) {
		t.Errorf("the second voice is not indented 1.5em: other %v indent %v size %v", second.other, second.indent, second.size)
	}
	if !together.headed || together.other {
		t.Errorf("the line sung together has no heading, or is indented: %+v", together)
	}
	if first.headed || second.headed {
		t.Error("Together stands over a line that is not sung together")
	}
	// Never voices on opposite sides: both start at the column's left edge,
	// the second only indented in.
	b1, ok1 := r.tt.Find("Out past the breakwater")
	b2, ok2 := r.tt.Find("the radio was low")
	if !ok1 || !ok2 {
		t.Fatalf("lines missing: %q", r.tt.Texts())
	}
	if d := b2.X - b1.X; math.Abs(float64(d-second.indent)) > 4 {
		t.Errorf("the second voice starts %v right of the first, want its indent %v", d, second.indent)
	}
	// Background vocals sit under their line at 0.42em.
	if first.bg == nil {
		t.Fatal("no background vocals under the first line")
	}
	found := false
	for k := range st.cache {
		if k.line == 0 && k.part == 1 {
			found = true
			if want := max(round32(first.size*stageSub), 15); k.size != want {
				t.Errorf("the background vocals are %v, want %v (0.42em)", k.size, want)
			}
		}
	}
	if !found {
		t.Error("the background vocals were not shaped")
	}
	if !r.tt.HasText("(oh, the tide)") {
		t.Errorf("the background vocals are not on screen: %q", r.tt.Texts())
	}
}

func TestInBreaksNothingIsLit(t *testing.T) {
	lines := heldLines(2000, 2000, 2000)
	lines[1].BeginMs += 9000
	lines[1].EndMs += 9000
	lines[2].BeginMs += 9000
	lines[2].EndMs += 9000
	r := stageRig(t, lines, lines[0].EndMs+3000, nil)
	r.settle()
	st := r.p.np.stage
	if st.active != -1 {
		t.Fatalf("active %d: a line is sung in the break", st.active)
	}
	for i := range lines {
		if got := st.lineState(i, stageIn{anchor: st.anchor, active: st.active}); got == stActive {
			t.Errorf("line %d is lit in a break", i)
		}
	}
}

func TestFullScreenLyricsAccessibility(t *testing.T) {
	r := stageRig(t, heldLines(3000, 3000, 3000, 3000), 1500, nil)
	r.tt.SetFocused(true)
	tr := access.Read(t, r.tt.Tester)
	view, ok := tr.Find(ui.RoleGroup, "Full-Screen Lyrics")
	if !ok {
		t.Fatalf("no group named Full-Screen Lyrics:\n%s", tr.Dump())
	}
	if view.Description == "" {
		t.Error("the keys are not described")
	}
	col, ok := tr.Find(ui.RoleGroup, "Lyrics")
	if !ok || !tr.Within(col, ui.RoleGroup) {
		t.Fatalf("the lines are not a group inside the view:\n%s", tr.Dump())
	}
	for _, l := range []string{"Held 0", "Held 1", "Held 2", "Held 3"} {
		if n, ok := tr.Find(ui.RoleText, l); !ok || !tr.Within(n, ui.RoleGroup) {
			t.Errorf("%q is not text in the lyrics group", l)
		}
	}
	if r.tt.HasText("Exit full screen") {
		t.Error("a control is built with the strip hidden: Tab would land on it unseen")
	}
	// Any key brings the strip, and Tab walks it in reading order.
	r.tt.Key(0, ui.KeyTab)
	r.tt.Until("the strip", r.tt.Shows("Exit full screen"))
	stops := access.TabWalk(t, r.tt.Tester, 12)
	access.InOrder(t, access.StopLabels(stops), "Previous", "Pause", "Next", "Seek", "Show lyrics 0.5 s earlier", "Show lyrics 0.5 s later", "Exit full screen")
	tr = access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleSlider, "Seek"); !ok || n.Value == "" {
		t.Errorf("the scrubber is not a labelled slider that says where it is: %+v", n)
	}
	access.NoNestedControls(t, r.tt.Tester)
}

// duetLines are invented: a duet whose first line has background vocals, whose
// third is sung together with a held word.
func duetLines() []services.LyricLine {
	word := func(begin, end int64, text string, bg bool) services.LyricWord {
		return services.LyricWord{BeginMs: begin, EndMs: end, Text: text, Background: bg}
	}
	return []services.LyricLine{
		{
			BeginMs: 1000, EndMs: 4000, Text: "Out past the breakwater", Background: "(oh, the tide)", Agent: "v1", AgentType: "person",
			Words: []services.LyricWord{
				word(1000, 1400, "Out ", false), word(1400, 1800, "past ", false), word(1800, 2200, "the ", false), word(2200, 3600, "breakwater", false),
				word(2400, 3900, "(oh, the tide)", true),
			},
		},
		{
			BeginMs: 4200, EndMs: 7000, Text: "the radio was low", Agent: "v2", AgentType: "person",
			Words: []services.LyricWord{word(4200, 4800, "the ", false), word(4800, 5400, "radio ", false), word(5400, 5900, "was ", false), word(5900, 6900, "low", false)},
		},
		{
			BeginMs: 7200, EndMs: 12000, Text: "for the tide", Agent: "v1000", AgentType: "group",
			Words: []services.LyricWord{word(7200, 7800, "for ", false), word(7800, 8300, "the ", false), word(8300, 11800, "tide", false)},
		},
		{
			BeginMs: 12400, EndMs: 14400, Text: "and it came back", Agent: "v1", AgentType: "person",
			Words: []services.LyricWord{word(12400, 12900, "and ", false), word(12900, 13300, "it ", false), word(13300, 13800, "came ", false), word(13800, 14300, "back", false)},
		},
	}
}

// A climb and a sweep move in frames that only paint: the view is built when
// the sung line changes (and a timer says so), not once a frame.
func TestTheClimbAndTheSweepDoNotBuildTheViewPerFrame(t *testing.T) {
	builds := 0
	r := newNPRigWith(t, nil, func(r *npRig) {
		r.width, r.height = 1920, 1080
		r.animate = true
		r.inspect = func(*ui.Context) { builds++ }
	})
	lines := duetLines()
	r.lyr.set(services.Lyrics{Timing: "Word", Lines: lines}, nil)
	r.play(lines[1].BeginMs + 300)
	r.p.ui.openRoom()
	r.tt.Until("the room and its lines", r.tt.Shows("Full-Screen Lyrics", "Out past the breakwater"))
	r.settle()
	r.advance(time.Second)
	before := builds
	// A second of a held word filling, and a climb to the next line: 60 frames.
	r.h.Clock.Advance(0)
	for range 60 {
		r.h.Clock.Advance(16 * time.Millisecond)
		r.tt.Advance(16 * time.Millisecond)
	}
	if got := builds - before; got > 6 {
		t.Errorf("the view was built %d times in 60 frames of sweep and climb", got)
	}
}

// A change of line builds the stage once, and the cascade of lines gliding
// over the next 90 frames builds it no more: each line's glide is its own
// pose, which only paints.
func TestTheCascadeBuildsTheStageOnceForALineNotPerFrame(t *testing.T) {
	builds := 0
	r := newNPRigWith(t, nil, func(r *npRig) {
		r.width, r.height = 1920, 1080
		r.animate = true
		r.inspect = func(*ui.Context) { builds++ }
	})
	lines := heldLines(2500, 2500, 2500, 2500, 2500, 2500, 2500)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: lines}, nil)
	r.play(1500)
	r.p.ui.openRoom()
	r.tt.Until("the room and its lines", r.tt.Shows("Full-Screen Lyrics", "Held 0"))
	r.settle()
	r.advance(2 * time.Second)
	before := builds
	r.play(1000 + 2500 + 600 - 300) // a little before line 1 begins
	frames := 0
	for range 90 {
		r.h.Clock.Advance(16 * time.Millisecond)
		r.tt.Advance(16 * time.Millisecond)
		frames++
	}
	if r.p.np.stage.anchor != 1 {
		t.Fatalf("line %d is the stage's: the change did not come", r.p.np.stage.anchor)
	}
	t.Logf("a line change and %d frames of its cascade built the stage %d times", frames, builds-before)
	if got := builds - before; got > 2 {
		t.Errorf("the stage was built %d times over one line change and %d frames", got, frames)
	}
}

// glideCase is a song whose sung line changes while Full-Screen Lyrics is open.
type glideCase struct {
	name   string
	timing string
	lines  []services.LyricLine
	at     int64    // where the song is when the room opens
	labels []string // lines on the stage, the dots' label first where they show
}

var glideCases = []glideCase{
	// A word-timed line ends and the next begins, which fills through clips
	// while the column still glides.
	{"lines", "Word", sketchLines(), 3_000, []string{"Out past the breakwater", "the radio was low", "We left the lights on", "for the tide"}},
	// The dots of a gap are the sung line, then the lines climb.
	{"dots", "Line", gapLines(), 6_000, []string{"Instrumental break", "Gap line 0", "Gap line 1"}},
}

// glideRig opens Full-Screen Lyrics over the case, 1280×720 with animations
// on, and lets it settle: the first column is built and at rest.
func glideRig(t *testing.T, c glideCase, onBuild func()) *npRig {
	t.Helper()
	r := newNPRigWith(t, nil, func(r *npRig) {
		r.width, r.height = 1280, 720
		r.animate = true
		r.inspect = func(*ui.Context) { onBuild() }
	})
	r.lyr.set(services.Lyrics{Timing: c.timing, Lines: c.lines}, nil)
	r.play(c.at)
	r.p.ui.openRoom()
	r.tt.Until("the room and its lines", r.tt.Shows("Full-Screen Lyrics", c.labels[1]))
	r.settle()
	r.advance(600 * time.Millisecond)
	return r
}

// A glide moves and fades the lines; it never scales one. The sung line paints
// its fill through clips (kit.SweepText), and MyGo cuts a clip inside a scaled
// element with a coverage mask that the CPU rasterises and uploads again in
// every frame the scale moves: on a real GPU, 12 to 45 masks and up to 80 ms a
// frame at each line change, with build, layout and paint all under 1 ms
// (#690). A translation or a fade costs none of it. A label's box is its
// element's laid out, moved and scaled by its pose, so a box that keeps its
// size from before the change until the cascade is done has only been moved.
func TestTheGlideNeverScalesALine(t *testing.T) {
	for _, c := range glideCases {
		t.Run(c.name, func(t *testing.T) {
			r := glideRig(t, c, func() {})
			rest, top := map[string]ui.Rect{}, map[string]float32{}
			for _, l := range c.labels {
				if box, ok := r.tt.Find(l); ok {
					rest[l], top[l] = box, box.Y
				}
			}
			if len(rest) < 2 {
				t.Fatalf("only %d of the labels are on the stage: %q", len(rest), r.tt.Texts())
			}
			anchor, changed, moved := r.p.np.stage.anchor, -1, float32(0)
			for i := 0; i < 400 && (changed < 0 || i < changed+70); i++ {
				r.advance(16 * time.Millisecond)
				if changed < 0 && r.p.np.stage.anchor != anchor {
					changed = i
				}
				for l, was := range rest {
					box, ok := r.tt.Find(l)
					if !ok {
						continue // the dots are gone once the line has begun
					}
					if math.Abs(float64(box.W-was.W)) > 0.05 || math.Abs(float64(box.H-was.H)) > 0.05 {
						t.Errorf("frame %d: %q is %.2f×%.2f, it was %.2f×%.2f at rest: the glide scaled it", i, l, box.W, box.H, was.W, was.H)
					}
					moved = max(moved, float32(math.Abs(float64(box.Y-top[l]))))
				}
			}
			if changed < 0 {
				t.Fatal("the sung line never changed")
			}
			if moved < 20 {
				t.Errorf("the lines moved %.1f DIP at the most: no glide ran", moved)
			}
		})
	}
}

// The glide, with the next line filling through it, is frames that only paint:
// the stage is built when the sung line changes (and when a timer says so) and
// not once a frame, through 100 frames from before the change to after it.
func TestTheGlideOfASweptLinePaintsOnly(t *testing.T) {
	for _, c := range glideCases {
		t.Run(c.name, func(t *testing.T) {
			builds := 0
			r := glideRig(t, c, func() { builds++ })
			anchor, before := r.p.np.stage.anchor, builds
			var frames int
			for frames = 0; frames < 600 && r.p.np.stage.anchor == anchor; frames++ {
				r.h.Clock.Advance(16 * time.Millisecond)
				r.tt.Advance(16 * time.Millisecond)
			}
			if r.p.np.stage.anchor == anchor {
				t.Fatal("the sung line never changed")
			}
			for range 100 {
				r.h.Clock.Advance(16 * time.Millisecond)
				r.tt.Advance(16 * time.Millisecond)
			}
			t.Logf("%s: %d frames to the change and 100 after it built the stage %d times", c.name, frames, builds-before)
			if got := builds - before; got > 6 {
				t.Errorf("the stage was built %d times over %d frames of glide and sweep", got, frames+100)
			}
		})
	}
}

// A verse break is a gap of 0.9em where lines are a tenth of an em apart,
// the sheet's rule at the stage's sizes.
func TestVerseGapsFollowTheSheetsRule(t *testing.T) {
	lines := heldLines(2500, 2500, 2500, 2500, 2500, 2500)
	for i := 3; i < len(lines); i++ {
		lines[i].BeginMs += 5000 // a pause of three seconds or more before line 3
		lines[i].EndMs += 5000
	}
	r := stageRig(t, lines, lines[2].BeginMs+500, nil)
	st := r.p.np.stage
	if st.verseOf[2] == st.verseOf[3] || st.verseOf[1] != st.verseOf[2] {
		t.Fatalf("verses %v: a break was not found before line 3", st.verseOf)
	}
	a, b := st.block(2), st.block(3)
	if got, want := st.gap(3), 0.9*min((a.size+b.size)/2, 72*st.g.scale); got != want {
		t.Errorf("the verse gap is %v, want 0.9em = %v", got, want)
	}
	if got, want := st.gap(2), round32(0.14*(st.block(1).size+a.size)/2); got != want {
		t.Errorf("the gap between lines of a verse is %v, want %v", got, want)
	}
	if st.gap(3) < 4*st.gap(2) {
		t.Errorf("a verse break (%v) is not clearly wider than a line gap (%v)", st.gap(3), st.gap(2))
	}
}

// Romanisation and translation, when they are switched on, sit under their
// line, muted, and not when they are off.
func TestTranslationAndPronunciationShowUnderTheLineWhenSwitchedOn(t *testing.T) {
	lines := heldLines(2500, 2500, 2500, 2500)
	for i := range lines {
		lines[i].Translation = &services.LyricText{Text: fmt.Sprintf("Translated %d", i)}
		lines[i].Transliteration = &services.LyricText{Text: fmt.Sprintf("Romanised %d", i)}
	}
	r := newNPRigWith(t, nil, func(r *npRig) { r.width, r.height = 1920, 1080 })
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: lines, TranslationLanguage: "en-US", TransliterationLanguage: "ja-Latn"}, nil)
	r.play(lines[1].BeginMs + 500)
	r.p.ui.openRoom()
	r.tt.Until("the room", r.tt.Shows("Full-Screen Lyrics", "Held 1"))
	r.settle()
	if r.tt.HasText("Translated 1") || r.tt.HasText("Romanised 1") {
		t.Errorf("the extras show while they are off: %q", r.tt.Texts())
	}
	r.p.prefsFor().set(lyricsync.Prefs{Translation: true, Pronunciation: true})
	r.tt.Until("the extras", r.tt.Shows("Translated 1", "Romanised 1"))
	lead, _ := r.tt.Find("Held 1")
	tr, _ := r.tt.Find("Translated 1")
	pr, _ := r.tt.Find("Romanised 1")
	if !(pr.Y >= lead.Y+lead.H && tr.Y >= pr.Y+pr.H) {
		t.Errorf("the romanisation (%v) and the translation (%v) are not in order under the line (%v)", pr, tr, lead)
	}
}

// The window goes full screen while the view is open, and only if it was not
// already; the screen is kept on only while the music plays and the view is
// open.
func TestFullScreenLyricsGoesFullScreenAndKeepsTheScreenOnWhilePlaying(t *testing.T) {
	r := newNPRig(t)
	full, screen := false, 0
	r.h.Env.Window.Bind(func() bool { return full }, func(on bool) { full = on })
	r.h.Env.Window.BindScreen(func() func() { screen++; return func() { screen-- } })
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(10, false)}, nil)
	r.play(2000)
	r.tt.Frame()
	if full || screen != 0 {
		t.Fatalf("full %v screen %d before the view opens", full, screen)
	}
	r.p.ui.openRoom()
	r.tt.Until("the room", r.tt.Shows("Full-Screen Lyrics"))
	if !full || screen != 1 {
		t.Errorf("full %v screen %d with the view open and playing, want full screen and one hold", full, screen)
	}
	r.play(2000, func(s *services.PlaybackState) { s.State = store.StatePaused })
	r.settle()
	if screen != 0 {
		t.Errorf("the screen is still held while paused (%d)", screen)
	}
	r.play(2000)
	r.settle()
	if screen != 1 {
		t.Errorf("the screen is not held after play resumed (%d)", screen)
	}
	r.p.ui.closeOverlay()
	r.settle()
	if full || screen != 0 {
		t.Errorf("full %v screen %d after leaving: the view left the window as it found it", full, screen)
	}
	// A window already full screen stays so.
	full = true
	r.p.ui.openRoom()
	r.settle()
	r.p.ui.closeOverlay()
	r.settle()
	if !full {
		t.Error("leaving took the user's full screen away")
	}
}

// At a song change the column of the song before goes, the wash crossfades
// (kit.Backdrop's own fade) and the new song's lines come in; the strip and
// the drift go on as they were.
func TestASongChangeStartsANewColumnAndKeepsTheChrome(t *testing.T) {
	r := stageRig(t, heldLines(2500, 2500, 2500, 2500), 1500, nil)
	r.wake()
	r.settle()
	first := r.p.np.stage
	wake := first.wake
	if wake.IsZero() {
		t.Fatal("the pointer did not wake the view")
	}
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: []services.LyricLine{
		{BeginMs: 500, EndMs: 3000, Text: "Another song, first line", Agent: "v1", AgentType: "person"},
		{BeginMs: 3200, EndMs: 6000, Text: "Another song, second line", Agent: "v1", AgentType: "person"},
	}}, nil)
	r.play(1000, func(s *services.PlaybackState) { s.Current.CatalogID = "1002" })
	r.tt.Until("the new lyrics", r.tt.Shows("Another song, first line"))
	second := r.p.np.stage
	if second == first || second.songID != "1002" {
		t.Fatalf("the stage is the old one: %q", second.songID)
	}
	if !second.wake.Equal(wake) {
		t.Error("the chrome's clock started again with the song")
	}
	if r.tt.HasText("Held 0") {
		t.Errorf("the old song's lines stay: %q", r.tt.Texts())
	}
}
