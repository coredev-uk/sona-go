package measure

import (
	"fmt"
	"slices"
	"time"
)

// Run is one scroll pass (down and back up) summarised.
type Run struct {
	// Interval is the display's frame interval in milliseconds.
	Interval float64
	// Px is how far the page scrolled each way.
	Px float64
	// Frames is how many frames were presented; Dropped how many the
	// display interval says were missed (a gap of n intervals is n-1).
	Frames, Dropped int
	// Long counts the frames more than 50 ms after the one before.
	Long int
	// P50, P95 and Max are of the gaps between frames, in milliseconds.
	P50, P95, Max float64
	// WorkP50, WorkP95 are of the CPU time to build, lay out and paint a
	// frame, PresentP50 of the time to present it.
	WorkP50, WorkP95, PresentP50 float64
	// Wall is how long the pass took; CPU the process tree's CPU use over
	// it, in percent of one core.
	Wall time.Duration
	CPU  float64
}

// Gaps are the milliseconds between consecutive frames.
func Gaps(fs []Frame) []float64 {
	if len(fs) < 2 {
		return nil
	}
	out := make([]float64, len(fs)-1)
	for i := 1; i < len(fs); i++ {
		out[i-1] = float64(fs[i].At.Sub(fs[i-1].At).Microseconds()) / 1000
	}
	return out
}

// Quantile is the q-quantile of v (0 for none).
func Quantile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	return s[min(len(s)-1, int(q*float64(len(s)-1)+0.5))]
}

// Median is the median of v.
func Median(v []float64) float64 { return Quantile(v, 0.5) }

// Summarise reads a scroll pass's frames against the display's frame
// interval in milliseconds.
func Summarise(fs []Frame, interval, px float64, wall time.Duration, cpu float64) Run {
	gaps := Gaps(fs)
	r := Run{Interval: interval, Px: px, Frames: len(gaps), Wall: wall, CPU: cpu}
	r.P50, r.P95, r.Max = Quantile(gaps, 0.5), Quantile(gaps, 0.95), Quantile(gaps, 1)
	for _, g := range gaps {
		if interval > 0 && g > interval*1.5 {
			r.Dropped += int(g/interval+0.5) - 1
		}
		if g > 50 {
			r.Long++
		}
	}
	work := make([]float64, len(fs))
	present := make([]float64, len(fs))
	for i, f := range fs {
		work[i], present[i] = f.Work(), f.Present
	}
	r.WorkP50, r.WorkP95, r.PresentP50 = Quantile(work, 0.5), Quantile(work, 0.95), Quantile(present, 0.5)
	return r
}

// MedianRun takes each field's median over the runs.
func MedianRun(rs []Run) Run {
	pick := func(f func(Run) float64) float64 {
		v := make([]float64, len(rs))
		for i, r := range rs {
			v[i] = f(r)
		}
		return Median(v)
	}
	return Run{
		Interval:   pick(func(r Run) float64 { return r.Interval }),
		Px:         pick(func(r Run) float64 { return r.Px }),
		Frames:     int(pick(func(r Run) float64 { return float64(r.Frames) })),
		Dropped:    int(pick(func(r Run) float64 { return float64(r.Dropped) })),
		Long:       int(pick(func(r Run) float64 { return float64(r.Long) })),
		P50:        pick(func(r Run) float64 { return r.P50 }),
		P95:        pick(func(r Run) float64 { return r.P95 }),
		Max:        pick(func(r Run) float64 { return r.Max }),
		WorkP50:    pick(func(r Run) float64 { return r.WorkP50 }),
		WorkP95:    pick(func(r Run) float64 { return r.WorkP95 }),
		PresentP50: pick(func(r Run) float64 { return r.PresentP50 }),
		Wall:       time.Duration(pick(func(r Run) float64 { return float64(r.Wall) })),
		CPU:        pick(func(r Run) float64 { return r.CPU }),
	}
}

// Row is a line of the table: one page's median scroll, and the process at
// rest after it.
type Row struct {
	Page string
	Run  Run
	// IdleCPU is the process tree's CPU at rest, percent of one core
	// (negative: not measured).
	IdleCPU float64
	// RSS and PSS are the process tree's, in KiB.
	RSS, PSS int
}

// TableHeader is the table's first two lines; the columns are v0.2.0's
// uiprobe table where a native UI has the same thing (frame gaps, dropped
// frames, frames over 50 ms, process CPU while scrolling), and MyGo's own
// frame stages where only it has them.
const TableHeader = "| page | frame p50 | frame p95 | dropped frames | frames > 50 ms | process CPU (scrolling) | CPU work per frame p50 / p95 | present p50 | idle CPU | RSS | PSS |\n" +
	"|---|---|---|---|---|---|---|---|---|---|---|\n"

// Format is the row's table line. A page with no frames (the control)
// shows dashes where frames would be.
func (r Row) Format() string {
	m := r.Run
	mem := fmt.Sprintf("%d MiB | %d MiB", r.RSS>>10, r.PSS>>10)
	if r.RSS == 0 && r.PSS == 0 {
		mem = "– | –"
	}
	idle := "–"
	if r.IdleCPU >= 0 {
		idle = fmt.Sprintf("%.1f%%", r.IdleCPU)
	}
	if m.Frames < 10 { // the control: a few frames say nothing about gaps
		return fmt.Sprintf("| %s | – | – | – | – | %.0f%% | – | – | %s | %s |\n", r.Page, m.CPU, idle, mem)
	}
	return fmt.Sprintf("| %s | %.1f ms | %.1f ms | %d of %d (%.1f%%) | %d | %.0f%% | %.2f / %.2f ms | %.2f ms | %s | %s |\n",
		r.Page, m.P50, m.P95, m.Dropped, m.Frames+m.Dropped, 100*float64(m.Dropped)/float64(max(1, m.Frames+m.Dropped)),
		m.Long, m.CPU, m.WorkP50, m.WorkP95, m.PresentP50, idle, mem)
}

// Line is the run's one-line summary (stderr, per pass).
func (r Run) Line(name string, n int) string {
	return fmt.Sprintf("%s run %d: %.0f px in %d ms, interval %.2f ms, p50 %.1f p95 %.1f max %.1f, dropped %d of %d, CPU %.0f%%, CPU work per frame p50 %.2f p95 %.2f ms, present p50 %.2f ms",
		name, n, r.Px, r.Wall.Milliseconds(), r.Interval, r.P50, r.P95, r.Max, r.Dropped, r.Frames+r.Dropped, r.CPU, r.WorkP50, r.WorkP95, r.PresentP50)
}
