// Command uiprobe measures Sona's native UI on a running probe build: the
// Albums scroll (frame gaps, dropped frames, process CPU, memory, CPU at
// rest) and the start-up. It drives the app through MyGo's automation
// socket, as a touchpad does (wheel events at a fixed speed, down the whole
// page and back), and reads the frame times of MyGo's own statistics from
// the app's log: the same table `sona -probe` prints in-process and that
// v0.2.0's uiprobe printed for the web page, so before and after compare.
//
// The app is a probe build (`go build -tags probe,sqlite_fts5 ./cmd/sona`)
// started with MYGO_AUTOMATION=<socket> and MYGO_FRAME_STATS=all, which put
// the frame statistics in <state>/sona.log:
//
//	MYGO_AUTOMATION=/tmp/sona.sock MYGO_FRAME_STATS=all SONA_PRIVATE=1 bin/sona-probe &
//	uiprobe -socket /tmp/sona.sock -log $XDG_STATE_HOME/sona/sona.log [-runs 3] [-speed 2000] [-max 20000] [-idle 10s] [-interval ms]
//	uiprobe -startup -socket … -log …
//	uiprobe -firstpaint run1/sona.log run2/sona.log …
//
// Not measured yet: Home, New and the motion artwork scenarios of v0.2.0's
// uiprobe. They need those pages (the album page, Now Playing, Home and
// Discover); the probe grows them as the pages land.
//
// How to run it headless: CONTRIBUTING.md, "Measuring UI performance".
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/probe/measure"
)

func main() {
	socket := flag.String("socket", os.Getenv("MYGO_AUTOMATION"), "the app's automation socket (MYGO_AUTOMATION)")
	logFile := flag.String("log", "", "the app's sona.log, where MYGO_FRAME_STATS=all writes the frame times (required)")
	pid := flag.Int("pid", 0, "the app's process ID, to read its and its children's CPU time and memory (default: the one in the log)")
	runs := flag.Int("runs", 3, "scroll passes (down and back up)")
	speed := flag.Float64("speed", 2000, "scroll speed in DIPs per second")
	maxPx := flag.Float64("max", 20000, "scroll at most this far down (DIPs); the page's end if nearer")
	idle := flag.Duration("idle", 10*time.Second, "how long the process is watched at rest after the passes")
	interval := flag.Float64("interval", 0, "the display's frame interval in ms (default: measured; set it, e.g. 16.67, when the machine is loaded)")
	startup := flag.Bool("startup", false, "only report when the first frame and the covers were on screen, since the process start")
	firstPaintOnly := flag.Bool("firstpaint", false, "summarise `startup: first paint` from the sona.log files given as arguments")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: uiprobe -socket PATH -log sona.log [-pid PID] [-runs 3] [-speed 2000] [-max 20000] [-idle 10s] [-interval ms] [-startup]")
		fmt.Fprintln(os.Stderr, "       uiprobe -firstpaint run1/sona.log run2/sona.log …")
		fmt.Fprintln(os.Stderr, "Measures the Albums scroll of a probe build started with MYGO_AUTOMATION and MYGO_FRAME_STATS=all.")
		fmt.Fprintln(os.Stderr, "Home, New and the motion artwork scenarios of v0.2.0's uiprobe come back once those pages exist.")
		flag.PrintDefaults()
	}
	flag.Parse()

	var err error
	if *firstPaintOnly {
		err = summariseFirstPaint(flag.Args())
	} else {
		err = probe(*socket, *logFile, *pid, measure.Config{
			Runs: *runs, Speed: *speed, Max: *maxPx, Idle: *idle, Interval: *interval, StartupOnly: *startup,
			Out: os.Stdout, Log: os.Stderr,
		})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "uiprobe:", err)
		os.Exit(1)
	}
}

// probe runs the Albums scenario against the app on socket, reading frames
// from its log.
func probe(socket, logFile string, pid int, cfg measure.Config) error {
	if socket == "" {
		return fmt.Errorf("-socket: the app's MYGO_AUTOMATION socket")
	}
	if logFile == "" {
		return fmt.Errorf("-log: the app's sona.log, where MYGO_FRAME_STATS=all writes the frame times")
	}
	// The frames of the run the log's last start line begins.
	started, logPID, off, err := measure.LastStart(logFile)
	if err != nil {
		return err
	}
	cfg.Started = started
	if pid == 0 {
		pid = logPID
	}
	cfg.PID = pid
	cfg.Frames = measure.Follow(logFile, off)
	c, err := measure.Dial(socket, 10*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	cfg.Client = c
	return measure.Albums(cfg)
}
