package services

// ConcertsRequest selects which concerts the Concerts page lists, near the
// chosen place (ConcertsService.Location). From and To are calendar days
// ("2026-11-14"), both or neither (from today on); Genre is an Apple genre
// ID ("" any); Limit is how many concerts at most (0: 100, at most 500).
type ConcertsRequest struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Genre string `json:"genre"`
	Limit int    `json:"limit"`
}

// ConcertLocation is where concerts are looked for: a city from Sona's own
// list (internal/places). IsDefault: the user never chose, so it is the
// main city of the Apple Music storefront.
type ConcertLocation struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	CountryCode string  `json:"countryCode"` // ISO 3166-1 alpha-2, lower case
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	IsDefault   bool    `json:"isDefault"`
}

// ConcertPlace is one city of the location picker's list.
type ConcertPlace struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CountryCode string `json:"countryCode"`
	Selected    bool   `json:"selected"`
}

// ConcertArtist is an artist playing a concert; ID is the catalogue
// artist's page.
type ConcertArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Art  Art    `json:"art"`
}

// ConcertVenue is where a concert is. City and Region are as the venue
// gives them; CountryCode is ISO 3166-1 alpha-2, upper case ("" unknown).
type ConcertVenue struct {
	Name        string  `json:"name"`
	Address     string  `json:"address"`
	City        string  `json:"city"`
	Region      string  `json:"region"`
	CountryCode string  `json:"countryCode"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
}

// ConcertTicket is one seller of tickets; URL is an https link the
// UI opens in the browser.
type ConcertTicket struct {
	Vendor string `json:"vendor"`
	URL    string `json:"url"`
}

// Concert is one show. StartsAt and EndsAt are Unix ms (EndsAt 0 when
// unknown); Timezone is the venue's IANA zone, in which the show's day
// and time are read. Lineup is the name without its venue ("Band with
// Opener"). DistanceKm is the straight line from the chosen place (0:
// unknown). YourArtist: by an artist the user follows on Apple Music.
type Concert struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Lineup     string          `json:"lineup"`
	Artists    []ConcertArtist `json:"artists"`
	StartsAt   int64           `json:"startsAt"`
	EndsAt     int64           `json:"endsAt"`
	Timezone   string          `json:"timezone"`
	Venue      *ConcertVenue   `json:"venue"`
	Tickets    []ConcertTicket `json:"tickets"`
	DistanceKm int             `json:"distanceKm"`
	YourArtist bool            `json:"yourArtist"`
}

// ConcertsPage is the concerts near Location, soonest first. More: Apple
// may have more than Limit of them (ask again with a higher Limit).
// YourArtists counts the concerts by the user's own artists (0 when signed
// out); Genres lists the genres to filter by, "any" first as ID "".
type ConcertsPage struct {
	Location    ConcertLocation `json:"location"`
	Concerts    []Concert       `json:"concerts"`
	Limit       int             `json:"limit"`
	More        bool            `json:"more"`
	YourArtists int             `json:"yourArtists"`
	Genres      []Genre         `json:"genres"`
	FetchedAt   int64           `json:"fetchedAt"`
	Stale       bool            `json:"stale"`
}

// ArtistConcerts is an artist's upcoming concerts: Nearby the chosen
// place, Elsewhere the rest.
type ArtistConcerts struct {
	Location  ConcertLocation `json:"location"`
	Nearby    []Concert       `json:"nearby"`
	Elsewhere []Concert       `json:"elsewhere"`
	FetchedAt int64           `json:"fetchedAt"`
	Stale     bool            `json:"stale"`
}
