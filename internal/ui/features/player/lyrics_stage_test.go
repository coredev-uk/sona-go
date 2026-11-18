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
// unless a mutate says otherwise.
func stageRig(t *testing.T, lines []services.LyricLine, posMs int64, opts []uitest.Option, mutate ...func(*npRig)) *npRig {
	t.Helper()
	r := newNPRigWith(t, opts, append([]func(*npRig){func(r *npRig) { r.width, r.height = 1920, 1080 }}, mutate...)...)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: lines}, nil)
	r.play(posMs)
	r.p.ui.openRoom()
	r.tt.Until("the room", r.tt.Shows("Full-Screen Lyrics"))
	r.settle()
	return r
}

func TestHeldTimeSetsTheSizeStepOfALine(t *testing.T) {
	span := func(ds ...int64) []lyricsync.Span {
		var out []lyricsync.Span
		at := int64(0)
		for _, d := range ds {
			out = append(out, lyricsync.Span{BeginMs: at, EndMs: at + d})
			at += d + 100
		}
		return out
	}
	// Longer held, bigger: five steps from the quickest to the longest.
	got := heldSteps(span(1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000, 10000, 11000))
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Errorf("a longer line has a smaller step: %v", got)
		}
	}
	if got[0] != 0 || got[len(got)-1] != stageSteps-1 {
		t.Errorf("the quickest and the longest lines are not at the ends of the steps: %v", got)
	}
	seen := map[int]bool{}
	for _, s := range got {
		seen[s] = true
	}
	if len(seen) != stageSteps {
		t.Errorf("%d steps used of %d: %v", len(seen), stageSteps, got)
	}
	// Lines held alike are all at the middle step, not stretched to the ends.
	for _, s := range heldSteps(span(3000, 3100, 3000, 3200, 3000)) {
		if s != stageSteps/2 {
			t.Errorf("even lines got step %d", s)
		}
	}
	// One held line among even ones is the largest, the rest the smallest.
	got = heldSteps(span(2000, 2000, 2000, 2000, 2000, 2000, 2000, 2000, 2000, 2000, 30000))
	if got[len(got)-1] != stageSteps-1 || got[0] != 0 {
		t.Errorf("a held line among even ones: %v", got)
	}
	// A line with no length takes the song's middle line's step.
	got = heldSteps(span(1000, 2000, 3000, 4000, 5000, 6000, 7000, 8000, 9000, 10000, 11000, 0))
	if got[len(got)-1] != got[5] {
		t.Errorf("a line with no length is at step %d, the middle line's is %d", got[len(got)-1], got[5])
	}
}

func TestStageSizesFollowHeldTimeInStepsAndAreCapped(t *testing.T) {
	for _, c := range []struct {
		w, h        int
		lo, hi, tol float32
	}{
		{1920, 1080, 40, 128, 0},
		{3840, 2160, 80, 256, 0}, // scales with the window's height
		{1280, 540, 22, 64, 0},   // and has a floor in a small window
	} {
		t.Run(fmt.Sprintf("%dx%d", c.w, c.h), func(t *testing.T) {
			// Quick lines, then held ones, then one held long.
			lines := heldLines(1200, 1800, 2400, 3000, 4200, 5600, 7000, 9000, 12000, 14000)
			r := stageRig(t, lines, 1000+1200+600+1800+600+2400+600+500, nil, func(r *npRig) { r.width, r.height = c.w, c.h })
			st := r.p.np.stage
			sizes := map[float32]bool{}
			prev := float32(0)
			for i := st.anchor - stageReach; i <= st.anchor+stageReach; i++ {
				if i < 0 || i >= len(lines) {
					continue
				}
				b := st.block(i)
				if b.size < prev {
					t.Errorf("line %d (held longer) is set smaller (%v) than the line before (%v)", i, b.size, prev)
				}
				prev = b.size
				sizes[b.size] = true
				if b.size > c.hi || b.size < c.lo {
					t.Errorf("line %d is %v, outside %v to %v", i, b.size, c.lo, c.hi)
				}
			}
			if len(sizes) < 3 || len(sizes) > stageSteps {
				t.Errorf("%d sizes in the column: %v", len(sizes), sizes)
			}
			// Every line of the song stays in the steps and under the cap.
			for i := range lines {
				if s := st.steps[i]; s < 0 || s >= stageSteps {
					t.Errorf("line %d has step %d", i, s)
				}
			}
		})
	}
}

func TestAHugeHeldLineWrapsToTwoLinesAtMostBeforeItsSizeStepsDown(t *testing.T) {
	lines := heldLines(1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 1500, 20000, 1500)
	lines[9].Text = strings.Repeat("the tide came back slower than we thought ", 3)
	r := stageRig(t, lines, lines[9].BeginMs+500, nil)
	st := r.p.np.stage
	b := st.block(9)
	if st.steps[9] != stageSteps-1 {
		t.Fatalf("the long line is at step %d, want the top", st.steps[9])
	}
	if b.step >= st.steps[9] {
		t.Errorf("a line that would wrap to %d lines kept its step %d", b.lines(), b.step)
	}
	if b.lines() > stageMaxWrap && b.step > 0 {
		t.Errorf("wrapped to %d lines at step %d", b.lines(), b.step)
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

func TestTheColumnClimbsWhenTheSungLineAdvances(t *testing.T) {
	r := stageRig(t, heldLines(2500, 2500, 2500, 2500, 2500, 2500, 2500), 1500, nil, func(r *npRig) { r.animate = true })
	r.settle()
	r.advance(time.Second)
	mid := func(label string) float32 {
		t.Helper()
		box, ok := r.tt.Find(label)
		if !ok {
			t.Fatalf("%q is not on screen: %q", label, r.tt.Texts())
		}
		return box.Y + box.H/2
	}
	focus := float32(r.height) * stageFocus
	if got := mid("Held 0"); math.Abs(float64(got-focus)) > 6 {
		t.Fatalf("the sung line is at %v, want the optical centre %v", got, focus)
	}
	was := mid("Held 1") // coming
	// The next line begins: the column climbs to it over 600 ms.
	r.play(1000 + 2500 + 600 + 400)
	r.tt.Drain() // the view builds and the climb starts
	r.advance(60 * time.Millisecond)
	early := mid("Held 1")
	if !(early < was && early > focus+6) {
		t.Errorf("60 ms into the climb Held 1 is at %v: not between where it was (%v) and the centre (%v)", early, was, focus)
	}
	r.advance(150 * time.Millisecond)
	later := mid("Held 1")
	if !(later < early && later > focus) {
		t.Errorf("it did not keep climbing: %v then %v", early, later)
	}
	r.advance(700 * time.Millisecond)
	if got := mid("Held 1"); math.Abs(float64(got-focus)) > 6 {
		t.Errorf("after the climb Held 1 is at %v, want the centre %v", got, focus)
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
	access.InOrder(t, access.StopLabels(stops), "Show lyrics 0.5 s earlier", "Show lyrics 0.5 s later", "Exit full screen")
	tr = access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleProgress, "Song progress"); !ok || n.Value == "" {
		t.Errorf("the progress is not a labelled progress bar with a value: %+v", n)
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
	r.tt.Until("the room", r.tt.Shows("Full-Screen Lyrics"))
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
