package store

import "testing"

// BenchmarkKeyLookup is what Use pays every frame to find a query's entry: a
// key of strings, an int and a bool (the shape of the library and lyrics keys).
func BenchmarkKeyLookup(b *testing.B) {
	cl := NewClient(nil)
	key := Key{"library", "albums", "i.l.abc123", 50, true}
	cl.entries[key.id()] = &entry{}
	b.ReportAllocs()
	for b.Loop() {
		cl.mu.Lock()
		e := cl.entryLocked(key, cl.now())
		cl.mu.Unlock()
		if e == nil {
			b.Fatal("no entry")
		}
	}
}
