package services

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

func TestMembership(t *testing.T) {
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ref, err := d.Q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
		LibraryID: db.NullString("l.abc"), CatalogID: db.NullString("1440"), Name: "A", UpdatedAt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Q.MarkLibraryItem(ctx, dbq.MarkLibraryItemParams{Kind: kindAlbum, Ref: ref, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	// A library row without membership (left by an unfinished sync) does not count.
	if _, err := d.Q.UpsertLibraryAlbum(ctx, dbq.UpsertLibraryAlbumParams{
		LibraryID: db.NullString("l.old"), CatalogID: db.NullString("1441"), Name: "B", UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1000, 0)
	m := newMembership(d)
	m.now = func() time.Time { return now }
	if lid, ok := m.lookup(ctx, kindAlbum, "1440"); !ok || lid != "l.abc" {
		t.Errorf("synced album: %q %v", lid, ok)
	}
	for _, id := range []string{"1441", "1442", ""} {
		if lid, ok := m.lookup(ctx, kindAlbum, id); ok || lid != "" {
			t.Errorf("album %q: %q %v", id, lid, ok)
		}
	}

	// Just added: in the library until a sync that started later answers.
	m.markAdded("albums", []string{"1442"})
	m.markAdded("playlists", []string{"pl.x"})
	if _, ok := m.lookup(ctx, kindAlbum, "1442"); !ok {
		t.Error("added album not in library")
	}
	if _, ok := m.lookup(ctx, kindPlaylist, "pl.x"); !ok {
		t.Error("added playlist not in library")
	}
	if _, ok := m.lookup(ctx, kindPlaylist, "1442"); ok {
		t.Error("kinds are not separate")
	}
	m.synced(now) // a sync that started before the add keeps it
	if _, ok := m.lookup(ctx, kindAlbum, "1442"); !ok {
		t.Error("an earlier sync dropped the add")
	}
	m.synced(now.Add(time.Second))
	if _, ok := m.lookup(ctx, kindAlbum, "1442"); ok {
		t.Error("a later sync kept the add")
	}
	m.markAdded("albums", []string{"1443"})
	m.clear()
	if _, ok := m.lookup(ctx, kindAlbum, "1443"); ok {
		t.Error("clear kept the add")
	}
}
