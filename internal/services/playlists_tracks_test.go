package services

import (
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/library"
)

// playlistTrackEnv stores library playlist "p.mix" holding tracks (JSON
// songs, as Apple lists a library playlist's tracks) and returns the
// services over it.
func playlistTrackEnv(t *testing.T, tracks ...string) (*PlaylistsService, *LibraryService) {
	t.Helper()
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	e := &Env{Deps: Deps{
		DB: d, Library: library.New(d, nil), Emit: &recorder{}, Storefront: func() string { return "gb" },
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: time.Now,
	}, members: newMembership(d)}
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
	return &PlaylistsService{e: e, m: e.m, locks: map[string]*sync.Mutex{}}, &LibraryService{o: e, m: e.m}
}

const (
	// A catalogue song of the playlist: not a library song; the artist and
	// album it names are the catalogue's.
	catalogTrack = `{"id":"100","type":"songs","attributes":{"name":"Catalog","artistName":"Cat Artist","albumName":"Cat Album"},
		"relationships":{"albums":{"data":[{"id":"200","type":"albums"}]},"artists":{"data":[{"id":"300","type":"artists"}]}}}`
	// A library song of the playlist, with its library album and artist.
	libraryTrack = `{"id":"i.1","type":"library-songs","attributes":{"name":"Own","artistName":"Own Artist","albumName":"Own Album","playParams":{"catalogId":"101"}},
		"relationships":{"albums":{"data":[{"id":"l.1","type":"library-albums"}]},"artists":{"data":[{"id":"r.1","type":"library-artists"}]}}}`
)

// The tracks of a library playlist (a collaborative one holds mostly songs
// that are not in the library) name their album and artist, so that the
// track list links them: a catalogue song by its catalogue IDs, a library
// song by its library IDs.
func TestLibraryPlaylistTracksNameAlbumAndArtist(t *testing.T) {
	pl, _ := playlistTrackEnv(t, catalogTrack, libraryTrack)
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
