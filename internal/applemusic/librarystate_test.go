package applemusic

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestCatalogLibraryStatesBatchesAndMapsFormerIDs(t *testing.T) {
	var mu sync.Mutex
	var sizes []int
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/catalog/gb" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("relate") != "library" || q.Get("platform") != "web" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		var res []string
		n := 0
		for key, v := range q {
			typ, ok := strings.CutPrefix(key, "ids[")
			if !ok {
				continue
			}
			typ = strings.TrimSuffix(typ, "]")
			if v[0] == "" {
				t.Errorf("empty %s sent", key) // Apple answers 400
			}
			if q.Get("fields["+typ+"]") != "inLibrary,personalRating" {
				t.Errorf("fields[%s] = %q", typ, q.Get("fields["+typ+"]"))
			}
			for _, id := range strings.Split(v[0], ",") {
				n++
				switch {
				case id == "old-7":
					// Answered under the new ID with the requested one in formerIds.
					res = append(res, `{"id":"7","type":"songs","attributes":{"inLibrary":true,"personalRating":-1},
						"relationships":{"library":{"data":[{"id":"i.7","type":"library-songs"}]}},"meta":{"formerIds":["old-7"]}}`)
				case id == "a5":
					res = append(res, `{"id":"a5","type":"albums","attributes":{"inLibrary":true},
						"relationships":{"library":{"data":[{"id":"l.5","type":"library-albums"}]}}}`)
				case id == "gone":
				default:
					res = append(res, fmt.Sprintf(`{"id":%q,"type":%q,"attributes":{"inLibrary":false,"personalRating":1},"relationships":{"library":{"data":[]}}}`, id, typ))
				}
			}
		}
		mu.Lock()
		sizes = append(sizes, n)
		mu.Unlock()
		fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(res, ","))
	}))
	c.AMPBase = c.Base

	songs := []string{"old-7", "gone"}
	for i := range 230 {
		songs = append(songs, fmt.Sprint("s", i))
	}
	songs = append(songs, "s1") // duplicates are asked once
	got, err := c.CatalogLibraryStates(context.Background(), "gb", map[string][]string{
		TypeSongs: songs, TypeAlbums: {"a5"}, TypePlaylists: {},
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(sizes)
	if !slices.Equal(sizes, []int{33, 100, 100}) {
		t.Fatalf("batch sizes = %v, want 233 IDs in batches of <=100", sizes)
	}
	if st := got[ItemKey{TypeSongs, "old-7"}]; st != (LibraryState{InLibrary: true, LibraryID: "i.7", Rating: -1}) {
		t.Fatalf("old-7 = %+v", st)
	}
	if st := got[ItemKey{TypeAlbums, "a5"}]; st != (LibraryState{InLibrary: true, LibraryID: "l.5"}) {
		t.Fatalf("a5 = %+v", st)
	}
	if st := got[ItemKey{TypeSongs, "s3"}]; st != (LibraryState{Rating: 1}) {
		t.Fatalf("s3 = %+v", st)
	}
	if _, ok := got[ItemKey{TypeSongs, "gone"}]; ok {
		t.Fatal("unknown ID reported")
	}
	if len(got) != 232 {
		t.Fatalf("%d states", len(got))
	}
}

func TestLibraryItemStates(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/v1/me/library" || q.Get("ids[library-songs]") != "i.1,i.2" || q.Get("fields[library-songs]") != "name,personalRating" {
			t.Errorf("%s ? %s", r.URL.Path, r.URL.RawQuery)
		}
		// i.2 was deleted: Apple leaves it out.
		fmt.Fprint(w, `{"data":[{"id":"i.1","type":"library-songs","attributes":{"name":"x","personalRating":1}}]}`)
	}))
	c.AMPBase = c.Base
	got, err := c.LibraryItemStates(context.Background(), map[string][]string{TypeLibrarySongs: {"i.1", "i.2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[ItemKey{TypeLibrarySongs, "i.1"}] != (LibraryState{InLibrary: true, LibraryID: "i.1", Rating: 1}) {
		t.Fatalf("states = %+v", got)
	}
}

func TestFavoritesAndDeleteFromLibrary(t *testing.T) {
	var got []string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
		if r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/me/library/") {
			w.WriteHeader(http.StatusNotFound) // already gone: still success
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	c.AMPBase = c.Base
	ctx := context.Background()
	if err := c.Favorite(ctx, TypeArtists, []string{"166566"}); err != nil {
		t.Fatal(err)
	}
	if err := c.Unfavorite(ctx, TypeSongs, []string{"1", "2"}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteFromLibrary(ctx, TypeAlbums, "l.5"); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteFromLibrary(ctx, "library-albums", "l.5"); err == nil {
		t.Fatal("unknown collection accepted")
	}
	want := []string{
		"POST /v1/me/favorites?ids%5Bartists%5D=166566",
		"DELETE /v1/me/favorites?ids%5Bsongs%5D=1%2C2",
		"DELETE /v1/me/library/albums/l.5?",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("requests:\n%s", strings.Join(got, "\n"))
	}
}
