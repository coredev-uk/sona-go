package kit

import (
	"image/color"
	"math"
	"strconv"
	"testing"
	"time"

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

// fluidWidths are window widths that give a shelf's tiles a fractional width
// (the fixed 648 of shelfApp makes them exactly 160, which hides it).
var fluidWidths = []int{650, 701, 733, 790, 851}

// fluidShelf is an ItemShelf of items in a window w wide.
func fluidShelf(t *testing.T, w int, items []services.Item, sc float32) (*itemsApp, *ui.Tester) {
	t.Helper()
	st := &CoverShelfState{}
	a, tt := newItemsApp(t, w, 500, paletteArt{}, func(c *ui.Context, ctx ItemsCtx) {
		ItemShelf(c, st, "New Releases", items, ctx, ItemShelfOpts{First: true})
	})
	tt.SetScale(sc)
	a.settle(t, tt)
	return a, tt
}

// coverAround is the cover drawn around a tile's bottom-left corner mark
// (disc, a rect), found in the picture: the first tile's cover starts at the
// disc's left less 8, the window's others are found the same way.
func coverAround(t *testing.T, tt *ui.Tester, sc float32, mark ui.Rect) ui.Rect {
	t.Helper()
	art, ok := overlay.ArtBounds(tt.Image(), sc, ui.Rect{X: mark.X - 14, Y: mark.Y + mark.H - 400, W: 150 + 40, H: 400 + 14})
	if !ok {
		t.Fatalf("no cover near %v", mark)
	}
	return art
}

// A tile's play disc keeps its 8 DIP inset from its cover's corner at any
// window width, tiles of a fraction of a DIP included, at 1x and 2x; the
// first tile's disc is on whole pixels (the others start where the grid's
// gap puts them).
func TestTilePlayDiscKeepsItsInsetOnFluidTiles(t *testing.T) {
	collectionsFonts(t)
	for _, w := range fluidWidths {
		for _, sc := range overlayScales {
			_, tt := fluidShelf(t, w, albumItems(3), sc)
			for i, n := range []string{"Album 1", "Album 2", "Album 3"} {
				disc, ok := tt.Find("Play " + n)
				if !ok {
					t.Fatalf("w %d scale %v: no play disc on %s", w, sc, n)
				}
				what := n + " at " + strconv.Itoa(w)
				cover := coverAround(t, tt, sc, disc)
				// The picture snaps the cover and the disc to device pixels
				// each; the layout's fractional x of a later tile is not
				// the inset.
				drawn := disc
				drawn.X = float32(math.Round(float64(disc.X*sc))) / sc
				overlay.InsetBottomLeft(t, what+": play disc", drawn, cover, 8, 8)
				if i == 0 {
					overlay.WholePixels(t, what+": play disc", disc, sc)
				}
			}
		}
	}
}

// The play mark of a station that plays on open sits in the same 8 DIP corner
// as the disc, at any window width. It is measured in the picture: its amber
// disc against the cover's pixels. (The release badge is anchored the same way
// from the same whole cover height, coverArtHeight.)
func TestTilePlaysOnOpenMarkKeepsItsInsetOnFluidTiles(t *testing.T) {
	collectionsFonts(t)
	amber := func(c color.RGBA) bool { return c.R > 150 && c.G > 90 && c.G < 150 && c.B < 40 }
	for _, w := range []int{650, 701, 733, 790} {
		for _, sc := range overlayScales {
			st := stationItem(false, true)
			st.Art = services.Art{Medium: "/art/ks1/400x400.jpg"}
			_, tt := fluidShelf(t, w, []services.Item{st, albumItem(2), albumItem(3)}, sc)
			tt.Move(60, 100)
			tt.Advance(time.Second)
			tt.Frame()
			cover, ok := overlay.ArtBounds(tt.Image(), sc, ui.Rect{X: 0, Y: 0, W: 195, H: 400})
			if !ok {
				t.Fatalf("w %d scale %v: no cover drawn", w, sc)
			}
			r := box(tt.Image(), amber)
			if r.Empty() {
				t.Fatalf("w %d scale %v: the play mark does not show on hover", w, sc)
			}
			mark := ui.Rect{X: float32(r.Min.X) / sc, Y: float32(r.Min.Y) / sc, W: float32(r.Dx()) / sc, H: float32(r.Dy()) / sc}
			overlay.InsetBottomLeft(t, "play mark at "+strconv.Itoa(w), mark, cover, 8, 8)
		}
	}
}

// A wide card's play disc keeps its 12 DIP inset from its frame's corner when
// the frame's height (width over ratio) is a fraction of a DIP. The art is
// held at its gate, so the frame is the bare item colour the layout gives it:
// a decode that landed would paint the picture's 1 px edge line over the
// frame's outermost pixels, and which of the two sizes asked for (the window
// opens at another width) lands first is a race.
func TestWideCardPlayDiscKeepsItsInsetOnAFractionalFrame(t *testing.T) {
	collectionsFonts(t)
	for _, w := range []int{301, 317, 333, 349} {
		for _, sc := range overlayScales {
			_, tt, _ := wideApp(t, &fakeArt{gate: make(chan struct{})}, []services.Item{wideItem("1", artPath("1", 1000, 420), "")}, 2.4)
			tt.SetSize(w, 400)
			tt.SetScale(sc)
			tt.Frame()
			disc, ok := tt.Find("Play Item 1")
			if !ok {
				t.Fatalf("w %d: no play disc", w)
			}
			r := box(tt.Image(), isFrame)
			frame := ui.Rect{X: float32(r.Min.X) / sc, Y: float32(r.Min.Y) / sc, W: float32(r.Dx()) / sc, H: float32(r.Dy()) / sc}
			overlay.InsetBottomLeft(t, "wide card disc at "+strconv.Itoa(w), disc, frame, 12, 12)
			overlay.WholePixels(t, "wide card disc at "+strconv.Itoa(w), disc, sc)
		}
	}
}
