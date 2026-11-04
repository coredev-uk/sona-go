package kit

import (
	"bytes"
	"context"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// paletteArt stands in for the artwork cache for the evidence pictures: every
// art path answers with its own diagonal gradient at the box asked for, so a
// shelf reads as covers rather than as one colour.
type paletteArt struct{}

func (paletteArt) Template(p string) (string, bool) {
	if strings.HasPrefix(p, "/art/k") {
		return "palette:" + p, true
	}
	return "", false
}

func (paletteArt) Get(_ context.Context, tpl string, w, h int) ([]byte, error) {
	f := fnv.New32a()
	f.Write([]byte(tpl))
	hue := float64(f.Sum32() % 360)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			t := (float64(x)/float64(w) + float64(y)/float64(h)) / 2
			img.SetRGBA(x, y, hsl(hue+t*40, 0.55, 0.72-0.3*t))
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 92}); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func hsl(h, s, l float64) color.RGBA {
	h = math.Mod(h, 360) / 360
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	ch := func(t float64) uint8 {
		t = math.Mod(t+1, 1)
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(v*255 + 0.5)
	}
	return color.RGBA{ch(h + 1.0/3), ch(h), ch(h - 1.0/3), 255}
}

// settle draws frames until the art in view has been decoded.
func (a *itemsApp) settle(t *testing.T, tt *ui.Tester) {
	t.Helper()
	for range 6 {
		tt.Frame()
		a.mt.run(t, 0)
		time.Sleep(25 * time.Millisecond)
	}
	a.mt.run(t, 0)
	tt.Frame()
}

// shoot builds a page in both modes and writes its pictures (SONA_EVIDENCE).
func shoot(t *testing.T, name string, w, h int, build func(c *ui.Context, ctx ItemsCtx), tweak func(*itemsApp, *ui.Tester)) {
	t.Helper()
	for _, mode := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(name+"-"+mode.name, func(t *testing.T) {
			a, tt := newItemsApp(t, w, h, paletteArt{}, build)
			if mode.dark {
				theme.SetMode(theme.ModeDark)
				tt.SetDark(true)
			}
			a.settle(t, tt)
			tt.Advance(time.Second) // the faces ease to the other mode
			if tweak != nil {
				tweak(a, tt)
				a.settle(t, tt)
				tt.Advance(time.Second)
			}
			evidence(t, tt, "items-"+name+"-"+mode.name)
		})
	}
}

func coloured(it services.Item, hex string) services.Item { it.Color = hex; return it }

func TestItemsEvidence(t *testing.T) {
	// WideRow: Apple's hero cards, cropped, with the pager; the first card is hovered.
	heroes := []services.Item{
		coloured(wideNamed("1", "Midnight Drive", "New Music Daily", services.ItemAlbum), "#2a3b5c"),
		coloured(wideNamed("2", "Soft Machines", "Editors' Pick", services.ItemPlaylist), "#5c3b2a"),
		coloured(wideNamed("3", "Field Recordings", "Out Now", services.ItemAlbum), "#2a5c3f"),
	}
	hero := &WideRowState{}
	shoot(t, "widerow", 1000, 470, func(c *ui.Context, ctx ItemsCtx) {
		WideRow(c, hero, heroes, ctx, WideRowOpts{Title: "Featured Releases", Min: 420, Ratio: heroRatio, Crop: true, First: true})
	}, func(a *itemsApp, tt *ui.Tester) {
		r, _ := tt.Find("Midnight Drive") // the card
		tt.Move(r.X+r.W/2, r.Y+r.H/3)
	})

	// LinkList: Apple's genre wall, in even columns read down.
	genres := []string{"Alternative", "Blues", "Classical", "Country", "Dance", "Electronic", "Folk", "Hip-Hop/Rap", "Jazz",
		"K-Pop", "Latin", "Metal", "Pop", "R&B/Soul", "Reggae", "Rock", "Singer/Songwriter", "Soundtrack", "World", "Wellbeing", "Moods and Activities"}
	var links []services.Item
	for i, g := range genres {
		links = append(links, linkItem(services.ItemCharts, strconv.Itoa(i+10), g, ""))
	}
	links[20] = linkItem(services.ItemLink, "", "Concerts near you", "https://music.apple.com/concerts")
	shoot(t, "linklist", 1000, 400, func(c *ui.Context, ctx ItemsCtx) {
		LinkList(c, &LinkListState{}, links, ctx, LinkListOpts{Title: "Browse by Genre", First: true})
	}, nil)

	// StationList: stations with and without pages, one playing with its song on air.
	stations := []services.Station{
		stationFull("ra.1", "Apple Music Hits", "Apple Music", "The biggest songs of every era", true, true),
		stationFull("ra.2", "Apple Music Country", "Apple Music", "Today's country, live", true, true),
		stationFull("ra.3", "Capital FM", "TuneIn", "", false, true),
		stationFull("ra.4", "Heart Dance", "TuneIn", "Non-stop dance", false, true),
		stationFull("ra.5", "Radio Elsewhere", "TuneIn", "", false, false),
		stationFull("ra.6", "Chill Hits", "Apple Music", "Easy listening", true, true),
	}
	stations[4].Unavailable = services.StationNoWidevine
	shoot(t, "stationlist", 1000, 330, func(c *ui.Context, ctx ItemsCtx) {
		ctx.CurrentStation = func(s services.Station) bool { return s.ID == "ra.3" }
		ctx.Playing = true
		ctx.OnAir = &services.OnAir{Artist: "Dua Lipa", Title: "Houdini"}
		StationList(c, &StationListState{}, stations, ctx, StationListOpts{Label: "Live now", OnPlay: func(services.Station) {}})
	}, nil)

	// TrackShelf: three columns of four, the third song playing, one explicit.
	songs := trackItems(12)
	for i, n := range []string{"Houdini", "Midnight Drive", "Soft Machines", "Field Recordings", "Glass Houses", "Paper Planes", "Low Tide", "Northern Lights", "Satellite", "Slow Burn", "Old Haunts", "Afterglow"} {
		songs[i].Name, songs[i].Song.Name = n, n
		songs[i].Song.Artist = []string{"Dua Lipa", "Kiasmos", "Hollow Coves", "Nils Frahm", "Mount Kimbie", "Bonobo"}[i%6]
	}
	songs[1].Song.Explicit = true
	shoot(t, "trackshelf", 1000, 330, func(c *ui.Context, ctx ItemsCtx) {
		ctx.Current = func(s services.Song) bool { return s.ID == "3" }
		ctx.Playing = true
		TrackShelf(c, &TrackShelfState{}, songs, ctx, TrackShelfOpts{Title: "Top Songs", First: true, More: "/room/1"})
	}, func(a *itemsApp, tt *ui.Tester) {
		r, _ := tt.Find("Low Tide")
		tt.Move(r.X+80, r.Y)
	})

	// ItemShelf: albums, an artist, a playlist, a station and an album not out yet.
	mixed := []services.Item{
		namedItem(albumItem(1), "Midnight Drive", "Kiasmos"),
		namedItem(albumItem(2), "Soft Machines", "Hollow Coves"),
		namedArtist("Nils Frahm"),
		namedPlaylist("Deep Focus", "Apple Music Chill"),
		upcomingAlbum(5, "Glass Houses", "Mount Kimbie"),
		namedItem(albumItem(6), "Paper Planes", "Bonobo"),
		namedItem(albumItem(7), "Low Tide", "Hollow Coves"),
		namedItem(albumItem(8), "Satellite", "Kiasmos"),
		namedItem(albumItem(9), "Afterglow", "Nils Frahm"),
	}
	shelf := &CoverShelfState{}
	shoot(t, "itemshelf", 1000, 360, func(c *ui.Context, ctx ItemsCtx) {
		ItemShelf(c, shelf, "New for You", mixed, ctx, ItemShelfOpts{First: true})
	}, func(a *itemsApp, tt *ui.Tester) {
		r, _ := tt.Find("Soft Machines") // the tile
		tt.Move(r.X+r.W/2, r.Y+r.H/3)
	})
}

func wideNamed(id, name, sub, kind string) services.Item {
	it := wideItem(id, "/art/kw"+id+"/1000x250.jpg", "/art/kc"+id+"/400x400.jpg")
	it.Name, it.Subtitle, it.Kind = name, sub, kind
	if kind == services.ItemPlaylist {
		it.Album, it.Playlist = nil, &services.Playlist{ID: id, Name: name}
	} else {
		it.Album.Name = name
	}
	return it
}

func stationFull(id, name, provider, tagline string, hasPage, playable bool) services.Station {
	return services.Station{
		ID: id, Name: name, Provider: provider, Tagline: tagline, HasPage: hasPage, Playable: playable,
		Kind: services.StationLive, Art: services.Art{Small: "/art/k" + id + "/96x96.jpg"},
	}
}

func namedItem(it services.Item, name, artist string) services.Item {
	it.Name, it.Subtitle = name, artist
	it.Album.Name = name
	return it
}

func namedArtist(name string) services.Item {
	return services.Item{Kind: services.ItemArtist, ID: "a1", Name: name, Subtitle: "Artist",
		Art: services.Art{Medium: "/art/kar1/400x400.jpg"}, Artist: &services.Artist{ID: "a1", Name: name}}
}

func namedPlaylist(name, curator string) services.Item {
	return services.Item{Kind: services.ItemPlaylist, ID: "p1", Name: name, Subtitle: curator,
		Art: services.Art{Medium: "/art/kp1/400x400.jpg"}, Playlist: &services.Playlist{ID: "p1", Name: name}}
}

func upcomingAlbum(i int, name, artist string) services.Item {
	it := namedItem(albumItem(i), name, artist)
	it.Album.Prerelease = true
	it.Album.ReleaseAt = time.Date(2026, 11, 19, 0, 0, 0, 0, time.Local).UnixMilli() // after the harness clock
	return it
}
