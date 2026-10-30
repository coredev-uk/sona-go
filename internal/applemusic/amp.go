package applemusic

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// AMPQuery is the query vocabulary of the web player's amp-api requests.
// Zero fields are omitted; Values always adds platform=web.
type AMPQuery struct {
	Views   []string            // views=top-songs,full-albums
	Include []string            // include=artists,tracks
	Extend  []string            // extend=editorialArtwork,editorialVideo
	With    []string            // with=lyrics,serverBubbles
	Fields  map[string][]string // fields[albums]=name,artwork
	Limit   int                 // limit=
	Offset  int                 // offset=
	Extra   url.Values          // anything else (term, filter[…], ids, l, …)
}

// Values encodes q.
func (q AMPQuery) Values() url.Values {
	v := url.Values{"platform": {"web"}}
	set := func(k string, s []string) {
		if len(s) > 0 {
			v.Set(k, strings.Join(s, ","))
		}
	}
	set("views", q.Views)
	set("include", q.Include)
	set("extend", q.Extend)
	set("with", q.With)
	for typ, f := range q.Fields {
		set("fields["+typ+"]", f)
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Offset > 0 {
		v.Set("offset", strconv.Itoa(q.Offset))
	}
	for k, vals := range q.Extra {
		v[k] = append(v[k], vals...)
	}
	return v
}

// CatalogPath builds "/v1/catalog/{sf}/{parts...}" with each part escaped.
func CatalogPath(sf string, parts ...string) string { return catalogPath(sf, parts...) }

// AMP fetches path (e.g. CatalogPath(sf, "albums", id)) from the web
// player's API host, amp-api.music.apple.com, with q, decoding the JSON
// body into T. Use it for endpoints only amp-api serves (rooms, grouping,
// charts, editorial views, stations) or that need its query vocabulary.
func AMP[T any](ctx context.Context, c *Client, path string, q AMPQuery) (T, error) {
	var out T
	err := c.Get(ctx, c.ampBase()+path, q.Values(), &out)
	return out, err
}

func (c *Client) ampBase() string {
	if c.AMPBase != "" {
		return c.AMPBase
	}
	return AMPBaseURL
}

// AMPResource fetches one catalog resource of type typ (e.g. TypeAlbums,
// "stations", "music-videos", "curators") and returns its data[0], as the
// typed resource T (e.g. Album, or Resource[MyAttributes]).
func AMPResource[T any](ctx context.Context, c *Client, sf, typ, id string, q AMPQuery) (T, error) {
	body, err := AMP[Page[T]](ctx, c, CatalogPath(sf, typ, id), q)
	if err != nil {
		var zero T
		return zero, err
	}
	return first(body.Data, typ+" "+id)
}

// AMPResources fetches catalog resources of type typ by ID (ids=…), in
// batches of 100; unknown IDs are omitted.
func AMPResources[T any](ctx context.Context, c *Client, sf, typ string, ids []string, q AMPQuery) ([]T, error) {
	var out []T
	for len(ids) > 0 {
		n := min(len(ids), 100)
		qq := q
		qq.Extra = url.Values{}
		for k, v := range q.Extra {
			qq.Extra[k] = v
		}
		qq.Extra.Set("ids", strings.Join(ids[:n], ","))
		body, err := AMP[Page[T]](ctx, c, CatalogPath(sf, typ), qq)
		if err != nil {
			return nil, err
		}
		out = append(out, body.Data...)
		ids = ids[n:]
	}
	return out, nil
}
