package scrobble

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coredev-uk/sona-go/internal/fsutil"
)

// DefaultMaxQueue is how many listens a provider's queue keeps while the
// service is unreachable; older ones are dropped first.
const DefaultMaxQueue = 2000

// queue is the per-provider backlog of undelivered listens, persisted as
// JSON (listen data only) after every change. Not safe for concurrent use.
type queue struct {
	path    string // "" keeps it in memory only
	max     int
	items   map[string][]Listen
	dropped map[string]int // listens removed from a head since start (see pop)
}

func newQueue(path string, max int) *queue {
	return &queue{path: path, max: max, items: map[string][]Listen{}, dropped: map[string]int{}}
}

// load reads the saved queue; a missing file is empty.
func (q *queue) load() error {
	if q.path == "" {
		return nil
	}
	b, err := os.ReadFile(q.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var items map[string][]Listen
	if err := json.Unmarshal(b, &items); err != nil {
		return fmt.Errorf("scrobble: parse %s: %w", q.path, err)
	}
	for name, ls := range items {
		if len(ls) > q.max {
			ls = ls[len(ls)-q.max:]
		}
		if len(ls) > 0 {
			q.items[name] = ls
		}
	}
	return nil
}

func (q *queue) len(name string) int { return len(q.items[name]) }

// push appends l, dropping the oldest listen when full.
func (q *queue) push(name string, l Listen) {
	ls := append(q.items[name], l)
	if over := len(ls) - q.max; over > 0 {
		ls = ls[over:]
		q.dropped[name] += over
	}
	q.items[name] = ls
}

// head copies up to n listens from the front, with the drop count to hand
// back to pop.
func (q *queue) head(name string, n int) ([]Listen, int) {
	ls := q.items[name]
	return append([]Listen(nil), ls[:min(n, len(ls))]...), q.dropped[name]
}

// pop removes the n listens head returned, minus those dropped from the
// front since (overflow or clear).
func (q *queue) pop(name string, n, mark int) {
	n -= q.dropped[name] - mark
	ls := q.items[name]
	n = min(max(n, 0), len(ls))
	if n == len(ls) {
		delete(q.items, name)
		return
	}
	q.items[name] = ls[n:]
}

// clear drops every listen of name.
func (q *queue) clear(name string) {
	q.dropped[name] += len(q.items[name])
	delete(q.items, name)
}

// save writes the queue atomically (0600), or removes the file when empty.
func (q *queue) save() error {
	if q.path == "" {
		return nil
	}
	if len(q.items) == 0 {
		if err := os.Remove(q.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(q.items)
	if err != nil {
		return err
	}
	dir := filepath.Dir(q.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(q.path, b, 0o600)
}
