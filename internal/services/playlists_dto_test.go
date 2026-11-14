package services

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/library"
)

// A library playlist carries when Apple's library added it (its creation)
// into the DTO; a playlist with no date maps to zero.
func TestLibPlaylistMapsDateAdded(t *testing.T) {
	at := time.Date(2024, 3, 3, 10, 30, 0, 0, time.UTC)
	got := mapper{}.libPlaylist(library.Playlist{LibraryID: "p.1", Name: "Road Trip", DateAdded: at})
	if got.DateAdded != at.UnixMilli() {
		t.Errorf("DateAdded = %d, want %d", got.DateAdded, at.UnixMilli())
	}
	if none := (mapper{}).libPlaylist(library.Playlist{LibraryID: "p.2"}); none.DateAdded > 0 {
		t.Errorf("an undated playlist maps DateAdded %d, want none", none.DateAdded)
	}
}
