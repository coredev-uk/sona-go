package applestrings

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeApple serves a page, an index script and French and German
// bundles, counting requests.
type fakeApple struct {
	mu      sync.Mutex
	release string // index script hash
	french  string // the French bundle's "Add"
	hits    map[string]int
	srv     *httptest.Server
}

func newFakeApple(t *testing.T) *fakeApple {
	a := &fakeApple{release: "aaa111", french: "Ajouter", hits: map[string]int{}}
	a.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.mu.Lock()
		defer a.mu.Unlock()
		a.hits[r.URL.Path]++
		if r.Header.Get("User-Agent") != UserAgent {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		frChunk := "translations~f" + a.release + ".js"
		switch r.URL.Path {
		case indexPath:
			_, _ = fmt.Fprintf(w, `<html><script type="module" src="/assets/index~%s.js"></script></html>`, a.release)
		case "/assets/index~" + a.release + ".js":
			_, _ = fmt.Fprintf(w, `x({"../locales/fr-fr/translations.json":()=>k(()=>import("./%s"),[]),"../locales/de-de/translations.json":()=>k(()=>import("./translations~d0d0.js"),[]),"../locales/en-gb/translations.json":()=>k(()=>import("./translations~e0e0.js"),[])})`, frChunk)
		case "/assets/" + frChunk:
			_, _ = fmt.Fprintf(w, `const e=%q,o={"A":e,"P":"Lire @@title@@","S":"Mélanger","N.one":"@@count@@ morceau","N.other":"@@count@@ morceaux","X":"pas mappé"};export{o as default};`, a.french)
		case "/assets/translations~d0d0.js":
			_, _ = fmt.Fprint(w, `const o={"A":"Hinzufügen"};export{o as default};`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(a.srv.Close)
	return a
}

func (a *fakeApple) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, c := range a.hits {
		n += c
	}
	return n
}

var testCatalogue = Catalogue{
	Mappings: []Mapping{
		{Message: "Add", Key: "A"},
		{Message: "Play {name}", Key: "P", Placeholders: map[string]string{"title": "name"}},
		{Message: "Shuffle {name}", Key: "S", Placeholders: map[string]string{"title": "name"}},
		{Message: "{n, plural, one {# song} other {# songs}}", Key: "N", Plural: true, Placeholders: map[string]string{"count": "n"}},
	},
	Messages: 10,
	Version:  "map1",
}

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newTestStore(t *testing.T, dir string, a *fakeApple, clk *clock, cat Catalogue) (*Store, chan Locale) {
	t.Helper()
	s, err := New(Options{Dir: dir, HTTP: a.srv.Client(), Origin: a.srv.URL, Catalogue: &cat, Now: clk.now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	got := make(chan Locale, 4)
	s.Subscribe(func(l Locale) { got <- l })
	return s, got
}

func waitLocale(t *testing.T, ch chan Locale) Locale {
	t.Helper()
	select {
	case l := <-ch:
		return l
	case <-time.After(5 * time.Second):
		t.Fatal("no update")
		return Locale{}
	}
}

// refreshNow runs the refresh the worker runs after Start, on the test's
// goroutine.
func refreshNow(t *testing.T, s *Store) {
	t.Helper()
	s.refresh(t.Context())
}

func TestStoreFetchesAfterStartThenServesCacheOffline(t *testing.T) {
	a := newFakeApple(t)
	dir := t.TempDir()
	clk := &clock{t: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	s, updates := newTestStore(t, dir, a, clk, testCatalogue)

	if l := s.Lookup("fr-FR"); l.Tag != "" || len(l.Messages) != 0 {
		t.Fatalf("first run lookup = %+v", l)
	}
	if a.count() != 0 {
		t.Fatal("requests before Start")
	}
	s.Start()
	l := waitLocale(t, updates)
	add, song := MessageID("Add", ""), MessageID("{n, plural, one {# song} other {# songs}}", "")
	// "Shuffle {name}" is refused: Apple's text drops the name.
	if l.Tag != "fr-FR" || len(l.Messages) != 3 || l.Messages[add] != "Ajouter" || l.Messages[MessageID("Play {name}", "")] != "Lire {name}" ||
		l.Messages[song] != "{n, plural, one {# morceau} other {# morceaux}}" || l.Version != "translations~faaa111.js" {
		t.Fatalf("update = %+v", l)
	}
	if l.Coverage() != 0.3 {
		t.Fatalf("coverage = %v", l.Coverage())
	}
	if got := s.Lookup("fr"); got.Messages[add] != "Ajouter" {
		t.Fatalf("lookup after download = %+v", got)
	}

	// The next start, offline: the cache serves at once and, within a
	// day, nothing is requested.
	a.srv.Close()
	before := a.count()
	s2, updates2 := newTestStore(t, dir, a, clk, testCatalogue)
	if got := s2.Lookup("fr-FR"); got.Messages[add] != "Ajouter" || got.Tag != "fr-FR" {
		t.Fatalf("cached lookup = %+v", got)
	}
	refreshNow(t, s2)
	if a.count() != before {
		t.Fatalf("requests within a day: %d", a.count()-before)
	}
	// A day later, offline: the check fails, the cache still serves.
	clk.add(25 * time.Hour)
	s3, _ := newTestStore(t, dir, a, clk, testCatalogue)
	s3.Lookup("fr-FR")
	refreshNow(t, s3)
	if got := s3.Lookup("fr-FR"); got.Messages[add] != "Ajouter" {
		t.Fatalf("offline lookup = %+v", got)
	}
	select {
	case l := <-updates2:
		t.Fatalf("unexpected update %+v", l)
	default:
	}
}

func TestStoreRefreshesDailyAndOnNewRelease(t *testing.T) {
	a := newFakeApple(t)
	dir := t.TempDir()
	clk := &clock{t: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	s, updates := newTestStore(t, dir, a, clk, testCatalogue)
	s.Lookup("fr-FR")
	s.Start()
	waitLocale(t, updates)

	// A day later, the same release: only the page is asked for.
	clk.add(25 * time.Hour)
	s2, updates2 := newTestStore(t, dir, a, clk, testCatalogue)
	s2.Lookup("fr-FR")
	before := a.count()
	refreshNow(t, s2)
	if n := a.count() - before; n != 1 {
		t.Fatalf("same release: %d requests, want 1 (the page)", n)
	}

	// A new release with new French text: downloaded and announced.
	a.mu.Lock()
	a.release, a.french = "bbb222", "Ajouter à"
	a.mu.Unlock()
	clk.add(25 * time.Hour)
	s3, updates3 := newTestStore(t, dir, a, clk, testCatalogue)
	s3.Lookup("fr-FR")
	s3.Start()
	if l := waitLocale(t, updates3); l.Messages[MessageID("Add", "")] != "Ajouter à" || l.Version != "translations~fbbb222.js" {
		t.Fatalf("new release = %+v", l)
	}
	select {
	case l := <-updates2:
		t.Fatalf("unexpected update %+v", l)
	default:
	}
}

func TestStoreReconvertsForANewMapping(t *testing.T) {
	a := newFakeApple(t)
	dir := t.TempDir()
	clk := &clock{t: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	s, updates := newTestStore(t, dir, a, clk, testCatalogue)
	s.Lookup("fr-FR")
	s.Start()
	waitLocale(t, updates)

	cat := testCatalogue
	cat.Version = "map2"
	cat.Mappings = append(cat.Mappings[:1:1], Mapping{Message: "Shuffle", Key: "S"})
	s2, updates2 := newTestStore(t, dir, a, clk, cat)
	// Before the download, the cache keeps only messages still mapped.
	if l := s2.Lookup("fr-FR"); len(l.Messages) != 1 {
		t.Fatalf("stale cache = %+v", l.Messages)
	}
	s2.Start()
	l := waitLocale(t, updates2)
	if l.Messages[MessageID("Shuffle", "")] != "Mélanger" || len(l.Messages) != 2 {
		t.Fatalf("reconverted = %+v", l.Messages)
	}
}

func TestStoreIgnoresEnglishAndDamagedCache(t *testing.T) {
	a := newFakeApple(t)
	dir := t.TempDir()
	clk := &clock{t: time.Now()}
	s, _ := newTestStore(t, dir, a, clk, testCatalogue)
	if l := s.Lookup("en-GB"); l.Tag != "" {
		t.Fatalf("english = %+v", l)
	}
	refreshNow(t, s)
	if a.count() != 0 {
		t.Fatalf("%d requests for English only", a.count())
	}

	// Damaged and foreign cache files are ignored, never trusted.
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"format":1,"script":"https://evil.example/assets/x.js","locales":{"fr-fr":"https://evil.example/assets/t.js"},"checkedAt":"`+clk.now().Format(time.RFC3339)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fr-fr.json"), []byte(strings.Repeat("{", 10)), 0o600); err != nil {
		t.Fatal(err)
	}
	s2, updates := newTestStore(t, dir, a, clk, testCatalogue)
	if l := s2.Lookup("fr-FR"); l.Tag != "" {
		t.Fatalf("untrusted cache served: %+v", l)
	}
	s2.Start()
	if l := waitLocale(t, updates); l.Tag != "fr-FR" {
		t.Fatalf("after re-download = %+v", l)
	}
}

func TestStoreRefusesOversizedBundle(t *testing.T) {
	big := strings.Repeat("a", maxBundleBytes+1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()
	f := &Fetcher{HTTP: srv.Client(), Origin: srv.URL}
	if _, err := f.Bundle(t.Context(), srv.URL+"/assets/translations~x.js"); err == nil || !strings.Contains(err.Error(), ErrTooLarge.Error()) {
		t.Fatalf("err = %v, want too large", err)
	}
}

func TestStoreStaysOnApplesOrigin(t *testing.T) {
	a := newFakeApple(t)
	away := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, a.srv.URL+r.URL.Path, http.StatusFound)
	}))
	defer away.Close()
	// Apple's origin (here `away`) redirecting elsewhere is refused.
	s, err := New(Options{Dir: t.TempDir(), HTTP: away.Client(), Origin: away.URL, Catalogue: &testCatalogue})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Lookup("fr-FR")
	refreshNow(t, s)
	if a.count() != 0 {
		t.Fatalf("followed a redirect off Apple's origin: %v", a.hits)
	}
}
