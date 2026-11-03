package services

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Sign-out removes every artwork only the user's data references (library
// playlist covers, history) and keeps catalogue artwork, including a
// library album's cover that the catalogue shares.
func TestSignOutRemovesUserArtwork(t *testing.T) {
	ctx := context.Background()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	tmpl := func(name string) string {
		return "https://is1-ssl.mzstatic.com/image/thumb/" + name + "/{w}x{h}bb.jpg"
	}
	cover, played, shared, catalog := tmpl("user-cover"), tmpl("played"), tmpl("shared"), tmpl("catalog")
	// A playlist's mosaic is composed from catalogue albums, yet it is the
	// user's own cover.
	mosaic := artwork.Mosaic([]string{catalog, shared, tmpl("m3"), tmpl("m4")})
	q := d.Q
	for _, err := range []error{
		func() error {
			_, err := q.UpsertLibraryPlaylist(ctx, dbq.UpsertLibraryPlaylistParams{LibraryID: db.NullString("p.1"), Name: "Mine", ArtworkTemplate: cover, UpdatedAt: 1})
			return err
		}(),
		func() error {
			_, err := q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{LibraryID: db.NullString("l.1"), Name: "A", ArtworkTemplate: shared, UpdatedAt: 1})
			return err
		}(),
		func() error {
			_, err := q.UpsertCatalogAlbum(ctx, dbq.UpsertCatalogAlbumParams{CatalogID: db.NullString("1"), Name: "A", ArtworkTemplate: shared, UpdatedAt: 1})
			return err
		}(),
		func() error {
			_, err := q.UpsertCatalogAlbum(ctx, dbq.UpsertCatalogAlbumParams{CatalogID: db.NullString("2"), Name: "B", ArtworkTemplate: catalog, UpdatedAt: 1})
			return err
		}(),
		func() error {
			_, err := q.InsertHistory(ctx, dbq.InsertHistoryParams{PlayedAt: 1, ItemID: "9", ArtworkTemplate: played})
			return err
		}(),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Put each template's artwork on disk as the cache stores it, then
	// open the cache over it.
	dir := t.TempDir()
	probe, err := artwork.New(artwork.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	keyOf := map[string]string{}
	for _, tp := range []string{cover, played, shared, catalog, mosaic} {
		key := strings.Split(strings.TrimPrefix(probe.URL(tp, 128, 128), "/art/"), "/")[0]
		keyOf[tp] = key
		if err := os.MkdirAll(filepath.Join(dir, key), 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range map[string]string{"template": tp, "src-600.jpg": "jpeg", "128.jpg": "jpeg"} {
			if err := os.WriteFile(filepath.Join(dir, key, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	art, err := artwork.New(artwork.Options{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	before := art.Size()

	if err := wipeUserArtwork(ctx, d, art, discard()); err != nil {
		t.Fatal(err)
	}
	if err := d.WipeUserData(ctx); err != nil {
		t.Fatal(err)
	}
	for tp, wantKept := range map[string]bool{cover: false, played: false, shared: true, catalog: true, mosaic: false} {
		_, err := os.Stat(filepath.Join(dir, keyOf[tp]))
		if kept := err == nil; kept != wantKept {
			t.Errorf("%s: on disk %v, want %v", tp, kept, wantKept)
		}
	}
	if art.Size() >= before {
		t.Fatalf("cache size %d → %d", before, art.Size())
	}
}
