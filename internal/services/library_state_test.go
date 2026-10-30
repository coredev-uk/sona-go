package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

type stubDev struct{}

func (stubDev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev", Expires: time.Now().Add(time.Hour)}, nil
}
func (stubDev) Invalidate() {}

// fakeState answers the library-state and mutation endpoints from a small
// world: catalog song 100 (library i.100, favourite), album 200 (not in
// the library), artist 300 (rated -1), and private playlist p.9.
type fakeState struct {
	mu   sync.Mutex
	reqs []string
}

func (f *fakeState) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.reqs = append(f.reqs, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	q := r.URL.Query()
	switch {
	case r.Method != http.MethodGet:
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/v1/catalog/gb":
		var data []string
		for _, id := range strings.Split(q.Get("ids[songs]"), ",") {
			if id == "100" {
				data = append(data, `{"id":"100","type":"songs","attributes":{"inLibrary":true,"personalRating":1},"relationships":{"library":{"data":[{"id":"i.100"}]}}}`)
			}
		}
		if q.Get("ids[albums]") == "200" {
			data = append(data, `{"id":"200","type":"albums","attributes":{"inLibrary":false},"relationships":{"library":{"data":[]}}}`)
		}
		fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(data, ","))
	case r.URL.Path == "/v1/me/library" && q.Get("ids[library-songs]") == "i.100":
		// Apple acknowledged the DELETE but kept the song (add→remove race).
		fmt.Fprint(w, `{"data":[{"id":"i.100","type":"library-songs","attributes":{"name":"x"}}]}`)
	case r.URL.Path == "/v1/me/library":
		fmt.Fprint(w, `{"data":[{"id":"p.9","type":"library-playlists","attributes":{"name":"Mine"}}]}`)
	case r.URL.Path == "/v1/me/ratings/artists":
		fmt.Fprint(w, `{"data":[{"id":"300","type":"ratings","attributes":{"value":-1}}]}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeState) take() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.reqs
	f.reqs = nil
	slices.Sort(out)
	return out
}

func newStateService(t *testing.T) (*LibraryService, *fakeState, *db.DB, *string) {
	t.Helper()
	fake := &fakeState{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	api := applemusic.New(srv.Client(), stubDev{}, func() string { return "mut" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	e := &Env{Deps: Deps{
		DB: d, API: api, Library: library.New(d, api), Storefront: func() string { return "gb" },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
	}, members: newMembership(d)}
	copied := new(string)
	s := &LibraryService{o: e, m: e.m, setClipboard: func(s string) bool { *copied = s; return true }}
	return s, fake, d, copied
}

func TestStatesBatchCacheAndFallback(t *testing.T) {
	s, fake, _, _ := newStateService(t)
	ctx := t.Context()
	refs := []ItemRef{
		{Kind: "songs", ID: "i.100", Source: SourceLibrary, CatalogID: "100", LibraryID: "i.100"},
		{Kind: "albums", ID: "200", Source: SourceCatalog, CatalogID: "200"},
		{Kind: "artists", ID: "300", Source: SourceCatalog, CatalogID: "300"},
		{Kind: "playlists", ID: "p.9", Source: SourceLibrary, LibraryID: "p.9"},
		{Kind: "songs", ID: "100", Source: SourceCatalog},
		{Kind: "songs", ID: "404", Source: SourceCatalog},
	}
	got, err := s.states(ctx, refs, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []ItemState{
		{Kind: "songs", ID: "i.100", InLibrary: true, LibraryID: "i.100", Rating: 1},
		{Kind: "albums", ID: "200"},
		{Kind: "artists", ID: "300", Rating: -1},
		{Kind: "playlists", ID: "p.9", InLibrary: true, LibraryID: "p.9"},
		{Kind: "songs", ID: "100", InLibrary: true, LibraryID: "i.100", Rating: 1},
		{Kind: "songs", ID: "404"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("states:\n got %+v\nwant %+v", got, want)
	}
	// One request per source: catalog (songs and albums together), library, artist ratings.
	if r := fake.take(); !slices.Equal(r, []string{"GET /v1/catalog/gb", "GET /v1/me/library", "GET /v1/me/ratings/artists"}) {
		t.Fatalf("requests = %v", r)
	}
	// Cached: no requests the second time.
	if _, err := s.states(ctx, refs, true); err != nil {
		t.Fatal(err)
	}
	if r := fake.take(); len(r) != 0 {
		t.Fatalf("cached states refetched: %v", r)
	}
	// Bad refs are rejected.
	if _, err := s.states(ctx, []ItemRef{{Kind: "stations", ID: "ra.1"}}, true); err == nil {
		t.Fatal("station state accepted")
	}

	// Offline: answers from the synced library and local ratings.
	s2, fake2, d, _ := newStateService(t)
	ref, err := d.Q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{LibraryID: db.NullString("l.7"), CatalogID: db.NullString("700"), Name: "A", UpdatedAt: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: kindAlbum, Ref: ref, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.Q.UpsertFavorite(ctx, dbq.UpsertFavoriteParams{Kind: "library-albums", AppleID: "l.7", Value: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	got, _ = s2.states(ctx, []ItemRef{{Kind: "albums", ID: "700", Source: SourceCatalog}}, false)
	if got[0] != (ItemState{Kind: "albums", ID: "700", InLibrary: true, LibraryID: "l.7", Rating: 1}) {
		t.Fatalf("offline state = %+v", got[0])
	}
	if r := fake2.take(); len(r) != 0 {
		t.Fatalf("offline asked Apple: %v", r)
	}
}

func TestRateRemoveAndCopyLink(t *testing.T) {
	s, fake, _, copied := newStateService(t)
	ctx := t.Context()
	album := ItemRef{Kind: "albums", ID: "200", Source: SourceCatalog}

	st, err := s.Favorite(ctx, album, true)
	if err != nil || st != (ItemState{Kind: "albums", ID: "200", Rating: 1}) {
		t.Fatalf("favourite = %+v, %v", st, err)
	}
	st, err = s.SuggestLess(ctx, ItemRef{Kind: "artists", ID: "300", Source: SourceCatalog}, true)
	if err != nil || st.Rating != -1 {
		t.Fatalf("suggest less = %+v, %v", st, err)
	}
	// The user's change wins over Apple's (lagging) answer, without a request.
	got, _ := s.states(ctx, []ItemRef{album}, true)
	if got[0].Rating != 1 {
		t.Fatalf("held state = %+v", got[0])
	}
	if r := fake.take(); !slices.Equal(r, []string{"POST /v1/me/favorites", "PUT /v1/me/ratings/artists/300"}) {
		t.Fatalf("requests = %v", r)
	}

	// Remove asks Apple for the library copy when it is not known yet.
	song := ItemRef{Kind: "songs", ID: "100", Source: SourceCatalog}
	st, err = s.Remove(ctx, song)
	if err != nil || st != (ItemState{Kind: "songs", ID: "100", Rating: 1}) {
		t.Fatalf("remove = %+v, %v", st, err)
	}
	if r := fake.take(); !slices.Equal(r, []string{"DELETE /v1/me/library/songs/i.100", "GET /v1/catalog/gb"}) {
		t.Fatalf("requests = %v", r)
	}
	if _, err := s.Remove(ctx, album); err == nil {
		t.Fatal("removed an album that is not in the library")
	}
	if _, err := s.Remove(ctx, ItemRef{Kind: "playlists", ID: "p.9", Source: SourceLibrary}); err == nil {
		t.Fatal("playlists are deleted by the playlist service")
	}

	u, err := s.CopyLink(ItemRef{Kind: "songs", ID: "i.100", Source: SourceLibrary, CatalogID: "100"})
	if err != nil || u != "https://music.apple.com/gb/song/100" || *copied != u {
		t.Fatalf("copy link = %q (%q), %v", u, *copied, err)
	}
	if u, _ := s.CopyLink(ItemRef{Kind: "stations", ID: "ra.978194965", Source: SourceCatalog}); u != "https://music.apple.com/gb/station/ra.978194965" {
		t.Fatalf("station link = %q", u)
	}
	if _, err := s.CopyLink(ItemRef{Kind: "playlists", ID: "p.9", Source: SourceLibrary}); err == nil {
		t.Fatal("private playlist has a link")
	}
}

func TestStateCacheHoldsChanges(t *testing.T) {
	now := time.Unix(1000, 0)
	m := newMembership(nil)
	m.now = func() time.Time { return now }
	k := applemusic.ItemKey{Type: "songs", ID: "1"}
	m.remember(k, applemusic.LibraryState{Rating: 1})
	if st, ok := m.state(k); !ok || st.Rating != 1 {
		t.Fatalf("remembered = %+v %v", st, ok)
	}
	m.hold(k, applemusic.LibraryState{Rating: -1})
	now = now.Add(stateTTL + time.Second)
	m.remember(k, applemusic.LibraryState{Rating: 1}) // Apple lags: ignored while held
	if st, _ := m.state(k); st.Rating != -1 {
		t.Fatalf("hold lost to Apple's answer: %+v", st)
	}
	now = now.Add(stateHold)
	if _, ok := m.state(k); ok {
		t.Fatal("hold never expired")
	}
	m.remember(k, applemusic.LibraryState{Rating: 1})
	m.clear()
	if _, ok := m.state(k); ok {
		t.Fatal("clear kept the state")
	}
}

func TestRemoveRetriesWhenAppleKeepsTheItem(t *testing.T) {
	s, fake, _, _ := newStateService(t)
	s.removeRecheck = 10 * time.Millisecond
	song := ItemRef{Kind: "songs", ID: "i.100", Source: SourceLibrary, CatalogID: "100", LibraryID: "i.100"}
	st, err := s.Remove(t.Context(), song)
	if err != nil || st.InLibrary {
		t.Fatalf("remove = %+v, %v", st, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		got := fake.take()
		n := 0
		for _, r := range got {
			if r == "DELETE /v1/me/library/songs/i.100" {
				n++
			}
		}
		fake.mu.Lock()
		fake.reqs = append(fake.reqs, got...)
		fake.mu.Unlock()
		if n == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("requests = %v, want the DELETE repeated once", got)
		}
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond) // and only once
	got := fake.take()
	if n := slices.Index(got, "GET /v1/me/library"); n < 0 || len(got) != 3 {
		t.Fatalf("requests = %v, want DELETE, recheck, DELETE", got)
	}
	// The local answer stays "removed" while Apple catches up.
	states, _ := s.states(t.Context(), []ItemRef{song}, true)
	if states[0].InLibrary {
		t.Fatalf("state after removal = %+v", states[0])
	}
}
