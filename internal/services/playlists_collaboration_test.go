package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
)

// fakeSocialAPI answers the social calls of a collaborative playlist
// (pl.u-collab, from testdata/playlists/collaboration.json: synthetic
// people in the shape Apple returns) and the user's own profile. Like
// Apple, it attributes tracks only when asked to.
type fakeSocialAPI struct {
	t     *testing.T
	me    string // the user's profile ID; "" answers 404
	mu    sync.Mutex
	calls []string
}

func (f *fakeSocialAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.URL.Path)
	f.mu.Unlock()
	switch r.URL.Path {
	case "/v1/me/social/profile":
		if f.me == "" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"id": "me", "type": "personal-social-profiles",
			"relationships": map[string]any{"social-profile": map[string]any{"data": []any{map[string]any{"id": f.me, "type": "social-profiles"}}}},
		}}})
	case "/v1/catalog/gb/playlists/pl.u-collab":
		b, err := os.ReadFile(filepath.Join("testdata", "playlists", "collaboration.json"))
		if err != nil {
			f.t.Error(err)
			return
		}
		var body map[string]any
		if err := json.Unmarshal(b, &body); err != nil {
			f.t.Error(err)
			return
		}
		pl := body["data"].([]any)[0].(map[string]any)
		q := r.URL.Query()
		if q.Get("associate[playlists:tracks]") != "collaborators" {
			for _, tr := range pl["relationships"].(map[string]any)["tracks"].(map[string]any)["data"].([]any) {
				delete(tr.(map[string]any), "meta")
			}
		}
		if !strings.Contains(q.Get("views"), "collaborators") {
			delete(pl, "views")
		}
		_ = json.NewEncoder(w).Encode(body)
	case "/v1/catalog/gb/playlists/pl.u-solo":
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"id": "pl.u-solo", "type": "playlists", "attributes": map[string]any{"hasCollaboration": false},
			"relationships": map[string]any{"tracks": map[string]any{"data": []any{map[string]any{"id": "1001", "type": "songs"}}}},
			"views":         map[string]any{"collaborators": map[string]any{"data": []any{}}},
		}}})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeSocialAPI) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// newCollaborationEnv stores library playlists: p.shared (a collaborative
// playlist the user added), p.mine (the user's own copy: editable),
// p.solo (shared, not collaborative), p.gone (shared, since removed from
// Apple Music) and p.private (never shared).
func newCollaborationEnv(t *testing.T, f *fakeSocialAPI, mut string) *PlaylistsService {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return mut })
	api.Base, api.AMPBase = srv.URL, srv.URL
	art, err := artwork.New(artwork.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	e := &Env{Deps: Deps{
		DB: d, API: api, Auth: &auth.Manager{}, Library: library.New(d, api), Storefront: func() string { return "gb" },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, m: mapper{art: art}, members: newMembership(d)}
	for _, p := range []struct {
		id, global string
		canEdit    bool
	}{
		{"p.shared", "pl.u-collab", false},
		{"p.mine", "pl.u-collab", true},
		{"p.solo", "pl.u-solo", true},
		{"p.gone", "pl.u-gone", false},
		{"p.private", "", true},
	} {
		pl := applemusic.Playlist{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
			ID: p.id, Type: applemusic.TypeLibraryPlaylists,
			Attributes: applemusic.PlaylistAttributes{Name: p.id, CanEdit: p.canEdit, CanDelete: true, PlayParams: &applemusic.PlayParams{ID: p.id, GlobalID: p.global}},
		}}
		if err := e.Library.StorePlaylist(t.Context(), pl, "", nil); err != nil {
			t.Fatal(err)
		}
	}
	return &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}}
}

func profileIDs(ps []SocialProfile) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.ID
	}
	return out
}

func TestCollaborationMapsPeopleAndTracks(t *testing.T) {
	f := &fakeSocialAPI{t: t, me: "sp.mika"}
	s := newCollaborationEnv(t, f, "mut")
	c, err := s.Collaboration(t.Context(), "p.shared")
	if err != nil {
		t.Fatal(err)
	}
	if !c.Collaborative || c.OwnerID != "sp.robin" {
		t.Fatalf("collaborative %v, owner %q", c.Collaborative, c.OwnerID)
	}
	// The owner leads; nobody is listed twice.
	if got := profileIDs(c.Collaborators); !slices.Equal(got, []string{"sp.robin", "sp.ada", "sp.mika"}) {
		t.Fatalf("collaborators = %v", got)
	}
	// Jules added a song but is no longer a collaborator.
	if got := profileIDs(c.Former); !slices.Equal(got, []string{"sp.jules"}) {
		t.Fatalf("former = %v", got)
	}
	want := []TrackAddedBy{
		{"1001", "sp.ada"}, {"1002", "sp.robin"}, {"1003", ""}, {"1004", "sp.jules"}, {"1001", "sp.mika"}, {"2001", "sp.ada"},
	}
	if !slices.Equal(c.AddedBy, want) {
		t.Fatalf("added by = %v, want %v", c.AddedBy, want)
	}
	robin := c.Collaborators[0]
	if robin.Name != "Robin Hale" || robin.Handle != "robinhale" || robin.URL != "https://music.apple.com/profile/robinhale" {
		t.Fatalf("owner = %+v", robin)
	}
	// Avatars are served by Go; a profile without one has none.
	if !strings.HasPrefix(robin.Art.Small, "/art/") || c.Collaborators[2].Art.Small != "" {
		t.Fatalf("avatars = %q, %q", robin.Art.Small, c.Collaborators[2].Art.Small)
	}
	if c.Viewer != RoleCollaborator {
		t.Fatalf("viewer = %q, want collaborator", c.Viewer)
	}
}

func TestCollaborationViewer(t *testing.T) {
	for _, c := range []struct {
		name, id, me, mut string
		want              CollaborationRole
	}{
		{"owner by profile", "p.shared", "sp.robin", "mut", RoleOwner},
		{"collaborator", "p.mine", "sp.ada", "mut", RoleCollaborator},
		{"listener", "p.shared", "sp.someone", "mut", RoleListener},
		// Without the user's profile, only their editable copy says owner.
		{"no profile, own copy", "p.mine", "", "mut", RoleOwner},
		{"no profile", "p.shared", "", "mut", RoleListener},
		{"signed out", "pl.u-collab", "sp.robin", "", RoleListener},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeSocialAPI{t: t, me: c.me}
			s := newCollaborationEnv(t, f, c.mut)
			got, err := s.Collaboration(t.Context(), c.id)
			if err != nil {
				t.Fatal(err)
			}
			if got.Viewer != c.want {
				t.Fatalf("viewer = %q, want %q", got.Viewer, c.want)
			}
			if c.mut == "" && slices.Contains(f.requests(), "/v1/me/social/profile") {
				t.Fatal("asked for the profile while signed out")
			}
		})
	}
}

func TestCollaborationProfileAskedOnce(t *testing.T) {
	f := &fakeSocialAPI{t: t, me: "sp.ada"}
	s := newCollaborationEnv(t, f, "mut")
	for range 2 {
		if _, err := s.Collaboration(t.Context(), "p.shared"); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for _, p := range f.requests() {
		if p == "/v1/me/social/profile" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("profile asked %d times", n)
	}
}

func TestNotCollaborative(t *testing.T) {
	for _, c := range []struct {
		id      string
		network bool
	}{
		{"p.private", false},                           // never shared
		{"pl.f4d106fed2bd41149aaacabb233eb5eb", false}, // editorial
		{"pl.pm-1234", false},                          // personal mix
		{"p.unknown", false},                           // not in the library
		{"p.solo", true},                               // shared, nobody else in it
		{"p.gone", true},                               // Apple no longer has it
	} {
		t.Run(c.id, func(t *testing.T) {
			f := &fakeSocialAPI{t: t}
			s := newCollaborationEnv(t, f, "")
			got, err := s.Collaboration(t.Context(), c.id)
			if err != nil {
				t.Fatal(err)
			}
			if got.Collaborative || got.Collaborators == nil || len(got.Collaborators) != 0 || got.Former == nil || got.AddedBy == nil || len(got.AddedBy) != 0 {
				t.Fatalf("collaboration = %+v", got)
			}
			if asked := len(f.requests()) > 0; asked != c.network {
				t.Fatalf("network requests %v, want %v", f.requests(), c.network)
			}
		})
	}
}
