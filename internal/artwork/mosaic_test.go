package artwork

import (
	"bytes"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// solidTmpl is the template of an album whose cover is the single colour rgb
// ("rrggbb").
func solidTmpl(rgb string) string { return tmplFor("solid-" + rgb) }

var (
	red    = solidTmpl("ff0000")
	green  = solidTmpl("00ff00")
	blue   = solidTmpl("0000ff")
	yellow = solidTmpl("ffff00")
	white  = solidTmpl("ffffff")
)

func TestMosaicSelection(t *testing.T) {
	const bad = "https://evil.example/{w}x{h}bb.jpg"
	tests := []struct {
		name string
		in   []string
		want []string // tiles of the mosaic, or the single template
	}{
		{"nothing", nil, nil},
		{"empty playlist", []string{}, nil},
		{"no artwork at all", []string{"", "  ", ""}, nil},
		{"one album, many tracks", []string{red, red, red, red, red, red}, []string{red}},
		{"two albums", []string{red, green, red, green}, []string{red}},
		{"three albums", []string{red, green, blue, blue, green}, []string{red}},
		{"four albums", []string{red, green, blue, yellow}, []string{red, green, blue, yellow}},
		{"more than four: the first four", []string{red, green, blue, yellow, white}, []string{red, green, blue, yellow}},
		{"duplicates are skipped wherever they fall", []string{red, red, green, red, blue, green, yellow, white}, []string{red, green, blue, yellow}},
		{"tracks without artwork are skipped", []string{"", red, "", green, blue, "", yellow}, []string{red, green, blue, yellow}},
		{"missing artwork leaves too few", []string{"", red, "", green, blue, ""}, []string{red}},
		{"foreign hosts are skipped", []string{bad, red, green, bad, blue, yellow}, []string{red, green, blue, yellow}},
		{"only foreign hosts", []string{bad, bad}, nil},
		{"the first usable one stands in", []string{"", bad, green, red}, []string{green}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Mosaic(tt.in)
			switch len(tt.want) {
			case 0:
				if got != "" {
					t.Fatalf("Mosaic = %q, want none", got)
				}
			case 1:
				if got != tt.want[0] {
					t.Fatalf("Mosaic = %q, want the single %q", got, tt.want[0])
				}
			default:
				if _, _, err := parseTemplate(got); err != nil || !isMosaic(got) {
					t.Fatalf("Mosaic = %q (%v), want a valid mosaic", got, err)
				}
				if tiles := mosaicTiles(got); !slices.Equal(tiles, tt.want) {
					t.Fatalf("tiles = %v, want %v", tiles, tt.want)
				}
			}
		})
	}
}

func TestMosaicRejectsForeignTiles(t *testing.T) {
	c := newCache(t, newImageServer(t), "", 0)
	good := strings.Join([]string{red, green, blue, yellow}, " ")
	for name, tmpl := range map[string]string{
		"three tiles":    mosaicPrefix + strings.Join([]string{red, green, blue}, " "),
		"five tiles":     mosaicPrefix + good + " " + white,
		"foreign tile":   mosaicPrefix + strings.Join([]string{red, green, blue, "https://evil.example/{w}x{h}bb.jpg"}, " "),
		"nested mosaic":  mosaicPrefix + strings.Join([]string{red, green, blue, mosaicPrefix + yellow}, " "),
		"no tiles":       mosaicPrefix,
		"plain http":     mosaicPrefix + strings.Join([]string{red, green, blue, strings.Replace(yellow, "https", "http", 1)}, " "),
		"file URL tiles": mosaicPrefix + strings.Join([]string{red, green, blue, "file:///etc/passwd"}, " "),
	} {
		if u := c.URL(tmpl, 100, 100); u != "" {
			t.Errorf("%s: URL = %q, want none", name, u)
		}
		if _, err := c.Get(t.Context(), tmpl, 100, 100); err == nil {
			t.Errorf("%s: Get succeeded", name)
		}
	}
	if u := c.URL(mosaicPrefix+good, 100, 100); u == "" {
		t.Fatal("a valid mosaic has no URL")
	}
}

func rgbAt(t *testing.T, b []byte, x, y int) (r, g, bl int) {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	r32, g32, b32, _ := img.At(x, y).RGBA()
	return int(r32 >> 8), int(g32 >> 8), int(b32 >> 8)
}

// near reports whether (r,g,b) is within JPEG's error of want ("rrggbb"
// channels 0 or 255 only).
func near(r, g, b int, want [3]int) bool {
	for i, v := range [3]int{r, g, b} {
		if d := v - want[i]; d > 40 || d < -40 {
			return false
		}
	}
	return true
}

func TestMosaicLayoutAndSize(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	ctx := t.Context()
	m := Mosaic([]string{red, green, blue, yellow, white})

	for _, size := range []int{48, 96, 100, 300, 512} {
		b, err := c.Get(ctx, m, size, size)
		if err != nil {
			t.Fatalf("%d: %v", size, err)
		}
		if got := bounds(t, b); got.Dx() != size || got.Dy() != size {
			t.Fatalf("%d px mosaic is %v", size, got)
		}
		q := size / 4
		for i, want := range [][3]int{{255, 0, 0}, {0, 255, 0}, {0, 0, 255}, {255, 255, 0}} {
			x, y := q+i%2*size/2, q+i/2*size/2
			if r, g, bl := rgbAt(t, b, x, y); !near(r, g, bl, want) {
				t.Fatalf("%d px: tile %d at (%d,%d) is (%d,%d,%d), want about %v", size, i, x, y, r, g, bl, want)
			}
		}
		// Tiles meet edge to edge: no gutter, no uncovered pixel.
		for _, p := range [][2]int{{size/2 - 1, size/2 - 1}, {size / 2, size / 2}, {size - 1, size - 1}, {0, 0}} {
			if r, g, bl := rgbAt(t, b, p[0], p[1]); r+g+bl < 200 {
				t.Fatalf("%d px: pixel %v is dark (%d,%d,%d)", size, p, r, g, bl)
			}
		}
	}

	// A box that is not square holds the square mosaic fitted inside it.
	b, err := c.Get(ctx, m, 200, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, b); got.Dx() != 100 || got.Dy() != 100 {
		t.Fatalf("200×100 box: mosaic is %v, want 100×100", got)
	}

	// The fifth artwork is never fetched, and each tile once per size.
	if n := s.count(host, "/image/thumb/Music/solid-ffffff/64x64bb.jpg"); n != 0 {
		t.Fatalf("fifth artwork fetched %d times", n)
	}
}

func TestMosaicFetchesEachTileOnce(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	ctx := t.Context()
	m := Mosaic([]string{red, green, blue, yellow})
	for _, size := range []int{96, 80, 120, 64, 128} {
		if _, err := c.Get(ctx, m, size, size); err != nil {
			t.Fatal(err)
		}
	}
	// 64–128 px mosaics are composed from the tiles' 64 px sources, which
	// the tiles share with every other use of those albums.
	if n := s.total(); n != 4 {
		t.Fatalf("fetches = %d, want 4 (one 64 px source per album): %v", n, s.hits)
	}
	// The albums alone then cost nothing more at that size.
	if _, err := c.Get(ctx, red, 64, 64); err != nil {
		t.Fatal(err)
	}
	if n := s.total(); n != 4 {
		t.Fatalf("an album's own artwork refetched: %v", s.hits)
	}
}

func TestMosaicKeyFollowsTheChosenFour(t *testing.T) {
	c := newCache(t, newImageServer(t), "", 0)
	url := func(in ...string) string { return c.URL(Mosaic(in), 100, 100) }

	base := url(red, green, blue, yellow)
	if base == "" {
		t.Fatal("no URL")
	}
	if got := url(red, green, red, blue, green, yellow, white, red); got != base {
		t.Errorf("repeats and later artworks changed the cover: %s vs %s", got, base)
	}
	if got := url(red, green, blue, white, yellow); got == base {
		t.Error("a different fourth artwork kept the cover")
	}
	if got := url(green, red, blue, yellow); got == base {
		t.Error("swapped tiles kept the cover")
	}
	if got := url(red, green, blue); got == base || got != c.URL(red, 100, 100) {
		t.Errorf("three artworks = %s, want the first album's own %s", got, c.URL(red, 100, 100))
	}
	if got := url(red, green, blue, yellow, white); got != base {
		t.Error("a fifth artwork changed the cover")
	}
}

func TestMosaicInvalidatesWhenTheFourChange(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	ctx := t.Context()
	before := Mosaic([]string{red, green, blue, yellow})
	after := Mosaic([]string{red, green, blue, white}) // an album replaced a track

	b1, err := c.Get(ctx, before, 128, 128)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := c.Get(ctx, after, 128, 128)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(b1, b2) {
		t.Fatal("the cover did not change with its albums")
	}
	if r, g, bl := rgbAt(t, b2, 96, 96); !near(r, g, bl, [3]int{255, 255, 255}) {
		t.Fatalf("new fourth tile is (%d,%d,%d), want white", r, g, bl)
	}
	// Only the new album was fetched; the other three tiles were reused.
	if n := s.total(); n != 5 {
		t.Fatalf("fetches = %d, want 5: %v", n, s.hits)
	}
	// The previous cover still resolves for pages that hold its URL.
	if _, err := c.Get(ctx, before, 128, 128); err != nil {
		t.Fatal(err)
	}
}

func TestMosaicPersistsAndIsServed(t *testing.T) {
	s := newImageServer(t)
	dir := t.TempDir()
	c := newCache(t, s, dir, 0)
	m := Mosaic([]string{red, green, blue, yellow})
	u := c.URL(m, 120, 120)
	want, err := c.Get(t.Context(), m, 120, 120)
	if err != nil {
		t.Fatal(err)
	}
	fetched := s.total()

	// A new process finds the composed cover on disk and serves it at the
	// same URL without fetching or composing again.
	c2 := newCache(t, s, dir, 0)
	rec := httptest.NewRecorder()
	c2.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, u, nil))
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "image/jpeg" {
		t.Fatalf("served %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if !bytes.Equal(rec.Body.Bytes(), want) {
		t.Fatal("served cover differs from the one composed")
	}
	if s.total() != fetched {
		t.Fatalf("refetched after restart: %d → %d", fetched, s.total())
	}
}

func TestMosaicBackdropDerivesFromIt(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	m := Mosaic([]string{red, blue, green, yellow})
	b, err := c.Backdrop(t.Context(), m)
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, b); got.Dx() != 64 || got.Dy() != 64 {
		t.Fatalf("backdrop is %v", got)
	}
	// Top-left red, top-right blue, bottom-left green: the blur softens the
	// tiles but each quadrant keeps its hue.
	if r, g, bl := rgbAt(t, b, 8, 8); r < g+60 || r < bl+60 {
		t.Errorf("top-left is (%d,%d,%d), want red", r, g, bl)
	}
	if r, g, bl := rgbAt(t, b, 56, 8); bl < r+60 || bl < g+60 {
		t.Errorf("top-right is (%d,%d,%d), want blue", r, g, bl)
	}
	if r, g, bl := rgbAt(t, b, 8, 56); g < r+60 || g < bl+60 {
		t.Errorf("bottom-left is (%d,%d,%d), want green", r, g, bl)
	}
	cols, err := c.DominantColors(t.Context(), m)
	if err != nil || len(cols) < 2 {
		t.Fatalf("colours = %v, %v", cols, err)
	}
	if n := s.total(); n != 4 {
		t.Fatalf("fetches = %d, want the four 64 px sources", n)
	}
}

func TestMosaicFailureIsNotCached(t *testing.T) {
	s := newImageServer(t)
	s.notReady = 1
	c := newCache(t, s, "", 0)
	slow := tmplFor("gen/solid-00ff00") // answers 406 once, like a cover still rendering
	m := Mosaic([]string{red, slow, blue, yellow})

	if _, err := c.Get(t.Context(), m, 128, 128); err == nil {
		t.Fatal("composed a mosaic with a tile missing")
	}
	b, err := c.Get(t.Context(), m, 128, 128)
	if err != nil {
		t.Fatalf("not retried: %v", err)
	}
	if r, g, bl := rgbAt(t, b, 96, 32); !near(r, g, bl, [3]int{0, 255, 0}) {
		t.Fatalf("second tile is (%d,%d,%d), want green", r, g, bl)
	}
}

func TestRemoveMosaics(t *testing.T) {
	s := newImageServer(t)
	dir := t.TempDir()
	c := newCache(t, s, dir, 0)
	ctx := t.Context()
	m := Mosaic([]string{red, green, blue, yellow})
	if _, err := c.Get(ctx, m, 128, 128); err != nil {
		t.Fatal(err)
	}
	u := c.URL(m, 128, 128)

	freed, err := c.RemoveMosaics()
	if err != nil || freed == 0 {
		t.Fatalf("freed %d, %v", freed, err)
	}
	key := strings.Split(strings.TrimPrefix(u, "/art/"), "/")[0]
	if _, err := os.Stat(filepath.Join(dir, key)); !os.IsNotExist(err) {
		t.Fatalf("mosaic directory remains: %v", err)
	}
	// Plain artwork stays: it is catalogue artwork that other pages use.
	before := s.total()
	if _, err := c.Get(ctx, red, 64, 64); err != nil {
		t.Fatal(err)
	}
	if s.total() != before {
		t.Fatal("RemoveMosaics removed an album's own artwork")
	}
}
