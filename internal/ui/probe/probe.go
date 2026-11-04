//go:build probe

// Package probe is `sona -probe`: Sona measuring its own Albums scroll the
// way cmd/uiprobe measures it from outside, in a build made with `-tags
// probe` (which links MyGo's automation plugin; never a release).
//
// The probe starts the app again with MYGO_FRAME_STATS=all (MyGo reads it as
// its packages initialise) and MYGO_AUTOMATION naming a private socket. In
// that process it takes MyGo's frame statistics lines off the standard
// logger, drives the page through its own automation socket (wheel events
// at a fixed speed, as a touchpad sends them) and prints the same table as
// cmd/uiprobe (package measure), then quits. `-startup` quits once the
// covers are on screen instead.
//
//	sona -probe [-speed 2000] [-max 20000] [-runs 3] [-idle 10s] [-interval ms]
//	sona -startup
package probe

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/ui/probe/measure"
)

const (
	// coversTimeout is how long the covers may take to be drawn.
	coversTimeout = 60 * time.Second
	// shutdownGrace is how long the app has to quit after the probe.
	shutdownGrace = 15 * time.Second
)

// environment the probe needs in the process that runs the app.
const (
	envFrameStats = "MYGO_FRAME_STATS"
	envAutomation = "MYGO_AUTOMATION"
)

// Reexec runs this binary again with the environment the probe needs, with
// the same arguments, and returns its exit status. It is what a probe
// start-up does first: MyGo reads both variables as its packages
// initialise, too early for the process to set them itself. It returns
// ok=false (and does nothing) when the environment already has them.
func Reexec(args []string) (status int, ok bool, err error) {
	if os.Getenv(envFrameStats) == "all" && os.Getenv(envAutomation) != "" {
		return 0, false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return 1, true, err
	}
	env := os.Environ()
	env = append(env, envFrameStats+"=all")
	if os.Getenv(envAutomation) == "" {
		base := os.Getenv("XDG_RUNTIME_DIR")
		dir, err := os.MkdirTemp(base, "sona-probe-")
		if err != nil {
			return 1, true, err
		}
		defer func() { _ = os.RemoveAll(dir) }()
		env = append(env, envAutomation+"="+filepath.Join(dir, "automation.sock"))
	}
	cmd := exec.Command(exe, args...)
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), true, nil
		}
		return 1, true, err
	}
	return 0, true, nil
}

// Begin starts the probe in the running app: call it from MyGo's WhenReady
// callback registered at init, which runs after main installed Sona's logger
// and before the window opens. started is when the process began (taken at
// init).
func Begin(o Options, started time.Time) {
	// MyGo writes its frame statistics to the standard logger, which
	// Sona's logger bridges into sona.log: take them here instead, and
	// pass anything else on to stderr.
	rec := &measure.Recorder{Other: os.Stderr}
	log.SetFlags(0)
	log.SetOutput(rec)
	go func() {
		err := within(o.Limit(coversTimeout), func() error { return run(o, rec, started) })
		finish(err, os.Stderr, mygo.App.Quit, os.Exit, shutdownGrace)
	}()
}

func run(o Options, rec *measure.Recorder, started time.Time) error {
	c, err := measure.Dial(os.Getenv(envAutomation), 30*time.Second)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	return measure.Albums(measure.Config{
		Client: c, Frames: rec, PID: os.Getpid(),
		Speed: o.Speed, Max: o.Max, Runs: o.Runs, Idle: o.Idle, Interval: o.Interval,
		Started: started, StartupOnly: o.Startup && !o.Probe, CoversTimeout: coversTimeout,
		Out: os.Stdout, Log: os.Stderr,
	})
}
