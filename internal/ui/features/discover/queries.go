package discover

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// Query keys (v0.2.0's discoverKeys); every key starts with "discover" so a
// sign-in change can drop them all.
var keyDiscover = store.Key{"discover"}

func keyPage(kind, id string) store.Key { return store.Key{"discover", "page", kind, id} }
func keyRoom(id, sort string) store.Key { return store.Key{"discover", "room", id, sort} }
func keyCharts(genre string) store.Key  { return store.Key{"discover", "charts", genre} }
func keyChart(key, chartID, genre string) store.Key {
	return store.Key{"discover", "chart", key, chartID, genre}
}

// editorialStale: editorial pages change hourly at most; Go caches them and
// serves stale copies offline.
const editorialStale = 10 * time.Minute

// The editorial pages, by kind.
const (
	kindNew       = "new"
	kindCurator   = "curator"
	kindMultiroom = "multiroom"
	kindGrouping  = "grouping"
)

// editorial is an editorial page: New, a curator, a multiroom or a grouping.
func (d *discover) editorial(kind, id string) store.Result[services.EditorialPage] {
	return store.Use(d.env.Queries, keyPage(kind, id), func(ctx context.Context) (services.EditorialPage, error) {
		switch kind {
		case kindCurator:
			return d.api.Curator(ctx, id)
		case kindMultiroom:
			return d.api.Multiroom(ctx, id)
		case kindGrouping:
			return d.api.Grouping(ctx, id)
		}
		return d.api.New(ctx)
	}, store.Options{Stale: editorialStale})
}

// room is the first page of a room in an order ("" is Apple's default).
func (d *discover) roomFirst(id, sort string) store.Result[services.RoomPage] {
	return store.Use(d.env.Queries, keyRoom(id, sort), func(ctx context.Context) (services.RoomPage, error) {
		return d.api.Room(ctx, id, sort)
	}, store.Options{Stale: editorialStale})
}

// chartsPage is Charts for a genre ("" is all music).
func (d *discover) chartsFor(genre string) store.Result[services.ChartsPage] {
	return store.Use(d.env.Queries, keyCharts(genre), func(ctx context.Context) (services.ChartsPage, error) {
		return d.api.Charts(ctx, genre)
	}, store.Options{Stale: editorialStale})
}

// pager is the further pages of a room or chart: they load only on demand
// (v0.2.0's useMorePages, fetchNextPage), from the first page's next offset,
// one when the list nears its end.
type pager struct {
	// key identifies the list the pages belong to; another list starts over.
	key string
	// want is how many further pages were asked for.
	want int
}

// reset starts over for the list key.
func (p *pager) reset(key string) {
	if p.key != key {
		*p = pager{key: key}
	}
}

// itemPages is a list in pages: the first page, and the further ones that
// arrived.
type itemPages struct {
	first []services.Item
	more  []services.ItemPage
	// hasMore: there is a page to come (asked for or not); fetching: one is
	// on its way.
	hasMore, fetching bool
	// count is the items loaded.
	count int
}

// at is item i, nil past the loaded ones.
func (p *itemPages) at(i int) *services.Item {
	if i < len(p.first) {
		return &p.first[i]
	}
	i -= len(p.first)
	for k := range p.more {
		if i < len(p.more[k].Items) {
			return &p.more[k].Items[i]
		}
		i -= len(p.more[k].Items)
	}
	return nil
}

// morePages reads the pages asked for so far: base is the list's key, first
// its first page and next that page's next offset (0: none), and load fetches
// the page at an offset. Call it every frame; a page is fetched by Use when
// it is first asked for.
func (d *discover) morePages(p *pager, base store.Key, first []services.Item, next int,
	load func(ctx context.Context, offset int) (services.ItemPage, error),
) itemPages {
	out := itemPages{first: first, count: len(first)}
	off := next
	for k := 0; k < p.want && off > 0; k++ {
		offset := off
		key := append(append(store.Key(nil), base...), "more", next, offset)
		res := store.Use(d.env.Queries, key, func(ctx context.Context) (services.ItemPage, error) {
			return load(ctx, offset)
		}, store.Options{Stale: editorialStale})
		if res.Loading || res.Fetching {
			out.fetching = true
		}
		if !res.HasData {
			break
		}
		out.more = append(out.more, res.Data)
		out.count += len(res.Data.Items)
		off = res.Data.Next
	}
	out.hasMore = off > 0
	return out
}

// ask is the list reporting the rows it built, first to last: when they come
// within near of the loaded end, the next page is asked for (once at a time).
func (p *pager) ask(pages *itemPages, end, near int) {
	if pages.hasMore && !pages.fetching && end >= pages.count-near && p.want == len(pages.more) {
		p.want++
	}
}
