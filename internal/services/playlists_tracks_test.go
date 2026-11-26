package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
)

// storePlaylist stores library playlist "p.mix" holding tracks (JSON songs,
// as Apple lists a library playlist's tracks) in e's library.
func storePlaylist(t *testing.T, e *Env, tracks ...string) {
	t.Helper()
	songs := make([]applemusic.Song, len(tracks))
	for i, js := range tracks {
		if err := json.Unmarshal([]byte(js), &songs[i]); err != nil {
			t.Fatal(err)
		}
	}
	p := applemusic.Playlist{Resource: applemusic.Resource[applemusic.PlaylistAttributes]{
		ID: "p.mix", Type: applemusic.TypeLibraryPlaylists,
		Attributes: applemusic.PlaylistAttributes{Name: "Mix", CanEdit: true, LastModifiedDate: "2026-11-01T10:00:00Z"},
	}}
	if err := e.Library.StorePlaylist(t.Context(), p, "", songs); err != nil {
		t.Fatal(err)
	}
}

// leaveLibrary takes library song libraryID out of the library, as the
// user's removal or Apple's does, while the playlist that holds it keeps its
// row (db DeleteOrphanLibraryTracks).
func leaveLibrary(t *testing.T, e *Env, libraryID string) {
	t.Helper()
	_, err := e.DB.W.ExecContext(t.Context(), `DELETE FROM library_items WHERE kind = 'song'
		AND ref = (SELECT id FROM tracks WHERE library_id = ?)`, libraryID)
	if err != nil {
		t.Fatal(err)
	}
}

func playlistTrackEnv(t *testing.T) *Env {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return &Env{Deps: Deps{
		DB: d, Library: library.New(d, nil), Emit: &recorder{}, Storefront: func() string { return "gb" },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
	}, members: newMembership(d)}
}

const (
	// A catalogue song of the playlist: not a library song; the artist and
	// album it names are the catalogue's.
	catalogTrack = `{"id":"100","type":"songs","attributes":{"name":"Catalog","artistName":"Cat Artist","albumName":"Cat Album"},
		"relationships":{"albums":{"data":[{"id":"200","type":"albums"}]},"artists":{"data":[{"id":"300","type":"artists"}]}}}`
	// A library song of the playlist, with its library album and artist.
	libraryTrack = `{"id":"i.1","type":"library-songs","attributes":{"name":"Own","artistName":"Own Artist","albumName":"Own Album","playParams":{"catalogId":"101"}},
		"relationships":{"albums":{"data":[{"id":"l.1","type":"library-albums"}]},"artists":{"data":[{"id":"r.1","type":"library-artists"}]}}}`
	// Two library songs that are, after leaveLibrary, in no library: one
	// with a catalogue copy, one an upload.
	strayTrack  = `{"id":"i.2","type":"library-songs","attributes":{"name":"Stray","artistName":"Stray Artist","albumName":"Stray Album","playParams":{"catalogId":"102"}}}`
	uploadTrack = `{"id":"i.3","type":"library-songs","attributes":{"name":"Upload","artistName":"Up Artist","albumName":"Up Album"}}`
)

// The tracks of a library playlist (a collaborative one holds mostly songs
// that are not in the library) name their album and artist, so that the
// track list links them: a catalogue song by its catalogue IDs, a library
// song by its library IDs.
func TestLibraryPlaylistTracksNameAlbumAndArtist(t *testing.T) {
	e := playlistTrackEnv(t)
	storePlaylist(t, e, catalogTrack, libraryTrack)
	pl := &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}}
	d, err := pl.Playlist(t.Context(), "p.mix")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Tracks) != 2 {
		t.Fatalf("tracks = %d", len(d.Tracks))
	}
	for i, want := range []struct {
		source        Source
		id, album, by string
	}{
		{SourceCatalog, "100", "200", "300"},
		{SourceLibrary, "i.1", "l.1", "r.1"},
	} {
		s := d.Tracks[i]
		if s.Source != want.source || s.ID != want.id || s.AlbumID != want.album || s.ArtistID != want.by {
			t.Errorf("track %d = {%s %q album %q artist %q}, want {%s %q album %q artist %q}",
				i, s.Source, s.ID, s.AlbumID, s.ArtistID, want.source, want.id, want.album, want.by)
		}
	}
}

// A playlist's song that is not in the library is a catalogue song, whatever
// library ID its stored row has: no library action applies to it. An upload
// (no catalogue copy) stays a library song.
func TestPlaylistSongsOutsideTheLibraryAreCatalogSongs(t *testing.T) {
	e := playlistTrackEnv(t)
	storePlaylist(t, e, catalogTrack, libraryTrack, strayTrack, uploadTrack)
	leaveLibrary(t, e, "i.2")
	leaveLibrary(t, e, "i.3")
	pl := &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}}
	d, err := pl.Playlist(t.Context(), "p.mix")
	if err != nil {
		t.Fatal(err)
	}
	type view struct {
		Source        Source
		ID, Lib, Cat  string
		Playable      bool
		Album, Artist string
	}
	var got []view
	for _, s := range d.Tracks {
		got = append(got, view{s.Source, s.ID, s.LibraryID, s.CatalogID, s.Playable, s.AlbumID, s.ArtistID})
	}
	want := []view{
		{SourceCatalog, "100", "", "100", true, "200", "300"},
		{SourceLibrary, "i.1", "i.1", "101", true, "l.1", "r.1"},
		{SourceCatalog, "102", "", "102", true, "", ""},
		{SourceLibrary, "i.3", "i.3", "", true, "", ""},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("tracks:\n got %+v\nwant %+v", got, want)
	}

	// Their states: only the song the library lists is in it.
	ls := &LibraryService{o: e, m: e.m}
	refs := make([]ItemRef, len(d.Tracks))
	for i, s := range d.Tracks {
		refs[i] = ItemRef{Kind: "songs", ID: s.ID, Source: s.Source, CatalogID: s.CatalogID, LibraryID: s.LibraryID}
	}
	st, err := ls.states(t.Context(), refs, false)
	if err != nil {
		t.Fatal(err)
	}
	var in []bool
	for _, s := range st {
		in = append(in, s.InLibrary)
	}
	if !slices.Equal(in, []bool{false, true, false, false}) {
		t.Fatalf("in library = %v", in)
	}
}

// A song naming a library ID is not in the library for that: the synced
// library says, and Apple's answer stands where it does not list the song.
func TestSongStateDoesNotTakeTheLibraryIDForMembership(t *testing.T) {
	refs := []ItemRef{
		{Kind: "songs", ID: "i.2", Source: SourceLibrary, CatalogID: "102", LibraryID: "i.2"}, // not in the library
		{Kind: "songs", ID: "i.3", Source: SourceLibrary, LibraryID: "i.3"},                   // an upload not in it
		{Kind: "songs", ID: "i.4", Source: SourceLibrary, CatalogID: "104", LibraryID: "i.4"}, // in it, Apple unaware
		{Kind: "songs", ID: "i.100", Source: SourceLibrary, CatalogID: "100", LibraryID: "i.100"},
	}
	for _, online := range []bool{true, false} {
		s, _, _, _ := newStateService(t)
		storePlaylist(t, s.o, strayTrack, uploadTrack,
			`{"id":"i.4","type":"library-songs","attributes":{"name":"Held","playParams":{"catalogId":"104"}}}`)
		leaveLibrary(t, s.o, "i.2")
		leaveLibrary(t, s.o, "i.3")
		got, err := s.states(t.Context(), refs, online)
		if err != nil {
			t.Fatal(err)
		}
		var in []bool
		for _, st := range got {
			in = append(in, st.InLibrary)
		}
		// Apple lists catalogue song 100 (in the library) only.
		want := []bool{false, false, true, online}
		if !slices.Equal(in, want) {
			t.Fatalf("online %v: in library = %v, want %v", online, in, want)
		}
	}
}
