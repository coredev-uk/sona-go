package applemusic

import (
	"net/http"
	"net/url"
	"os"
	"testing"
)

func concertsFixture(t *testing.T, name string, seen *url.Values, path *string) *Client {
	t.Helper()
	body, err := os.ReadFile("testdata/concerts/" + name)
	if err != nil {
		t.Fatal(err)
	}
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen, *path = r.URL.Query(), r.URL.Path
		_, _ = w.Write(body)
	}))
	c.AMPBase = c.Base
	return c
}

func TestConcertsNearJoinsVenuesAndArtists(t *testing.T) {
	var q url.Values
	var path string
	c := concertsFixture(t, "hub.json", &q, &path)
	got, err := ConcertsNear(t.Context(), c, "gb", ConcertsQuery{
		GeoHash: "gcpv", From: "2027-04-01", To: "2027-06-30", Genres: []string{"20", "21"}, Limit: 5000, Favourites: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if path != "/v1/concerts/gb/hub" {
		t.Errorf("path = %s", path)
	}
	for k, want := range map[string]string{
		"geoHashLocation":                  "gcpv",
		"containers":                       "popular-concerts,favorite-artists-concerts",
		"limit[popular-concerts]":          "500", // capped
		"filter[date]":                     "2027-04-01..2027-06-30",
		"filter[genre]":                    "20,21",
		"extend[concerts]":                 "tickets",
		"include[concerts]":                "venues,artists",
		"format[resources]":                "map",
		"platform":                         "web",
		"limit[favorite-artists-concerts]": "500",
	} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}

	// ce.0404 is listed but not in the resources; ce.0003 comes only from the user's artists.
	ids := []string{}
	for _, c := range got.Concerts {
		ids = append(ids, c.ID)
	}
	if want := "ce.0001 ce.0002 ce.0003"; join(ids) != want {
		t.Errorf("concerts = %s, want %s", join(ids), want)
	}
	if join(got.Favourites) != "ce.0002 ce.0003" {
		t.Errorf("favourites = %v", got.Favourites)
	}

	first := got.Concerts[0]
	if first.Name != "Example Band with Opener Duo @ Example Hall" || first.Timezone != "Europe/London" {
		t.Errorf("first = %+v", first)
	}
	if len(first.Tickets) != 1 || first.Tickets[0].Vendor != "Example Tickets" {
		t.Errorf("tickets = %+v (a ticket without a link is dropped)", first.Tickets)
	}
	if v := first.Venue; v == nil || v.Name != "Example Hall" || v.City != "London" || v.CountryCode != "GB" || v.Region != "" || v.Latitude != 51.5 {
		t.Errorf("venue = %+v", v)
	}
	if len(first.Artists) != 1 || first.Artists[0].Name != "Example Band" || first.Artists[0].Artwork.Template() == "" {
		t.Errorf("artists = %+v", first.Artists)
	}
	if st := first.StartTime(); st.Hour() != 17 || st.Month() != 4 {
		t.Errorf("start = %v", st)
	}
	if v := got.Concerts[2].Venue; v != nil {
		t.Errorf("a concert with no venue has %+v", v)
	}
	if got.Concerts[1].Artists[0].Artwork != nil {
		t.Errorf("an artist without artwork got some")
	}
}

func TestConcertsNearNeedsAnArea(t *testing.T) {
	var q url.Values
	var path string
	c := concertsFixture(t, "hub.json", &q, &path)
	if _, err := ConcertsNear(t.Context(), c, "gb", ConcertsQuery{}); err == nil {
		t.Fatal("Apple answers 500 without a geohash; Sona must not ask")
	}
	if path != "" {
		t.Errorf("asked Apple anyway: %s", path)
	}
	// One date alone is no range; defaults apply and favourites are not asked for.
	if _, err := ConcertsNear(t.Context(), c, "gb", ConcertsQuery{GeoHash: "gcpv", From: "2027-04-01"}); err != nil {
		t.Fatal(err)
	}
	if q.Has("filter[date]") || q.Get("limit[popular-concerts]") != "100" || q.Get("containers") != "popular-concerts" {
		t.Errorf("query = %v", q)
	}
}

func TestArtistConcerts(t *testing.T) {
	var q url.Values
	var path string
	c := concertsFixture(t, "artist.json", &q, &path)
	got, err := ArtistConcerts(t.Context(), c, "gb", "900001", "gcpv")
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/catalog/gb/artists/900001" || q.Get("views") != "all-upcoming-concerts,nearby-upcoming-concerts" || q.Get("geoHashLocation") != "gcpv" {
		t.Errorf("request = %s %v", path, q)
	}
	if len(got.All) != 2 || len(got.Nearby) != 1 || got.Nearby[0].ID != "ce.0001" {
		t.Fatalf("got %+v", got)
	}
	// The concerts name no artists; the page's artist stands in.
	if a := got.All[1].Artists; len(a) != 1 || a[0].Name != "Example Band" || a[0].ID != "900001" {
		t.Errorf("artists = %+v", a)
	}
	if v := got.All[1].Venue; v == nil || v.Region != "NY" || v.Name != "Faraway Stadium" {
		t.Errorf("venue = %+v", v)
	}

	// Without an area the nearby view is not asked for.
	got, err = ArtistConcerts(t.Context(), c, "gb", "900001", "")
	if err != nil || q.Get("views") != "all-upcoming-concerts" || q.Has("geoHashLocation") {
		t.Errorf("without area: %v %v", q, err)
	}
	if got.Nearby == nil {
		t.Error("Nearby is nil, want an empty list")
	}
}

func join(s []string) string {
	out := ""
	for i, v := range s {
		if i > 0 {
			out += " "
		}
		out += v
	}
	return out
}
