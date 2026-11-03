package services

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
)

// Library playlists Apple rendered no cover for show one made from their
// tracks on every surface that lists or opens them: the page and its
// backdrop, the sidebar tree, the library list, tiles in search.
func TestLibraryPlaylistCoversFromTracks(t *testing.T) {
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	art, err := artwork.New(artwork.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	lib := library.New(d, nil)
	e := &Env{Deps: Deps{
		DB: d, Library: lib, Art: art, Emit: &recorder{}, Storefront: func() string { return "gb" },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
	}, m: mapper{art: art}, members: newMembership(d)}
	pl := &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}}
	ls := &LibraryService{o: e, m: e.m}

	albumArt := func(n string) string {
		return "https://is1-ssl.mzstatic.com/image/thumb/Music/" + n + "/{w}x{h}bb.jpg"
	}
	store := func(id, own string, albums ...string) {
		t.Helper()
		songs := make([]applemusic.Song, len(albums))
		for i, a := range albums {
			songs[i] = applemusic.Song{Resource: applemusic.Resource[applemusic.SongAttributes]{
				ID: fmt.Sprintf("i.%s.%d", id, i), Type: applemusic.TypeLibrarySongs,
				Attributes: applemusic.SongAttributes{Name: "S", Artwork: &applemusic.Artwork{URL: albumArt(a)}},
			}}
		}
		p := applemusic.Playlist{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
			ID: id, Type: applemusic.TypeLibraryPlaylists,
			Attributes: applemusic.PlaylistAttributes{Name: id, CanEdit: true},
		}}
		if own != "" {
			p.Attributes.Artwork = &applemusic.Artwork{URL: own}
		}
		if err := lib.StorePlaylist(ctx, p, "", songs); err != nil {
			t.Fatal(err)
		}
	}
	store("p.mosaic", "", "a", "b", "a", "c", "d", "e")
	store("p.single", "", "a", "a", "a")
	store("p.two", "", "b", "a", "b")
	store("p.own", albumArt("own"), "a", "b", "c", "d")
	store("p.empty", "")

	url := func(tmpl string) string { return art.URL(tmpl, artLarge, artLarge) }
	mosaic := artwork.Mosaic([]string{albumArt("a"), albumArt("b"), albumArt("c"), albumArt("d")})
	want := map[string]string{
		"p.mosaic": url(mosaic),
		"p.single": url(albumArt("a")),
		"p.two":    url(albumArt("b")),
		"p.own":    url(albumArt("own")),
		"p.empty":  "",
	}
	if want["p.mosaic"] == "" || want["p.mosaic"] == want["p.single"] {
		t.Fatalf("test setup: %v", want)
	}

	tree, err := pl.Tree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range tree.Playlists {
		if p.Playlist.Art.Large != want[p.Playlist.ID] {
			t.Errorf("tree %s: %q, want %q", p.Playlist.ID, p.Playlist.Art.Large, want[p.Playlist.ID])
		}
	}
	page, err := ls.Playlists(ctx, "title", false, 0, 50)
	if err != nil || len(page.Items) != len(want) {
		t.Fatalf("list = %d, %v", len(page.Items), err)
	}
	for _, p := range page.Items {
		if p.Art.Large != want[p.ID] {
			t.Errorf("list %s: %q, want %q", p.ID, p.Art.Large, want[p.ID])
		}
	}
	for id, w := range want {
		d, err := pl.Playlist(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if d.Playlist.Art.Large != w {
			t.Errorf("page %s: %q, want %q", id, d.Playlist.Art.Large, w)
		}
		if (w == "") != (d.Header.Backdrop == "") {
			t.Errorf("page %s: backdrop %q for cover %q", id, d.Header.Backdrop, w)
		}
	}
	// The page's backdrop is the mosaic's own, not its first album's.
	d1, _ := pl.Playlist(ctx, "p.mosaic")
	d2, _ := pl.Playlist(ctx, "p.single")
	if d1.Header.Backdrop == d2.Header.Backdrop || !strings.HasPrefix(d1.Header.Backdrop, "/art/") {
		t.Errorf("backdrops: %q %q", d1.Header.Backdrop, d2.Header.Backdrop)
	}
}
