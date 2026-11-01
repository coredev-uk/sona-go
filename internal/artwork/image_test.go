package artwork

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// tiffPayload is a little-endian TIFF header whose only IFD declares a
// 65535×65535 8-bit image: the shape of the crafted files that crash
// TIFF decoders.
var tiffPayload = func() []byte {
	b := []byte("II*\x00")
	b = binary.LittleEndian.AppendUint32(b, 8)
	b = binary.LittleEndian.AppendUint16(b, 3)
	for _, e := range [][4]uint32{{256, 4, 1, 65535}, {257, 4, 1, 65535}, {258, 3, 1, 8}} {
		b = binary.LittleEndian.AppendUint16(b, uint16(e[0]))
		b = binary.LittleEndian.AppendUint16(b, uint16(e[1]))
		b = binary.LittleEndian.AppendUint32(b, e[2])
		b = binary.LittleEndian.AppendUint32(b, e[3])
	}
	return binary.LittleEndian.AppendUint32(b, 0)
}()

// tiffDecodes counts calls into a TIFF decoder registered with the image
// package, as any dependency could register one. Sona must never use it.
var tiffDecodes atomic.Int32

func init() {
	trap := func(io.Reader) (image.Image, error) {
		tiffDecodes.Add(1)
		return image.NewGray(image.Rect(0, 0, 1, 1)), nil
	}
	trapConfig := func(io.Reader) (image.Config, error) {
		tiffDecodes.Add(1)
		return image.Config{ColorModel: color.GrayModel, Width: 1, Height: 1}, nil
	}
	image.RegisterFormat("tiff", "II*\x00", trap, trapConfig)
	image.RegisterFormat("tiff", "MM\x00*", trap, trapConfig)
}

func encoded(t *testing.T, enc func(io.Writer, image.Image) error) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	var buf bytes.Buffer
	if err := enc(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeAcceptsOnlyJPEGAndPNG(t *testing.T) {
	jpg := encoded(t, func(w io.Writer, m image.Image) error { return jpeg.Encode(w, m, nil) })
	pngb := encoded(t, png.Encode)
	for name, b := range map[string][]byte{"jpeg": jpg, "png": pngb} {
		cfg, err := decodeConfig(b)
		if err != nil || cfg.Width != 8 || cfg.Height != 4 {
			t.Errorf("%s: config %+v, %v", name, cfg, err)
		}
		img, err := decodeImage(b)
		if err != nil || img.Bounds().Dx() != 8 || img.Bounds().Dy() != 4 {
			t.Errorf("%s: decode %v, %v", name, img, err)
		}
	}

	rejected := map[string][]byte{
		"tiff little-endian": tiffPayload,
		"tiff big-endian":    append([]byte("MM\x00*"), tiffPayload[4:]...),
		"gif":                []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00;"),
		"bmp":                append([]byte("BM"), make([]byte, 52)...),
		"webp":               []byte("RIFF\x1a\x00\x00\x00WEBPVP8 \x0e\x00\x00\x00"),
		"jpeg soi only":      {0xFF, 0xD8},
		"png signature cut":  pngMagic[:7],
		"empty":              nil,
	}
	for name, b := range rejected {
		if _, err := decodeConfig(b); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: decodeConfig err = %v, want ErrFormat", name, err)
		}
		if _, err := decodeImage(b); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: decodeImage err = %v, want ErrFormat", name, err)
		}
	}
	if n := tiffDecodes.Load(); n != 0 {
		t.Fatalf("registered TIFF decoder ran %d times", n)
	}
}

// A TIFF from the CDN is refused at fetch: nothing is cached, every
// surface reports ErrFormat, the handler answers 502, and no TIFF decoder
// runs, even one registered with the image package.
func TestTIFFSourceRejected(t *testing.T) {
	tiffDecodes.Store(0)
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	tmpl := "https://" + host + "/image/thumb/Music/tif/{w}x{h}bb.tif"

	if _, err := c.Get(t.Context(), tmpl, 100, 100); !errors.Is(err, ErrFormat) {
		t.Fatalf("Get err = %v, want ErrFormat", err)
	}
	if _, err := c.Backdrop(t.Context(), tmpl); !errors.Is(err, ErrFormat) {
		t.Fatalf("Backdrop err = %v, want ErrFormat", err)
	}
	if _, err := c.DominantColors(t.Context(), tmpl); !errors.Is(err, ErrFormat) {
		t.Fatalf("DominantColors err = %v, want ErrFormat", err)
	}
	rec := httptest.NewRecorder()
	c.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.URL(tmpl, 200, 200), nil))
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("handler: %d", rec.Code)
	}
	if c.Size() != 0 {
		t.Fatalf("cached %d bytes of a rejected source", c.Size())
	}
	if n := tiffDecodes.Load(); n != 0 {
		t.Fatalf("registered TIFF decoder ran %d times", n)
	}
}

func TestPNGSource(t *testing.T) {
	s := newImageServer(t)
	c := newCache(t, s, "", 0)
	tmpl := "https://" + host + "/image/thumb/Music/p/{w}x{h}bb.png"
	b, err := c.Get(t.Context(), tmpl, 100, 100)
	if err != nil {
		t.Fatal(err)
	}
	if !isJPEG(b) || bounds(t, b).Dx() != 100 {
		t.Fatalf("variant: jpeg %v, %v", isJPEG(b), bounds(t, b))
	}
	bd, err := c.Backdrop(t.Context(), tmpl)
	if err != nil {
		t.Fatal(err)
	}
	if got := bounds(t, bd); got.Dx() != 64 || got.Dy() != 64 {
		t.Fatalf("backdrop is %v", got)
	}
	cols, err := c.DominantColors(t.Context(), tmpl)
	if err != nil {
		t.Fatal(err)
	}
	var red, blue bool
	for _, col := range cols {
		red = red || (col.R > 150 && col.B < 100)
		blue = blue || (col.B > 150 && col.R < 100)
	}
	if !red || !blue {
		t.Fatalf("want a red and a blue swatch, got %v", cols)
	}
}

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestFitKeepsAspect(t *testing.T) {
	for _, tc := range []struct{ w, h, bw, bh, ww, wh int }{
		{300, 150, 100, 100, 100, 50},
		{150, 300, 100, 100, 50, 100},
		{300, 300, 128, 128, 128, 128},
		{1000, 1, 100, 100, 100, 1},
		{2048, 853, 1200, 500, 1200, 500},  // 2.4:1 source into a 2.4:1 box
		{4320, 1080, 1000, 417, 1000, 250}, // 4:1 into 2.4:1: width-bound
		{1080, 1080, 1000, 417, 417, 417},  // square into 2.4:1: height-bound
	} {
		got := fit(solid(tc.w, tc.h, color.RGBA{10, 20, 30, 255}), tc.bw, tc.bh).Bounds()
		if got.Dx() != tc.ww || got.Dy() != tc.wh {
			t.Errorf("fit %dx%d into %dx%d = %v, want %dx%d", tc.w, tc.h, tc.bw, tc.bh, got, tc.ww, tc.wh)
		}
	}
	small := solid(80, 60, color.RGBA{A: 255})
	if fit(small, 100, 60) != image.Image(small) {
		t.Error("an image that fits was resampled")
	}
}

// cover takes the centre square of a wide image: of green|red|blue thirds,
// only red survives, with no bleed from the cropped-off sides.
func TestCoverCropsCentre(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 100))
	for y := range 100 {
		for x := range 300 {
			c := color.RGBA{0, 255, 0, 255}
			if x >= 100 && x < 200 {
				c = color.RGBA{255, 0, 0, 255}
			} else if x >= 200 {
				c = color.RGBA{0, 0, 255, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	out := cover(img, 32)
	if out.Bounds() != image.Rect(0, 0, 32, 32) {
		t.Fatalf("bounds %v", out.Bounds())
	}
	for i := 0; i < len(out.Pix); i += 4 {
		if p := out.Pix[i : i+4]; p[0] != 255 || p[1] != 0 || p[2] != 0 || p[3] != 255 {
			t.Fatalf("pixel %d = %v, want pure red", i/4, p)
		}
	}
}

// A flat image stays flat under blur: weights are renormalised at the
// edges instead of fading to black.
func TestBlurKeepsFlatImage(t *testing.T) {
	c := color.RGBA{120, 60, 30, 255}
	img := solid(16, 9, c)
	blur(img, 4)
	for i := 0; i < len(img.Pix); i += 4 {
		if p := img.Pix[i : i+4]; p[0] != c.R || p[1] != c.G || p[2] != c.B || p[3] != c.A {
			t.Fatalf("pixel %d = %v, want %v", i/4, p, c)
		}
	}
}

func TestAdjust(t *testing.T) {
	for _, tc := range []struct {
		name       string
		in         color.RGBA
		sat, light float64
		want       color.RGBA
	}{
		{"identity", color.RGBA{200, 100, 50, 255}, 0, 0, color.RGBA{200, 100, 50, 255}},
		{"saturate 25%", color.RGBA{190, 90, 60, 255}, 25, 0, color.RGBA{206, 81, 44, 255}},
		{"saturate 100%", color.RGBA{150, 100, 100, 255}, 100, 0, color.RGBA{175, 75, 75, 255}},
		{"saturation caps at full", color.RGBA{250, 5, 5, 255}, 100, 0, color.RGBA{255, 0, 0, 255}},
		{"grey keeps no hue", color.RGBA{128, 128, 128, 255}, 25, -15, color.RGBA{90, 90, 90, 255}},
		{"darken clamps at black", color.RGBA{10, 20, 30, 255}, 0, -15, color.RGBA{0, 0, 0, 255}},
		{"percentages clamp", color.RGBA{10, 20, 30, 255}, 0, 500, color.RGBA{255, 255, 255, 255}},
		{"transparent untouched", color.RGBA{}, 25, 50, color.RGBA{}},
	} {
		img := solid(1, 1, tc.in)
		adjust(img, tc.sat, tc.light)
		if got := img.RGBAAt(0, 0); got != tc.want {
			t.Errorf("%s: %v → %v, want %v", tc.name, tc.in, got, tc.want)
		}
	}
}
