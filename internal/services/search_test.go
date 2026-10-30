package services

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/db"
)

type staticToken struct{}

func (staticToken) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev", Expires: time.Now().Add(time.Hour)}, nil
}
func (staticToken) Invalidate() {}

// newSearchEnv builds a SearchService over a real SQLite database and a
// fake amp-api.
func newSearchEnv(t *testing.T, h http.Handler) (*SearchService, *db.DB, *time.Time) {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return "mut" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	sf := func() string { return "gb" }
	now := time.UnixMilli(1_000_000)
	e := &Env{Deps: Deps{
		DB: d, API: api, Catalog: catalog.New(d, api, sf, nil), Storefront: sf,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return now },
	}}
	return newSearchService(e), d, &now
}

func TestRecentSearches(t *testing.T) {
	s, d, now := newSearchEnv(t, http.NotFoundHandler())
	ctx := t.Context()
	step := func() { *now = now.Add(time.Second) }

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(s.RememberTerm(ctx, " Blue  Moon ", ScopeCatalog))
	step()
	must(s.RememberItem(ctx, RecentItem{Kind: ItemArtist, ID: "9", Name: "Blue Band", Art: "/art/k/96.jpg"}))
	step()
	must(s.RememberTerm(ctx, "blue moon", ScopeLibrary))
	step()
	// The same term again moves to the top and keeps one entry.
	must(s.RememberTerm(ctx, "BLUE moon", ScopeCatalog))
	if err := s.RememberTerm(ctx, "x", "web"); err == nil {
		t.Error("bad scope accepted")
	}
	if err := s.RememberItem(ctx, RecentItem{Kind: ItemAlbum}); err == nil {
		t.Error("item without ID accepted")
	}
	must(s.RememberTerm(ctx, "   ", ScopeCatalog)) // ignored

	got, err := s.Recent(ctx)
	must(err)
	keys := func(r []RecentSearch) (out []string) {
		for _, x := range r {
			out = append(out, x.Key)
		}
		return out
	}
	want := []string{"term:catalog:blue moon", "term:library:blue moon", "item:artist:catalog:9"}
	if !slices.Equal(keys(got), want) {
		t.Fatalf("recent = %v, want %v", keys(got), want)
	}
	if got[0].Term != "BLUE moon" || got[0].Scope != ScopeCatalog || got[0].Source != "" {
		t.Errorf("term entry = %+v", got[0])
	}
	if it := got[2]; it.Kind != ItemArtist || it.ItemID != "9" || it.Source != SourceCatalog || it.Art != "/art/k/96.jpg" || it.Scope != "" {
		t.Errorf("item entry = %+v", it)
	}

	must(s.ForgetRecent(ctx, "term:library:blue moon"))
	if got, _ := s.Recent(ctx); len(got) != 2 {
		t.Fatalf("after forget: %v", keys(got))
	}

	// Only the newest keepRecent are kept.
	for i := range keepRecent + 5 {
		step()
		must(s.RememberTerm(ctx, "term "+string(rune('a'+i)), ScopeCatalog))
	}
	got, _ = s.Recent(ctx)
	if len(got) != keepRecent || got[0].Term != "term "+string(rune('a'+keepRecent+4)) {
		t.Fatalf("trimmed = %d, first %+v", len(got), got[0])
	}

	// Sign-out wipes them.
	must(d.WipeUserData(ctx))
	if got, _ := s.Recent(ctx); len(got) != 0 {
		t.Fatalf("after wipe: %v", keys(got))
	}
	must(s.RememberTerm(ctx, "again", ScopeCatalog))
	must(s.ClearRecent(ctx))
	if got, _ := s.Recent(ctx); len(got) != 0 {
		t.Fatalf("after clear: %v", keys(got))
	}
}

func TestSearchCatalogSections(t *testing.T) {
	var social, groupQuery string
	s, _, _ := newSearchEnv(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/catalog/gb/search":
			if g := r.URL.Query().Get("groups"); g != "" {
				groupQuery = r.URL.RawQuery
				w.Write([]byte(`{"results":{"song":{"data":[{"id":"2","type":"songs"}]}},
					"resources":{"songs":{"2":{"id":"2","type":"songs","attributes":{"name":"Blue Two","playParams":{"id":"2","kind":"song"}}}}}}`))
				return
			}
			w.Write([]byte(`{"results":{
				"top":{"data":[{"id":"9","type":"artists"},{"id":"1","type":"songs"}]},
				"song":{"next":"/v1/catalog/gb/search?groups=song&offset=21&term=blue","data":[{"id":"1","type":"songs"}]},
				"video_extra":{"data":[{"id":"u1","type":"uploaded-videos"}]}},
				"resources":{
				"songs":{"1":{"id":"1","type":"songs","attributes":{"name":"Blue","artistName":"Blue Band","playParams":{"id":"1","kind":"song"}}}},
				"artists":{"9":{"id":"9","type":"artists","attributes":{"name":"Blue Band"}}},
				"uploaded-videos":{"u1":{"id":"u1","type":"uploaded-videos","attributes":{"name":"Interview"}}}},
				"meta":{"results":{"order":["top","song","video_extra"]}}}`))
		case "/v1/social/gb/search":
			social = r.URL.RawQuery
			w.Write([]byte(`{"results":{
				"profile":{"next":"/v1/social/gb/search?groups=profile&offset=21","data":[{"id":"sp.1","type":"social-profiles"}]},
				"playlist":{"data":[{"id":"pl.u-1","type":"playlists"}]}},
				"resources":{
				"social-profiles":{"sp.1":{"id":"sp.1","type":"social-profiles","attributes":{"name":"Taylor","handle":"taylor","url":"https://music.apple.com/profile/taylor"}}},
				"playlists":{"pl.u-1":{"id":"pl.u-1","type":"playlists","attributes":{"name":"Mix","curatorName":"Taylor"}}}}}`))
		case "/v1/recommendations/gb":
			w.Write([]byte(`{"data":[{"id":"r1","type":"personal-recommendation"},{"id":"r2","type":"personal-recommendation"}],"resources":{
				"personal-recommendation":{
					"r1":{"id":"r1","type":"personal-recommendation","relationships":{"contents":{"data":[{"id":"988556214","type":"apple-curators"}]}}},
					"r2":{"id":"r2","type":"personal-recommendation","relationships":{"contents":{"data":[{"id":"6447688262","type":"editorial-items"}]}}}},
				"apple-curators":{"988556214":{"id":"988556214","type":"apple-curators","attributes":{"name":"Apple Music Alternative","shortName":"Alternative","kind":"Genre"}}},
				"editorial-items":{"6447688262":{"id":"6447688262","type":"editorial-items","attributes":{"plainEditorialNotes":{"name":"Ibiza"},
					"link":{"url":"https://music.apple.com/gb/multi-room/6447561826","target":"internal"}}}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	ctx := t.Context()

	cats, err := s.Browse(ctx)
	if err != nil || len(cats) != 2 || cats[0].Kind != ItemCurator || cats[0].Name != "Alternative" ||
		cats[1].Kind != ItemMultiroom || cats[1].ID != "6447561826" || cats[1].Name != "Ibiza" {
		t.Fatalf("browse = %+v, %v", cats, err)
	}

	res, err := s.Catalog(ctx, "  blue ")
	if err != nil {
		t.Fatal(err)
	}
	if social == "" {
		t.Error("social search not called")
	}
	var groups []string
	for _, sec := range res.Sections {
		groups = append(groups, sec.Group)
	}
	// Video extras have no page in Sona: dropped, with their group.
	if !slices.Equal(groups, []string{"top", "song", GroupProfiles, GroupSharedPlaylists}) {
		t.Fatalf("groups = %v", groups)
	}
	top := res.Sections[0]
	if len(top.Items) != 2 || top.Items[0].Kind != ItemArtist || top.Items[1].Kind != ItemSong || top.Items[1].Song == nil || top.Next != 0 {
		t.Fatalf("top = %+v", top)
	}
	if songs := res.Sections[1]; songs.Next != 21 || songs.Items[0].Song.Name != "Blue" {
		t.Fatalf("songs = %+v", songs)
	}
	if p := res.Sections[2]; p.Next != 21 || p.Items[0].Kind != ItemProfile || p.Items[0].ID != "sp.1" || p.Items[0].Subtitle != "@taylor" {
		t.Fatalf("profiles = %+v", p)
	}
	if pl := res.Sections[3]; pl.Items[0].Kind != ItemPlaylist || pl.Items[0].Playlist == nil {
		t.Fatalf("shared playlists = %+v", pl)
	}

	page, err := s.Group(ctx, "blue", "song", 21)
	if err != nil || page.Group != "song" || len(page.Items) != 1 || page.Items[0].ID != "2" || page.Next != 0 {
		t.Fatalf("group page = %+v, %v", page, err)
	}
	if groupQuery == "" {
		t.Fatal("group page not requested")
	}
	if _, err := s.Group(ctx, "blue", "top", 0); err == nil {
		t.Error("top has no pages")
	}
	if empty, err := s.Catalog(ctx, " "); err != nil || len(empty.Sections) != 0 || empty.Sections == nil {
		t.Errorf("blank = %+v, %v", empty, err)
	}
}
