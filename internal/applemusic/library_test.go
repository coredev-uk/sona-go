package applemusic

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
)

// The library album walk goes to the web player's host with the catalog
// album's release fields included: api.music.apple.com leaves isPrerelease
// out of the include, amp-api serves it. A pre-added album arrives with no
// releaseDate of its own.
func TestLibraryAlbumsAskForTheCatalogAlbumsReleaseFields(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		q := r.URL.Query()
		if r.URL.Path != "/amp/v1/me/library/albums" || q.Get("platform") != "web" || q.Get("include") != "catalog" ||
			q.Get("fields[albums]") != "isSingle,releaseDate,isPrerelease" {
			t.Errorf("request = %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"l.1","type":"library-albums","attributes":{"name":"Soon","trackCount":34},
			"relationships":{"catalog":{"data":[{"id":"7","type":"albums","attributes":{"isSingle":false,"releaseDate":"2026-11-20","isPrerelease":true}}]}}}],
			"meta":{"total":1}}`)
	}))
	c.AMPBase = c.Base + "/amp"
	var got []Album
	if _, err := c.LibraryAlbums(context.Background(), PageOptions{}, func(_ int, a []Album) error { got = append(got, a...); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Attributes.ReleaseDate != "" {
		t.Fatalf("albums = %+v", got)
	}
	cat := got[0].Relationships.Catalog.Data[0].Attributes
	if !cat.IsPrerelease || cat.ReleaseDate != "2026-11-20" {
		t.Fatalf("catalog album = %+v", cat)
	}
	page, err := c.LibraryAlbumsPage(context.Background(), 10, 0)
	if err != nil || len(page.Data) != 1 {
		t.Fatalf("page = %+v, %v", page, err)
	}
}

func TestLibraryByIDAsksForTheItemsAndNothingElse(t *testing.T) {
	var path, ids string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, ids = r.URL.Path, r.URL.Query().Get("ids")
		if q := r.URL.Query(); q.Has("sort") || q.Has("limit") || q.Has("offset") {
			t.Errorf("a lookup by ID is sorted or paged: %s", r.URL.RawQuery) // Apple answers 400
		}
		if ids == "gone" {
			http.Error(w, `{"errors":[]}`, http.StatusNotFound)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"i.1","type":"library-songs","attributes":{"name":"Out","playParams":{"id":"i.1","kind":"song","isLibrary":true}}}]}`)
	}))
	c.AMPBase = c.Base
	songs, err := c.LibrarySongsByID(context.Background(), []string{"i.1", "i.2"})
	if err != nil || len(songs) != 1 || songs[0].Attributes.PlayParams == nil {
		t.Fatalf("songs = %+v, %v", songs, err)
	}
	if path != "/v1/me/library/songs" || ids != "i.1,i.2" {
		t.Fatalf("asked %s ids=%s", path, ids)
	}
	if none, err := c.LibrarySongsByID(context.Background(), nil); err != nil || none != nil {
		t.Fatalf("no IDs = %v, %v", none, err)
	}
	if none, err := c.LibrarySongsByID(context.Background(), []string{"gone"}); err != nil || none != nil {
		t.Fatalf("a 404 = %v, %v", none, err)
	}
	if _, err := c.LibraryAlbumsByID(context.Background(), make([]string, 101)); err == nil {
		t.Fatal("101 IDs in one request")
	}
}

func TestPlaylistModifiedFollowsTheCatalogCopyOfAPersonalMix(t *testing.T) {
	mk := func(globalID, libStamp, catStamp string) Playlist {
		p := Playlist{Resource: Resource[PlaylistAttributes]{
			ID: "p.1", Type: TypeLibraryPlaylists,
			Attributes: PlaylistAttributes{LastModifiedDate: libStamp, PlayParams: &PlayParams{GlobalID: globalID}},
		}}
		if catStamp != "" {
			p.Relationships.Catalog.Data = []Playlist{{Resource: Resource[PlaylistAttributes]{
				ID: globalID, Type: TypePlaylists, Attributes: PlaylistAttributes{LastModifiedDate: catStamp},
			}}}
		}
		return p
	}
	for _, c := range []struct {
		name string
		p    Playlist
		want string
	}{
		{"a mix takes its catalog copy's stamp", mk("pl.pm-1", "2019", "2026"), "2026"},
		{"a mix without a catalog copy keeps its own", mk("pl.pm-1", "2019", ""), "2019"},
		{"another playlist keeps its own", mk("pl.u-1", "2019", "2026"), "2019"},
		{"a private playlist keeps its own", mk("", "2019", ""), "2019"},
	} {
		if got := c.p.Modified(); got != c.want {
			t.Errorf("%s: Modified = %q, want %q", c.name, got, c.want)
		}
	}
}
