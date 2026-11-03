package services

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/places"
)

// Concerts cache policy: the lists change as shows are announced and sell
// out, not by the minute. A cached list is also keyed by the day, so
// yesterday's never answers for today.
const (
	concertsTTL       = 3 * time.Hour
	artistConcertsTTL = 6 * time.Hour

	concertsDefaultLimit = 100
	concertsPickerRows   = 12
	// concertsDateLayout is the day format of ConcertsRequest.
	concertsDateLayout = "2006-01-02"
)

// ConcertsService is the Concerts page and the artist pages' "Upcoming
// concerts": Apple's concert listings (internal/applemusic, Shazam's
// events) near a city the user picks from Sona's own list
// (internal/places) or, until they do, the main city of their storefront.
// The chosen city is a setting (config.Concerts) and never leaves this
// computer: Apple is asked about the four-character geohash of its area,
// and nothing else. The lists are cached in discover_cache like other
// catalogue pages.
type ConcertsService struct {
	e *Env
}

func newConcertsService(e *Env) *ConcertsService { return &ConcertsService{e: e} }

func (s *ConcertsService) fail(err error) error { return s.e.Fail(err) }

// signedIn: a Music User Token is available (the user's own artists need it).
func (s *ConcertsService) signedIn() bool {
	return s.e.API.UserToken != nil && s.e.API.UserToken() != ""
}

// city is the place concerts are looked for, and whether it is only the
// default.
func (s *ConcertsService) city() (places.City, bool) {
	if c, ok := places.ByID(s.e.Settings.Get().Concerts.City); ok {
		return c, false
	}
	return places.Default(s.e.Storefront()), true
}

func locationOf(c places.City, isDefault bool) ConcertLocation {
	return ConcertLocation{
		ID: c.ID, Name: c.Name, CountryCode: c.Country,
		Latitude: c.Latitude, Longitude: c.Longitude, IsDefault: isDefault,
	}
}

// Location returns where concerts are looked for now.
func (s *ConcertsService) Location() ConcertLocation { return locationOf(s.city()) }

// SetLocation chooses the city (an ID from Places) concerts are looked for
// near; "" goes back to the storefront's main city. It returns the new
// location; settings:changed carries the saved settings.
func (s *ConcertsService) SetLocation(id string) (ConcertLocation, error) {
	if _, ok := places.ByID(id); !ok && id != "" {
		return ConcertLocation{}, fmt.Errorf("%w: city %q", ErrInvalidArgument, id)
	}
	saved, err := s.e.Settings.Update(func(st *config.Settings) { st.Concerts.City = id })
	if err != nil {
		return ConcertLocation{}, err
	}
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventSettingsChanged, saved)
	}
	return s.Location(), nil
}

// Places searches Sona's list of cities for the location picker; an empty
// query lists the storefront's cities and some well-known ones. Nothing is
// sent anywhere.
func (s *ConcertsService) Places(query string) []ConcertPlace {
	var found []places.City
	if strings.TrimSpace(query) == "" {
		found = places.Suggested(s.e.Storefront(), concertsPickerRows)
	} else {
		found = places.Search(query, s.e.Storefront(), concertsPickerRows)
	}
	cur, _ := s.city()
	out := make([]ConcertPlace, 0, len(found))
	for _, c := range found {
		out = append(out, ConcertPlace{ID: c.ID, Name: c.Name, CountryCode: c.Country, Selected: c.ID == cur.ID})
	}
	return out
}

// Concerts returns the concerts near the chosen place, soonest first. A
// cached list is served as it is before the first paint, and when Apple
// cannot be asked.
func (s *ConcertsService) Concerts(ctx context.Context, req ConcertsRequest) (ConcertsPage, error) {
	q, err := s.query(req)
	if err != nil {
		return ConcertsPage{}, err
	}
	city, isDefault := s.city()
	q.GeoHash = city.Geohash()
	signed := s.signedIn()
	q.Favourites = signed
	key := fmt.Sprintf("concerts|%s|%s|%s|%s|%d|%t|%s", q.GeoHash, q.From, q.To, strings.Join(q.Genres, ","),
		q.Limit, signed, s.day())
	r, info, err := cachedPage(ctx, s.e, key, signed, concertsTTL,
		func(ctx context.Context, sf string) (applemusic.ConcertsResult, error) {
			return applemusic.ConcertsNear(ctx, s.e.API, sf, q)
		}, nil)
	if err != nil {
		return ConcertsPage{}, s.fail(err)
	}
	loc := locationOf(city, isDefault)
	following := map[string]bool{}
	for _, id := range r.Favourites {
		following[id] = true
	}
	p := ConcertsPage{
		Location: loc, Concerts: s.concerts(r.Concerts, city, following), Limit: q.Limit,
		FetchedAt: info.FetchedAt, Stale: info.Stale, Genres: []Genre{},
	}
	p.More = len(r.Concerts) >= q.Limit && q.Limit < applemusic.ConcertsMaxLimit
	for _, c := range p.Concerts {
		if c.YourArtist {
			p.YourArtists++
		}
	}
	gs, gerr := s.genres(ctx)
	if gerr != nil {
		s.e.Logger.Warn("concerts: genres", "err", gerr)
	} else {
		p.Genres = gs
	}
	return p, nil
}

// ForArtist returns the upcoming concerts of catalogue artist id, those
// near the chosen place first.
func (s *ConcertsService) ForArtist(ctx context.Context, id string) (ArtistConcerts, error) {
	if id == "" || !isDigits(id) {
		return ArtistConcerts{}, fmt.Errorf("%w: artist %q", ErrInvalidArgument, id)
	}
	city, isDefault := s.city()
	geo := city.Geohash()
	r, info, err := cachedPage(ctx, s.e, fmt.Sprintf("artist-concerts|%s|%s|%s", id, geo, s.day()), false, artistConcertsTTL,
		func(ctx context.Context, sf string) (applemusic.ArtistConcertsResult, error) {
			return applemusic.ArtistConcerts(ctx, s.e.API, sf, id, geo)
		}, nil)
	if err != nil {
		return ArtistConcerts{}, s.fail(err)
	}
	near := map[string]bool{}
	for _, c := range r.Nearby {
		near[c.ID] = true
	}
	var rest []applemusic.Concert
	for _, c := range r.All {
		if !near[c.ID] {
			rest = append(rest, c)
		}
	}
	return ArtistConcerts{
		Location:  locationOf(city, isDefault),
		Nearby:    s.concerts(r.Nearby, city, nil),
		Elsewhere: s.concerts(rest, city, nil),
		FetchedAt: info.FetchedAt, Stale: info.Stale,
	}, nil
}

// day is today's date, which a cached list is keyed by.
func (s *ConcertsService) day() string { return s.e.Now().Format(concertsDateLayout) }

// query validates req into the Apple request.
func (s *ConcertsService) query(req ConcertsRequest) (applemusic.ConcertsQuery, error) {
	q := applemusic.ConcertsQuery{Limit: req.Limit}
	if q.Limit <= 0 {
		q.Limit = concertsDefaultLimit
	}
	q.Limit = min(q.Limit, applemusic.ConcertsMaxLimit)
	if (req.From == "") != (req.To == "") {
		return q, fmt.Errorf("%w: a date range needs both ends", ErrInvalidArgument)
	}
	if req.From != "" {
		from, err1 := time.Parse(concertsDateLayout, req.From)
		to, err2 := time.Parse(concertsDateLayout, req.To)
		if err1 != nil || err2 != nil || to.Before(from) {
			return q, fmt.Errorf("%w: dates %q to %q", ErrInvalidArgument, req.From, req.To)
		}
		q.From, q.To = req.From, req.To
	}
	if req.Genre != "" {
		if !isDigits(req.Genre) {
			return q, fmt.Errorf("%w: genre %q", ErrInvalidArgument, req.Genre)
		}
		q.Genres = []string{req.Genre}
	}
	return q, nil
}

// genres lists the genres to filter by; the cache entry is Discover's.
func (s *ConcertsService) genres(ctx context.Context) ([]Genre, error) {
	gs, _, err := cachedPage(ctx, s.e, "genres", false, genresTTL,
		func(ctx context.Context, sf string) ([]applemusic.Resource[applemusic.GenreAttributes], error) {
			return applemusic.Genres(ctx, s.e.API, sf)
		}, nil)
	if err != nil {
		return nil, err
	}
	out := []Genre{{ID: "", Name: "All genres"}}
	for _, g := range gs {
		if g.ID != applemusic.GenreAllMusic {
			out = append(out, Genre{ID: g.ID, Name: g.Attributes.Name})
		}
	}
	return out, nil
}

// concerts maps Apple's concerts, soonest first, with each one's distance
// from city.
func (s *ConcertsService) concerts(in []applemusic.Concert, from places.City, following map[string]bool) []Concert {
	out := make([]Concert, 0, len(in))
	for _, c := range in {
		out = append(out, s.concert(c, from, following[c.ID]))
	}
	slices.SortStableFunc(out, func(a, b Concert) int {
		switch {
		case a.StartsAt < b.StartsAt:
			return -1
		case a.StartsAt > b.StartsAt:
			return 1
		}
		return 0
	})
	return out
}

func (s *ConcertsService) concert(c applemusic.Concert, from places.City, yours bool) Concert {
	lineup, _, _ := strings.Cut(c.Name, " @ ")
	out := Concert{
		ID: c.ID, Name: c.Name, Lineup: strings.TrimSpace(lineup), Artists: []ConcertArtist{},
		Timezone: c.Timezone, Tickets: []ConcertTicket{}, YourArtist: yours,
	}
	if t := c.StartTime(); !t.IsZero() {
		out.StartsAt = t.UnixMilli()
	}
	if t := c.EndTime(); !t.IsZero() {
		out.EndsAt = t.UnixMilli()
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		out.Timezone = "UTC"
	}
	for _, a := range c.Artists {
		out.Artists = append(out.Artists, ConcertArtist{ID: a.ID, Name: a.Name, Art: s.e.m.artwork(a.Artwork)})
	}
	for _, t := range c.Tickets {
		if isHTTPS(t.URL) {
			out.Tickets = append(out.Tickets, ConcertTicket{Vendor: t.Vendor, URL: t.URL})
		}
	}
	if v := c.Venue; v != nil {
		out.Venue = &ConcertVenue{
			Name: v.Name, Address: v.Address, City: v.City, Region: v.Region,
			CountryCode: strings.ToUpper(v.CountryCode), Latitude: v.Latitude, Longitude: v.Longitude,
		}
		if v.Latitude != 0 || v.Longitude != 0 {
			out.DistanceKm = int(math.Round(places.DistanceKm(from.Latitude, from.Longitude, v.Latitude, v.Longitude)))
		}
	}
	return out
}

// isHTTPS: ticket links open in the browser, and only secure web links do.
func isHTTPS(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
}
