package app

import (
	"strconv"

	"github.com/egoist/mygo/ui"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// PLAYLISTS_EVIDENCE_DIR writes the playlist PR's pictures of the shell with
// the real modules over the synthetic library: the playlist page at both
// window sizes, its edit mode, the dialogs and the confirm, light and dark.
// It writes nothing (and the test is skipped) without it.
func TestPlaylistsEvidence(t *testing.T) {
	dir := os.Getenv("PLAYLISTS_EVIDENCE_DIR")
	if dir == "" {
		t.Skip("PLAYLISTS_EVIDENCE_DIR is not set")
	}
	save := func(name string, img *image.RGBA) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
	for _, dark := range []bool{false, true} {
		mode := "light"
		var opts []uitest.Option
		if dark {
			mode = "dark"
			opts = append(opts, uitest.Dark())
		}
		for _, size := range [][2]int{{1132, 1253}, {940, 600}} {
			w, hgt := size[0], size[1]
			t.Run(mode+"-"+itoa(w), func(t *testing.T) {
				h := uitest.New(t, opts...)
				a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now}, factories)
				t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
				a.firstPaint = nil
				a.connect()
				tt := h.Tester(t, a.view, w, hgt)
				tt.Art = append(tt.Art, a.env.Art)
				tt.Until("the sidebar's playlists", tt.Shows("Sunday morning", "Road trip"))
				click := func(s string) {
					t.Helper()
					if err := tt.Click(s); err != nil {
						t.Fatalf("%v; texts %q", err, tt.Texts())
					}
					tt.Advance(250 * time.Millisecond)
					tt.Drain()
				}
				shot := func(name string) {
					t.Helper()
					tt.Advance(300 * time.Millisecond)
					tt.Settle()
					save("playlists-"+name+"-"+mode+"-"+itoa(w), tt.Image())
				}
				// The sidebar's folder and New playlist menu, over the albums page.
				click("New playlist or folder")
				tt.Until("the menu", tt.Shows("New folder…"))
				shot("sidebar-menu")
				click("New folder…")
				tt.Until("the folder dialog", tt.Shows("Folders group playlists in the sidebar on all your devices."))
				shot("dialog-folder")
				tt.Key(0, ui.KeyEscape)
				tt.Advance(300 * time.Millisecond)
				tt.Drain()

				click("Road trip")
				tt.Until("the page", tt.Shows("Songs in Road trip"))
				tt.Until("the covers", func() bool { return h.Covers.Hits() > 0 })
				shot("page")

				click("Edit")
				tt.Until("edit mode", tt.Shows("Done", "Name and description"))
				shot("page-edit")
				click("Name and description")
				tt.Until("the details dialog", tt.Shows("Changes appear in Apple Music on all your devices."))
				shot("dialog-details")
				tt.Key(0, ui.KeyEscape)
				tt.Advance(300 * time.Millisecond)
				tt.Drain()
				click("Done")

				click("More for Road trip")
				tt.Until("the playlist's menu", tt.Shows("Add to playlist"))
				shot("page-menu")
				click("Add to playlist")
				tt.Until("the picker", tt.Shows("Find a playlist"))
				shot("dialog-add")
			})
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
