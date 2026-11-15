package kit

import (
	"runtime"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// A brick row mixing Apple's 4:1 art, 2:1 art and a square banner (a curator
// with only a square subscriptionCover): every frame is filled by its
// picture, none is a small block between flat bands. The fake art draws a
// gradient at the box asked for, as Apple's CDN returns the crop.
func TestBricksGolden(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("golden images are compared on Linux only: macOS and Windows draw text differently")
	}
	collectionsFonts(t)
	brick := func(id, name, sub, colour, banner string) services.Item {
		it := wideNamed(id, name, sub, services.ItemPlaylist)
		it.Color = colour
		it.Wide = services.Art{Medium: banner, Large: banner}
		it.Art = services.Art{}
		return it
	}
	items := []services.Item{
		brick("1", "Spatial Audio", "Apple Music", "#f83046", "/art/kb1/1000x250.jpg"),
		brick("2", "Best of Apple Music Club", "Apple Music", "#000000", "/art/kb2/1000x250.jpg"),
		brick("3", "Warehouse Project", "DJ Mixes", "#343434", "/art/kb3/1000x500.jpg"),
		brick("4", "Festival Sets", "Apple Music", "#f4f4f4", "/art/kb4/1000x1000.jpg"),
	}
	for _, dark := range []bool{false, true} {
		mode := "light"
		if dark {
			mode = "dark"
		}
		t.Run(mode, func(t *testing.T) {
			st := &WideRowState{}
			a, tt := newItemsApp(t, 4*240+3*24+48, 250, paletteArt{}, func(c *ui.Context, ctx ItemsCtx) {
				WideRow(c, st, items, ctx, WideRowOpts{Title: "New for November", Min: 240, Ratio: brickRatio, First: true})
			})
			if dark {
				theme.SetMode(theme.ModeDark)
				tt.SetDark(true)
			}
			a.settle(t, tt)
			tt.Advance(time.Second)
			collectionsGolden(t, "bricks-"+mode, tt.Image())
		})
	}
}
