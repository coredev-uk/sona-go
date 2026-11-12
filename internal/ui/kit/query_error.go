package kit

import (
	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// QueryError is the ErrorBlock of a failed query: res's error under title,
// with Try again invalidating keys (and so asking again) while res fetches.
func QueryError[T any](c *ui.Context, q *store.Client, title string, res store.Result[T], keys ...store.Key) ui.Element {
	return ErrorBlock(c, title, res.Err, ErrorBlockOpts{
		OnRetry:  func() { q.Invalidate(keys...) },
		Retrying: res.Fetching,
	})
}

// QueryFailed is QueryError for a block that never says "Retrying…": Try
// again invalidates keys and the block stays as it is while the fetch runs.
func QueryFailed(c *ui.Context, q *store.Client, title string, err error, keys ...store.Key) ui.Element {
	return ErrorBlock(c, title, err, ErrorBlockOpts{OnRetry: func() { q.Invalidate(keys...) }})
}

// DetailQueryError is DetailError of a failed query: Try again invalidates
// keys while res fetches; back is the "not found" link.
func DetailQueryError[T any](c *ui.Context, noun DetailNoun, q *store.Client, res store.Result[T], back EmptyAction, signIn func(), keys ...store.Key) ui.Element {
	return DetailError(c, noun, res.Err, DetailErrorOpts{
		Retry:    func() { q.Invalidate(keys...) },
		Retrying: res.Fetching,
		Back:     back,
		SignIn:   signIn,
	})
}

// BackAction is the "not found" link of a detail page: label opens path
// through push, and runs nothing (the link is not drawn) unless routed says a
// module serves path.
func BackAction(label, path string, routed bool, push func(string)) EmptyAction {
	back := EmptyAction{Label: label}
	if routed {
		back.Run = func() { push(path) }
	}
	return back
}
