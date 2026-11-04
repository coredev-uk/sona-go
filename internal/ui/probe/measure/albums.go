package measure

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Config is what the Albums scenario needs.
type Config struct {
	// Client drives the app; Frames reads the frames it draws; PID is the
	// process tree whose CPU and memory are read (0: none).
	Client *Client
	Frames Frames
	PID    int
	// Speed is the scroll speed in DIPs per second, Max how far down it
	// scrolls at most (the page's end otherwise), Runs how many passes
	// (down and back up) are measured.
	Speed, Max float64
	Runs       int
	// Idle is how long the process is watched at rest after the passes.
	Idle time.Duration
	// Interval is the display's frame interval in milliseconds; zero
	// measures it. Set it when the machine is loaded: the measured one
	// then includes the compositor's delays, and fewer frames count as
	// dropped.
	Interval float64
	// Quiet is how long without a frame means the page has settled (500
	// ms when zero).
	Quiet time.Duration
	// Started is when the process started, for the start-up line; zero
	// leaves the line out.
	Started time.Time
	// StartupOnly stops once the covers are on screen.
	StartupOnly bool
	// Out receives the tables, Log the per-pass lines and progress.
	Out, Log io.Writer
}

// tick is how often the scroll is advanced: each step is one wheel event
// of the distance the speed covers since the last (a touchpad's stream).
const tick = 8 * time.Millisecond

// Albums runs the Albums scroll scenario of v0.2.0's uiprobe: wait for the
// covers, measure the display interval, scroll the page down and back at
// a fixed speed Runs times, then watch the process at rest. It prints the
// start-up line and the table to cfg.Out.
func Albums(cfg Config) error {
	if cfg.Out == nil {
		cfg.Out = io.Discard
	}
	if cfg.Log == nil {
		cfg.Log = io.Discard
	}
	if cfg.Quiet == 0 {
		cfg.Quiet = 500 * time.Millisecond
	}
	s := &scenario{Config: cfg}
	return s.run()
}

type scenario struct {
	Config
	// x and y are where the wheel turns: the middle of the page.
	x, y float64
	win  Rect
	page Node
}

func (s *scenario) logf(format string, args ...any) { _, _ = fmt.Fprintf(s.Log, format+"\n", args...) }

// outf writes to the tables' writer.
func (s *scenario) outf(format string, args ...any) { _, _ = fmt.Fprintf(s.Out, format, args...) }

func (s *scenario) run() error {
	c := s.Client
	win, err := c.WaitWindow(60 * time.Second)
	if err != nil {
		return err
	}
	s.win = win
	page, err := s.waitTiles(60 * time.Second)
	if err != nil {
		return err
	}
	s.page = page
	s.x, s.y = page.Bounds.X+page.Bounds.Width/2, page.Bounds.Y+page.Bounds.Height/2

	// The covers are on screen once the tiles in view show pictures; then
	// the page settles (no frame for Quiet) before anything is measured.
	first, last, ok := s.waitCovers(60 * time.Second)
	if !ok {
		s.logf("warning: the covers were not all drawn after 60 s (offline? the artwork cache empty?)")
	}
	s.printStartup(first, last)
	if s.StartupOnly {
		return nil
	}
	if _, _, ok := s.waitQuiet(s.Quiet, 60*time.Second); !ok {
		s.logf("warning: frames kept coming for 60 s: the page never settled")
	}
	if load, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := bytes.Fields(load); len(f) >= 3 {
			s.outf("load average: %s\n", bytes.Join(f[:3], []byte(" ")))
		}
	}
	if fs := s.Frames.Since(0); len(fs) > 0 {
		s.outf("frames: %s\n", fs[len(fs)-1].Path)
	}
	s.outf("\n")

	// Start-up work still running (the audio registry scan in a child, the
	// Widevine check) is not the page's rest: wait for it to end.
	s.waitCalm(30 * time.Second)
	atRest := TreeUsage(s.PID)

	dist, err := s.pageDistance()
	if err != nil {
		return err
	}
	px := dist
	if s.Max > 0 {
		px = math.Min(px, s.Max)
	}
	interval := s.Interval
	if interval == 0 {
		if interval, err = s.displayInterval(); err != nil {
			return err
		}
	}
	s.logf("display interval %.2f ms; the page scrolls %.0f px, %.0f px each way here at %.0f px/s", interval, dist, px, s.Speed)

	var rows []Row
	// The control: the same wheel stream where nothing scrolls (over the
	// top bar). It costs the input path and the driver, not any drawing.
	if err := s.toTop(); err != nil {
		return err
	}
	ctl, err := s.pass("empty (control)", interval, px, s.x, 24)
	if err != nil {
		return err
	}
	rows = append(rows, Row{Page: "empty (control: wheel over the top bar)", Run: ctl, IdleCPU: -1})

	var runs []Run
	for i := range s.Runs {
		if err := s.toTop(); err != nil {
			return err
		}
		r, err := s.pass("albums", interval, px, s.x, s.y)
		if err != nil {
			return err
		}
		s.logf("%s", r.Line("albums", i+1))
		runs = append(runs, r)
	}

	// At rest after the scrolls: the images stay decoded, the page stays
	// built; what is left is the process's idle cost.
	if err := s.toTop(); err != nil {
		return err
	}
	time.Sleep(3 * time.Second)
	u0, t0 := TreeUsage(s.PID), time.Now()
	time.Sleep(s.Idle)
	u1 := TreeUsage(s.PID)
	idle := 100 * (u1.CPU - u0.CPU).Seconds() / time.Since(t0).Seconds()
	rows = append(rows, Row{Page: "albums", Run: MedianRun(runs), IdleCPU: idle, RSS: u1.RSS, PSS: u1.PSS})

	s.outf("%s", TableHeader)
	for _, r := range rows {
		s.outf("%s", r.Format())
	}
	m := MedianRun(runs)
	s.outf("display interval %.2f ms, %.0f px scrolled each way at %.0f px/s (%.0f ms per pass), median of %d passes\n", m.Interval, m.Px, s.Speed, float64(m.Wall.Milliseconds()), len(runs))
	if atRest.OK {
		s.outf("at rest with the covers on screen, before scrolling: RSS %d MiB, PSS %d MiB (%d processes); after the passes: RSS %d MiB, PSS %d MiB (%d processes)\n",
			atRest.RSS>>10, atRest.PSS>>10, atRest.Procs, u1.RSS>>10, u1.PSS>>10, u1.Procs)
	}
	return nil
}

// printStartup prints how long after the process start the first frame
// and the covers were on screen.
func (s *scenario) printStartup(first, last time.Time) {
	if s.Started.IsZero() || first.IsZero() {
		return
	}
	s.outf("startup (ms since process start): first frame %d, covers on screen %d\n",
		first.Sub(s.Started).Milliseconds(), last.Sub(s.Started).Milliseconds())
}

// waitTiles waits until the page has album tiles.
func (s *scenario) waitTiles(timeout time.Duration) (Node, error) {
	deadline := time.Now().Add(timeout)
	for {
		nodes, err := s.Client.Tree()
		if err != nil {
			return Node{}, err
		}
		if tiles, page := Tiles(nodes); len(tiles) > 0 {
			return page, nil
		}
		if time.Now().After(deadline) {
			return Node{}, errors.New("the Albums page showed no tiles: is the library empty, or another page open?")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// waitQuiet waits until no frame has been drawn for d, and returns the
// time of the first frame ever and of the last one. ok is false after
// timeout.
func (s *scenario) waitQuiet(d, timeout time.Duration) (first, last time.Time, ok bool) {
	deadline := time.Now().Add(timeout)
	for {
		if n := s.Frames.Count(); n > 0 {
			last = s.Frames.Since(n - 1)[0].At
			if time.Since(last) >= d {
				if first.IsZero() {
					first = s.Frames.Since(0)[0].At
				}
				return first, last, true
			}
		}
		if time.Now().After(deadline) {
			return first, last, false
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// waitCalm waits until the process tree uses almost no CPU, or timeout
// passes: start-up work (the audio registry scan in a child process, the
// Widevine check, the first sync) has finished.
func (s *scenario) waitCalm(timeout time.Duration) {
	if s.PID <= 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	for {
		u0 := TreeUsage(s.PID)
		time.Sleep(time.Second)
		u1 := TreeUsage(s.PID)
		if !u0.OK || !u1.OK || u1.CPU-u0.CPU < 20*time.Millisecond && u1.Procs == 1 {
			return
		}
		if time.Now().After(deadline) {
			s.logf("warning: the process was still busy after %s: the memory and CPU at rest include start-up work", timeout)
			return
		}
	}
}

// toTop scrolls to the page's top and lets it settle (probe.js's top():
// 300 ms).
func (s *scenario) toTop() error {
	if err := s.Client.Wheel(s.x, s.y, -1e6); err != nil {
		return err
	}
	if err := s.Client.Idle(10 * time.Second); err != nil {
		return err
	}
	s.waitQuiet(s.Quiet*3/5, 20*time.Second)
	return nil
}

var countText = regexp.MustCompile(`^(\d[\d.,\s]*)\s`)

// pageDistance is how far the page scrolls from the top to its end. The
// grid's rows are as tall as the distance between the first two; the end
// is where the last row sits after a scroll as far as it goes.
func (s *scenario) pageDistance() (float64, error) {
	if err := s.toTop(); err != nil {
		return 0, err
	}
	nodes, err := s.Client.Tree()
	if err != nil {
		return 0, err
	}
	tiles, _ := Tiles(nodes)
	ys := rowYs(tiles)
	if len(ys) < 2 {
		return 0, errors.New("the page has fewer than two rows of albums: nothing to scroll")
	}
	ys0, rowH := ys[0], ys[1]-ys[0]
	cols := 0
	for _, t := range tiles {
		if math.Abs(t.Bounds.Y-ys[0]) < 1 {
			cols++
		}
	}
	// "653 albums": the count under the title.
	total := 0
	for _, n := range nodes {
		if n.Parent == s.page.ID && n.Role == "text" && n.Bounds.Y < ys[0] {
			if m := countText.FindStringSubmatch(n.Label); m != nil {
				digits := strings.Map(func(r rune) rune {
					if r >= '0' && r <= '9' {
						return r
					}
					return -1
				}, m[1])
				if v, err := strconv.Atoi(digits); err == nil {
					total = v
					break
				}
			}
		}
	}
	if total == 0 {
		return 0, errors.New(`the page shows no "N albums" count to size its rows from`)
	}
	rows := (total + cols - 1) / cols

	if err := s.Client.Wheel(s.x, s.y, 1e6); err != nil {
		return 0, err
	}
	if err := s.Client.Idle(10 * time.Second); err != nil {
		return 0, err
	}
	s.waitQuiet(s.Quiet*3/5, 20*time.Second)
	nodes, err = s.Client.Tree()
	if err != nil {
		return 0, err
	}
	tiles, _ = Tiles(nodes)
	ys = rowYs(tiles)
	// The last row's top at the end against where it would be at the top.
	atTop := ys0 + float64(rows-1)*rowH
	return atTop - ys[len(ys)-1], nil
}

// rowYs are the distinct tile tops, ascending.
func rowYs(tiles []Node) []float64 {
	var ys []float64
	for _, t := range tiles {
		y := t.Bounds.Y
		if !slices.ContainsFunc(ys, func(v float64) bool { return math.Abs(v-y) < 1 }) {
			ys = append(ys, y)
		}
	}
	slices.Sort(ys)
	return ys
}

// displayInterval is the frame interval: the median gap between frames
// while the page moves a DIP back and forth, which draws one frame per
// display refresh.
func (s *scenario) displayInterval() (float64, error) {
	if err := s.toTop(); err != nil {
		return 0, err
	}
	mark := s.Frames.Count()
	start := time.Now()
	for i := 0; time.Since(start) < time.Second; i++ {
		d := 1.0
		if i%2 == 1 {
			d = -1
		}
		if err := s.Client.Wheel(s.x, s.y, d); err != nil {
			return 0, err
		}
		time.Sleep(tick / 2)
	}
	if err := s.Client.Idle(5 * time.Second); err != nil {
		return 0, err
	}
	gaps := Gaps(s.Frames.Since(mark))
	if len(gaps) < 5 {
		return 0, fmt.Errorf("only %d frames while measuring the display interval", len(gaps)+1)
	}
	return Median(gaps), nil
}

// pass is one measured scroll: px down and back up at the speed, the wheel
// turning at (x, y).
func (s *scenario) pass(name string, interval, px, x, y float64) (Run, error) {
	mark := s.Frames.Count()
	u0, t0 := TreeUsage(s.PID), time.Now()
	for _, sign := range []float64{1, -1} {
		start := time.Now()
		sent := 0.0
		tk := time.NewTicker(tick)
		for {
			want := math.Min(px, s.Speed*time.Since(start).Seconds())
			if d := want - sent; d > 0 {
				if err := s.Client.Wheel(x, y, sign*d); err != nil {
					tk.Stop()
					return Run{}, fmt.Errorf("%s: %w", name, err)
				}
				sent = want
			}
			if sent >= px {
				break
			}
			<-tk.C
		}
		tk.Stop()
	}
	wall := time.Since(t0)
	u1 := TreeUsage(s.PID)
	if err := s.Client.Idle(10 * time.Second); err != nil {
		return Run{}, fmt.Errorf("%s: %w", name, err)
	}
	cpu := 0.0
	if u0.OK && u1.OK {
		cpu = 100 * (u1.CPU - u0.CPU).Seconds() / wall.Seconds()
	}
	return Summarise(s.Frames.Since(mark), interval, px, wall, cpu), nil
}
