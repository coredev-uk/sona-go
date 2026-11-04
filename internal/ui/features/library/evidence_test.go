package library

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// TestLibraryEvidence writes a picture of every library page, light and dark,
// when LIBRARY_EVIDENCE_DIR is set: the page column of a 1132×1253 and a
// 940×600 window (the window less the 232 DIP sidebar and its line), over the
// synthetic library. They are the PR's evidence, not a regression test.
func TestLibraryEvidence(t *testing.T) {
	dir := os.Getenv("LIBRARY_EVIDENCE_DIR")
	if dir == "" {
		t.Skip("set LIBRARY_EVIDENCE_DIR to write the evidence pictures")
	}
	type shoot func(t *testing.T) (*pageRig, string)
	pages := []struct {
		name  string
		build shoot
		ready []string
	}{
		{"recently-added", func(t *testing.T) (*pageRig, string) {
			return newPageRig(t, recentPath, 900, 1253, nil), "recently-added"
		}, []string{"27 albums and playlists"}},
		{"artists", func(t *testing.T) (*pageRig, string) { return newPageRig(t, artistsPath, 900, 1253, nil), "artists" }, []string{"Filter artists"}},
		{"albums", func(t *testing.T) (*pageRig, string) { return newPageRig(t, albumsPath, 900, 1253, nil), "albums" }, []string{"24 albums"}},
		{"songs", func(t *testing.T) (*pageRig, string) { return newPageRig(t, songsPath, 900, 1253, nil), "songs" }, []string{"60 songs"}},
		{"playlists", func(t *testing.T) (*pageRig, string) {
			return newPageRig(t, playlistsPath, 900, 1253, nil), "playlists"
		}, []string{"3 playlists"}},
		{"music-videos", func(t *testing.T) (*pageRig, string) {
			r, _ := newVideosRig(t, videoFixtures(8), false)
			return r.pageRig, "music-videos"
		}, []string{"8 music videos"}},
		{"favourites", func(t *testing.T) (*pageRig, string) {
			r := newFavouritesRig(t, nil)
			for n, at := range []int64{5, 4, 3, 2, 1} {
				favouriteSong(r, n, 0, at)
			}
			r.l.env.Queries.Invalidate(keyFavourites)
			return r, "favourites"
		}, []string{"5 songs"}},
		{"history", func(t *testing.T) (*pageRig, string) {
			r, _, _ := historyRig(t, fixed(twoDays()))
			return r, "history"
		}, []string{"Today", "Yesterday"}},
	}
	for _, p := range pages {
		for _, mode := range []struct {
			name string
			dark bool
		}{{"light", false}, {"dark", true}} {
			for _, size := range [][2]int{{1132 - 233, 1253}, {940 - 233, 600}} {
				t.Run(fmt.Sprintf("%s-%s-%dx%d", p.name, mode.name, size[0]+233, size[1]), func(t *testing.T) {
					r, _ := p.build(t)
					if mode.dark {
						theme.SetMode(theme.ModeDark)
						r.tt.SetDark(true)
					}
					r.tt.SetSize(size[0], size[1])
					r.tt.Until("the page", r.tt.Shows(p.ready...))
					r.tt.Until("the covers", func() bool {
						return r.h.Covers.Hits() > 0 || p.name == "history" || p.name == "music-videos" || p.name == "artists"
					})
					r.tt.Settle()
					f, err := os.Create(filepath.Join(dir, fmt.Sprintf("library-%s-%s-%dx%d.png", p.name, mode.name, size[0]+233, size[1])))
					if err != nil {
						t.Fatal(err)
					}
					defer func() { _ = f.Close() }()
					if err := png.Encode(f, r.tt.Image()); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}
