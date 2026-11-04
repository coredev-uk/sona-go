package probe

import (
	"bytes"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type exits struct {
	mu    sync.Mutex
	codes []int
}

func (e *exits) exit(code int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.codes = append(e.codes, code)
}

func (e *exits) got() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]int(nil), e.codes...)
}

func TestAFailedProbeQuitsTheAppAndExitsNonZero(t *testing.T) {
	var out bytes.Buffer
	var e exits
	quit := 0
	t.Cleanup(func() { status.Store(0) })
	// The app shuts down by itself in time: main then exits with Status.
	finish(errors.New("the covers were not all drawn after 60s"), &out, func() { quit++ }, e.exit, time.Hour)
	if quit != 1 || Status() != 1 || len(e.got()) != 0 {
		t.Errorf("quit %d, status %d, exits %v", quit, Status(), e.got())
	}
	// The app never ends by itself: the probe still ends the process.
	var e2 exits
	finish(errors.New("again"), &out, func() {}, e2.exit, 10*time.Millisecond)
	deadline := time.Now().Add(2 * time.Second)
	for len(e2.got()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := e2.got(); len(got) != 1 || got[0] != 1 {
		t.Errorf("exit codes = %v, want [1]", got)
	}
	if !strings.Contains(out.String(), "sona -probe: the covers were not all drawn") {
		t.Errorf("output = %q", out.String())
	}
}

func TestASuccessfulProbeLeavesTheExitToTheApp(t *testing.T) {
	var out bytes.Buffer
	var e exits
	// The app shuts down: the process ends by itself before the watchdog.
	finish(nil, &out, func() {}, e.exit, time.Hour)
	if got := e.got(); len(got) != 0 || Status() != 0 {
		t.Errorf("exited at once: %v", got)
	}
	// An app that does not shut down is stopped with a failure.
	var e2 exits
	finish(nil, &out, func() {}, e2.exit, 20*time.Millisecond)
	deadline := time.Now().Add(2 * time.Second)
	for len(e2.got()) == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := e2.got(); len(got) != 1 || got[0] != 1 {
		t.Errorf("exit codes = %v, want [1]", got)
	}
	if !strings.Contains(out.String(), "did not shut down") {
		t.Errorf("output = %q", out.String())
	}
}

func TestWithinGivesUpOnAProbeThatNeverReturns(t *testing.T) {
	if err := within(time.Second, func() error { return nil }); err != nil {
		t.Errorf("finished probe: %v", err)
	}
	want := errors.New("boom")
	if err := within(time.Second, func() error { return want }); !errors.Is(err, want) {
		t.Errorf("failed probe: %v", err)
	}
	block := make(chan struct{})
	defer close(block)
	if err := within(20*time.Millisecond, func() error { <-block; return nil }); !errors.Is(err, errTimeout) {
		t.Errorf("hung probe: %v", err)
	}
}

func TestLimitCoversEveryPhase(t *testing.T) {
	o := Options{Speed: 2000, Max: 20000, Runs: 3, Idle: 10 * time.Second}
	// Passes of 20 s plus a minute each, the covers, the rest and 3 minutes.
	want := 60*time.Second + 3*(20*time.Second+time.Minute) + 10*time.Second + 3*time.Minute
	if got := o.Limit(60 * time.Second); got != want {
		t.Errorf("limit = %v, want %v", got, want)
	}
}
