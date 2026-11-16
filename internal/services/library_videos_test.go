package services

import (
	"database/sql"
	"testing"

	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/library"
)

// Before the music videos have synced the list comes from Apple (so it fails
// offline); once the domain has completed a sync it comes from the stored copy
// and asks Apple nothing, so the page lists the videos with no connection.
func TestMusicVideosReadTheSyncedCopy(t *testing.T) {
	s, fake, d, _ := newStateService(t)
	ctx := t.Context()
	if _, err := s.MusicVideos(ctx, 0, 100); err == nil {
		t.Error("an unsynced list read with no videos from Apple did not fail")
	}
	if got := fake.take(); len(got) != 1 || got[0] != "GET /v1/me/library/music-videos" {
		t.Fatalf("before the sync Apple was asked %q, want its music videos", got)
	}

	for i, name := range []string{"Second", "First"} {
		err := d.Q.UpsertLibraryMusicVideo(ctx, dbq.UpsertLibraryMusicVideoParams{
			LibraryID: "i." + name, CatalogID: "c" + name, Name: name, ArtistName: "Band", DurationMs: 200_000, Explicit: 1,
			ArtworkTemplate: "https://art/{w}x{h}.jpg", ArtworkWidth: 1920, ArtworkHeight: 1080, Position: int64(1 - i), Generation: 1, UpdatedAt: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Stored rows without a completed sync are not the library's list.
	if _, err := s.MusicVideos(ctx, 0, 100); err == nil {
		t.Error("rows of an unfinished sync were listed")
	}
	fake.take()
	dom := string(library.DomainMusicVideos)
	if err := d.Q.StartSync(ctx, dbq.StartSyncParams{Domain: dom, Generation: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.Q.FinishSync(ctx, dbq.FinishSyncParams{LastSuccess: sql.NullInt64{Int64: 2, Valid: true}, Domain: dom}); err != nil {
		t.Fatal(err)
	}
	fake.take()

	page, err := s.MusicVideos(ctx, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := fake.take(); len(got) != 0 {
		t.Errorf("the synced list asked Apple %q", got)
	}
	if page.Total != 2 || len(page.Items) != 2 || page.Items[0].Name != "First" || page.Items[1].Name != "Second" {
		t.Fatalf("page %+v, want First then Second (Apple's order)", page)
	}
	v := page.Items[0]
	if v.Source != SourceLibrary || v.LibraryID != "i.First" || v.CatalogID != "cFirst" || !v.Explicit {
		t.Errorf("mapped video %+v", v)
	}
	if one, _ := s.MusicVideos(ctx, 1, 1); len(one.Items) != 1 || one.Items[0].Name != "Second" || one.Total != 2 {
		t.Errorf("second page of one: %+v", one)
	}
}
