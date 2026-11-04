package probe

import (
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"time"
)

// status is the exit status of a failed probe, for main to exit with once
// the app has shut down.
var status atomic.Int32

// Status is the exit status the process should end with: 1 after a failed
// probe, else 0. main reads it when the app's run returns, so a probe that
// failed still lets the app shut down cleanly (the log's "shutdown
// complete", the queue saved) and ends non-zero.
func Status() int { return int(status.Load()) }

// errTimeout is a probe that ran past its overall limit.
var errTimeout = errors.New("the probe did not finish in time")

// within runs fn and returns its error, or errTimeout when it has not
// returned after limit (fn is left running: the process is about to end).
func within(limit time.Duration, fn func() error) error {
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(limit):
		return errTimeout
	}
}

// Limit is how long a probe of o may take in all: the covers, the passes
// at the speed over the distance down and up, what each does besides
// scrolling, the rest period, and room for a slow machine.
func (o Options) Limit(covers time.Duration) time.Duration {
	pass := time.Duration(2*o.Max/o.Speed*float64(time.Second)) + time.Minute
	return covers + time.Duration(o.Runs)*pass + o.Idle + 3*time.Minute
}

// finish ends the process the probe ran in: it reports err, asks the app to
// quit, and makes sure the process is gone. A failed probe sets Status to 1
// (main exits with it once the app has shut down) and is stopped with
// status 1 if the app has not shut down after grace; a successful one
// exits by itself when the app has quit, and is stopped with status 1 if
// the app has not shut down after grace. exit is os.Exit.
func finish(err error, w io.Writer, quit func(), exit func(int), grace time.Duration) {
	if err != nil {
		_, _ = fmt.Fprintln(w, "sona -probe:", err)
		status.Store(1)
	}
	quit()
	time.AfterFunc(grace, func() {
		_, _ = fmt.Fprintln(w, "sona -probe: the app did not shut down after the probe")
		exit(1)
	})
}
