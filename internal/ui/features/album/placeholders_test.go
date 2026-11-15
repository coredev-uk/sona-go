package album

import (
	"fmt"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// addPlaceholderAlbum is the pre-added library album l.9 as the library
// lists it before its release: two songs out and three placeholders, named
// "Track N" as Apple names them, with no artist (the services drop Apple's
// "unknown"), not playable.
func (r *rig) addPlaceholderAlbum(release time.Time) {
	r.addPreAddedLibraryAlbum(release)
	page := r.cat.pages["100"]
	page.Tracks = slicesClone(page.Tracks)
	d := r.lib.albums["l.9"]
	d.Tracks = slicesClone(d.Tracks)
	for i := 2; i < 5; i++ {
		name := fmt.Sprintf("Track %d", i+1)
		page.Tracks[i].Name, page.Tracks[i].Artist = name, ""
		d.Tracks[i].Name, d.Tracks[i].Artist, d.Tracks[i].CatalogID, d.Tracks[i].Playable = name, "", "", false
	}
	r.cat.pages["100"] = page
	r.lib.albums["l.9"] = d
}

func slicesClone(s []services.Song) []services.Song { return append([]services.Song(nil), s...) }

// The page of a pre-added album lists its placeholder songs as songs to come:
// by Apple's "Track N", with no artist, not playable, under the day they are
// out.
func TestPreAddedAlbumListsPlaceholdersAsUpcoming(t *testing.T) {
	r := newRig(t, "/album/l.9", 1132, 1000)
	r.addPlaceholderAlbum(r.now.Add(5*time.Hour + 12*time.Minute))
	r.shows("Out in 5 hours, 12 minutes", "Track 3", "Track 4", "Track 5", "Out now")
	r.wantNoTexts("unknown", "Unknown", "Play Track 3", "Play Track 4", "Play Track 5", "Unavailable")
	r.wantTexts("Play Low Tide", "Play Salt Flats")
}

func TestGoldenPreAddedAlbumPlaceholders(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"album-preadded-light", nil},
		{"album-preadded-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			goldenRegional(t)
			r := newRig(t, "/album/l.9", 899, 1133, c.opts...)
			r.addPlaceholderAlbum(r.now.Add(5*time.Hour + 12*time.Minute))
			r.shows("Salt Flats", "Out in 5 hours, 12 minutes", "Track 3")
			settleGolden(t, r, c.name)
		})
	}
}
