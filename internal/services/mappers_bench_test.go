package services

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/library"
)

// BenchmarkLibSong is the per-song cost of mapping a library page: the
// songs of an album share one artwork template, as they do on a real page.
func BenchmarkLibSong(b *testing.B) {
	art, err := artwork.New(artwork.Options{Dir: b.TempDir()})
	if err != nil {
		b.Fatal(err)
	}
	m := mapper{art: art, now: time.Now}
	s := library.Song{
		LibraryID: "i.1", CatalogID: "1", Name: "Song",
		ArtworkTemplate: "https://is1-ssl.mzstatic.com/image/thumb/Music/a/b/c/{w}x{h}bb.jpg",
	}
	b.ReportAllocs()
	for range b.N {
		_ = m.libSong(s)
	}
}
