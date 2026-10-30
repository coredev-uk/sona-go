package applemusic

import (
	"net/http"
	"slices"
	"testing"
)

const searchBody = `{"results":{
 "top":{"name":"Top Results","data":[{"id":"9","type":"artists","href":"/v1/catalog/gb/artists/9"},{"id":"1","type":"songs"}]},
 "song":{"name":"Songs","next":"/v1/catalog/gb/search?groups=song&offset=21&term=blue&types=songs","data":[{"id":"1","type":"songs"},{"id":"404","type":"songs"}]},
 "album":{"name":"Albums","data":[{"id":"7","type":"albums"}]},
 "station":{"name":"Stations","data":[]}
},
"resources":{
 "songs":{"1":{"id":"1","type":"songs","attributes":{"name":"Blue"},"relationships":{"albums":{"data":[{"id":"7","type":"albums"}]}}}},
 "artists":{"9":{"id":"9","type":"artists","attributes":{"name":"Blue Band"}}},
 "albums":{"7":{"id":"7","type":"albums","attributes":{"name":"Blue Album"}}}
},
"meta":{"results":{"order":["top","artist","song","station"]}}}`

func TestCatalogSearch(t *testing.T) {
	var queries []map[string]string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/gb/search" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := map[string]string{}
		for k, v := range r.URL.Query() {
			q[k] = v[0]
		}
		queries = append(queries, q)
		w.Write([]byte(searchBody))
	}))
	c.AMPBase = c.Base

	res, err := CatalogSearch(t.Context(), c, "gb", "blue", SearchOptions{Limit: 21})
	if err != nil {
		t.Fatal(err)
	}
	q := queries[0]
	for k, want := range map[string]string{
		"term": "blue", "limit": "21", "platform": "web", "format[resources]": "map",
		"include[songs]": "artists", "relate[songs]": "albums", "omit[resource]": "autos",
	} {
		if q[k] != want {
			t.Errorf("%s = %q, want %q", k, q[k], want)
		}
	}
	if _, ok := q["groups"]; ok {
		t.Errorf("first page sent groups=%q", q["groups"])
	}
	if _, ok := q["offset"]; ok {
		t.Errorf("first page sent offset")
	}
	// serverBubbles is required for group pages; every type is searched.
	if q["with"] != "lyricHighlights,lyrics,naturalLanguage,serverBubbles,subtitles" ||
		q["types"] != "activities,albums,apple-curators,artists,curators,editorial-items,music-movies,music-videos,playlists,record-labels,songs,stations,tv-episodes,uploaded-videos" {
		t.Errorf("with=%q types=%q", q["with"], q["types"])
	}

	// Apple's order, empty groups dropped, unordered groups last.
	if got := res.Order(); !slices.Equal(got, []string{"top", "song", "album"}) {
		t.Errorf("order = %v", got)
	}
	top := res.Resources.Resolve(res.Groups[GroupTop].Data)
	if len(top) != 2 || top[0].Type != "artists" || top[1].ID != "1" {
		t.Fatalf("top = %+v", top)
	}
	// References the response did not include are skipped.
	if songs := res.Resources.Resolve(res.Groups[GroupSong].Data); len(songs) != 1 {
		t.Fatalf("songs = %+v", songs)
	}
	if NextOffset(res.Groups[GroupSong].Next) != 21 || res.Groups[GroupAlbum].Next != "" {
		t.Errorf("next: song %q album %q", res.Groups[GroupSong].Next, res.Groups[GroupAlbum].Next)
	}
	song, err := As[Song](res.Resources.Resolve(res.Groups[GroupSong].Data)[0])
	if err != nil || song.Attributes.Name != "Blue" || song.Relationships.Albums.Data[0].ID != "7" {
		t.Fatalf("song = %+v, %v", song, err)
	}

	// A group page.
	if _, err := CatalogSearch(t.Context(), c, "gb", "blue", SearchOptions{Group: GroupSong, Offset: 21, Limit: 99}); err != nil {
		t.Fatal(err)
	}
	q = queries[1]
	if q["groups"] != "song" || q["offset"] != "21" || q["limit"] != "25" {
		t.Errorf("group page query = %v", q)
	}
}

func TestSearchSuggestions(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/v1/catalog/gb/search/suggestions" || q.Get("term") != "blu" || q.Get("kinds") != "terms,topResults" ||
			q.Get("limit[results:terms]") != "5" || q.Get("limit[results:topResults]") != "6" || q.Get("format[resources]") != "map" {
			t.Errorf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Write([]byte(`{"results":{"suggestions":[
			{"kind":"terms","searchTerm":"blue","displayTerm":"blue"},
			{"kind":"topResults","content":{"id":"9","type":"artists"}}]},
			"resources":{"artists":{"9":{"id":"9","type":"artists","attributes":{"name":"Blue Band"}}}}}`))
	}))
	c.AMPBase = c.Base
	res, err := SearchSuggestions(t.Context(), c, "gb", "blu", 5, 6)
	if err != nil {
		t.Fatal(err)
	}
	s := res.Results.Suggestions
	if len(s) != 2 || s[0].SearchTerm != "blue" || s[0].Content != nil || s[1].Content == nil {
		t.Fatalf("suggestions = %+v", s)
	}
	if n, ok := res.Resources.Get(*s[1].Content); !ok || n.ID != "9" {
		t.Fatalf("top result not resolved: %+v", n)
	}
}

func TestSocialSearchLandingAndProfile(t *testing.T) {
	var paths []string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/v1/social/gb/search":
			if q.Get("types") != "playlists,social-profiles" || q.Get("groups") != "profile" || q.Get("offset") != "25" {
				t.Errorf("social query = %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"results":{"profile":{"data":[{"id":"sp.1","type":"social-profiles"}],"next":"/v1/social/gb/search?offset=50"}},
				"resources":{"social-profiles":{"sp.1":{"id":"sp.1","type":"social-profiles","attributes":{"name":"Taylor","handle":"t"}}}}}`))
		case "/v1/recommendations/gb":
			if q.Get("name") != "search-landing" || q.Get("format[resources]") != "map" {
				t.Errorf("landing query = %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"data":[{"id":"r1","type":"personal-recommendation"}],"resources":{
				"personal-recommendation":{"r1":{"id":"r1","type":"personal-recommendation","attributes":{"title":{"stringForDisplay":"Browse Categories"}},
					"relationships":{"contents":{"data":[{"id":"988556214","type":"apple-curators"}]}}}},
				"apple-curators":{"988556214":{"id":"988556214","type":"apple-curators","attributes":{"name":"Apple Music Alternative","kind":"Genre"}}}}}`))
		case "/v1/social/gb/social-profiles":
			if q.Get("ids") != "sp.1" || q.Get("include") != "shared-playlists" {
				t.Errorf("profile query = %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"data":[{"id":"sp.1","type":"social-profiles"}],"resources":{
				"social-profiles":{"sp.1":{"id":"sp.1","type":"social-profiles","attributes":{"name":"Taylor"},
					"relationships":{"shared-playlists":{"data":[{"id":"pl.u-1","type":"playlists"}]}}}},
				"playlists":{"pl.u-1":{"id":"pl.u-1","type":"playlists","attributes":{"name":"Mix"}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	c.AMPBase = c.Base

	soc, err := SocialSearch(t.Context(), c, "gb", "taylor", SearchOptions{Group: GroupProfile, Offset: 25})
	if err != nil || len(soc.Resources.Resolve(soc.Groups[GroupProfile].Data)) != 1 || NextOffset(soc.Groups[GroupProfile].Next) != 50 {
		t.Fatalf("social = %+v, %v", soc, err)
	}

	land, err := SearchLanding(t.Context(), c, "gb")
	if err != nil {
		t.Fatal(err)
	}
	shelves := land.Primary()
	if len(shelves) != 1 || len(land.Resources.Resolve(shelves[0].Rel("contents"))) != 1 {
		t.Fatalf("landing = %+v", land)
	}

	p, res, err := SocialProfile(t.Context(), c, "gb", "sp.1")
	if err != nil || p.ID != "sp.1" {
		t.Fatalf("profile = %+v, %v", p, err)
	}
	if pl := res.Resolve(p.Rel("shared-playlists")); len(pl) != 1 || pl[0].ID != "pl.u-1" {
		t.Fatalf("shared playlists = %+v", pl)
	}
}
