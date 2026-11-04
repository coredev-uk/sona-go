package album

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// goldenAlbum is a catalogue album with everything the page shows: badges,
// a popular song, an explicit one, fine print, notes and a shelf.
func goldenAlbum(p *services.CatalogAlbum) {
	p.Album.Explicit = true
	p.Album.Traits = services.Traits{Lossless: true, Atmos: true}
	p.Tracks[3].Explicit = true
	p.Header = services.Header{Backdrop: "/art/100/backdrop.jpg", Colors: []string{"#c4793a"}}
	p.MoreByArtist = []services.Album{
		{ID: "200", Name: "Blue Hour", Artist: "Mia Vale", ReleaseDate: "2021-05-01", Art: services.Art{Medium: "/art/200/m.jpg"}},
		{ID: "201", Name: "Dust", Artist: "Mia Vale", ReleaseDate: "2017-02-10", Art: services.Art{Medium: "/art/201/m.jpg"}},
	}
}

// goldenRegional writes dates and durations as an en-GB system does, as the
// reference shots do.
func goldenRegional(t *testing.T) {
	i18n.SetRegional("en-GB")
	t.Cleanup(func() { i18n.SetRegional() })
}

// The goldens are the shell's page column: 1132×1253 less the sidebar (233),
// the top bar (48) and the player bar (72) is 899×1133; the smallest window,
// 940×600, is 707×480.
func TestGoldenAlbumPage(t *testing.T) {
	for _, c := range []struct {
		name string
		opts []uitest.Option
	}{
		{"album-light", nil},
		{"album-dark", []uitest.Option{uitest.Dark()}},
	} {
		t.Run(c.name, func(t *testing.T) {
			goldenRegional(t)
			r := newRig(t, "/album/100", 899, 1133, c.opts...)
			r.addCatalog(saltFlats(goldenAlbum))
			r.shows("Salt Flats", "Blue Hour", "Notes")
			r.tt.Art = append(r.tt.Art, r.h.Env.Art)
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), c.name)
		})
	}
}

func TestGoldenAlbumPageMinimumWindow(t *testing.T) {
	goldenRegional(t)
	r := newRig(t, "/album/100", 707, 480)
	r.addCatalog(saltFlats(goldenAlbum))
	r.shows("Salt Flats", "Notes")
	r.tt.Art = append(r.tt.Art, r.h.Env.Art)
	r.tt.Settle()
	uitest.Golden(t, r.tt.Image(), "album-940x600-light")
}
