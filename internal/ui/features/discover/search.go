package discover

import (
	"net/url"

	"github.com/egoist/mygo/ui"
)

// The routes' search (v0.2.0's search-params.ts). A value is a string or it
// is dropped: an empty one is no value, as zod's catch(undefined) made of
// anything that was not a string. In the router's URL every value is a
// string.

// roomSearch is /room/{id}?sort=…: one of the room's sorts ("" is Apple's
// default order).
func roomSearch(r *ui.Route) (sort string) { return r.Query("sort") }

// chartsSearch is /charts?genre=…: a chart genre ID ("" is all music).
func chartsSearch(r *ui.Route) (genre string) { return r.Query("genre") }

// chartSearch is /charts/{chart}?genre=…&chartId=…: one chart in full.
func chartSearch(r *ui.Route) (genre, chartID string) {
	return r.Query("genre"), r.Query("chartId")
}

// roomURL is a room's page: sort "" (Apple's default order) is left out.
func roomURL(id, sort string) string {
	return withQuery("/room/"+url.PathEscape(id), "sort", sort)
}

// chartsURL is the charts overview for a genre ("" is all music).
func chartsURL(genre string) string { return withQuery("/charts", "genre", genre) }

// chartURL is one chart in full: its genre and, for city and country charts,
// its chart ID.
func chartURL(key, genre, chartID string) string {
	return withQuery("/charts/"+url.PathEscape(key), "genre", genre, "chartId", chartID)
}

// withQuery appends the pairs whose value is not empty.
func withQuery(path string, kv ...string) string {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			q.Set(kv[i], kv[i+1])
		}
	}
	if len(q) == 0 {
		return path
	}
	return path + "?" + q.Encode()
}
