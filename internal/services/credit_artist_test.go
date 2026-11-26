package services

import (
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
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/db"
)

// artistSearch is Apple's catalogue search answering a term from a table of
// artists (id and name, in Apple's order), as the live search does with
// types=artists (checked 2026-11-26: one "artist" group, the best match
// first, and the artists that share the name together). A term the table
// does not hold finds no artist; a term in fail is refused.
type artistSearch struct {
	mu       sync.Mutex
	results  map[string][][2]string
	fail     map[string]bool
	requests []string // the query of each request
}

func (f *artistSearch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.RawQuery)
	found, fail := f.results[q.Get("term")], f.fail[q.Get("term")]
	f.mu.Unlock()
	if fail {
		http.Error(w, `{"errors":[{"status":"400","code":"40008"}]}`, http.StatusBadRequest)
		return
	}
	var refs, nodes []string
	for _, a := range found {
		refs = append(refs, fmt.Sprintf(`{"id":%q,"type":"artists"}`, a[0]))
		nodes = append(nodes, fmt.Sprintf(`%q:{"id":%[1]q,"type":"artists","attributes":{"name":%q}}`, a[0], a[1]))
	}
	_, _ = fmt.Fprintf(w, `{"results":{"artist":{"data":[%s]}},"resources":{"artists":{%s}}}`, strings.Join(refs, ","), strings.Join(nodes, ","))
}

func (f *artistSearch) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

// creditsEnv is a CatalogService over a real SQLite cache and the fake search.
func creditsEnv(t *testing.T, f *artistSearch) *CatalogService {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return "mut" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	sf := func() string { return "gb" }
	e := &Env{Deps: Deps{
		DB: d, API: api, Catalog: catalog.New(d, api, sf, nil), Storefront: sf,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
	}}
	return newCatalogService(e)
}

// The song page's own artists.
var creditOwn = []Artist{{ID: "657515", Name: "Radiohead"}, {ID: "3", Name: "Björk"}, {Name: "No ID"}}

// A credited person whom exactly one artist of the catalogue is named after is
// that artist: the search asks for artists only, a few of them, and the
// answer is kept, so the same name is asked once.
func TestCreditedArtistExactMatchOpensThatArtist(t *testing.T) {
	f := &artistSearch{results: map[string][][2]string{
		"thom yorke": {{"39753073", "Thom Yorke"}, {"547192558", "Atoms for Peace"}, {"657515", "Radiohead"}},
	}}
	cs := creditsEnv(t, f)
	for range 2 {
		id, err := cs.CreditedArtist(t.Context(), "Thom Yorke", creditOwn)
		if err != nil || id != "39753073" {
			t.Fatalf("CreditedArtist = %q, %v, want 39753073", id, err)
		}
	}
	reqs := f.asked()
	if len(reqs) != 1 {
		t.Fatalf("%d requests %q, want one (the second answer is the cache's)", len(reqs), reqs)
	}
	for _, want := range []string{"types=artists", "limit=10", "term=thom+yorke"} {
		if !strings.Contains(reqs[0], want) {
			t.Errorf("request %q lacks %q", reqs[0], want)
		}
	}
}

// Apple lists three artists called "Philip Selway": which one is credited is
// not known, so none is chosen.
func TestCreditedArtistSeveralMatchesChooseNone(t *testing.T) {
	f := &artistSearch{results: map[string][][2]string{
		"philip selway": {{"1624083", "Philip Selway"}, {"1467571603", "Philip Selway"}, {"1470458147", "Philip Selway"}, {"657515", "Radiohead"}},
	}}
	id, err := creditsEnv(t, f).CreditedArtist(t.Context(), "Philip Selway", creditOwn)
	if err != nil || id != "" {
		t.Fatalf("CreditedArtist = %q, %v, want none and no error", id, err)
	}
	if n := len(f.asked()); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

// A name no artist has (the search finds a band the person plays in, or a
// near name) is no artist: "Edward O'Brien" is not "Ed O'Brien".
func TestCreditedArtistNoMatchChoosesNone(t *testing.T) {
	f := &artistSearch{results: map[string][][2]string{
		"colin greenwood": {{"657515", "Radiohead"}},
		"edward o'brien":  {{"156334706", "Ed O'Brien"}, {"1733399908", "Edward J O'brien"}},
		"nobody":          nil,
	}}
	cs := creditsEnv(t, f)
	for _, name := range []string{"Colin Greenwood", "Edward O'Brien", "Nobody", "  "} {
		if id, err := cs.CreditedArtist(t.Context(), name, creditOwn); err != nil || id != "" {
			t.Errorf("CreditedArtist(%q) = %q, %v, want none and no error", name, id, err)
		}
	}
	if n := len(f.asked()); n != 3 {
		t.Errorf("%d requests %q, want 3 (a blank name asks nothing)", n, f.asked())
	}
}

// A credit named like one of the song's own artists is that artist, with no
// request at all: ignoring case, accents and a typographic apostrophe. An
// artist the page holds without an ID is no artist to open.
func TestCreditedArtistOwnArtistNeedsNoRequest(t *testing.T) {
	f := &artistSearch{}
	cs := creditsEnv(t, f)
	for name, want := range map[string]string{"Radiohead": "657515", "RADIOHEAD": "657515", "  radiohead ": "657515", "Bjork": "3", "BJÖRK": "3"} {
		id, err := cs.CreditedArtist(t.Context(), name, creditOwn)
		if err != nil || id != want {
			t.Errorf("CreditedArtist(%q) = %q, %v, want %q", name, id, err, want)
		}
	}
	if reqs := f.asked(); len(reqs) != 0 {
		t.Errorf("requests %q for the song's own artists, want none", reqs)
	}
	if _, ok := OwnArtist("No ID", creditOwn); ok {
		t.Error("an artist with no ID is opened")
	}
	if a, ok := OwnArtist("Ed O’Brien", []Artist{{ID: "9", Name: "Ed O'Brien"}}); !ok || a.ID != "9" {
		t.Errorf("a typographic apostrophe: %+v, %v", a, ok)
	}
}

// Names are compared without case or accents, so a credit written plainly
// finds the artist's accented name, and the one match is taken.
func TestCreditedArtistFoldsCaseAndAccents(t *testing.T) {
	f := &artistSearch{results: map[string][][2]string{"sigur ros": {{"73720797", "Sigur Rós"}, {"1", "Sigur Rós Tribute"}}}}
	id, err := creditsEnv(t, f).CreditedArtist(t.Context(), "SIGUR ROS", nil)
	if err != nil || id != "73720797" {
		t.Fatalf("CreditedArtist = %q, %v, want 73720797", id, err)
	}
}

// A search that fails is the caller's error to show as it likes (the page
// opens the search for the name).
func TestCreditedArtistSearchFailureIsAnError(t *testing.T) {
	f := &artistSearch{fail: map[string]bool{"thom yorke": true}}
	id, err := creditsEnv(t, f).CreditedArtist(t.Context(), "Thom Yorke", creditOwn)
	if err == nil || id != "" {
		t.Fatalf("CreditedArtist = %q, %v, want an error", id, err)
	}
}
