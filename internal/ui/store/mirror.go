package store

// Mirror keeps key in step with Go's events: each of events replaces the
// mirror with its payload. snapshot, when there is one, reads Go's state once
// off the main thread and fills the mirror only while it is still empty, so
// an event that lands before the answer (at least as new) wins. Call it from
// a module's Init.
func Mirror[T any](bus *Bus, cl *Client, post Poster, key Key, snapshot func() T, events ...string) {
	for _, ev := range events {
		On(bus, ev, func(s T) { Set(cl, key, s) })
	}
	Seed(cl, post, key, snapshot)
}

// Seed fills key with snapshot()'s answer, read off the main thread, unless
// the mirror was filled meanwhile. A nil snapshot does nothing.
func Seed[T any](cl *Client, post Poster, key Key, snapshot func() T) {
	if snapshot == nil {
		return
	}
	go func() {
		s := snapshot()
		post(func() { SetIfEmpty(cl, key, s) })
	}()
}
