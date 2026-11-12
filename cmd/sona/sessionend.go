package main

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/egoist/mygo"
)

// sessionEndBudget bounds the flush when the OS ends the session: macOS and
// Windows end the process soon after the quit listeners return, so the work
// is synchronous and short. The play-activity report is a network call; what
// it cannot deliver in time stays in the state dir for the next start.
const sessionEndBudget = 3 * time.Second

// minReporterBudget is what the reporter keeps even when the queue save used
// the whole budget.
const minReporterBudget = 250 * time.Millisecond

// activityCloser is the play-activity reporter (*activity.Reporter).
type activityCloser interface {
	Close(ctx context.Context) error
}

// flushOnSessionEnd is the OnWillQuit listener that saves what a normal quit
// saves after Run returns (main.go's deferred calls), which a session end
// never reaches: MyGo calls the quit listeners, then OnQuit, and the process
// may end. A normal quit leaves it to those defers.
//
// coord.Close ends the current play, writes the queue file (with the position)
// and hands the end event to the reporter; reporter.Close sends it, or keeps
// it on disk when the budget runs out. Both are idempotent, so the defers that
// follow (if the process lives on) find nothing left to do.
func flushOnSessionEnd(coord io.Closer, reporter activityCloser, budget time.Duration, log *slog.Logger) func(*mygo.QuitEvent) {
	return func(e *mygo.QuitEvent) {
		if !e.SessionEnding {
			return
		}
		end := time.Now().Add(budget)

		saved := make(chan error, 1)
		go func() { saved <- coord.Close() }()
		timer := time.NewTimer(budget)
		defer timer.Stop()
		select {
		case err := <-saved:
			if err != nil {
				log.Warn("session end: save queue", "err", err)
			}
		case <-timer.C:
			log.Warn("session end: saving the queue took longer than its budget", "budget", budget)
		}
		// The reporter gets what is left of the budget, at least enough to
		// write its undelivered events to disk.
		ctx, cancel := context.WithTimeout(context.Background(), max(time.Until(end), minReporterBudget))
		defer cancel()
		if err := reporter.Close(ctx); err != nil {
			log.Warn("session end: play activity", "err", err)
		}
	}
}
