//go:build darwin || windows

package main

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestInhibitorHoldsOneAssertionAtATime(t *testing.T) {
	var reasons []string
	held := 0
	i := newDesktopInhibitor(func() string { return "Playing music" })
	i.take = func(reason string) func() {
		reasons = append(reasons, reason)
		held++
		return func() { held-- }
	}
	if i.Backend() != "" {
		t.Fatalf("before Acquire: %q", i.Backend())
	}
	for range 2 { // a second Acquire holds the same assertion
		if err := i.Acquire(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if held != 1 || !slices.Equal(reasons, []string{"Playing music"}) || i.Backend() != backendPowerAssertion {
		t.Fatalf("held=%d reasons=%v backend=%q", held, reasons, i.Backend())
	}
	for range 2 { // releasing twice is harmless
		if err := i.Release(); err != nil {
			t.Fatal(err)
		}
	}
	if held != 0 || i.Backend() != "" {
		t.Fatalf("after Release: held=%d backend=%q", held, i.Backend())
	}
	// The reason is read each time it is taken, so it follows the language.
	i.why = func() string { return "Musique en cours" }
	_ = i.Acquire(context.Background())
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if held != 0 || !slices.Equal(reasons, []string{"Playing music", "Musique en cours"}) {
		t.Fatalf("after Close: held=%d reasons=%v", held, reasons)
	}
}

func TestInhibitorWithAnEndedContextHoldsNothing(t *testing.T) {
	i := newDesktopInhibitor(func() string { return "" })
	i.take = func(string) func() { t.Error("took the assertion"); return func() {} }
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := i.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire = %v", err)
	}
}

// A system that refuses the hold (MyGo returns no release) leaves nothing
// held, so the status says sleep can't be blocked, and the next Acquire
// asks again.
func TestInhibitorRefusedHoldsNothing(t *testing.T) {
	asked := 0
	i := newDesktopInhibitor(func() string { return "Playing music" })
	i.take = func(string) func() { asked++; return nil }
	for range 2 {
		if err := i.Acquire(context.Background()); err != nil {
			t.Fatal(err)
		}
		if i.Backend() != "" {
			t.Fatalf("Backend = %q after a refusal", i.Backend())
		}
	}
	if asked != 2 {
		t.Fatalf("asked %d times, want each Acquire to ask", asked)
	}
	if err := i.Release(); err != nil {
		t.Fatal(err)
	}
}
