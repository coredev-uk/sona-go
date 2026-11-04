package video

import (
	"bytes"
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestSurfaceNext(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewSurface()
		ctx := t.Context()

		// Nothing on show: Next waits until ctx ends.
		short, cancel := context.WithTimeout(ctx, time.Second)
		f, ok := s.Next(short, 0)
		cancel()
		if ok || f.Seq != 0 {
			t.Fatalf("empty surface: %+v %v", f, ok)
		}

		data := []byte{1, 2, 3, 4, 5, 6}
		s.Show(2, 2, 40*time.Millisecond, data)
		f, ok = s.Next(ctx, 0)
		if !ok || f.Seq != 1 || f.Width != 2 || f.Height != 2 || f.Position != 40*time.Millisecond || !bytes.Equal(f.Data, data) {
			t.Fatalf("first picture: %+v %v", f, ok)
		}

		// Next blocks until a picture newer than after is shown.
		got := make(chan Frame, 1)
		go func() {
			f, _ := s.Next(ctx, 1)
			got <- f
		}()
		synctest.Wait()
		select {
		case f := <-got:
			t.Fatalf("Next returned %+v before a newer picture", f)
		default:
		}
		s.Show(2, 2, 80*time.Millisecond, data)
		if f := <-got; f.Seq != 2 || f.Position != 80*time.Millisecond {
			t.Fatalf("second picture: %+v", f)
		}

		// Clear wakes a waiter with false and the new sequence.
		type result struct {
			f  Frame
			ok bool
		}
		cleared := make(chan result, 1)
		go func() {
			f, ok := s.Next(ctx, 2)
			cleared <- result{f, ok}
		}()
		synctest.Wait()
		s.Clear()
		if r := <-cleared; r.ok || r.f.Seq != 3 {
			t.Fatalf("after Clear: %+v %v", r.f, r.ok)
		}
		// A second Clear changes nothing.
		s.Clear()
		if f, ok := s.Next(ctx, 0); ok || f.Seq != 3 {
			t.Fatalf("cleared surface: %+v %v", f, ok)
		}
		// A seq at the current one waits.
		short, cancel = context.WithTimeout(ctx, time.Second)
		defer cancel()
		if f, ok := s.Next(short, 3); ok || f.Seq != 0 {
			t.Fatalf("Next(3) on a cleared surface: %+v %v", f, ok)
		}
	})
}

func TestSurfaceSeq(t *testing.T) {
	s := NewSurface()
	if got := s.Seq(); got != 0 {
		t.Fatalf("empty surface Seq %d, want 0", got)
	}
	s.Show(2, 2, 0, make([]byte, 6))
	if got := s.Seq(); got != 1 {
		t.Errorf("after a picture Seq %d, want 1", got)
	}
	s.Clear()
	if got := s.Seq(); got != 2 {
		t.Errorf("after the clear Seq %d, want 2", got)
	}
	s.Clear() // nothing on show: no change
	if got := s.Seq(); got != 2 {
		t.Errorf("a clear of nothing moved Seq to %d", got)
	}
}
