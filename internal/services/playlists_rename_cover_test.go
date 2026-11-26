package services

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
)

// genCover is a cover Apple generated for a playlist called name: the name
// is in it as base64 (t=).
func genCover(name string) string {
	return "https://is1-ssl.mzstatic.com/image/thumb/gen/1200x1200AM.PDCXS11.jpg?c1=ff0000&t=" +
		url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(name))) + "&tc=ffffff&signature=" + url.QueryEscape("sig-"+name)
}

// stored is genCover as the library keeps it: a {w}x{h} template.
func stored(name string) string {
	return (&applemusic.Artwork{URL: genCover(name)}).Template()
}

// fakeRenameAPI answers a library playlist's PATCH and GET. The GET answers
// with covers[i] on its i-th call (the last one after that); "" is a
// playlist without artwork.
type fakeRenameAPI struct {
	mu     sync.Mutex
	covers []string
	gets   int
	patchs int
}

func (f *fakeRenameAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodPatch:
		f.patchs++
		w.WriteHeader(http.StatusAccepted)
	case http.MethodGet:
		cover := f.covers[min(f.gets, len(f.covers)-1)]
		f.gets++
		attrs := map[string]any{"lastModifiedDate": "2026-11-26T10:00:00Z"}
		if cover != "" {
			attrs["artwork"] = map[string]any{"url": cover, "width": 1200, "height": 1200}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"id": "p.1", "type": "library-playlists", "attributes": attrs,
		}}})
	default:
		http.NotFound(w, r)
	}
}

func newRenameEnv(t *testing.T, f *fakeRenameAPI, cover string) (*PlaylistsService, *recorder) {
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
	art, err := artwork.New(artwork.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	authm, err := auth.NewManager(auth.Config{Dev: testDev{}, Open: func() (store.Store, error) { return &recordStore{}, nil }, Accounts: AccountStore{DB: d}, Logger: discard()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(authm.Close)
	rec := &recorder{}
	e := &Env{Deps: Deps{
		DB: d, API: api, Auth: authm, Library: library.New(d, api), Emit: rec,
		Storefront: func() string { return "gb" }, Now: time.Now,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, m: mapper{art: art}, members: newMembership(d)}
	pl := applemusic.Playlist{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
		ID: "p.1", Type: applemusic.TypeLibraryPlaylists,
		Attributes: applemusic.PlaylistAttributes{Name: "Old", CanEdit: true, CanDelete: true, Artwork: &applemusic.Artwork{URL: cover}},
	}}
	if err := e.Library.StorePlaylist(t.Context(), pl, "", nil); err != nil {
		t.Fatal(err)
	}
	return &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}}, rec
}

func nameIn(t *testing.T, tmpl string) string {
	t.Helper()
	u, err := url.Parse(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	b, err := base64.StdEncoding.DecodeString(u.Query().Get("t"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Renaming a playlist shows Apple's generated cover with the new name at
// once, in the design the user chose, without waiting for a sync. Apple's
// read right after the rename may still hold the old name; it is read again
// until the cover has the new one.
func TestRenameStoresTheCoverOfTheNewName(t *testing.T) {
	renameCoverWait = time.Millisecond
	t.Cleanup(func() { renameCoverWait = 500 * time.Millisecond })
	f := &fakeRenameAPI{covers: []string{genCover("Old"), genCover("Old"), genCover("New name")}}
	s, rec := newRenameEnv(t, f, genCover("Old"))
	ctx := t.Context()
	if err := s.e.Library.SetPlaylistCover(ctx, "p.1", library.CoverChoice{Kind: library.CoverPreset, Template: "PDCXS14"}); err != nil {
		t.Fatal(err)
	}

	if err := s.Update(ctx, "p.1", PlaylistInput{Name: " New name "}); err != nil {
		t.Fatal(err)
	}

	if f.gets != 3 {
		t.Fatalf("playlist read %d times, want 3 (two answers with the old name)", f.gets)
	}
	auto, err := s.e.Library.AutoCover(ctx, "p.1")
	if err != nil {
		t.Fatal(err)
	}
	if auto != stored("New name") {
		t.Fatalf("stored cover = %q, want %q", auto, stored("New name"))
	}
	// The chosen design is drawn from the new URL, on the page's surfaces
	// and in the picker's previews.
	shown, ok := s.e.chosenPlaylistCover(ctx, "p.1")
	if want, _ := artwork.WithPattern(stored("New name"), "PDCXS14"); !ok || shown != want || nameIn(t, shown) != "New name" {
		t.Fatalf("chosen cover = %q, %v; want %q", shown, ok, want)
	}
	covers, err := s.CoverOptions(ctx, "p.1")
	if err != nil {
		t.Fatal(err)
	}
	if covers.Current != "PDCXS14" {
		t.Fatalf("picker's current design = %q", covers.Current)
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.events) != 1 || rec.events[0] != EventPlaylistsChanged {
		t.Fatalf("events = %v, want one %s", rec.events, EventPlaylistsChanged)
	}
}

// A read that never carries the new name is stored as Apple gave it (no URL
// is made up) after a few attempts.
func TestRenameGivesUpWaitingForTheNewCover(t *testing.T) {
	renameCoverWait = time.Millisecond
	t.Cleanup(func() { renameCoverWait = 500 * time.Millisecond })
	f := &fakeRenameAPI{covers: []string{genCover("Old")}}
	s, _ := newRenameEnv(t, f, genCover("Old"))

	if err := s.Update(t.Context(), "p.1", PlaylistInput{Name: "New name"}); err != nil {
		t.Fatal(err)
	}
	if f.gets != renameCoverAttempts {
		t.Fatalf("playlist read %d times, want %d", f.gets, renameCoverAttempts)
	}
	if auto, _ := s.e.Library.AutoCover(t.Context(), "p.1"); auto != stored("Old") {
		t.Fatalf("stored cover = %q", auto)
	}
}

// A playlist whose answer has no artwork keeps the cover it has, and a
// cover that is not generated needs no second read.
func TestRenameKeepsTheCoverWhenAppleSendsNone(t *testing.T) {
	f := &fakeRenameAPI{covers: []string{""}}
	s, _ := newRenameEnv(t, f, genCover("Old"))
	if err := s.Update(t.Context(), "p.1", PlaylistInput{Name: "New name"}); err != nil {
		t.Fatal(err)
	}
	if auto, _ := s.e.Library.AutoCover(t.Context(), "p.1"); auto != stored("Old") {
		t.Fatalf("stored cover = %q", auto)
	}

	photo := "https://is1-ssl.mzstatic.com/image/thumb/Music/x/1200x1200bb.jpg"
	f = &fakeRenameAPI{covers: []string{photo}}
	s, _ = newRenameEnv(t, f, genCover("Old"))
	if err := s.Update(t.Context(), "p.1", PlaylistInput{Name: "New name"}); err != nil {
		t.Fatal(err)
	}
	if f.gets != 1 {
		t.Fatalf("playlist read %d times, want 1", f.gets)
	}
}
