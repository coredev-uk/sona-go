package store

import "testing"

func TestMirrorFollowsEventsAndTheSnapshotOnlyFillsAnEmptyMirror(t *testing.T) {
	post := func(fn func()) { fn() }
	key := Key{"mirror-test"}

	// An empty mirror takes the snapshot, then every event.
	cl, b := NewClient(post), NewBus(nil)
	b.SetPoster(post)
	Mirror(b, cl, post, key, func() int { return 1 }, "a", "b")
	eventually(t, "the snapshot fills the mirror", func() bool { v, ok := Peek[int](cl, key); return ok && v == 1 })
	b.Emit("a", 2)
	b.Emit("b", 3)
	if v, _ := Peek[int](cl, key); v != 3 {
		t.Errorf("mirror %d after events a and b, want 3", v)
	}

	// An event that lands before the snapshot's answer wins.
	cl, b = NewClient(post), NewBus(nil)
	b.SetPoster(post)
	release, seeded := make(chan struct{}), make(chan struct{})
	Mirror(b, cl, func(fn func()) { fn(); close(seeded) }, key, func() int { <-release; return 1 }, "a")
	b.Emit("a", 9)
	close(release)
	<-seeded
	if v, _ := Peek[int](cl, key); v != 9 {
		t.Errorf("mirror %d, want the event's 9", v)
	}
}
