package applemusic

import (
	"context"
	"net/url"
	"slices"
	"strconv"
	"strings"
)

// Search, as the web player does it (amp-api, format[resources]=map): one
// call returns every result group ("top", "artist", "album", "song",
// "playlist", "radio_show", "radio_episode", "station", "music_video",
// "video_extra", …) in meta.results.order; each group pages on its own
// with groups=<group>&offset=N. Suggestions (term completions and top
// result lockups), the search landing page (browse categories) and the
// social search (profiles and user playlists) are separate calls.

// Search result groups the catalog search returns. GroupTop mixes types.
const (
	GroupTop          = "top"
	GroupArtist       = "artist"
	GroupAlbum        = "album"
	GroupSong         = "song"
	GroupPlaylist     = "playlist"
	GroupRadioShow    = "radio_show"
	GroupRadioEpisode = "radio_episode"
	GroupStation      = "station"
	GroupMusicVideo   = "music_video"
	GroupVideoExtra   = "video_extra"
	GroupCategory     = "category"
	GroupCurator      = "curator"
	GroupRecordLabel  = "record_label"
	// Social search groups.
	GroupProfile = "profile"
)

// searchTypes are the resource types the web player searches for.
var searchTypes = []string{
	"activities", "albums", "apple-curators", "artists", "curators", "editorial-items",
	"music-movies", "music-videos", "playlists", "record-labels", "songs", "stations",
	"tv-episodes", "uploaded-videos",
}

// MaxSearchLimit is the largest page Apple serves per result group.
const MaxSearchLimit = 25

// SearchGroup is one group of results: references into the response's
// Resources, and Next (non-empty when the group has a further page).
type SearchGroup struct {
	Name string `json:"name"` // Apple's display name ("Top Results"); not shown as-is
	Next string `json:"next"`
	Data []Ref  `json:"data"`
}

// SearchResults is a search response: its groups by key, the group order
// Apple recommends, and the resources the groups reference.
type SearchResults struct {
	Groups    map[string]SearchGroup `json:"results"`
	Resources Resources              `json:"resources"`
	Meta      struct {
		Results struct {
			Order []string `json:"order"`
		} `json:"results"`
	} `json:"meta"`
}

// Order returns the group keys in Apple's order, keeping only groups with
// results.
func (r SearchResults) Order() []string {
	out := make([]string, 0, len(r.Groups))
	seen := map[string]bool{}
	for _, k := range r.Meta.Results.Order {
		if g, ok := r.Groups[k]; ok && len(g.Data) > 0 && !seen[k] {
			out = append(out, k)
			seen[k] = true
		}
	}
	// Groups Apple left out of the order still show, after the rest.
	var rest []string
	for k, g := range r.Groups {
		if !seen[k] && len(g.Data) > 0 {
			rest = append(rest, k)
		}
	}
	slices.Sort(rest)
	return append(out, rest...)
}

// SearchOptions selects one page. Group "" asks for the first page of
// every group; a group asks for a page of that group only, at Offset.
type SearchOptions struct {
	Group  string
	Limit  int // per group; 0 or above MaxSearchLimit means MaxSearchLimit
	Offset int
}

func (o SearchOptions) apply(q *AMPQuery) {
	q.Limit = o.Limit
	if q.Limit <= 0 || q.Limit > MaxSearchLimit {
		q.Limit = MaxSearchLimit
	}
	if o.Group != "" {
		q.Extra.Set("groups", o.Group)
		q.Offset = max(o.Offset, 0)
	}
}

// CatalogSearch searches storefront sf's catalog for every result group.
// Without with=serverBubbles a group page (groups=…&offset=…) comes back
// empty, so it is always sent.
func CatalogSearch(ctx context.Context, c *Client, sf, term string, opt SearchOptions) (SearchResults, error) {
	q := AMPQuery{
		With:   []string{"lyricHighlights", "lyrics", "naturalLanguage", "serverBubbles", "subtitles"},
		Extend: []string{"artistUrl"},
		Extra: url.Values{
			"term":                  {term},
			"types":                 {strings.Join(searchTypes, ",")},
			"include[albums]":       {"artists"},
			"include[songs]":        {"artists"},
			"include[music-videos]": {"artists"},
			"include[stations]":     {"radio-show"},
			"relate[albums]":        {"artists"},
			"relate[songs]":         {"albums"},
			"art[url]":              {"f"},
			"art[music-videos:url]": {"c"},
			"format[resources]":     {"map"},
			"omit[resource]":        {"autos"},
		},
	}
	opt.apply(&q)
	return AMP[SearchResults](ctx, c, CatalogPath(sf, "search"), q)
}

// SocialSearch searches Apple Music profiles and user-shared playlists
// (groups "top", "playlist", "profile"). It needs the Music User Token
// (403 without).
func SocialSearch(ctx context.Context, c *Client, sf, term string, opt SearchOptions) (SearchResults, error) {
	q := AMPQuery{
		With: []string{"lyrics", "serverBubbles"},
		Extra: url.Values{
			"term":                     {term},
			"types":                    {"playlists,social-profiles"},
			"art[social-profiles:url]": {"c"},
			"format[resources]":        {"map"},
		},
	}
	opt.apply(&q)
	return AMP[SearchResults](ctx, c, "/v1/social/"+url.PathEscape(sf)+"/search", q)
}

// Suggestion is one search hint: a term completion (Kind "terms") or a top
// result lockup (Kind "topResults", Content references Resources).
type Suggestion struct {
	Kind        string `json:"kind"`
	SearchTerm  string `json:"searchTerm"`
	DisplayTerm string `json:"displayTerm"`
	Content     *Ref   `json:"content"`
}

// Suggestions is a search-hints response.
type Suggestions struct {
	Results struct {
		Suggestions []Suggestion `json:"suggestions"`
	} `json:"results"`
	Resources Resources `json:"resources"`
}

// SearchSuggestions returns up to terms completions and results top-result
// lockups for the partial term (the web player calls it per keystroke).
func SearchSuggestions(ctx context.Context, c *Client, sf, term string, terms, results int) (Suggestions, error) {
	q := AMPQuery{
		With: []string{"naturalLanguage"},
		Extra: url.Values{
			"term":                      {term},
			"kinds":                     {"terms,topResults"},
			"limit[results:terms]":      {strconv.Itoa(max(terms, 1))},
			"limit[results:topResults]": {strconv.Itoa(max(results, 1))},
			"types":                     {"activities,albums,artists,editorial-items,music-movies,music-videos,playlists,record-labels,songs,stations,tv-episodes"},
			"art[url]":                  {"f"},
			"format[resources]":         {"map"},
			"omit[resource]":            {"autos"},
			"include[songs]":            {"artists"},
			"include[music-videos]":     {"artists"},
			"relate[songs]":             {"albums"},
		},
	}
	return AMP[Suggestions](ctx, c, CatalogPath(sf, "search", "suggestions"), q)
}

// SearchLanding fetches storefront sf's search landing page (no MUT
// needed): personal-recommendation shelves (display kind
// MusicNamedBrickShelf) whose "contents" are the browse categories —
// apple-curators (kind Genre or Curator) and editorial-items linking to
// multirooms or the charts.
func SearchLanding(ctx context.Context, c *Client, sf string) (MapResponse, error) {
	q := AMPQuery{
		Extend: []string{"editorialArtwork", "plainEditorialNotes"},
		Extra: url.Values{
			"name":           {"search-landing"},
			"types":          {"activities,apple-curators,editorial-items"},
			"art[url]":       {"c,f"},
			"omit[resource]": {"autos"},
		},
	}
	return AMPMap(ctx, c, "/v1/recommendations/"+url.PathEscape(sf), q)
}

// SocialProfile fetches social profile id ("sp.…") with its shared
// (public) playlists in Rel("shared-playlists"). listening-to, followers
// and followees need a request signature the web player computes (400
// "Invalid Request Signature"), so they are not requested.
func SocialProfile(ctx context.Context, c *Client, sf, id string) (Node, Resources, error) {
	q := AMPQuery{
		Extend: []string{"followState"},
		Extra: url.Values{
			"ids":      {id},
			"include":  {"shared-playlists"},
			"art[url]": {"c,f"},
		},
	}
	body, err := AMPMap(ctx, c, "/v1/social/"+url.PathEscape(sf)+"/social-profiles", q)
	if err != nil {
		return Node{}, nil, err
	}
	n, err := first(body.Primary(), "social profile "+id)
	return n, body.Resources, err
}
