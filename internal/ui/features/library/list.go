package library

import (
	"context"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// pageSize is how many rows a list asks of the service at a time
// (v0.2.0's PAGE).
const pageSize = 200

// listPage is one page of a long list and the size of the whole list.
type listPage[T any] struct {
	Items []T
	Total int
}

// pager is the part of a paged list a page keeps between frames: the pages
// the rows built last frame live in (pages of 200, fetched only for the rows
// on screen: v0.2.0's usePagedList).
type pager struct {
	first, last int // pages
}

// onRange is the list's OnRange: the rows built, first to last. The pages
// fetched follow it from the next frame.
func (p *pager) onRange(c *ui.Context) func(start, end int) {
	return func(start, end int) {
		next := [2]int{start / pageSize, end / pageSize}
		if next != [2]int{p.first, p.last} {
			p.first, p.last = next[0], next[1]
			c.Invalidate()
		}
	}
}

// listView is a paged list as the page reads it: At gives row i, nil for a
// row whose page is not here yet; Total is the full length from the first
// page, so the scroll bar is stable.
type listView[T any] struct {
	first store.Result[listPage[T]]
	pages map[int][]T
	// Total is the size of the whole list (0 until the first page arrives).
	Total int
	retry func()
}

// usePaged reads a list in pages of 200: the first page always, and those
// the rows in view (and their neighbours) live in. key is the query key of
// page n (keys mirror v0.2.0's, `["library", kind, sort, desc, n, type?]`);
// fetch reads rows offset…offset+limit. Pages are never stale until
// invalidated.
func usePaged[T any](l *library, p *pager, key func(page int) store.Key, fetch func(ctx context.Context, offset, limit int) (listPage[T], error)) *listView[T] {
	get := func(n int) store.Result[listPage[T]] {
		return store.Use(l.env.Queries, key(n), func(ctx context.Context) (listPage[T], error) {
			return fetch(ctx, n*pageSize, pageSize)
		}, store.Options{Stale: store.Forever})
	}
	v := &listView[T]{first: get(0), Total: 0}
	v.Total = v.first.Data.Total
	v.retry = func() { l.env.Queries.Invalidate(key(0)) }
	if !v.first.HasData {
		return v
	}
	v.pages = map[int][]T{0: v.first.Data.Items}
	lastPage := max(0, (v.Total+pageSize-1)/pageSize-1)
	for n := max(1, p.first-1); n <= min(lastPage, p.last+1); n++ {
		if res := get(n); res.HasData {
			v.pages[n] = res.Data.Items
		}
	}
	return v
}

// Pending: the first page is still being read.
func (v *listView[T]) Pending() bool { return !v.first.HasData && v.first.Err == nil }

// Err is why the first page could not be read; nil while there is data.
func (v *listView[T]) Err() error {
	if v.first.HasData {
		return nil
	}
	return v.first.Err
}

// Retrying: the first page is being read again after an error.
func (v *listView[T]) Retrying() bool { return v.first.Fetching && v.first.Err != nil }

// Retry reads the first page again.
func (v *listView[T]) Retry() { v.retry() }

// At is row i, nil when its page has not loaded.
func (v *listView[T]) At(i int) *T {
	items := v.pages[i/pageSize]
	if j := i % pageSize; j < len(items) {
		return &items[j]
	}
	return nil
}

// Loaded is the rows that have loaded as a slice of Total entries (nil for
// those that have not), for plays that need the songs.
func (v *listView[T]) Loaded() []*T {
	out := make([]*T, v.Total)
	for i := range out {
		out[i] = v.At(i)
	}
	return out
}
