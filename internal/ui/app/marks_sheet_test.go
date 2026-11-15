package app

import (
	"image"
	"image/draw"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// The marks contact sheet (docs/testing.md, "The marks contact sheet"): every
// small mark that sits on artwork or beside it (play discs and glyphs, the
// playing glyph, badges, checks, chevrons, focus rings, hover washes), over
// the synthetic library's generated covers, at 1x and 2x in light and dark,
// each crop magnified to 4 pixels per DIP with no smoothing, so a half-pixel
// offset, a lopsided glyph or a corner that does not match its cover shows.
// It is a golden: `go test ./internal/ui/app -run MarksContactSheet -update`
// redraws it, and a change to a mark moves it.

// sheetMark is one row of the sheet.
type sheetMark struct {
	name string
	// w and h are the crop's size in DIPs.
	w, h float32
	// at puts the mark in the state to show (a hover, the focus) and returns
	// the top left of the crop in DIPs.
	at func(t *testing.T, tt *uitest.Session) (x, y float32)
}

// sheetRoute is the marks shown on one page of the shell, or in one view the
// shell's own Env builds from the kit's components (the pages that need
// Apple's data to show them: catalogue tiles).
type sheetRoute struct {
	path   string                           // the shell's page; "" for view
	view   func(a *App) func(c *ui.Context) // a page of its own over the shell's Env
	ready  string                           // a text that shows when the page has drawn
	player bool                             // a song is playing
	marks  []sheetMark
}

// sheetVariant is one column of the sheet: a mode at a device scale.
type sheetVariant struct {
	name  string
	dark  bool
	scale float32
}

var sheetVariants = []sheetVariant{{"light 1x", false, 1}, {"light 2x", false, 2}, {"dark 1x", true, 1}, {"dark 2x", true, 2}}

// sheetMagnify is the device pixels each DIP of a crop becomes.
const sheetMagnify = 4

// restPointer parks the pointer over nothing and lets the faces settle.
func restPointer(tt *uitest.Session) {
	tt.Move(3, 3)
	tt.Settle()
}

func sheetFind(t *testing.T, tt *uitest.Session, name string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(name)
	if !ok {
		t.Fatalf("no %q on the page; texts %q", name, tt.Texts())
	}
	return r
}

// sheetHover puts the pointer on the middle of the element named name, moved
// by (dx, dy), and returns its box.
func sheetHover(t *testing.T, tt *uitest.Session, name string, dx, dy float32) ui.Rect {
	t.Helper()
	r := sheetFind(t, tt, name)
	tt.Move(r.X+r.W/2+dx, r.Y+r.H/2+dy)
	tt.Settle()
	return r
}

// sheetFocus tabs until the element named name has the keyboard focus.
func sheetFocus(t *testing.T, tt *uitest.Session, name string) {
	t.Helper()
	restPointer(tt)
	for range 80 {
		if tt.Focused(name) {
			tt.Settle()
			return
		}
		tt.Key(0, ui.KeyTab)
	}
	t.Fatalf("Tab never reached %q", name)
}

// sheetArt is the cover of fixture album n at every size an item takes.
func sheetArt(n int) services.Art {
	u := strings.NewReplacer("{w}", "400", "{h}", "400").Replace(fixture.Template(n))
	return services.Art{Small: u, Medium: u, Large: u}
}

func sheetAlbum(n int, name, artist string) services.Item {
	id := "sheet" + strconv.Itoa(n)
	return services.Item{
		Kind: services.ItemAlbum, ID: id, Name: name, Subtitle: artist, Art: sheetArt(n),
		Album: &services.Album{ID: id, Name: name, Artist: artist, Art: sheetArt(n)},
	}
}

// sheetPage is a page of the shell's own Env: the component in a padded
// column on the ground, as a feature's page has it.
func sheetPage(a *App, build func(c *ui.Context, ctx kit.ItemsCtx)) func(c *ui.Context) {
	return func(c *ui.Context) {
		ctx := a.env.ItemsCtx(nil)
		ui.Column(c).Fill().Padding(24).Background(theme.C("ground")).Children(func() { build(c, ctx) })
	}
}

// sheetSong is a catalogue song as an item, with the cover of fixture album n.
func sheetSong(n int, name, artist string) services.Item {
	id := "song" + strconv.Itoa(n)
	return services.Item{
		Kind: services.ItemSong, ID: id, Name: name, Subtitle: artist,
		Song: &services.Song{ID: id, Name: name, Artist: artist, Playable: true, Art: sheetArt(n)},
	}
}

// sheetCrop is a crop of the w×h DIPs whose top left is dx, dy from the box
// of the element named name, with the pointer on it moved by (px, py) from
// its middle (-1e6 px: none).
func sheetCrop(name string, dx, dy, px, py float32) func(*testing.T, *uitest.Session) (float32, float32) {
	return func(t *testing.T, tt *uitest.Session) (float32, float32) {
		var r ui.Rect
		if px == -1e6 {
			restPointer(tt)
			r = sheetFind(t, tt, name)
		} else {
			r = sheetHover(t, tt, name, px, py)
		}
		return r.X + dx, r.Y + dy
	}
}

func sheetRoutes() []sheetRoute {
	released := sheetAlbum(1, "Glass Harbour", "Nova Ketter")
	upcoming := sheetAlbum(2, "Paper Moons", "The Lantern Choir")
	upcoming.Album.Prerelease = true
	upcoming.Album.ReleaseAt = uitest.Epoch.AddDate(0, 0, 30).UnixMilli()
	station := services.Item{
		Kind: services.ItemStation, ID: "ra.1", Name: "Harbour FM", Subtitle: "Live", Art: sheetArt(3),
		Station: &services.Station{ID: "ra.1", Name: "Harbour FM", Kind: services.StationBroadcast, Playable: true},
	}
	songs := []services.Item{
		sheetSong(4, "Low Tide Radio", "Nova Ketter"), sheetSong(5, "Paper Lanterns", "The Lantern Choir"),
		sheetSong(6, "Salt and Lemon", "Mira Oyelaran"), sheetSong(7, "Quiet Machines", "Hollis & Vane"),
	}
	const none = -1e6
	return []sheetRoute{
		{view: func(a *App) func(*ui.Context) {
			st := &kit.CoverShelfState{}
			return sheetPage(a, func(c *ui.Context, ctx kit.ItemsCtx) {
				kit.ItemShelf(c, st, "New Releases", []services.Item{released, upcoming, station}, ctx, kit.ItemShelfOpts{First: true})
			})
		}, ready: "New Releases", marks: []sheetMark{
			{"Tile: the play disc on hover", 56, 56, sheetCrop("Play Glass Harbour", -10, -10, 0, -40)},
			{"Tile: the play disc with the keyboard focus", 56, 56, func(t *testing.T, tt *uitest.Session) (float32, float32) {
				sheetFocus(t, tt, "Glass Harbour")
				r := sheetFind(t, tt, "Play Glass Harbour")
				return r.X - 10, r.Y - 10
			}},
			{"Tile: the release badge on a cover not out yet", 96, 40, sheetCrop("Out Jan 31", -16, -12, none, 0)},
			{"Tile: the play mark of a station that plays on open", 56, 56, func(t *testing.T, tt *uitest.Session) (float32, float32) {
				r := sheetFind(t, tt, "Harbour FM")
				d := sheetFind(t, tt, "Play Glass Harbour")
				tt.Move(r.X+r.W/2, r.Y-80)
				tt.Settle()
				return r.X - 10, d.Y - 10
			}},
		}},
		{view: func(a *App) func(*ui.Context) {
			st := &kit.TrackShelfState{}
			return sheetPage(a, func(c *ui.Context, ctx kit.ItemsCtx) {
				ctx.Current, ctx.Playing = &services.QueueItem{ID: "song5"}, true
				kit.TrackShelf(c, st, songs, ctx, kit.TrackShelfOpts{Title: "Top Songs", First: true})
			})
		}, ready: "Top Songs", marks: []sheetMark{
			{"Track row: the cover's play wash on hover", 64, 64, sheetCrop("Play Low Tide Radio", -12, -12, 100, 0)},
			{"Track row: the playing glyph over its cover", 64, 64, sheetCrop("Now playing", -26, -26, none, 0)},
			{"Track row: the keyboard focus ring", 176, 64, func(t *testing.T, tt *uitest.Session) (float32, float32) {
				sheetFocus(t, tt, "Salt and Lemon")
				r := sheetFind(t, tt, "Play Salt and Lemon")
				return r.X - 12, r.Y - 12
			}},
		}},
		{path: "/library/songs", ready: "60 songs", marks: []sheetMark{
			{"Select: the chevron beside its value", 96, 48, sheetCrop("Sort", 160-84, -8, none, 0)},
		}},
		{path: "/settings", ready: "Sunday morning", marks: []sheetMark{
			{"Settings: the check beside the chosen theme", 128, 48, func(t *testing.T, tt *uitest.Session) (float32, float32) {
				if err := tt.Click("Appearance"); err != nil {
					t.Fatal(err)
				}
				tt.Settle()
				r := sheetFind(t, tt, "Soft grey, amber") // the chosen theme's hint, under its name
				return r.X - 8, r.Y - 28
			}},
		}},
		{path: "/library/albums", ready: "Sunday morning", marks: []sheetMark{
			{"Tile: the Explicit mark beside a title", 130, 24, sheetCrop("Explicit", -112, -6, none, 0)},
		}},
		{path: "/library/albums", ready: "Sunday morning", player: true, marks: []sheetMark{
			{"Player bar: the play and pause disc", 64, 64, sheetCrop("Pause", -8, -8, none, 0)},
			{"Tooltip: over the player bar's disc", 120, 96, func(t *testing.T, tt *uitest.Session) (float32, float32) {
				restPointer(tt)
				tt.Hover("Pause")
				tt.Settle()
				r := sheetFind(t, tt, "Pause")
				return r.X + r.W/2 - 60, r.Y - 64
			}},
			{"Sidebar: the shuffle button on a playlist's cover", 56, 56, func(t *testing.T, tt *uitest.Session) (float32, float32) {
				// At rest the button takes no room: its top left is where the
				// picture starts. Hover the row beside it, then find it.
				at := sheetFind(t, tt, "Shuffle Focus")
				tt.Move(at.X+80, at.Y+10)
				tt.Settle()
				b := sheetFind(t, tt, "Shuffle Focus")
				return b.X - 10, b.Y - 10
			}},
		}},
	}
}

func TestMarksContactSheet(t *testing.T) {
	// A picture of 16 renders of the whole shell: about 5 min under -race
	// against seconds without, and there's no concurrency in it to check.
	// The ui shards without -race (every pull request and push) draw it.
	if raceDetector {
		t.Skip("runs without -race only: a picture, no concurrency to check")
	}
	// Still: the playing glyph rests, faces do not ease, and the picture settles.
	theme.SetReducedMotion(true)
	t.Cleanup(func() { theme.SetReducedMotion(false) })
	routes := sheetRoutes()
	crops := map[string][]*image.RGBA{} // mark name → one crop per variant
	for _, v := range sheetVariants {
		for _, route := range routes {
			var opts []uitest.Option
			if v.dark {
				opts = append(opts, uitest.Dark())
			}
			h := uitest.New(t, opts...)
			a := newApp(Options{Core: h.Core, Settings: h.Settings, Bus: h.Bus, Poster: h.Main.Post, Art: h.Art, Now: h.Clock.Now, StartPath: route.path}, factories)
			t.Cleanup(func() { a.env.Art.Close(); a.env.Art.Wait() })
			a.firstPaint = nil
			a.connect()
			view := a.view
			if route.view != nil {
				view = route.view(a)
			}
			tt := h.Tester(t, view, 1132, 940)
			tt.Art = append(tt.Art, a.env.Art)
			tt.SetScale(v.scale)
			tt.Until("the page", tt.Shows(route.ready))
			tt.Until("the covers", func() bool { return h.Covers.Hits() > 0 })
			if route.player {
				s := store.EmptyState()
				s.State = store.StatePlaying
				s.Current = &services.QueueItem{ID: "i.1", Name: "Glass Harbour", Artist: "Dara Finch", Album: "Low Tide"}
				s.DurationMs, s.PositionMs, s.At = 240_000, 60_000, h.Clock.Now().UnixMilli()
				s.Controls = services.PlaybackControls{Previous: true, Next: true, Seek: true, Shuffle: true, Repeat: true}
				a.env.Play.Seed(s, services.QueueState{Index: -1})
				tt.Until("the player bar", tt.Shows("Pause"))
			}
			tt.Settle()
			for _, m := range route.marks {
				x, y := m.at(t, tt)
				crops[m.name] = append(crops[m.name], cropMagnified(tt.Image(), v.scale, x, y, m.w, m.h))
				restPointer(tt)
			}
		}
	}
	uitest.Golden(t, assembleSheet(t, routes, crops), "marks-contact-sheet")
}

// cropMagnified is the w×h DIP crop at (x, y) of im (drawn at scale), each
// device pixel repeated so a DIP is sheetMagnify pixels across.
func cropMagnified(im *image.RGBA, scale, x, y, w, h float32) *image.RGBA {
	f := sheetMagnify / int(scale)
	px, py := int(x*scale+0.5), int(y*scale+0.5)
	pw, ph := int(w*scale+0.5), int(h*scale+0.5)
	out := image.NewRGBA(image.Rect(0, 0, pw*f, ph*f))
	for oy := range ph {
		for ox := range pw {
			c := im.RGBAAt(px+ox, py+oy)
			for dy := range f {
				for dx := range f {
					out.SetRGBA(ox*f+dx, oy*f+dy, c)
				}
			}
		}
	}
	return out
}

// textImage is s set in the interface font at 2x, for a label of the sheet.
func textImage(t *testing.T, h *uitest.Harness, s string, w int) *image.RGBA {
	t.Helper()
	tt := h.Tester(t, func(c *ui.Context) {
		ui.Row(c).Fill().Padding(0, 4).AlignItems(ui.Center).Background(theme.C("ground")).Children(func() {
			ui.Text(c, s).FontSize(13).TextColor(theme.C("ink"))
		})
	}, w, 24)
	tt.SetScale(2)
	tt.Frame()
	return tt.Image()
}

// assembleSheet lays the crops out: a row of column titles, then one row per
// mark with its name at the left and the four variants side by side.
func assembleSheet(t *testing.T, routes []sheetRoute, crops map[string][]*image.RGBA) *image.RGBA {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	const gap, labelW = 16, 300 // pixels; labelW in DIPs
	var marks []sheetMark
	for _, r := range routes {
		marks = append(marks, r.marks...)
	}
	colW := make([]int, len(sheetVariants))
	rowH := make([]int, len(marks))
	for i, m := range marks {
		for j, c := range crops[m.name] {
			colW[j] = max(colW[j], c.Bounds().Dx())
			rowH[i] = max(rowH[i], c.Bounds().Dy())
		}
	}
	width := labelW*2 + gap
	for _, w := range colW {
		width += w + gap
	}
	const headH = 48
	height := headH + gap
	for _, rh := range rowH {
		height += rh + gap
	}
	bg := textImage(t, h, "", 10).RGBAAt(0, 0)
	sheet := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	put := func(im *image.RGBA, x, y int) {
		draw.Draw(sheet, image.Rect(x, y, x+im.Bounds().Dx(), y+im.Bounds().Dy()), im, im.Bounds().Min, draw.Src)
	}
	x := labelW*2 + gap
	for j, v := range sheetVariants {
		put(textImage(t, h, v.name+", 4 pixels a DIP", colW[j]/2), x, 0)
		x += colW[j] + gap
	}
	y := headH + gap
	for i, m := range marks {
		put(textImage(t, h, m.name, labelW), 0, y)
		x := labelW*2 + gap
		for j := range sheetVariants {
			put(crops[m.name][j], x, y)
			x += colW[j] + gap
		}
		y += rowH[i] + gap
	}
	return sheet
}
