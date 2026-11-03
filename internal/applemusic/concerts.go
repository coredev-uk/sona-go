package applemusic

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Concerts, as Apple's web player asks for them (music.apple.com/concerts
// and an artist's "Upcoming concerts"). The data is Shazam's event feed,
// served by amp-api with the web player's developer token; the lists need
// no Music User Token, except the one of the user's own artists.
//
// A request names its area with a geohash (ConcertsPrecision characters:
// without one the hub answers 500). The hub's "popular-concerts" container
// lists the concerts within about 100 km of it, soonest first, up to its
// limit, and honours the date and genre filters; the "this week", "next
// week" and "more upcoming" containers the web page shows ignore them, so
// Sona asks for the one container and groups by date itself.

// Concert containers of the hub (/v1/concerts/{sf}/hub?containers=…).
const (
	ConcertsPopular    = "popular-concerts"
	ConcertsFavourites = "favorite-artists-concerts" // the user's own artists: needs the Music User Token
)

// Artist views holding concerts.
const (
	ViewAllUpcomingConcerts    = "all-upcoming-concerts"
	ViewNearbyUpcomingConcerts = "nearby-upcoming-concerts" // only when asked with a geohash
)

// ConcertsMaxLimit is the most concerts one hub request is asked for; Apple
// answers up to about a thousand.
const ConcertsMaxLimit = 500

// ConcertTicket is one seller of tickets to a concert. URL is the link
// Apple gives (an affiliate redirect for most sellers), opened as it is.
type ConcertTicket struct {
	Vendor string `json:"vendor"`
	URL    string `json:"url"`
}

// ConcertVenue is where a concert is.
type ConcertVenue struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Address     string  `json:"address"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	CountryCode string  `json:"countryCode"` // ISO 3166-1 alpha-2, as Apple writes it
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// ConcertArtist is an artist playing a concert.
type ConcertArtist struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Artwork *Artwork `json:"artwork,omitempty"`
}

// Concert is one event with its venue and artists joined in. Start and End
// are the UTC instants Apple gives (RFC 3339); Timezone is the venue's
// IANA zone, in which the show's local date and time are read.
type Concert struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"` // "Artist with Support @ Venue"
	Start    string          `json:"start"`
	End      string          `json:"end"`
	Timezone string          `json:"timezone"`
	URL      string          `json:"url"` // the concert's page on music.apple.com
	Tickets  []ConcertTicket `json:"tickets"`
	Venue    *ConcertVenue   `json:"venue,omitempty"`
	Artists  []ConcertArtist `json:"artists"`
}

// StartTime parses Start; the zero time when Apple gave none.
func (c Concert) StartTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.Start)
	return t
}

// EndTime parses End; the zero time when Apple gave none.
func (c Concert) EndTime() time.Time {
	t, _ := time.Parse(time.RFC3339, c.End)
	return t
}

// ConcertsQuery selects the concerts near a place.
type ConcertsQuery struct {
	GeoHash string   // required: the area, ConcertsPrecision characters
	From    string   // first day, "2006-01-02", with To; "" for today onwards
	To      string   // last day
	Genres  []string // catalog genre IDs ("20" Alternative), any of
	Limit   int      // at most; default 100, capped at ConcertsMaxLimit
	// Favourites also asks for the user's own artists' concerts in the
	// area (needs the Music User Token).
	Favourites bool
}

// ConcertsResult is the hub's answer: the concerts in the area soonest
// first, and which of them are by the user's own artists.
type ConcertsResult struct {
	Concerts   []Concert `json:"concerts"`
	Favourites []string  `json:"favourites"` // IDs, a subset of Concerts when asked for
}

// ConcertsNear fetches the concerts near q.GeoHash. Dates are the
// calendar days of the venues' own time zones, as the web player sends
// them.
func ConcertsNear(ctx context.Context, c *Client, sf string, q ConcertsQuery) (ConcertsResult, error) {
	if q.GeoHash == "" {
		return ConcertsResult{}, fmt.Errorf("applemusic: concerts need a geohash")
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 100
	}
	limit = min(limit, ConcertsMaxLimit)
	containers := []string{ConcertsPopular}
	if q.Favourites {
		containers = append(containers, ConcertsFavourites)
	}
	v := url.Values{
		"containers":                     {strings.Join(containers, ",")},
		"limit[" + ConcertsPopular + "]": {strconv.Itoa(limit)},
		"geoHashLocation":                {q.GeoHash},
		"include[concerts]":              {"venues,artists"},
		"extend[concerts]":               {"tickets"},
		"fields[artists]":                {"name,artwork"},
		"art[url]":                       {"f"},
		"art[artists:url]":               {"c"},
		"omit[resource]":                 {"autos"},
		"format[resources]":              {"map"},
	}
	if q.Favourites {
		v.Set("limit["+ConcertsFavourites+"]", strconv.Itoa(limit))
	}
	if q.From != "" && q.To != "" {
		v.Set("filter[date]", q.From+".."+q.To)
	}
	if len(q.Genres) > 0 {
		v.Set("filter[genre]", strings.Join(q.Genres, ","))
	}
	var raw struct {
		Results struct {
			Containers map[string]struct {
				Data []Ref `json:"data"`
			} `json:"containers"`
		} `json:"results"`
		Resources concertResources `json:"resources"`
	}
	if err := c.Get(ctx, c.ampBase()+"/v1/concerts/"+url.PathEscape(sf)+"/hub", withPlatform(v), &raw); err != nil {
		return ConcertsResult{}, err
	}
	out := ConcertsResult{Concerts: []Concert{}, Favourites: []string{}}
	seen := map[string]bool{}
	for _, ref := range raw.Results.Containers[ConcertsPopular].Data {
		if con, ok := raw.Resources.concert(ref.ID); ok && !seen[con.ID] {
			seen[con.ID] = true
			out.Concerts = append(out.Concerts, con)
		}
	}
	for _, ref := range raw.Results.Containers[ConcertsFavourites].Data {
		if con, ok := raw.Resources.concert(ref.ID); ok {
			if !seen[con.ID] {
				seen[con.ID] = true
				out.Concerts = append(out.Concerts, con)
			}
			out.Favourites = append(out.Favourites, con.ID)
		}
	}
	return out, nil
}

// ArtistConcertsResult is an artist's upcoming concerts: those within
// reach of the area asked for (when there was one), and all of them.
type ArtistConcertsResult struct {
	Nearby []Concert `json:"nearby"`
	All    []Concert `json:"all"`
}

// ArtistConcerts fetches artist id's upcoming concerts, with the area's
// own first when geohash is not "". The artist is the only one named on
// each concert, since the page names the artist already.
func ArtistConcerts(ctx context.Context, c *Client, sf, id, geohash string) (ArtistConcertsResult, error) {
	views := []string{ViewAllUpcomingConcerts}
	v := url.Values{
		"include[concerts]": {"venues"},
		"extend[concerts]":  {"tickets"},
		"fields[artists]":   {"name,artwork"},
		"omit[resource]":    {"autos"},
		"format[resources]": {"map"},
	}
	if geohash != "" {
		views = append(views, ViewNearbyUpcomingConcerts)
		v.Set("geoHashLocation", geohash)
	}
	v.Set("views", strings.Join(views, ","))
	var raw struct {
		Resources concertResources `json:"resources"`
	}
	if err := c.Get(ctx, c.ampBase()+CatalogPath(sf, TypeArtists, id), withPlatform(v), &raw); err != nil {
		return ArtistConcertsResult{}, err
	}
	out := ArtistConcertsResult{Nearby: []Concert{}, All: []Concert{}}
	var artist ConcertArtist
	if a, ok := raw.Resources.Artists[id]; ok {
		artist = ConcertArtist{ID: a.ID, Name: a.Attributes.Name, Artwork: a.Attributes.Artwork}
	}
	pick := func(view string) []Concert {
		var list []Concert
		for _, ref := range raw.Resources.Artists[id].Views[view].Data {
			if con, ok := raw.Resources.concert(ref.ID); ok {
				if len(con.Artists) == 0 && artist.ID != "" {
					con.Artists = []ConcertArtist{artist}
				}
				list = append(list, con)
			}
		}
		if list == nil {
			list = []Concert{}
		}
		return list
	}
	out.All, out.Nearby = pick(ViewAllUpcomingConcerts), pick(ViewNearbyUpcomingConcerts)
	return out, nil
}

func withPlatform(v url.Values) url.Values {
	v.Set("platform", "web")
	return v
}

// concertResources is the normalised resources of a concerts response.
type concertResources struct {
	Concerts map[string]concertResource           `json:"concerts"`
	Venues   map[string]Resource[venueAttributes] `json:"venues"`
	Artists  map[string]concertArtistResource     `json:"artists"`
}

type concertResource struct {
	Resource[struct {
		Name     string          `json:"name"`
		Start    string          `json:"startISODateTime"`
		End      string          `json:"endISODateTime"`
		Timezone string          `json:"timezone"`
		URL      string          `json:"url"`
		Tickets  []ConcertTicket `json:"tickets"`
	}]
	Relationships struct {
		Artists Relationship[Ref] `json:"artists"`
		Venues  Relationship[Ref] `json:"venues"`
	} `json:"relationships"`
}

type concertArtistResource struct {
	Resource[ArtistAttributes]
	// Views are an artist's concert views (ArtistConcerts).
	Views map[string]struct {
		Data []Ref `json:"data"`
	} `json:"views"`
}

type venueAttributes struct {
	Name        string `json:"name"`
	GeoLocation struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"geoLocation"`
	Address struct {
		Address     string `json:"address"`
		City        string `json:"city"`
		Region      string `json:"region"`
		CountryCode string `json:"countryIsoCode"`
	} `json:"structuredAddress"`
}

// concert joins concert id with its venue and artists.
func (r concertResources) concert(id string) (Concert, bool) {
	n, ok := r.Concerts[id]
	if !ok {
		return Concert{}, false
	}
	a := n.Attributes
	c := Concert{
		ID: n.ID, Name: a.Name, Start: a.Start, End: a.End, Timezone: a.Timezone, URL: a.URL,
		Tickets: []ConcertTicket{}, Artists: []ConcertArtist{},
	}
	for _, t := range a.Tickets {
		if t.URL != "" {
			c.Tickets = append(c.Tickets, t)
		}
	}
	if refs := n.Relationships.Venues.Data; len(refs) > 0 {
		if v, ok := r.Venues[refs[0].ID]; ok {
			va := v.Attributes
			c.Venue = &ConcertVenue{
				ID: v.ID, Name: va.Name, Address: va.Address.Address, City: va.Address.City,
				Region: va.Address.Region, CountryCode: va.Address.CountryCode,
				Latitude: va.GeoLocation.Latitude, Longitude: va.GeoLocation.Longitude,
			}
		}
	}
	for _, ref := range n.Relationships.Artists.Data {
		if ar, ok := r.Artists[ref.ID]; ok {
			c.Artists = append(c.Artists, ConcertArtist{ID: ar.ID, Name: ar.Attributes.Name, Artwork: ar.Attributes.Artwork})
		}
	}
	return c, c.Start != ""
}
