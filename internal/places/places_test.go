package places

import (
	"math"
	"strings"
	"testing"
)

func TestGeohashMatchesApplesCells(t *testing.T) {
	// The cells Apple's own concerts hub uses for its default cities.
	for id, want := range map[string]string{
		"gb-london": "gcpv", "us-new-york": "dr5r", "us-los-angeles": "9q5c", "jp-tokyo": "xn76",
		"fr-paris": "u09t", "au-sydney": "r3gx", "de-berlin": "u33d", "ca-toronto": "dpz8", "kr-seoul": "wydm",
		"us-nashville": "dn6m", "gb-manchester": "gcw2", "us-seattle": "c23n",
	} {
		c, ok := ByID(id)
		if !ok {
			t.Fatalf("no city %s", id)
		}
		if got := c.Geohash(); got != want {
			t.Errorf("%s: geohash %s, want %s", id, got, want)
		}
	}
	// The reference value from the geohash definition (Wikipedia's example).
	if got := Geohash(57.64911, 10.40744, 11); got != "u4pruydqqvj" {
		t.Errorf("Geohash(57.64911, 10.40744, 11) = %s, want u4pruydqqvj", got)
	}
}

func TestDistance(t *testing.T) {
	london, _ := ByID("gb-london")
	paris, _ := ByID("fr-paris")
	d := DistanceKm(london.Latitude, london.Longitude, paris.Latitude, paris.Longitude)
	if math.Abs(d-344) > 5 {
		t.Errorf("London to Paris = %.0f km, want about 344", d)
	}
}

func TestListIsUsable(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range load().all {
		if seen[c.ID] {
			t.Errorf("duplicate ID %s", c.ID)
		}
		seen[c.ID] = true
		if !strings.HasPrefix(c.ID, c.Country+"-") {
			t.Errorf("%s does not start with its country %s", c.ID, c.Country)
		}
		if c.Latitude < -90 || c.Latitude > 90 || c.Longitude < -180 || c.Longitude > 180 {
			t.Errorf("%s has coordinates out of range", c.ID)
		}
	}
}

func TestDefaultIsTheMainCityOfTheStorefront(t *testing.T) {
	for sf, want := range map[string]string{
		"gb": "gb-london", "us": "us-new-york", "de": "de-berlin", "JP": "jp-tokyo", "au": "au-sydney",
		"zz": "gb-london", "": "gb-london",
	} {
		if got := Default(sf).ID; got != want {
			t.Errorf("Default(%q) = %s, want %s", sf, got, want)
		}
	}
}

func TestSearch(t *testing.T) {
	ids := func(cs []City) string {
		var out []string
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return strings.Join(out, " ")
	}
	for _, tc := range []struct {
		query, storefront, want string // want: the first result
	}{
		{"lond", "gb", "gb-london"},
		{"LONDON", "ca", "ca-london"}, // the storefront's own London first
		{"london uk", "ca", "gb-london"},
		{"munchen", "gb", "de-munich"}, // accents folded, other names found
		{"köln", "gb", "de-cologne"},
		{"new y", "gb", "us-new-york"},
		{"sao pau", "gb", "br-sao-paulo"},
		{"東京", "gb", "jp-tokyo"},
		{"paris fr", "gb", "fr-paris"},
	} {
		got := Search(tc.query, tc.storefront, 5)
		if len(got) == 0 || got[0].ID != tc.want {
			t.Errorf("Search(%q, %q) = %s, want %s first", tc.query, tc.storefront, ids(got), tc.want)
		}
	}
	if got := Search("zzzzqq", "gb", 5); len(got) != 0 {
		t.Errorf("Search(nonsense) = %s, want nothing", ids(got))
	}
	if got := Search("   ", "gb", 5); got != nil {
		t.Errorf("Search(blank) = %v, want nil", got)
	}
}

func TestSuggestedStartsWithTheStorefrontsCities(t *testing.T) {
	got := Suggested("de", 8)
	if len(got) != 8 || got[0].ID != "de-berlin" {
		t.Fatalf("Suggested(de, 8) = %v", got)
	}
	got = Suggested("ie", 8)
	if got[0].ID != "ie-dublin" {
		t.Errorf("first = %s, want ie-dublin", got[0].ID)
	}
	for _, c := range got[5:] {
		if c.Country == "ie" {
			t.Errorf("%s is Irish but listed among the world cities", c.ID)
		}
	}
}
