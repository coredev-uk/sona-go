package applemusic

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestMapResponseResolve(t *testing.T) {
	body := `{"data":[{"id":"1","type":"albums","meta":{"reason":{"stringForDisplay":"New Release"}}},{"id":"9","type":"albums"}],
	"next":"/v1/editorial/gb/rooms/5/contents?offset=100",
	"resources":{"albums":{"1":{"id":"1","type":"albums","attributes":{"name":"Abbey Road","artistName":"The Beatles"},"relationships":{"artists":{"data":[{"id":"136975","type":"artists"}]}}}}}}`
	var r MapResponse
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatal(err)
	}
	nodes := r.Primary()
	if len(nodes) != 1 || nodes[0].ID != "1" {
		t.Fatalf("primary = %+v (missing references must be skipped)", nodes)
	}
	if r.Data[0].Reason() != "New Release" || r.Data[1].Reason() != "" {
		t.Errorf("reasons = %q %q", r.Data[0].Reason(), r.Data[1].Reason())
	}
	a, err := As[Album](nodes[0])
	if err != nil || a.Attributes.Name != "Abbey Road" || a.Relationships.Artists.Data[0].ID != "136975" {
		t.Errorf("album = %+v, %v", a, err)
	}
	if NextOffset(r.Next) != 100 {
		t.Errorf("next offset = %d", NextOffset(r.Next))
	}

	// A Node round-trips through JSON unchanged (the discover cache stores
	// responses this way).
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var again MapResponse
	if err := json.Unmarshal(b, &again); err != nil {
		t.Fatal(err)
	}
	if n := again.Primary(); len(n) != 1 || string(n[0].Raw) != string(nodes[0].Raw) {
		t.Errorf("round trip lost the node: %s", b)
	}

	// Merge replaces by type and ID.
	r.Resources.Merge(Resources{"albums": {"1": {ID: "1", Type: "albums", Raw: json.RawMessage(`{"id":"1","type":"albums","attributes":{"name":"Let It Be"}}`)}}})
	if a, _ := As[Album](r.Primary()[0]); a.Attributes.Name != "Let It Be" {
		t.Errorf("merged = %+v", a)
	}
}

func TestDiscoverEndpoints(t *testing.T) {
	type call struct {
		method, path string
		q            url.Values
	}
	var calls []call
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, call{r.Method, r.URL.Path, r.URL.Query()})
		w.Write([]byte(`{"data":[],"resources":{},"results":{}}`))
	}))
	c.AMPBase = c.Base
	ctx := t.Context()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := Recommendations(ctx, c, "+01:00")
	must(err)
	_, err = RefreshRecommendation(ctx, c, "+01:00", "7-abc")
	must(err)
	_, err = RecommendationSeeAll(ctx, c, "+01:00", "7-abc")
	must(err)
	_, err = Grouping(ctx, c, "gb", "music")
	must(err)
	_, err = RoomContents(ctx, c, "gb", "55", "alphabet", 100)
	must(err)
	_, err = CuratorPage(ctx, c, "gb", "990050553")
	must(err)
	_, err = Charts(ctx, c, "gb", ChartsQuery{ChartID: "135", Offset: 50})
	must(err)

	want := []struct {
		method, path string
		q            map[string]string
	}{
		{"GET", "/v1/me/recommendations", map[string]string{"name": "listen-now", "platform": "web", "timezone": "+01:00", "format[resources]": "map", "with": "friendsMix,library,social"}},
		{"POST", "/v1/me/recommendations/7-abc", map[string]string{"action": "refresh", "name": "listen-now", "with": "library"}},
		{"GET", "/v1/me/recommendations/7-abc/view/see-all", map[string]string{"limit[contents]": "100", "include[personal-recommendation]": "contents"}},
		{"GET", "/v1/editorial/gb/groupings", map[string]string{"name": "music", "tabs": "subscriber", "format[resources]": "map"}},
		{"GET", "/v1/editorial/gb/rooms/55/contents", map[string]string{"offset": "100", "sort": "alphabet"}},
		{"GET", "/v1/catalog/gb", map[string]string{"ids[apple-curators]": "990050553", "ids[curators]": "990050553", "include": "grouping,playlists"}},
		{"GET", "/v1/catalog/gb/charts", map[string]string{"chartId": "135", "offset": "50", "genre": "34", "limit": "50", "format[resources]": "map"}},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v", calls)
	}
	for i, w := range want {
		got := calls[i]
		if got.method != w.method || got.path != w.path {
			t.Errorf("call %d = %s %s, want %s %s", i, got.method, got.path, w.method, w.path)
		}
		for k, v := range w.q {
			if got.q.Get(k) != v {
				t.Errorf("call %d %s: %s = %q, want %q", i, w.path, k, got.q.Get(k), v)
			}
		}
	}
}

func TestChartID(t *testing.T) {
	c := Chart{Href: "/v1/catalog/gb/charts?chartId=135&genre=34&limit=50"}
	if c.ChartID() != "135" || (Chart{Href: "/v1/catalog/gb/charts?chart=most-played"}).ChartID() != "" {
		t.Errorf("chart IDs = %q", c.ChartID())
	}
}
