package kit

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/overlay"
)

// overlayScales are the device scales every overlay check runs at: a mark
// that lies off by half a DIP is a pixel at 2x and a blur at 1x.
var overlayScales = []float32{1, 2}

// A track row's play wash covers exactly its cover, and the equaliser that
// replaces it for the song playing sits in the middle of it, at 1x and 2x.
// The covers are the evidence palette's gradients (saturated, so the picture
// shows where they are), never a flat fixture tile.
func TestTrackRowOverlaysLieOnTheirCover(t *testing.T) {
	collectionsFonts(t)
	for _, sc := range overlayScales {
		items := trackItems(4)
		st := &TrackShelfState{}
		a, tt := newItemsApp(t, 640, 300, paletteArt{}, func(c *ui.Context, ctx ItemsCtx) {
			ctx.Current = &services.QueueItem{ID: "2"}
			ctx.Playing = true
			TrackShelf(c, st, items, ctx, TrackShelfOpts{Title: "Top Songs", First: true})
		})
		tt.SetScale(sc)
		a.settle(t, tt)
		im := tt.Image()

		wash, ok := tt.Find("Play Song 1")
		if !ok {
			t.Fatalf("scale %v: no play button on Song 1: %q", sc, tt.Texts())
		}
		overlay.OnArtwork(t, "track row play wash", im, sc, wash)
		overlay.WholePixels(t, "track row play wash", wash, sc)

		bars, ok := tt.Find("Now playing")
		if !ok {
			t.Fatalf("scale %v: no equaliser on Song 2: %q", sc, tt.Texts())
		}
		cover := wash // Song 2's cover is one row down in the same column
		cover.Y += trackRowH
		overlay.Inside(t, "track row equaliser", bars, cover)
		overlay.Centred(t, "track row equaliser", bars, cover, 0.5)
		overlay.WholePixels(t, "track row equaliser", bars, sc)
	}
}

// A tile's play disc sits in its cover's bottom-left corner, 8 DIPs from
// both edges, whole pixels, and the cover is where the layout says it is.
func TestTilePlayDiscSitsInItsCoversCorner(t *testing.T) {
	collectionsFonts(t)
	for _, sc := range overlayScales {
		a, tt, _ := shelfApp(t, "New Releases", albumItems(3), ItemShelfOpts{})
		tt.SetScale(sc)
		a.settle(t, tt)
		im := tt.Image()
		for _, n := range []string{"Album 1", "Album 2", "Album 3"} {
			disc, ok := tt.Find("Play " + n)
			if !ok {
				t.Fatalf("scale %v: no play disc on %s: %q", sc, n, tt.Texts())
			}
			// The cover is square at the tile's width (160 here).
			cover := ui.Rect{X: disc.X - 8, Y: disc.Y + disc.H + 8 - 160, W: 160, H: 160}
			overlay.OnArtwork(t, n+"'s cover", im, sc, cover)
			overlay.WholePixels(t, n+"'s cover", cover, sc)
			overlay.InsetBottomLeft(t, n+"'s play disc", disc, cover, 8, 8)
			overlay.WholePixels(t, n+"'s play disc", disc, sc)
		}
	}
}
