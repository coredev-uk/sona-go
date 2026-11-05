package measure

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	frameA = "mygo: frame 412 took 17.3 ms: build 3.1 (2 passes), layout 1.2, paint 0.8, present 11.5 (drawn on the GPU), other 0.7; meanwhile the process made about 2312 allocations, 171.0 KB, 12 text layouts; GC: 1 cycles ended, 4.1 ms of assists"
	frameB = "mygo: frame 7 took 8.6 ms: build 0.5, layout 0.3, paint 0.3, present 7.4 (drawn in memory), other 0.1; meanwhile the process made about 26 allocations, 7.3 KB, 0 text layouts; no GC"
	frameC = "mygo: frame 8 took 1.1 ms: build 0.4 (painted again), layout 0.2, paint 0.2, present 0.1 (drawn in memory), other 0.2; meanwhile the process made about 29 allocations, 8.2 KB, 0 text layouts; no GC"
)

func TestParseFrame(t *testing.T) {
	at := time.Unix(100, 0)
	for _, c := range []struct {
		line                string
		n                   int
		took, work, present float64
		path                string
	}{
		{frameA, 412, 17.3, 5.1, 11.5, "drawn on the GPU"},
		{"2026/11/04 02:00:00 " + frameB, 7, 8.6, 1.1, 7.4, "drawn in memory"},
		{frameC, 8, 1.1, 0.8, 0.1, "drawn in memory"},
	} {
		f, ok := ParseFrame(c.line, at)
		if !ok {
			t.Fatalf("not a frame: %s", c.line)
		}
		if f.N != c.n || f.Took != c.took || f.Present != c.present || f.Path != c.path || !f.At.Equal(at) {
			t.Errorf("%s: got %+v", c.line, f)
		}
		if d := f.Work() - c.work; d > 1e-9 || d < -1e-9 {
			t.Errorf("%s: work %v, want %v", c.line, f.Work(), c.work)
		}
	}
	if _, ok := ParseFrame("startup: first paint", at); ok {
		t.Error("a log line that is not a frame parsed as one")
	}
}

type lines struct{ strings.Builder }

func TestRecorderTakesFramesAndPassesTheRestOn(t *testing.T) {
	var other lines
	r := &Recorder{Other: &other}
	for _, line := range []string{frameB + "\n", "mygo: something else\n", frameC} {
		if _, err := r.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	if r.Count() != 2 {
		t.Fatalf("frames = %d, want 2", r.Count())
	}
	if got := r.Since(1); len(got) != 1 || got[0].N != 8 {
		t.Errorf("Since(1) = %+v", got)
	}
	if got := r.Since(9); len(got) != 0 {
		t.Errorf("Since past the end = %+v", got)
	}
	if other.String() != "mygo: something else\n" {
		t.Errorf("other = %q", other.String())
	}
}

func jsonLine(at time.Time, msg string, extra string) string {
	return `{"time":"` + at.Format(time.RFC3339Nano) + `","level":"INFO","msg":"` + msg + `"` + extra + "}\n"
}

func TestLogFramesFollowsTheLastRun(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sona.log")
	t0 := time.Date(2026, 11, 4, 2, 0, 0, 0, time.UTC)
	old := jsonLine(t0, "startup: process start", `,"pid":10`) + jsonLine(t0.Add(time.Second), frameA, "")
	cur := jsonLine(t0.Add(time.Minute), "startup: process start", `,"pid":42`) + jsonLine(t0.Add(time.Minute+time.Second), frameB, "")
	if err := os.WriteFile(path, []byte(old+cur), 0o600); err != nil {
		t.Fatal(err)
	}
	at, pid, off, err := LastStart(path)
	if err != nil {
		t.Fatal(err)
	}
	if pid != 42 || !at.Equal(t0.Add(time.Minute)) || off != int64(len(old)) {
		t.Fatalf("LastStart = %v, %d, %d (want %d)", at, pid, off, len(old))
	}
	f := Follow(path, off)
	if n := f.Count(); n != 1 {
		t.Fatalf("frames of the last run = %d, want 1", n)
	}
	// A line written in two parts is read whole.
	next := jsonLine(t0.Add(time.Minute+2*time.Second), frameC, "")
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fh.Close() }()
	if _, err := fh.WriteString(next[:40]); err != nil {
		t.Fatal(err)
	}
	if n := f.Count(); n != 1 {
		t.Fatalf("a half line counted: %d", n)
	}
	if _, err := fh.WriteString(next[40:]); err != nil {
		t.Fatal(err)
	}
	got := f.Since(1)
	if len(got) != 1 || got[0].N != 8 || !got[0].At.Equal(t0.Add(time.Minute+2*time.Second)) {
		t.Fatalf("Since(1) = %+v", got)
	}
	// A rotated log starts over.
	if err := os.WriteFile(path, []byte(jsonLine(t0, frameB, "")), 0o600); err != nil {
		t.Fatal(err)
	}
	if n := f.Count(); n != 3 {
		t.Errorf("after rotation = %d frames, want 3", n)
	}
	if _, _, _, err := LastStart(filepath.Join(dir, "missing")); err == nil {
		t.Error("a missing log has a start")
	}
}

func frames(at time.Time, gapsMS ...float64) []Frame {
	out := []Frame{{At: at, Build: 1, Layout: 0.5, Paint: 0.5, Present: 5}}
	for _, g := range gapsMS {
		at = at.Add(time.Duration(g * float64(time.Millisecond)))
		out = append(out, Frame{At: at, Build: 1, Layout: 0.5, Paint: 0.5, Present: 5})
	}
	return out
}

func TestSummariseCountsDroppedFramesLikeV020(t *testing.T) {
	// Interval 16 ms: budget 24 ms. A 50 ms gap is round(50/16)-1 = 2 dropped
	// frames, a 100 ms gap 5, a 24 ms gap none; gaps over 50 ms are long.
	fs := frames(time.Unix(0, 0), 16, 16, 24, 50, 16, 100, 16)
	r := Summarise(fs, 16, 1000, 2*time.Second, 120)
	if r.Frames != 7 || r.Dropped != 2+5 || r.Long != 1 {
		t.Errorf("frames %d dropped %d long %d, want 7, 7, 1", r.Frames, r.Dropped, r.Long)
	}
	if r.P50 != 16 || r.Max != 100 {
		t.Errorf("p50 %v max %v", r.P50, r.Max)
	}
	if r.WorkP50 != 2 || r.PresentP50 != 5 || r.CPU != 120 {
		t.Errorf("work %v present %v cpu %v", r.WorkP50, r.PresentP50, r.CPU)
	}
	if got := Summarise(nil, 16, 0, 0, 0); got.Frames != 0 || got.P50 != 0 {
		t.Errorf("no frames: %+v", got)
	}
}

func TestMedianRunAndRows(t *testing.T) {
	m := MedianRun([]Run{{P50: 16, Dropped: 30, Frames: 100}, {P50: 18, Dropped: 2, Frames: 110}, {P50: 17, Dropped: 5, Frames: 120}})
	if m.P50 != 17 || m.Dropped != 5 || m.Frames != 110 {
		t.Errorf("median = %+v", m)
	}
	row := Row{Page: "albums", Run: Run{P50: 16.1, P95: 18.8, Dropped: 4, Frames: 431, Long: 0, CPU: 277, WorkP50: 0.9, WorkP95: 1.6, PresentP50: 6.3}, IdleCPU: 0.04, RSS: 231 << 10, PSS: 134 << 10}
	want := "| albums | 16.1 ms | 18.8 ms | 4 of 435 (0.9%) | 0 | 277% | 0.90 / 1.60 ms | 6.30 ms | 0.0% | 231 MiB | 134 MiB |\n"
	if got := row.Format(); got != want {
		t.Errorf("row = %q, want %q", got, want)
	}
	control := Row{Page: "empty", Run: Run{Frames: 3, P50: 60, CPU: 5}, IdleCPU: -1}
	if got, want := control.Format(), "| empty | – | – | – | – | 5% | – | – | – | – | – |\n"; got != want {
		t.Errorf("control = %q, want %q", got, want)
	}
	if !strings.HasPrefix(TableHeader, "| page |") || strings.Count(TableHeader, "|---") != 11 {
		t.Errorf("header = %q", TableHeader)
	}
}

func TestTreeUsageOfThisProcess(t *testing.T) {
	u := TreeUsage(os.Getpid())
	if !u.OK {
		t.Skip("no /proc")
	}
	if u.Procs < 1 || u.RSS <= 0 || u.PSS <= 0 {
		t.Errorf("usage = %+v", u)
	}
	if TreeUsage(0).OK {
		t.Error("pid 0 reported usage")
	}
}

func TestIntervalWarning(t *testing.T) {
	for _, c := range []struct {
		ms   float64
		warn bool
	}{{16.67, false}, {16.0, false}, {15.1, false}, {18.3, false}, {14.9, true}, {18.4, true}, {33.3, true}, {6.94, true}, {0, true}} {
		if got := IntervalWarning(c.ms, "test") != ""; got != c.warn {
			t.Errorf("IntervalWarning(%v) warns = %v, want %v", c.ms, got, c.warn)
		}
	}
}

const swayTreeJSON = `{"type":"root","nodes":[
 {"type":"output","name":"__i3","nodes":[{"type":"workspace","name":"__i3_scratch","nodes":[]}]},
 {"type":"output","name":"HEADLESS-1","nodes":[{"type":"workspace","name":"1","nodes":[{"type":"con","pid":111,"app_id":"other"}],
   "floating_nodes":[{"type":"floating_con","pid":222,"app_id":"dev.coredev.Sona"}]}]},
 {"type":"output","name":"HEADLESS-2","nodes":[{"type":"workspace","name":"2","nodes":[{"type":"con","pid":333,"app_id":"dev.coredev.Sona"}]}]}]}`

const swayOutputsJSON = `[
 {"name":"__i3","active":true,"current_mode":{"refresh":0}},
 {"name":"HEADLESS-1","active":true,"current_mode":{"width":2560,"height":1440,"refresh":60000}},
 {"name":"HEADLESS-2","active":true,"current_mode":{"width":1920,"height":1080,"refresh":144000}},
 {"name":"OFF","active":false,"current_mode":{"refresh":30000}}]`

func TestTheIntervalIsTheRefreshOfTheOutputTheWindowIsOn(t *testing.T) {
	var tree swayNode
	if err := json.Unmarshal([]byte(swayTreeJSON), &tree); err != nil {
		t.Fatal(err)
	}
	var outputs []swayOutput
	if err := json.Unmarshal([]byte(swayOutputsJSON), &outputs); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		pid  int
		out  string
		want float64
	}{{222, "HEADLESS-1", 16.667}, {333, "HEADLESS-2", 6.944}} {
		out := windowOutput(tree, c.pid, AppID)
		if out != c.out {
			t.Fatalf("pid %d is on %q, want %q", c.pid, out, c.out)
		}
		ms, err := outputInterval(outputs, out)
		if err != nil || ms < c.want-0.001 || ms > c.want+0.001 {
			t.Errorf("%s: interval %v, %v; want %v", out, ms, err, c.want)
		}
	}
	// With no pid the app id finds the first window; an unknown pid finds none.
	if out := windowOutput(tree, 0, AppID); out != "HEADLESS-1" {
		t.Errorf("by app id: %q", out)
	}
	if out := windowOutput(tree, 999, AppID); out != "" {
		t.Errorf("unknown pid: %q", out)
	}
	// Without a window to place, several outputs are ambiguous; one is not.
	if _, err := outputInterval(outputs, ""); err == nil {
		t.Error("three active outputs and no window gave an interval")
	}
	if ms, err := outputInterval(outputs[1:2], ""); err != nil || ms < 16.66 || ms > 16.67 {
		t.Errorf("the only output: %v, %v", ms, err)
	}
	if _, err := outputInterval(outputs[:1], ""); err == nil {
		t.Error("an output with no refresh gave an interval")
	}
}

func TestSwayIntervalOverIPC(t *testing.T) {
	path := filepath.Join(socketDir(t), "sway.sock")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			hdr := make([]byte, 14)
			if _, err := io.ReadFull(c, hdr); err == nil {
				body := swayOutputsJSON
				if binary.NativeEndian.Uint32(hdr[10:]) == 4 {
					body = swayTreeJSON
				}
				out := append([]byte("i3-ipc"), make([]byte, 8)...)
				binary.NativeEndian.PutUint32(out[6:], uint32(len(body)))
				_, _ = c.Write(append(out, body...))
			}
			_ = c.Close()
		}
	}()
	ms, err := swayInterval(path, 333)
	if err != nil || ms < 6.94 || ms > 6.95 {
		t.Errorf("interval = %v, %v; want 144 Hz's 6.944 ms", ms, err)
	}
	if _, err := swayInterval(filepath.Join(t.TempDir(), "none"), 1); err == nil {
		t.Error("a missing socket gave an interval")
	}
}

// socketDir is a folder for a test's socket. Not t.TempDir: its long
// names can exceed the 104 bytes macOS allows a socket path.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sona")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
