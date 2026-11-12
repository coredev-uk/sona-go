package artwork

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newCoverCache(t *testing.T) (*Cache, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "covers")
	c, err := New(Options{Dir: t.TempDir(), CoverDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	return c, dir
}

// quadrants is a w×h picture of four colours: red top left, green top
// right, blue bottom left, yellow bottom right.
func quadrants(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			switch {
			case x < w/2 && y < h/2:
				img.Set(x, y, color.RGBA{255, 0, 0, 255})
			case y < h/2:
				img.Set(x, y, color.RGBA{0, 255, 0, 255})
			case x < w/2:
				img.Set(x, y, color.RGBA{0, 0, 255, 255})
			default:
				img.Set(x, y, color.RGBA{255, 255, 0, 255})
			}
		}
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func encodeJPG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// withOrientation inserts an Exif header naming orientation o after the
// SOI of a JPEG (little endian, one tag).
func withOrientation(jpg []byte, o byte) []byte {
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 0x01, 3, 0, 1, 0, 0, 0, o, 0, 0, 0, 0, 0, 0, 0}
	seg := append([]byte("Exif\x00\x00"), tiff...)
	n := len(seg) + 2
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1, byte(n >> 8), byte(n)}
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

// pixelNear reports whether the pixel of img at (x, y) is within 40 of c per channel.
func pixelNear(img image.Image, x, y int, c color.RGBA) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	d := func(a uint32, b uint8) bool { return int(a>>8)-int(b) < 40 && int(b)-int(a>>8) < 40 }
	return d(r, c.R) && d(g, c.G) && d(b, c.B)
}

func TestSaveCoverCropsTheCentreSquare(t *testing.T) {
	c, dir := newCoverCache(t)
	// 600×400: a red left, blue right picture whose centre 400×400 is half each.
	img := image.NewRGBA(image.Rect(0, 0, 600, 400))
	draw.Draw(img, img.Rect, image.NewUniform(color.RGBA{0, 0, 255, 255}), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, 0, 300, 400), image.NewUniform(color.RGBA{255, 0, 0, 255}), image.Point{}, draw.Src)
	tmpl, err := c.SaveCover(bytes.NewReader(encodePNG(t, img)))
	if err != nil {
		t.Fatal(err)
	}
	if rest, ok := strings.CutPrefix(tmpl, coverPrefix); !ok || !validKey(rest) {
		t.Fatalf("%q is not a cover template", tmpl)
	}
	stored, err := os.ReadFile(coverFile(dir, tmpl))
	if err != nil {
		t.Fatal(err)
	}
	got, err := jpeg.Decode(bytes.NewReader(stored))
	if err != nil {
		t.Fatal(err)
	}
	if b := got.Bounds(); b.Dx() != 400 || b.Dy() != 400 {
		t.Fatalf("stored cover is %v, want 400×400 (the short side, never scaled up)", b)
	}
	if !pixelNear(got, 50, 200, color.RGBA{255, 0, 0, 255}) || !pixelNear(got, 350, 200, color.RGBA{0, 0, 255, 255}) {
		t.Error("the stored cover is not the centre of the picture")
	}
	// The same picture is the same cover.
	again, err := c.SaveCover(bytes.NewReader(encodePNG(t, img)))
	if err != nil || again != tmpl {
		t.Errorf("saving it again gave %q, %v; want %q", again, err, tmpl)
	}
}

func TestSaveCoverScalesLargePicturesDown(t *testing.T) {
	c, dir := newCoverCache(t)
	tmpl, err := c.SaveCover(bytes.NewReader(encodeJPG(t, quadrants(2400, 1800))))
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(coverFile(dir, tmpl))
	if b := bounds(t, stored); b.Dx() != CoverEdge || b.Dy() != CoverEdge {
		t.Errorf("stored cover is %v, want %d×%[2]d", b, CoverEdge)
	}
}

func TestSaveCoverHonoursJPEGOrientation(t *testing.T) {
	// Stored sideways: orientation 6 asks for a quarter turn clockwise, which
	// moves the top left (red) to the top right.
	for _, tt := range []struct {
		o              byte
		tl, tr, bl, br color.RGBA
	}{
		{1, red8, green8, blue8, yellow8},
		{3, yellow8, blue8, green8, red8},
		{6, blue8, red8, yellow8, green8},
		{8, green8, yellow8, red8, blue8},
	} {
		c, dir := newCoverCache(t)
		tmpl, err := c.SaveCover(bytes.NewReader(withOrientation(encodeJPG(t, quadrants(400, 400)), tt.o)))
		if err != nil {
			t.Fatalf("orientation %d: %v", tt.o, err)
		}
		stored, _ := os.ReadFile(coverFile(dir, tmpl))
		got, _ := jpeg.Decode(bytes.NewReader(stored))
		if !pixelNear(got, 100, 100, tt.tl) || !pixelNear(got, 300, 100, tt.tr) || !pixelNear(got, 100, 300, tt.bl) || !pixelNear(got, 300, 300, tt.br) {
			t.Errorf("orientation %d: corners are not %v %v %v %v", tt.o, tt.tl, tt.tr, tt.bl, tt.br)
		}
	}
}

var (
	red8    = color.RGBA{255, 0, 0, 255}
	green8  = color.RGBA{0, 255, 0, 255}
	blue8   = color.RGBA{0, 0, 255, 255}
	yellow8 = color.RGBA{255, 255, 0, 255}
)

func TestSaveCoverFlattensTransparencyOntoWhite(t *testing.T) {
	c, dir := newCoverCache(t)
	img := image.NewNRGBA(image.Rect(0, 0, 400, 400))
	draw.Draw(img, image.Rect(100, 100, 300, 300), image.NewUniform(color.NRGBA{255, 0, 0, 255}), image.Point{}, draw.Src)
	tmpl, err := c.SaveCover(bytes.NewReader(encodePNG(t, img)))
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := os.ReadFile(coverFile(dir, tmpl))
	got, _ := jpeg.Decode(bytes.NewReader(stored))
	if !pixelNear(got, 10, 10, color.RGBA{255, 255, 255, 255}) || !pixelNear(got, 200, 200, red8) {
		t.Error("transparent corners are not white around the red square")
	}
}

func TestSaveCoverRefusesWhatCannotBeACover(t *testing.T) {
	c, dir := newCoverCache(t)
	var g bytes.Buffer
	if err := gif.Encode(&g, quadrants(400, 400), nil); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"a GIF":               g.Bytes(),
		"text":                []byte("not an image"),
		"nothing":             nil,
		"too narrow":          encodeJPG(t, quadrants(MinCoverEdge-1, 800)),
		"too short":           encodeJPG(t, quadrants(800, MinCoverEdge-1)),
		"damaged":             encodeJPG(t, quadrants(400, 400))[:200],
		"more than the limit": append(encodeJPG(t, quadrants(400, 400)), make([]byte, MaxCoverBytes)...),
	} {
		if _, err := c.SaveCover(bytes.NewReader(data)); !errorsIs(err, ErrCoverImage) {
			t.Errorf("%s: err = %v, want ErrCoverImage", name, err)
		}
	}
	// A header that claims more pixels than are worth decoding, its CRC
	// fixed so that only the pixel cap can refuse it.
	huge := encodePNG(t, quadrants(400, 400))
	copy(huge[16:], []byte{0, 0, 0x4e, 0x20, 0, 0, 0x4e, 0x20}) // IHDR 20000×20000
	binary.BigEndian.PutUint32(huge[29:], crc32.ChecksumIEEE(huge[12:29]))
	if _, err := c.SaveCover(bytes.NewReader(huge)); !errorsIs(err, ErrCoverImage) || !strings.Contains(err.Error(), "too large") {
		t.Errorf("a 400 Mpx header: err = %v, want ErrCoverImage for its size", err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Errorf("a refused image left %d files in the covers directory", len(es))
	}
	none, err := New(Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := none.SaveCover(bytes.NewReader(encodeJPG(t, quadrants(400, 400)))); !errorsIs(err, ErrNoCoverDir) {
		t.Errorf("without a covers directory: %v", err)
	}
}

func TestStoredCoversAreServedLikeAnyArtwork(t *testing.T) {
	c, _ := newCoverCache(t)
	tmpl, err := c.SaveCover(bytes.NewReader(encodeJPG(t, quadrants(500, 500))))
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range []int{64, 300, 900} {
		b, err := c.Get(context.Background(), tmpl, side, side)
		if err != nil {
			t.Fatalf("at %d: %v", side, err)
		}
		got, _ := jpeg.Decode(bytes.NewReader(b))
		if bb := got.Bounds(); bb.Dx() != side || bb.Dy() != side {
			t.Errorf("at %d: %v", side, bb)
		}
		if !pixelNear(got, side/4, side/4, red8) || !pixelNear(got, side*3/4, side*3/4, yellow8) {
			t.Errorf("at %d: the picture is not the cover", side)
		}
	}
	// The cache may be emptied: the cover is user data and comes back.
	if err := c.Clear(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), tmpl, 128, 128); err != nil {
		t.Errorf("after the cache was cleared: %v", err)
	}
	// A cover nobody stored cannot be drawn.
	if _, err := c.Get(context.Background(), coverPrefix+"0123456789abcdef0123456789abcdef", 128, 128); err == nil {
		t.Error("a cover that was never stored was served")
	}
}

func TestRemoveCoversKeepsWhatIsReferenced(t *testing.T) {
	c, dir := newCoverCache(t)
	save := func(g uint8) string {
		img := image.NewRGBA(image.Rect(0, 0, 400, 400))
		draw.Draw(img, img.Rect, image.NewUniform(color.RGBA{g, g, 0, 255}), image.Point{}, draw.Src)
		tmpl, err := c.SaveCover(bytes.NewReader(encodeJPG(t, img)))
		if err != nil {
			t.Fatal(err)
		}
		return tmpl
	}
	a, b, d := save(10), save(100), save(200)
	if _, err := c.Get(context.Background(), b, 64, 64); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".leftover.jpg.123.tmp"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := c.RemoveCovers([]string{a, d})
	if err != nil || n != 1 {
		t.Fatalf("RemoveCovers = %d, %v; want 1", n, err)
	}
	var left []string
	es, _ := os.ReadDir(dir)
	for _, e := range es {
		left = append(left, e.Name())
	}
	want := []string{filepath.Base(coverFile(dir, a)), filepath.Base(coverFile(dir, d))}
	slices.Sort(want)
	if !slices.Equal(left, want) {
		t.Errorf("left %v, want %v", left, want)
	}
	if _, err := c.Get(context.Background(), b, 128, 128); err == nil {
		t.Error("a removed cover is still served")
	}
	if n, err := c.RemoveCovers(nil); err != nil || n != 2 {
		t.Errorf("RemoveCovers(nil) = %d, %v; want 2", n, err)
	}
}

func errorsIs(err, target error) bool { return err != nil && errors.Is(err, target) }
