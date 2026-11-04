package equalizer

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
)

func TestLatestSendsOneAtATimeAndCollapsesToTheNewest(t *testing.T) {
	var (
		mu       sync.Mutex
		sent     []float64
		inFlight int
		overlap  bool
	)
	first, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	l := &latest[config.Equalizer]{}
	l.send = func(eq config.Equalizer) {
		mu.Lock()
		inFlight++
		overlap = overlap || inFlight > 1
		sent = append(sent, eq.Preamp)
		n := len(sent)
		mu.Unlock()
		if n == 1 {
			close(first)
			<-release // the first call is slow
		}
		mu.Lock()
		inFlight--
		if n == 2 {
			close(done)
		}
		mu.Unlock()
	}
	l.Put(config.Equalizer{Preamp: 1})
	<-first
	for v := 2.0; v <= 5; v++ {
		l.Put(config.Equalizer{Preamp: -v}) // arrive while the first is in flight
	}
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the newest value never went out")
	}
	// Nothing else follows.
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if want := []float64{1, -5}; !slices.Equal(sent, want) {
		t.Errorf("sent %v, want %v (the first, then only the newest)", sent, want)
	}
	if overlap {
		t.Error("two sends were in flight at once")
	}
}

func TestLatestSendsAgainAfterGoingIdle(t *testing.T) {
	got := make(chan float64, 4)
	l := &latest[config.Equalizer]{send: func(eq config.Equalizer) { got <- eq.Preamp }}
	for _, v := range []float64{-1, -2} {
		l.Put(config.Equalizer{Preamp: v})
		select {
		case g := <-got:
			if g != v {
				t.Fatalf("sent %v, want %v", g, v)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%v never went out", v)
		}
		// Wait for the sender to go idle before the next one.
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
			l.mu.Lock()
			idle := !l.sending
			l.mu.Unlock()
			if idle {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the sender never went idle")
			}
		}
	}
}
