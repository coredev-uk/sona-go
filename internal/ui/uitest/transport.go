package uitest

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"
)

// Recorder records the service calls a feature makes on goroutines, and lets
// a test wait for them. Fail, when set, is the error every call returns.
type Recorder struct {
	Fail error

	mu    sync.Mutex
	calls []string
	done  chan struct{}
}

// NewRecorder returns a recorder that buffers up to 32 calls nobody waited
// for.
func NewRecorder() *Recorder { return &Recorder{done: make(chan struct{}, 32)} }

// Record notes a call and returns Fail.
func (r *Recorder) Record(name string) error {
	r.mu.Lock()
	r.calls = append(r.calls, name)
	err := r.Fail
	r.mu.Unlock()
	r.done <- struct{}{}
	return err
}

// Locked runs fn with the recorder's lock held: for a fake's own fields.
func (r *Recorder) Locked(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn()
}

// Wait blocks until n more calls were made (they run on goroutines) and
// returns every call so far.
func (r *Recorder) Wait(t *testing.T, n int) []string {
	t.Helper()
	for range n {
		select {
		case <-r.done:
		case <-time.After(5 * time.Second):
			t.Fatalf("expected %d service calls, got %v", n, r.Snapshot())
		}
	}
	return r.Snapshot()
}

// Snapshot is the calls so far.
func (r *Recorder) Snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

// Quiet fails the test if a call was made (after the frames that would have).
func (r *Recorder) Quiet(t *testing.T, what string) {
	t.Helper()
	select {
	case <-r.done:
		t.Errorf("%s: the service was called: %v", what, r.Snapshot())
	case <-time.After(30 * time.Millisecond):
	}
}

// Transport is a fake of the playback commands every feature that has a
// transport shares (kit.PlaybackService); a feature's own fake embeds it for
// the rest.
type Transport struct {
	*Recorder
	seeks []int64
	vols  []float64
}

// NewTransport returns a recording fake transport.
func NewTransport() *Transport { return &Transport{Recorder: NewRecorder()} }

func (f *Transport) Toggle(context.Context) error { return f.Record("toggle") }
func (f *Transport) Next(context.Context) error   { return f.Record("next") }
func (f *Transport) Prev(context.Context) error   { return f.Record("prev") }

func (f *Transport) Seek(_ context.Context, ms int64) error {
	f.Locked(func() { f.seeks = append(f.seeks, ms) })
	return f.Record("seek")
}

func (f *Transport) SetVolume(_ context.Context, v float64) error {
	f.Locked(func() { f.vols = append(f.vols, v) })
	return f.Record("volume")
}

// Seeks is the positions sought, in ms.
func (f *Transport) Seeks() (out []int64) {
	f.Locked(func() { out = slices.Clone(f.seeks) })
	return out
}

// Volumes is the volumes set.
func (f *Transport) Volumes() (out []float64) {
	f.Locked(func() { out = slices.Clone(f.vols) })
	return out
}
