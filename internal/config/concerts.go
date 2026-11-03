package config

import "github.com/coredev-uk/sona-go/internal/places"

// Concerts is where the Concerts page looks (internal/places).
type Concerts struct {
	// City is the ID of the chosen city in internal/places's list
	// ("gb-manchester"); "" is the main city of the Apple Music
	// storefront. It stays on this computer: only the geohash of the
	// city's area goes to Apple, with each concerts request.
	City string `json:"city"`
}

// normalized drops a city the list no longer has, so the page falls back to
// its default instead of failing.
func (c Concerts) normalized() Concerts {
	if _, ok := places.ByID(c.City); !ok {
		c.City = ""
	}
	return c
}
