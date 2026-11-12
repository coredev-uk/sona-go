package store

import (
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
)

func TestLatestSendsOneAtATimeAndCollapsesToTheNewest(t *testing.T) {
	synctest.Test(t, latestSendsOneAtATimeAndCollapsesToTheNewest)
}

func latestSendsOneAtATimeAndCollapsesToTheNewest(t *testing.T) {
	var (
		mu       sync.Mutex
		sent     []float64
		inFlight int
		overlap  bool
	)
	first, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	l := &Latest[config.Equalizer]{}
	l.Send = func(eq config.Equalizer) {
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
	// Nothing else follows: once the sender has finished, no goroutine is left to send.
	synctest.Wait()
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
	synctest.Test(t, latestSendsAgainAfterGoingIdle)
}

func latestSendsAgainAfterGoingIdle(t *testing.T) {
	got := make(chan float64, 4)
	l := &Latest[config.Equalizer]{Send: func(eq config.Equalizer) { got <- eq.Preamp }}
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
		// The sender goes idle before the next one.
		synctest.Wait()
		l.mu.Lock()
		idle := !l.sending
		l.mu.Unlock()
		if !idle {
			t.Fatal("the sender never went idle")
		}
	}
}
