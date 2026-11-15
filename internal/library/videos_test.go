package library

import (
	"context"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func videoNames(t *testing.T, e *env) []string {
	t.Helper()
	vs, total, err := e.lib.MusicVideos(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if total != len(vs) {
		t.Errorf("total %d for %d videos listed", total, len(vs))
	}
	var names []string
	for _, v := range vs {
		names = append(names, v.Name)
	}
	return names
}

// The music videos domain copies the library's videos in Apple's order, with
// what the page and a play need; nothing is listed as synced before it ran.
func TestSyncMusicVideosStoresThemInApplesOrder(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if ok, err := e.lib.MusicVideosSynced(ctx); err != nil || ok {
		t.Fatalf("synced before any sync: %v, %v", ok, err)
	}
	e.fake.videos = []map[string]any{musicVideo("i.9", "Zulu", "Band"), musicVideo("i.2", "Alpha", "Band"), musicVideo("i.5", "Mike", "Other")}
	if err := e.run(t, DomainMusicVideos); err != nil {
		t.Fatal(err)
	}
	if ok, err := e.lib.MusicVideosSynced(ctx); err != nil || !ok {
		t.Fatalf("not synced after the sync: %v, %v", ok, err)
	}
	if got, want := videoNames(t, e), []string{"Zulu", "Alpha", "Mike"}; !slices.Equal(got, want) {
		t.Errorf("videos %q, want Apple's order %q", got, want)
	}
	vs, _, _ := e.lib.MusicVideos(ctx, ListOptions{Offset: 1, Limit: 1})
	if len(vs) != 1 || vs[0].LibraryID != "i.2" {
		t.Fatalf("page 2 of size 1: %+v", vs)
	}
	v := vs[0]
	if v.CatalogID != "c2" || v.ArtistName != "Band" || v.AlbumName != "Clips" || v.DurationMS != 215000 || !v.Explicit || !v.Has4K || v.HasHDR ||
		v.ReleaseDate != "2021-05-01" || v.Genre != "Pop" || v.ArtworkTemplate != "https://art/i.2/{w}x{h}bb.jpg" || v.ArtworkWidth != 1920 || v.ArtworkHeight != 1080 {
		t.Errorf("stored video %+v", v)
	}
}

// A sync that Apple's list changed under refreshes the copy: new videos in,
// removed ones out, the order Apple's now.
func TestSyncMusicVideosRefreshesAndRemoves(t *testing.T) {
	e := newEnv(t)
	e.fake.videos = []map[string]any{musicVideo("i.1", "One", "A"), musicVideo("i.2", "Two", "A"), musicVideo("i.3", "Three", "A")}
	if err := e.run(t, DomainMusicVideos); err != nil {
		t.Fatal(err)
	}
	e.fake.videos = []map[string]any{musicVideo("i.4", "Four", "B"), musicVideo("i.3", "Three", "A"), musicVideo("i.1", "One!", "A")}
	if err := e.run(t, DomainMusicVideos); err != nil {
		t.Fatal(err)
	}
	if got, want := videoNames(t, e), []string{"Four", "Three", "One!"}; !slices.Equal(got, want) {
		t.Errorf("videos after a change %q, want %q", got, want)
	}
	// Apple's list emptied: the copy empties, and it still counts as synced
	// (the library has none; it is not unsynced).
	e.fake.videos = nil
	if err := e.run(t, DomainMusicVideos); err != nil {
		t.Fatal(err)
	}
	if got := videoNames(t, e); len(got) != 0 {
		t.Errorf("videos after Apple's list emptied: %q", got)
	}
	if ok, _ := e.lib.MusicVideosSynced(context.Background()); !ok {
		t.Error("an emptied list reads as unsynced")
	}
}

// A library synced before music videos were stored takes only the music
// videos on its next incremental sync: it must not run every other domain
// fully because this one has no history.
func TestMusicVideosDoNotMakeAnIncrementalSyncRunFully(t *testing.T) {
	e := newEnv(t)
	seedLibrary(t, e)
	var old []Domain
	for _, d := range AllDomains {
		if d != DomainMusicVideos {
			old = append(old, d)
		}
	}
	if err := e.runFull(t, old...); err != nil {
		t.Fatal(err)
	}
	e.fake.videos = []map[string]any{musicVideo("i.1", "One", "A")}
	e.fake.reset()
	if err := e.run(t); err != nil {
		t.Fatal(err)
	}
	if got := videoNames(t, e); !slices.Equal(got, []string{"One"}) {
		t.Errorf("videos %q after the incremental sync", got)
	}
	for _, r := range e.fake.seen() {
		path, query, _ := strings.Cut(r, "?")
		if path != "/v1/me/library/songs" {
			continue
		}
		if q, _ := url.ParseQuery(query); q.Get("limit") == "1" {
			t.Errorf("the songs ran fully (probe %s) though only music videos were new", r)
		}
	}
}

// Removing a music video from the library forgets the copy once Apple has
// accepted the delete.
func TestRemoveMusicVideoForgetsTheCopy(t *testing.T) {
	e := newEnv(t)
	e.fake.videos = []map[string]any{musicVideo("i.1", "One", "A"), musicVideo("i.2", "Two", "A")}
	if err := e.run(t, DomainMusicVideos); err != nil {
		t.Fatal(err)
	}
	rec := withAMP(t, e)
	changed := 0
	if err := e.lib.Remove(context.Background(), "music-videos", "i.1", func() { changed++ }); err != nil {
		t.Fatal(err)
	}
	if got := rec.take(); !slices.Equal(got, []string{"DELETE /v1/me/library/music-videos/i.1?"}) {
		t.Errorf("Apple was sent %q", got)
	}
	if got := videoNames(t, e); !slices.Equal(got, []string{"Two"}) || changed != 1 {
		t.Errorf("after the removal: videos %q, changed %d times", got, changed)
	}
}
