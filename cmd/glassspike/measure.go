//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/probe/measure"
)

// interval is a 60 Hz display's frame interval in milliseconds: a runner's
// virtual display says nothing better, and the same figure is used with and
// without glass.
const interval = 16.67

// measureAll runs the scenarios of the issue, with the glass off and on,
// interleaved so a drifting machine hits both alike: the Albums grid scroll
// and rest (cmd/uiprobe, the table the repo already uses), the same grid at
// rest with a song playing (the player bar's clock ticks over the glass), and
// Now Playing with the lyrics running.
func measureAll(reps int) {
	setAppearance(false)
	setAccessibility("reduceTransparency", false)
	setAccessibility("reduceMotion", false)
	rows := map[string][]string{}
	for rep := 1; rep <= reps; rep++ {
		for _, v := range []variant{{name: "off", glass: "0"}, {name: "glass", glass: "1"}} {
			v.measure = true
			logf("=== rep %d, %s: albums grid", rep, v.name)
			albumsRun(v, rep)
			for _, kind := range []string{"idle", "playing", "lyrics"} {
				logf("=== rep %d, %s: %s", rep, v.name, kind)
				if row := watchRun(v, rep, kind); row != "" {
					rows[kind] = append(rows[kind], row)
				}
			}
		}
	}
	for _, kind := range []string{"idle", "playing", "lyrics"} {
		table := "| run | glass | frames | frames/s | frame time p50 / p95 / max | frames over 16.7 ms | dropped (gaps of an animation) | app CPU | WindowServer CPU | drawn |\n|---|---|---|---|---|---|---|---|---|---|\n" + strings.Join(rows[kind], "")
		_ = os.WriteFile(filepath.Join(outDir, kind+"-table.md"), []byte(table), 0o644)
		logf("%s:\n%s", kind, table)
	}
}

// windowServerPID is the process that composites the screen: 0 if not found.
func windowServerPID() int {
	out, err := exec.Command("pgrep", "-x", "WindowServer").Output()
	if err != nil || len(strings.Fields(string(out))) == 0 {
		return 0
	}
	var pid int
	_, _ = fmt.Sscan(strings.Fields(string(out))[0], &pid)
	return pid
}

// albumsRun is `uiprobe` on the Albums grid: scroll passes at a fixed speed,
// then the process at rest.
func albumsRun(v variant, rep int) {
	v.name = fmt.Sprintf("albums-%s-%d", v.name, rep)
	s, err := startSession(v)
	if err != nil {
		logf("%s: start: %v", v.name, err)
		return
	}
	defer s.stop()
	if _, err := s.c.Wait(60*time.Second, "the Albums page", func(t autoTree) bool { return t.HasText("24 albums") }); !s.must("albums page", err) {
		return
	}
	s.activate()
	time.Sleep(3 * time.Second)
	// The app serves one automation client: uiprobe takes the socket over.
	_ = s.c.Close()
	s.c = nil
	args := []string{"-socket", s.sock, "-log", s.paths.LogFile(), "-runs", "5", "-speed", "600", "-idle", "15s", "-interval", fmt.Sprint(interval)}
	cmd := exec.Command(uiprobeBin, args...)
	out, err := cmd.CombinedOutput()
	_ = os.WriteFile(filepath.Join(outDir, v.name+".txt"), out, 0o644)
	if err != nil {
		logf("%s: uiprobe: %v\n%s", v.name, err, out)
		return
	}
	logf("%s: uiprobe:\n%s", v.name, out)
}

// watchRun plays the fixture's last album and watches the frames and the CPU
// for 20 s: kind "playing" on the Albums grid (the bar's clock ticks under
// the glass, nothing else moves), kind "lyrics" in Now Playing with the sung
// line moving.
func watchRun(v variant, rep int, kind string) string {
	v.name = fmt.Sprintf("%s-%s-%d", kind, v.name, rep)
	s, err := startSession(v)
	if err != nil {
		logf("%s: start: %v", v.name, err)
		return ""
	}
	defer s.stop()
	if _, err := s.c.Wait(60*time.Second, "the Albums page", func(t autoTree) bool { return t.HasText("24 albums") }); !s.must("albums page", err) {
		return ""
	}
	s.activate()
	time.Sleep(3 * time.Second)
	if kind != "idle" && !s.startPlaying() {
		return ""
	}
	switch kind {
	case "lyrics":
		s.key("N")
		s.pressRole("tab", "Lyrics")
		_, _ = s.c.Wait(10*time.Second, "the lyrics", func(t autoTree) bool { return t.HasText("Streetlights hum a borrowed tune") })
	case "playing":
		s.press("Back")
		_, _ = s.c.Wait(20*time.Second, "the Albums page", func(t autoTree) bool { return t.HasText("24 albums") })
	}
	_ = s.c.Close() // nothing talks to the app while it is watched
	s.c = nil
	time.Sleep(6 * time.Second)

	pid := s.cmd.Process.Pid
	var off int64
	if fi, err := os.Stat(s.paths.LogFile()); err == nil {
		off = fi.Size()
	}
	frames := measure.Follow(s.paths.LogFile(), off)
	const watch = 20 * time.Second
	// The window server and the compositor do the glass's last steps, and
	// the system's own glass entirely: their CPU is not the app's.
	ws := windowServerPID()
	u0, w0, t0 := measure.TreeUsage(pid), measure.TreeUsage(ws), time.Now()
	time.Sleep(watch)
	u1, w1, wall := measure.TreeUsage(pid), measure.TreeUsage(ws), time.Since(t0)
	wsCPU := 0.0
	if w0.OK && w1.OK {
		wsCPU = 100 * (w1.CPU - w0.CPU).Seconds() / wall.Seconds()
	}
	fs := frames.Since(0)
	cpu := 0.0
	if u0.OK && u1.OK {
		cpu = 100 * (u1.CPU - u0.CPU).Seconds() / wall.Seconds()
	}
	var took []float64
	slow, dropped := 0, 0
	path := "no frames"
	for _, f := range fs {
		took = append(took, f.Took)
		if f.Took > interval {
			slow++
		}
		path = f.Path
	}
	// A gap of 1.5 intervals up to 250 ms inside an animation is frames
	// missed; a longer gap is the window at rest between two changes.
	for _, g := range measure.Gaps(fs) {
		if g > interval*1.5 && g <= 250 {
			dropped += int(g/interval+0.5) - 1
		}
	}
	mx := 0.0
	if len(took) > 0 {
		mx = slices.Max(took)
	}
	glassCol := "off"
	if v.glass != "0" {
		glassCol = "glass"
	}
	if i := strings.Index(path, ":"); i > 0 {
		path = path[:i]
	}
	return fmt.Sprintf("| %d | %s | %d | %.1f | %.1f / %.1f / %.1f ms | %d | %d | %.1f%% | %.1f%% | %s |\n",
		rep, glassCol, len(fs), float64(len(fs))/wall.Seconds(), measure.Quantile(took, 0.5), measure.Quantile(took, 0.95), mx, slow, dropped, cpu, wsCPU, path)
}
