package applemusic

import (
	"cmp"
	"context"
	"net/url"
	"strconv"
	"strings"
)

// Chart groups in a charts response (results keys).
const (
	ChartSongs       = "songs"
	ChartAlbums      = "albums"
	ChartPlaylists   = "playlists"
	ChartMusicVideos = "music-videos"
	ChartCities      = "cityCharts"           // "Top 25" playlists by city
	ChartDailyGlobal = "dailyGlobalTopCharts" // "Daily Top 100" playlists by country
)

// GenreAllMusic is the charts genre covering all music.
const GenreAllMusic = "34"

// Chart is one chart: Chart is its kind ("most-played", "city-top",
// "daily-global-top"), Data the entries in rank order, Next the next page.
type Chart struct {
	Chart string `json:"chart"`
	Name  string `json:"name"`
	Href  string `json:"href"`
	Next  string `json:"next"`
	Data  []Ref  `json:"data"`
}

// ChartID returns the chartId of a city or daily chart ("" for the
// most-played charts), read from its href.
func (c Chart) ChartID() string {
	u, err := url.Parse(c.Href)
	if err != nil {
		return ""
	}
	return u.Query().Get("chartId")
}

// ChartsResponse is a normalised charts response: Results by group (see
// the Chart* constants), Order the groups in display order.
type ChartsResponse struct {
	Results   map[string][]Chart `json:"results"`
	Resources Resources          `json:"resources"`
	Meta      struct {
		Results struct {
			Order []string `json:"order"` // "most-played:songs", "city-top:cityCharts", …
		} `json:"results"`
	} `json:"meta"`
}

// ChartsQuery selects charts. Overview: Types (default all four) with
// Groups for the city and daily charts. One chart's page: either Types
// with one type and Chart "most-played", or ChartID; Offset pages it.
type ChartsQuery struct {
	Genre   string   // default GenreAllMusic
	Types   []string // songs, albums, playlists, music-videos
	Chart   string   // "most-played"
	ChartID string   // city / daily chart ID
	Cities  bool     // with=cityCharts,dailyGlobalTopCharts
	Limit   int      // default 50 (Apple's maximum)
	Offset  int
}

// Charts fetches charts (amp-api, no Music User Token needed).
func Charts(ctx context.Context, c *Client, sf string, cq ChartsQuery) (ChartsResponse, error) {
	v := url.Values{
		"genre":                 {cmp.Or(cq.Genre, GenreAllMusic)},
		"limit":                 {strconv.Itoa(cmp.Or(max(cq.Limit, 0), 50))},
		"include[songs]":        {"artists"},
		"include[albums]":       {"artists"},
		"include[music-videos]": {"artists"},
		"relate[songs]":         {"albums"},
		"art[url]":              {"f"},
		"omit[resource]":        {"autos"},
	}
	if len(cq.Types) > 0 {
		v.Set("types", strings.Join(cq.Types, ","))
	}
	if cq.Chart != "" {
		v.Set("chart", cq.Chart)
	}
	if cq.ChartID != "" {
		v.Set("chartId", cq.ChartID)
	}
	// editorialVideo: the albums' and playlists' tile motion.
	q := AMPQuery{Extend: []string{"artistUrl", "editorialVideo"}, Offset: cq.Offset, Extra: v}
	if cq.Cities {
		q.With = []string{"cityCharts", "dailyGlobalTopCharts"}
	}
	return AMP[ChartsResponse](ctx, c, CatalogPath(sf, "charts"), mapValues(q))
}

// GenreAttributes are a genre's attributes.
type GenreAttributes struct {
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
	URL      string `json:"url"`
}

// Genres returns the storefront's top-level chart genres ("34" is all
// music).
func Genres(ctx context.Context, c *Client, sf string) ([]Resource[GenreAttributes], error) {
	body, err := AMP[Page[Resource[GenreAttributes]]](ctx, c, CatalogPath(sf, "genres"), AMPQuery{})
	return body.Data, err
}
