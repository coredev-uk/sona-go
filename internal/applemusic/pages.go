package applemusic

import (
	"context"
	"maps"
	"net/url"
	"strconv"

	"golang.org/x/sync/errgroup"
)

// Page is a top-level paged collection response.
type Page[T any] struct {
	Data []T    `json:"data"`
	Next string `json:"next"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

// PageOptions tunes FetchPages.
type PageOptions struct {
	Start    int             // first offset to fetch, to resume an interrupted walk
	Size     int             // page size; default and Apple's maximum for /v1/me/library is 100
	Parallel int             // concurrent page requests; default 4
	OnTotal  func(total int) // called once with meta.total before the first page
}

const (
	defaultPageSize = 100
	defaultParallel = 4
)

// FetchPages walks the offset-paged collection at path (query q, without
// limit/offset). It reads meta.total with a limit=1 request, then fetches
// pages of opt.Size from opt.Start with opt.Parallel requests in flight and
// calls fn serially, in offset order, with each page. Pages are fetched at
// most opt.Parallel ahead of fn, so memory stays bounded. It returns the
// total.
//
// Collections without meta.total (most catalog relationships) are walked
// sequentially through their next links instead.
func FetchPages[T any](ctx context.Context, c *Client, path string, q url.Values, opt PageOptions, fn func(offset int, items []T) error) (int, error) {
	size := opt.Size
	if size <= 0 {
		size = defaultPageSize
	}
	par := opt.Parallel
	if par <= 0 {
		par = defaultParallel
	}
	var probe Page[T]
	if err := c.Get(ctx, path, withPage(q, 1, 0), &probe); err != nil {
		return 0, err
	}
	total := probe.Meta.Total
	if total == 0 && len(probe.Data) > 0 {
		return fetchSequential(ctx, c, path, q, opt, size, fn)
	}
	if opt.OnTotal != nil {
		opt.OnTotal(total)
	}
	if opt.Start >= total {
		return total, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	g, gctx := errgroup.WithContext(ctx)
	offsets := make([]int, 0, (total-opt.Start+size-1)/size)
	for off := opt.Start; off < total; off += size {
		offsets = append(offsets, off)
	}
	results := make([]chan []T, len(offsets))
	for i := range results {
		results[i] = make(chan []T, 1)
	}
	slots := make(chan struct{}, par) // released by the consumer
	g.Go(func() error {
		for i, off := range offsets {
			select {
			case slots <- struct{}{}:
			case <-gctx.Done():
				return gctx.Err()
			}
			g.Go(func() error {
				var p Page[T]
				if err := c.Get(gctx, path, withPage(q, size, off), &p); err != nil {
					return err
				}
				results[i] <- p.Data
				return nil
			})
		}
		return nil
	})

	var cerr error
pages:
	for i, off := range offsets {
		var items []T
		select {
		case items = <-results[i]:
		case <-gctx.Done():
			select {
			case items = <-results[i]: // fetched before the failure: still delivered
			default:
				break pages
			}
		}
		if cerr = fn(off, items); cerr != nil {
			cancel()
			break
		}
		<-slots
	}
	werr := g.Wait()
	if cerr != nil {
		return total, cerr
	}
	return total, werr
}

func fetchSequential[T any](ctx context.Context, c *Client, path string, q url.Values, opt PageOptions, size int, fn func(int, []T) error) (int, error) {
	off := opt.Start
	for {
		var p Page[T]
		if err := c.Get(ctx, path, withPage(q, size, off), &p); err != nil {
			return off, err
		}
		if len(p.Data) > 0 {
			if err := fn(off, p.Data); err != nil {
				return off, err
			}
		}
		off += len(p.Data)
		if p.Next == "" || len(p.Data) == 0 {
			return off, nil
		}
	}
}

// FollowNext appends the remaining pages of a relationship (from rel.Next)
// to rel.Data, 100 items per request, and clears rel.Next.
func FollowNext[T any](ctx context.Context, c *Client, rel *Relationship[T]) error {
	return followNext(ctx, c, rel, "", nil)
}

// followNext is FollowNext for any host: base is put before rel.Next's path
// ("" for the client's own) and extra is sent with each page.
func followNext[T any](ctx context.Context, c *Client, rel *Relationship[T], base string, extra url.Values) error {
	for rel.Next != "" {
		u, err := url.Parse(rel.Next)
		if err != nil {
			return err
		}
		q := u.Query()
		maps.Copy(q, extra)
		q.Set("limit", strconv.Itoa(defaultPageSize))
		var p Relationship[T]
		if err := c.Get(ctx, base+u.Path, q, &p); err != nil {
			return err
		}
		rel.Data = append(rel.Data, p.Data...)
		if p.Next == rel.Next || len(p.Data) == 0 {
			break
		}
		rel.Next = p.Next
	}
	rel.Next = ""
	return nil
}

func withPage(q url.Values, limit, offset int) url.Values {
	out := make(url.Values, len(q)+2)
	maps.Copy(out, q)
	out.Set("limit", strconv.Itoa(limit))
	if offset > 0 {
		out.Set("offset", strconv.Itoa(offset))
	}
	return out
}
