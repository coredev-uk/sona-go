package miniplayer

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// TestMiniPlayerGolden is the window as it ships, 420×132, light and dark: a
// song playing with a cover, a long title, the line being sung, and the seek
// line part-way along the foot.
func TestMiniPlayerGolden(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		name := "miniplayer-light"
		opts := []uitest.Option{uitest.Bare()}
		if dark {
			name = "miniplayer-dark"
			opts = append(opts, uitest.Dark())
		}
		t.Run(name, func(t *testing.T) {
			h := uitest.New(t, opts...)
			// The cover is the fixture's generated one, whatever the source asks for.
			loader := kit.NewArtLoaderFrom(coverSource{}, h.Main.Post)
			t.Cleanup(func() { loader.Close(); loader.Wait() })
			h.Env.Art = loader
			m := newMini(h.Env)
			svc, desk, lyr := newSvc(), newDesk(), &fakeLyrics{}
			m.svc, m.desk, m.lyr, m.now = svc, desk, lyr, h.Clock.Now
			lyricsync.Session().Clear()
			t.Cleanup(lyricsync.Session().Clear)
			lyr.set(services.Lyrics{Timing: "Line", Lines: []services.LyricLine{
				lineAt("Another line, then the one being sung", 0, 120_000),
				lineAt("We were never meant to hold the light", 120_000, 150_000),
			}})
			tt := h.Tester(t, func(c *ui.Context) {
				c.DisableAnimations()
				m.content(c)
			}, 420, 132)

			s := store.EmptyState()
			s.State = store.StatePlaying
			art := func(int) string { return "cover" }
			s.Current = &services.QueueItem{
				ID: "i.1", CatalogID: "1001", Name: "Everything In Its Right Place", Artist: "The Radiant Pairs", Album: "Kid A Mnesia",
				Art: services.Art{Small: art(96), Medium: art(400), Large: art(1000)}, Backdrop: art(64),
			}
			s.DurationMs, s.PositionMs, s.At = 251_000, 130_000, h.Clock.Now().UnixMilli()
			s.Controls = services.PlaybackControls{Previous: true, Next: true, Seek: true, Shuffle: true, Repeat: true}
			h.Env.Play.Seed(s, services.QueueState{Index: -1})
			tt.Frame()
			tt.Until("the song and the sung line", tt.Shows("Everything In Its Right Place", "We were never meant to hold the light"))
			tt.Settle()
			uitest.Golden(t, tt.Image(), name)
		})
	}
}

// coverSource serves one generated cover for every request.
type coverSource struct{}

func (coverSource) Template(p string) (string, bool) { return p, true }

func (coverSource) Get(_ context.Context, _ string, w, h int) ([]byte, error) {
	return fixture.Cover(3, max(w, 1), max(h, 1)), nil
}

func (coverSource) Backdrop(context.Context, string) ([]byte, error) {
	return fixture.Cover(3, 16, 16), nil
}
