package applemusic

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// TypePersonalRecommendations is a Home shelf's type.
const TypePersonalRecommendations = "personal-recommendation"

// RecommendationAttributes are a Home shelf's attributes. Display.Kind is
// the shelf style (MusicCoverShelf, MusicNotesHeroShelf,
// MusicSuperHeroShelf, MusicConcertsEmptyShelf, …); Kind the shelf's
// source (music-recommendations, recently-played, your-favorites,
// concerts-for-you).
type RecommendationAttributes struct {
	Title *struct {
		Text string `json:"stringForDisplay"`
	} `json:"title"`
	Kind    string `json:"kind"`
	Display struct {
		Kind string `json:"kind"`
	} `json:"display"`
	ResourceTypes  []string `json:"resourceTypes"`
	HasSeeAll      bool     `json:"hasSeeAll"`
	NextUpdateDate string   `json:"nextUpdateDate"` // RFC 3339
}

// Heading returns the shelf title ("" for untitled hero shelves).
func (a RecommendationAttributes) Heading() string {
	if a.Title == nil {
		return ""
	}
	return strings.TrimSpace(a.Title.Text)
}

// recommendationTypes are the resource types Home asks for.
const recommendationTypes = "activities,albums,apple-curators,artists,concerts,curators,editorial-items," +
	"library-albums,library-playlists,music-movies,music-videos,playlists,social-profiles,social-upsells," +
	"songs,stations,tv-episodes,tv-shows,uploaded-audios,uploaded-videos"

// recommendationQuery is the web player's listen-now request vocabulary.
func recommendationQuery(tz string, extra url.Values) AMPQuery {
	v := url.Values{
		"name":                       {"listen-now"},
		"timezone":                   {tz},
		"types":                      {recommendationTypes},
		"extend[playlists]":          {"artistNames"},
		"extend[stations]":           {"airTime,supportsAirTimeUpdates"},
		"include[albums]":            {"artists"},
		"include[library-playlists]": {"catalog"},
		"include[stations]":          {"radio-show"},
		"meta[stations]":             {"inflectionPoints"},
		"fields[artists]":            {"name,artwork,url"},
		"art[url]":                   {"f"},
		"omit[resource]":             {"autos"},
	}
	for k, vals := range extra {
		v[k] = vals
	}
	// editorialVideo: motion artwork for Home's tiles (beyond the web
	// player's request, as for editorial pages).
	return mapValues(AMPQuery{Extend: []string{"editorialArtwork", "editorialVideo", "plainEditorialNotes"}, Extra: v})
}

// Recommendations fetches Home: the ordered personal-recommendation
// shelves (data) with their contents in resources. tz is the UTC offset
// ("+01:00"). Needs the Music User Token.
func Recommendations(ctx context.Context, c *Client, tz string) (MapResponse, error) {
	q := recommendationQuery(tz, url.Values{
		"displayFilter[kind]":              {"MusicCircleCoverShelf,MusicConcertsEmptyShelf,MusicCoverGrid,MusicCoverShelf,MusicNotesHeroShelf,MusicSocialCardShelf,MusicSuperHeroShelf"},
		"include[personal-recommendation]": {"primary-content"},
	})
	q.With = []string{"friendsMix", "library", "social"}
	return AMP[MapResponse](ctx, c, "/v1/me/recommendations", q)
}

// RefreshRecommendation asks Apple to recompute shelf id (Recently Played
// is refreshed this way after Home loads) and returns it.
func RefreshRecommendation(ctx context.Context, c *Client, tz, id string) (MapResponse, error) {
	q := recommendationQuery(tz, url.Values{"action": {"refresh"}})
	q.With = []string{"library"}
	var out MapResponse
	err := c.Do(ctx, Request{
		Method: http.MethodPost,
		URL:    c.ampBase() + "/v1/me/recommendations/" + url.PathEscape(id),
		Query:  q.Values(),
	}, &out)
	return out, err
}

// RecommendationSeeAll fetches shelf id with up to 100 contents (its "See
// all" grid).
func RecommendationSeeAll(ctx context.Context, c *Client, tz, id string) (MapResponse, error) {
	q := recommendationQuery(tz, url.Values{
		"include[personal-recommendation]": {"contents"},
		"limit[contents]":                  {"100"},
	})
	q.With = []string{"library"}
	return AMP[MapResponse](ctx, c, "/v1/me/recommendations/"+url.PathEscape(id)+"/view/see-all", q)
}
