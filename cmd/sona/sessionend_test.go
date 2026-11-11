package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

// order records the calls the hook makes, in order.
type order struct {
	mu    sync.Mutex
	calls []string
}

func (o *order) add(s string) {
	o.mu.Lock()
	o.calls = append(o.calls, s)
	o.mu.Unlock()
}

func (o *order) get() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.calls...)
}

type fakeQueue struct {
	o     *order
	block chan struct{} // Close waits on it when set
	err   error
}

func (f *fakeQueue) Close() error {
	f.o.add("queue")
	if f.block != nil {
		<-f.block
	}
	return f.err
}

type fakeReporter struct {
	o      *order
	budget time.Duration // the deadline the context carried
	err    error
}

func (f *fakeReporter) Close(ctx context.Context) error {
	f.o.add("report")
	if d, ok := ctx.Deadline(); ok {
		f.budget = time.Until(d)
	}
	return f.err
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSessionEndSavesQueueThenReports(t *testing.T) {
	o := &order{}
	hook := flushOnSessionEnd(&fakeQueue{o: o}, &fakeReporter{o: o}, time.Second, quiet())
	hook(&mygo.QuitEvent{SessionEnding: true})
	got := o.get()
	if len(got) != 2 || got[0] != "queue" || got[1] != "report" {
		t.Errorf("calls = %v, want the queue saved before the end of the play is reported", got)
	}
}

// A quit the user asked for leaves the flush to the deferred closes after Run.
func TestOrdinaryQuitFlushesNothing(t *testing.T) {
	o := &order{}
	hook := flushOnSessionEnd(&fakeQueue{o: o}, &fakeReporter{o: o}, time.Second, quiet())
	hook(&mygo.QuitEvent{})
	if got := o.get(); len(got) != 0 {
		t.Errorf("calls = %v, want none", got)
	}
}

// The OS may end the process any moment: a save that does not return still
// lets the reporter keep its undelivered events, and the hook returns within
// the budget.
func TestSessionEndIsBounded(t *testing.T) {
	o := &order{}
	q := &fakeQueue{o: o, block: make(chan struct{})}
	defer close(q.block)
	r := &fakeReporter{o: o}
	hook := flushOnSessionEnd(q, r, 100*time.Millisecond, quiet())

	start := time.Now()
	hook(&mygo.QuitEvent{SessionEnding: true})
	took := time.Since(start)

	if took > time.Second {
		t.Errorf("the hook took %v with a 100 ms budget", took)
	}
	got := o.get()
	if len(got) != 2 || got[1] != "report" {
		t.Errorf("calls = %v, want the reporter still closed after the stuck save", got)
	}
	if r.budget <= 0 || r.budget > time.Second {
		t.Errorf("the reporter's context had %v left, want the short minimum", r.budget)
	}
}

func TestSessionEndSurvivesErrors(t *testing.T) {
	o := &order{}
	hook := flushOnSessionEnd(&fakeQueue{o: o, err: errors.New("disk full")}, &fakeReporter{o: o, err: errors.New("offline")}, time.Second, quiet())
	hook(&mygo.QuitEvent{SessionEnding: true})
	if got := o.get(); len(got) != 2 {
		t.Errorf("calls = %v, want both steps despite the errors", got)
	}
}
