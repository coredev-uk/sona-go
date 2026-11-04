package store

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestBusBuffersUntilPosterThenFlushesInOrder(t *testing.T) {
	b := NewBus(nil)
	var got []int
	On(b, "n", func(n int) { got = append(got, n) })
	for i := 1; i <= 3; i++ {
		if !b.Emit("n", i) {
			t.Error("Emit says nobody listens")
		}
	}
	if len(got) != 0 {
		t.Fatalf("delivered before a poster: %v", got)
	}
	var posted int
	b.SetPoster(func(fn func()) { posted++; fn() })
	if want := []int{1, 2, 3}; !slices.Equal(got, want) || posted != 3 {
		t.Fatalf("got %v in %d posts, want %v", got, posted, want)
	}
	b.Emit("n", 4)
	if want := []int{1, 2, 3, 4}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBusEmitDuringFlushKeepsOrder(t *testing.T) {
	b := NewBus(nil)
	var got []int
	On(b, "n", func(n int) {
		got = append(got, n)
		if n == 1 {
			b.Emit("n", 99) // from a handler, while the queue is still flushing
		}
	})
	b.Emit("n", 1)
	b.Emit("n", 2)
	b.SetPoster(nil)
	if want := []int{1, 2, 99}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestBusReportsSubscribersAndUnsubscribes(t *testing.T) {
	b := NewBus(nil)
	b.SetPoster(nil)
	if b.Emit("x", 1) {
		t.Error("Emit reports a subscriber where there is none")
	}
	n := 0
	off := On(b, "x", func(int) { n++ })
	if !b.Emit("x", 1) || n != 1 {
		t.Errorf("subscribed: n = %d", n)
	}
	off()
	off()
	if b.Emit("x", 1) || n != 1 {
		t.Errorf("after off: n = %d", n)
	}
}

func TestBusOffDuringDeliverySkipsTheSubscriber(t *testing.T) {
	b := NewBus(nil)
	b.SetPoster(nil)
	var offSecond func()
	var calls []string
	On(b, "x", func(int) { calls = append(calls, "first"); offSecond() })
	offSecond = On(b, "x", func(int) { calls = append(calls, "second") })
	b.Emit("x", 1)
	if want := []string{"first"}; !slices.Equal(calls, want) {
		t.Fatalf("calls %v, want %v", calls, want)
	}
}

func TestBusDropsAMismatchedPayloadAndLogsOnce(t *testing.T) {
	var logs bytes.Buffer
	b := NewBus(slog.New(slog.NewTextHandler(&logs, nil)))
	b.SetPoster(nil)
	var got []string
	On(b, "s", func(s string) { got = append(got, s) })
	b.Emit("s", 42)
	b.Emit("s", 43)
	b.Emit("s", "ok")
	if want := []string{"ok"}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if n := strings.Count(logs.String(), "another type"); n != 1 {
		t.Errorf("logged %d times: %s", n, logs.String())
	}
}

func TestBusEventWithoutPayloadDeliversTheZeroValue(t *testing.T) {
	b := NewBus(nil)
	b.SetPoster(nil)
	called := false
	On(b, "ping", func(s struct{}) { called = true })
	b.Emit("ping")
	if !called {
		t.Error("not delivered")
	}
}

func TestBusEmitIsSafeFromManyGoroutines(t *testing.T) {
	b := NewBus(nil)
	var mu sync.Mutex
	total := 0
	On(b, "n", func(n int) { mu.Lock(); total += n; mu.Unlock() })
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				b.Emit("n", 1)
			}
		}()
	}
	wg.Wait()
	b.SetPoster(nil)
	for range 100 {
		b.Emit("n", 1)
	}
	mu.Lock()
	defer mu.Unlock()
	if total != 900 {
		t.Errorf("total %d, want 900", total)
	}
}
