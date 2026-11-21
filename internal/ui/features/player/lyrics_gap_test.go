package player

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// gapLines are four lines with instrumental gaps: 8 s before the first, 8 s
// between the second and the third, and only 1 s between the others.
func gapLines() []services.LyricLine {
	line := func(begin, end int64, text string) services.LyricLine {
		return services.LyricLine{BeginMs: begin, EndMs: end, Text: text, Agent: "v1", AgentType: "person"}
	}
	return []services.LyricLine{
		line(8000, 11000, "Gap line 0"),
		line(12000, 15000, "Gap line 1"),
		line(23000, 26000, "Gap line 2"),
		line(27000, 30000, "Gap line 3"),
	}
}

// longGapLines are twelve lines, 3 s sung every 4 s from 1 s, with 10 s more
// before line 9: a gap of 11 s after line 8 ends at 36 s.
func longGapLines() []services.LyricLine {
	out := timedLines(12, false)
	for i := 9; i < len(out); i++ {
		out[i].BeginMs += 10_000
		out[i].EndMs += 10_000
	}
	return out
}

func amberish(c color.RGBA) bool { return int(c.R)-int(c.B) > 50 }

// litDots reads which of the three dots from x0 on, d across and space apart,
// centred on y, are in the lit word's colour at their middle and at their
// right edge.
func (r *npRig) litDots(x0, y, d, space float32) (middle, edge [3]bool) {
	img := r.tt.Image()
	for k := range 3 {
		x := x0 + float32(k)*(d+space)
		middle[k] = amberish(img.RGBAAt(int(x+d/2), int(y)))
		edge[k] = amberish(img.RGBAAt(int(x+d-2), int(y)))
	}
	return middle, edge
}

// amberIn reports amber anywhere in the box.
func (r *npRig) amberIn(b ui.Rect) bool {
	img := r.tt.Image()
	for py := int(b.Y); py < int(b.Y+b.H); py++ {
		for px := int(b.X); px < int(b.X+b.W); px++ {
			if amberish(img.RGBAAt(px, py)) {
				return true
			}
		}
	}
	return false
}

func sameImage(a, b *image.RGBA) bool { return bytes.Equal(a.Pix, b.Pix) }

// tick moves the song and the tester's clock on by d without asking for a
// frame, so that only what the view itself asked for (a timer) is drawn.
func (r *npRig) tick(d time.Duration) {
	r.h.Clock.Advance(d)
	r.tt.Advance(d)
}

// In Full-Screen Lyrics the dots are the sung line while the gap runs: in
// the middle, filling one by one, and cleared when the next line begins.
func TestTheDotsAreTheSungLineOfFullScreenLyricsWhileAGapRuns(t *testing.T) {
	r := stageRig(t, gapLines(), 2000, nil)
	st := r.p.np.stage
	if !st.synced || len(st.gaps) != 4 || st.gaps[0] != (lyricsync.Span{BeginMs: 0, EndMs: 8000}) || st.gaps[1].EndMs != 0 || st.gaps[2] != (lyricsync.Span{BeginMs: 15000, EndMs: 23000}) || st.gaps[3].EndMs != 0 {
		t.Fatalf("gaps %+v", st.gaps)
	}
	if st.anchor != 0 || st.active != -1 {
		t.Fatalf("anchor %d active %d before the first line", st.anchor, st.active)
	}
	// The dots stand where the sung line does.
	focus := float32(r.height) * stageFocus
	box, ok := r.tt.Find("Instrumental break")
	if !ok {
		t.Fatalf("no dots: %q", r.tt.Texts())
	}
	if got := box.Y + box.H/2; math.Abs(float64(got-focus)) > 6 {
		t.Errorf("the dots are at %v, want the sung place %v", got, focus)
	}
	b := st.block(0)
	if b.dotsH == 0 || b.dots.d <= 0 {
		t.Fatalf("line 0 has no dots: %+v", b)
	}
	if line, ok := r.tt.Find("Gap line 0"); !ok || line.Y < box.Y+box.H {
		t.Errorf("the line is not under the dots: %+v, dots %+v", line, box)
	}
	y := box.Y + box.H/2
	at := func(posMs int64) [3]bool {
		r.play(posMs)
		r.settle()
		mid, _ := r.litDots(st.g.left, y, b.dots.d, b.dots.space)
		return mid
	}
	// The 8 s gap fills a dot over each third (2.67 s), on the lyrics' clock,
	// which is 250 ms ahead of the song.
	for _, c := range []struct {
		pos  int64
		want [3]bool
	}{
		{500, [3]bool{false, false, false}},
		{2500, [3]bool{true, false, false}},
		{5000, [3]bool{true, true, false}},
		{7500, [3]bool{true, true, true}},
	} {
		if got := at(c.pos); got != c.want {
			t.Errorf("at %d ms the dots' middles are lit %v, want %v", c.pos, got, c.want)
		}
	}
	// The first line begins: the dots clear.
	r.play(8000)
	r.settle()
	if st.active != 0 {
		t.Fatalf("line %d is sung at 8 s", st.active)
	}
	// Where line 0's dots stood (above the line, in the column the stage
	// shows now) there is no lit dot.
	b0 := st.block(0)
	mid0 := st.top(0) + b0.lead.Height()/2
	cleared := ui.Rect{X: st.g.left, Y: st.top(0) - b0.above() + st.g.focus - mid0, W: b0.dots.width(), H: b0.dotsH}
	if r.amberIn(cleared) {
		t.Error("the dots are still lit when the next line has begun")
	}
	// A gap between lines: the dots take the sung place once the voice rests,
	// and the line before them has gone up.
	r.play(17000)
	r.settle()
	if st.anchor != 2 || st.active != -1 {
		t.Fatalf("anchor %d active %d in the gap before line 2", st.anchor, st.active)
	}
	if box, ok = r.tt.Find("Instrumental break"); !ok {
		t.Fatalf("no dots in the gap before line 2: %q", r.tt.Texts())
	}
	if got := box.Y + box.H/2; math.Abs(float64(got-focus)) > 6 {
		t.Errorf("the dots of the gap before line 2 are at %v, want the sung place %v", got, focus)
	}
}

// The dots' fill is stepped by timers: the view is not built while it runs,
// and it steps about twelve times a second.
func TestTheDotsFillInStepsByTimersAndNotByBuildingTheView(t *testing.T) {
	builds := 0
	r := stageRig(t, gapLines(), 500, nil, func(r *npRig) { r.inspect = func(*ui.Context) { builds++ } })
	r.settle()
	st := r.p.np.stage
	box, _ := r.tt.Find("Instrumental break")
	b := st.block(0)
	y := box.Y + box.H/2
	before, built, changes := r.tt.Image(), builds, 0
	for range 30 { // 3 s in 100 ms
		r.tick(100 * time.Millisecond)
		img := r.tt.Image()
		if !sameImage(before, img) {
			changes++
		}
		before = img
	}
	if builds-built > 2 {
		t.Errorf("the view was built %d times while the dots filled", builds-built)
	}
	// A dot 22 DIP across fills over 2.7 s: a pixel or so a step.
	if changes < 18 {
		t.Errorf("the dots changed in %d of 30 tenths of a second: they are not filling", changes)
	}
	if mid, _ := r.litDots(st.g.left, y, b.dots.d, b.dots.space); !mid[0] {
		t.Errorf("after 3 s the first dot is not lit: %v", mid)
	}
}

// Under reduced motion each dot lights whole as its third begins.
func TestUnderReducedMotionTheDotsLightWholeOneByOne(t *testing.T) {
	theme.SetReducedMotion(true)
	t.Cleanup(func() { theme.SetReducedMotion(false) })
	// 1.8 s in (2.05 s on the lyrics' clock) of the 8 s gap: the first third
	// has begun, and a fill in steps would not yet reach the dot's edge.
	r := stageRig(t, gapLines(), 1800, nil)
	st := r.p.np.stage
	box, ok := r.tt.Find("Instrumental break")
	if !ok {
		t.Fatalf("no dots: %q", r.tt.Texts())
	}
	b := st.block(0)
	y := box.Y + box.H/2
	mid, edge := r.litDots(st.g.left, y, b.dots.d, b.dots.space)
	if mid != [3]bool{true, false, false} || edge != [3]bool{true, false, false} {
		t.Errorf("first third: middles %v edges %v: want the first dot whole, to its edge", mid, edge)
	}
	r.play(3000)
	r.settle()
	mid, edge = r.litDots(st.g.left, y, b.dots.d, b.dots.space)
	if mid != [3]bool{true, true, false} || edge != [3]bool{true, true, false} {
		t.Errorf("second third: middles %v edges %v", mid, edge)
	}
	// Nothing moves inside a third.
	before := r.tt.Image()
	r.advance(900 * time.Millisecond)
	if !sameImage(before, r.tt.Image()) {
		t.Error("the dots changed inside a third under reduced motion")
	}
}

// The dots hold the places of lines: while a gap runs the line after them is
// next, and a gap still to come is a place of its own.
func TestTheDotsTakeAPlaceInTheColumnWhileTheyAreSungOrComing(t *testing.T) {
	r := stageRig(t, gapLines(), 17000, nil)
	st := r.p.np.stage
	in := stageIn{anchor: 2, active: -1}
	lines, dots := st.ranks(2, true, 0, 3)
	if lines[2] != 1 || lines[3] != 2 || lines[1] != -1 || lines[0] != -2 || dots[2] != 0 {
		t.Errorf("in the gap before line 2: lines %v dots %v", lines, dots)
	}
	// Line 2 sung: the dots before it are gone and take no place.
	in.active = 2
	lines, _ = st.ranks(2, false, 0, 3)
	if lines[2] != 0 || lines[3] != 1 || lines[1] != -1 {
		t.Errorf("line 2 sung: lines %v", lines)
	}
	if gapStateOf(2, in.active, in.anchor) != gapPast {
		t.Error("the dots before the sung line are not past")
	}
	// Line 1 sung: the dots of the gap before line 2 are coming and take the
	// next place, which pushes line 2 to the one after.
	lines, dots = st.ranks(1, false, 0, 3)
	if lines[1] != 0 || dots[2] != 1 || lines[2] != 2 || lines[3] != 3 {
		t.Errorf("line 1 sung: lines %v dots %v", lines, dots)
	}
}

// Now Playing's sheet sets the dots in the lyrics, where a line would be.
func TestTheSheetSetsTheDotsWhereALineWouldBeAndTurnsToThem(t *testing.T) {
	var view ui.Rect
	r := newNPRig(t, func(r *npRig) {
		r.inspect = func(c *ui.Context) {
			if sh := r.p.np.sheet; sh != nil {
				view = sh.box.Bounds(c)
			}
		}
	})
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: longGapLines()}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 0 sung words"))
	r.settle()
	sh := r.p.np.sheet
	if sh.gaps == nil || sh.gaps[9].BeginMs != 36_000 || sh.gaps[9].EndMs != 47_000 {
		t.Fatalf("the sheet's gaps: %+v", sh.gaps)
	}
	// The voice rests after line 8 (it ended at 36 s, rests from 36.5 s): the
	// sheet follows to the dots, which are the sung line now, at the reading
	// position.
	r.play(37_500)
	for range 40 {
		r.advance(50 * time.Millisecond)
	}
	if sh.active != -1 || sh.anchor != 9 {
		t.Fatalf("active %d anchor %d in the gap", sh.active, sh.anchor)
	}
	box, ok := r.tt.Find("Instrumental break")
	if !ok {
		t.Fatalf("no dots on the sheet: %q", r.tt.Texts())
	}
	if want := view.Y + view.H*readAt; math.Abs(float64(box.Y-want)) > 2 {
		t.Errorf("the dots are at %v, want the reading position %v", box.Y, want)
	}
	line, ok := r.tt.Find("Line 9 sung words")
	if !ok || line.Y < box.Y+box.H-2 {
		t.Errorf("the line is not under the dots: %+v %+v", line, box)
	}
	// Two seconds on (3.75 s into the 11 s gap on the lyrics' clock) the first
	// dot is full, in the lit word's colour, and the last not begun.
	look := sheetLook()
	x0, y := box.X+look.padX, box.Y+box.H/2
	g := dotsFor(look.size)
	if mid, _ := r.litDots(x0, y, g.d, g.space); !mid[0] || mid[2] {
		t.Errorf("the dots' middles are lit %v: want the first only", mid)
	}
	// Line 9 begins: the dots clear and the sheet does not move.
	was := sh.scroll.Y
	r.play(47_000)
	for range 20 {
		r.advance(50 * time.Millisecond)
	}
	if sh.active != 9 {
		t.Fatalf("active %d at 47 s", sh.active)
	}
	if r.amberIn(box) {
		t.Error("the dots are lit when the line has begun")
	}
	if math.Abs(float64(sh.scroll.Y-was)) > 1 {
		t.Errorf("the sheet moved from %v to %v when the dots cleared", was, sh.scroll.Y)
	}
}

// The sheet's dots step by timers, not by building the view, and not in a
// loop of frames: nothing is asked of the window between steps.
func TestTheSheetsDotsStepByTimers(t *testing.T) {
	builds := 0
	r := newNPRig(t, func(r *npRig) { r.inspect = func(*ui.Context) { builds++ } })
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
	if !r.tt.HasText("Instrumental break") {
		t.Fatalf("no dots before the first line: %q", r.tt.Texts())
	}
	built, before, changes := builds, r.tt.Image(), 0
	for range 20 { // 2 s in 100 ms
		r.tick(100 * time.Millisecond)
		img := r.tt.Image()
		if !sameImage(before, img) {
			changes++
		}
		before = img
	}
	if builds-built > 2 {
		t.Errorf("the sheet was built %d times while the dots filled", builds-built)
	}
	// A dot 10 DIP across fills over 3.7 s: a pixel every few steps.
	if changes < 4 {
		t.Errorf("the dots changed in %d of 20 tenths of a second: they are not filling", changes)
	}
}

// Lyrics with no timing have no gaps, and plain ones no dots.
func TestPlainLyricsAndShortBreaksHaveNoDots(t *testing.T) {
	r := newNPRig(t)
	lines := make([]services.LyricLine, 4)
	for i := range lines {
		lines[i] = services.LyricLine{Text: fmt.Sprintf("Plain %d", i)}
	}
	r.lyr.set(services.Lyrics{Lines: lines}, nil)
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Plain 0"))
	r.settle()
	if r.tt.HasText("Instrumental break") {
		t.Error("plain lyrics have dots")
	}

	r = newNPRig(t)
	r.lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(6, false)}, nil) // 1 s between the lines
	r.play(2000)
	r.open("")
	r.tt.Until("lyrics", r.tt.Shows("Line 0 sung words"))
	r.settle()
	if r.tt.HasText("Instrumental break") {
		t.Error("lines 1 s apart have dots between them")
	}
}

// Now Playing's sheet in a gap: the dots in the lyrics, filling.
func TestNowPlayingGapGolden(t *testing.T) {
	for _, mode := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(mode.name, func(t *testing.T) {
			if err := theme.RegisterFonts(); err != nil {
				t.Fatal(err)
			}
			var opts []uitest.Option
			if mode.dark {
				opts = append(opts, uitest.Dark())
			}
			r := newNPRigWith(t, opts)
			lines := timedLines(30, true)
			const shift = 8000 // a gap of 9 s before line 4, from 16 s to 25 s
			for i := 4; i < len(lines); i++ {
				lines[i].BeginMs += shift
				lines[i].EndMs += shift
				for k := range lines[i].Words {
					lines[i].Words[k].BeginMs += shift
					lines[i].Words[k].EndMs += shift
				}
			}
			r.lyr.set(services.Lyrics{Provider: "apple", Timing: "Word", Songwriters: []string{"A Writer"}, Lines: lines}, nil)
			r.play(2000)
			r.open("")
			r.tt.Until("lyrics", r.tt.Shows("Line 0 sung words"))
			r.settle()
			// The voice rests after line 3 (16 s): the sheet turns to the dots,
			// and two seconds later (21 s) the second is three quarters full.
			r.play(19_000)
			for range 40 {
				r.advance(50 * time.Millisecond)
			}
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), "nowplaying-gap-"+mode.name)
		})
	}
}

// The dots are sized from the font of the line they lead to, and an unlit dot
// is the ink of a line still to come: never the brightest thing on screen.
func TestTheDotsAreSizedFromTheLinesFontAndUnlitInTheComingInk(t *testing.T) {
	if g := dotsFor(24); g.d != 14 || g.space != 6 {
		t.Errorf("the sheet's dots: %+v, want 14 across and 6 apart", g)
	}
	r := stageRig(t, gapLines(), 500, nil)
	st := r.p.np.stage
	b := st.block(0)
	if b.dots != dotsFor(b.size) || b.dots.d < b.size/2 {
		t.Errorf("the dots %+v are not sized from the line's %v", b.dots, b.size)
	}
	box, _ := r.tt.Find("Instrumental break")
	img := r.tt.Image()
	// The third dot is unlit: as dim as a line still to come, and well under
	// the near-white of a sung word.
	x := st.g.left + 2*(b.dots.d+b.dots.space) + b.dots.d/2
	if c := img.RGBAAt(int(x), int(box.Y+box.H/2)); c.R > 190 || c.G > 190 || c.B > 190 {
		t.Errorf("an unlit dot is %v: brighter than the coming ink", c)
	}
}
