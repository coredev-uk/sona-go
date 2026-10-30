package applemusic

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestAMPQueryValues(t *testing.T) {
	q := AMPQuery{
		Views: []string{"top-songs", "full-albums"}, Include: []string{"artists"},
		Extend: []string{"editorialArtwork"}, With: []string{"serverBubbles"},
		Fields: map[string][]string{"albums": {"name", "artwork"}}, Limit: 10,
		Extra: url.Values{"l": {"en-GB"}},
	}
	got := q.Values().Encode()
	want := "extend=editorialArtwork&fields%5Balbums%5D=name%2Cartwork&include=artists&l=en-GB&limit=10&platform=web&views=top-songs%2Cfull-albums&with=serverBubbles"
	if got != want {
		t.Errorf("Values = %s\nwant      %s", got, want)
	}
	if got := (AMPQuery{}).Values().Encode(); got != "platform=web" {
		t.Errorf("zero query = %s", got)
	}
}

func TestAMPResourceAndResources(t *testing.T) {
	var paths []string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path+"?"+r.URL.RawQuery)
		if r.Header.Get("Origin") == "" || r.UserAgent() != UserAgent {
			t.Errorf("headers = %v", r.Header)
		}
		if ids := r.URL.Query().Get("ids"); ids != "" {
			var data []string
			for _, id := range strings.Split(ids, ",") {
				data = append(data, fmt.Sprintf(`{"id":%q,"type":"stations","attributes":{"name":"S%s"}}`, id, id))
			}
			fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(data, ","))
			return
		}
		w.Write([]byte(`{"data":[{"id":"ra.1","type":"stations","attributes":{"name":"Radio"}}]}`))
	}))
	c.AMPBase = c.Base
	type station = Resource[struct {
		Name string `json:"name"`
	}]
	st, err := AMPResource[station](t.Context(), c, "gb", "stations", "ra.1", AMPQuery{Extend: []string{"editorialVideo"}})
	if err != nil || st.Attributes.Name != "Radio" {
		t.Fatalf("resource = %+v, %v", st, err)
	}
	if paths[0] != "/v1/catalog/gb/stations/ra.1?extend=editorialVideo&platform=web" {
		t.Errorf("path = %s", paths[0])
	}
	ids := make([]string, 150)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	all, err := AMPResources[station](t.Context(), c, "gb", "stations", ids, AMPQuery{})
	if err != nil || len(all) != 150 || len(paths) != 3 {
		t.Fatalf("resources: %d items, %d requests, %v", len(all), len(paths), err)
	}
}
