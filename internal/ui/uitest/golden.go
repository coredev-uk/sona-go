package uitest

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden images of the tests that ran")

const (
	// Pictures are compared in blocks of blockSize×blockSize pixels by their
	// mean colour: another machine's Pango, FreeType and fontconfig rasterise
	// the same text with other antialiasing and sometimes a pixel's other
	// metrics, which no per-pixel tolerance absorbs without also missing real
	// changes, while a moved edge, a changed colour or a missing cover moves
	// the means of the blocks it crosses.
	blockSize = 8
	// A block differs when one of its mean channels is further than this (of
	// 255) from the golden's.
	blockTolerance = 12
	// The picture matches when at most this share of its blocks differ.
	blockShare = 0.003
)

// Golden compares img with testdata/golden/<name>.png (in the test's
// package directory), by the mean colour of blocks of 8×8 pixels, which
// absorbs the rasterising differences between machines (text antialiasing)
// but not a moved edge, a changed colour or a missing cover. With -update it
// writes the file instead. On a mismatch it saves the picture and a diff
// (differing blocks in red over the golden dimmed) beside the test's other
// artifacts and fails. On other systems than Linux it skips the test.
func Golden(t testing.TB, img image.Image, name string) {
	t.Helper()
	// The goldens are Linux's: Pango shapes the text there, Core Text and
	// DirectWrite place the glyphs elsewhere, which no tolerance should hide.
	// What the test asserted before the picture (the texts, the clicks) ran.
	if runtime.GOOS != "linux" {
		t.Skipf("golden images are compared on Linux only (text is shaped by Pango there; %s's text engine lays glyphs out differently)", runtime.GOOS)
	}
	registerFonts(t)
	path := filepath.Join("testdata", "golden", name+".png")
	got := toNRGBA(img)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		if err := png.Encode(&b, got); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden %s written", path)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("golden %s: %v (run the test with -update to create it)", path, err)
	}
	defer func() { _ = f.Close() }()
	wantImg, err := png.Decode(f)
	if err != nil {
		t.Fatalf("golden %s: %v", path, err)
	}
	want := toNRGBA(wantImg)
	if got.Bounds() != want.Bounds() {
		save(t, name, got, nil)
		t.Fatalf("golden %s: the picture is %v, the golden %v", name, got.Bounds().Size(), want.Bounds().Size())
	}
	diff, bad, total := compare(got, want)
	if float64(bad) > blockShare*float64(total) {
		save(t, name, got, diff)
		t.Fatalf("golden %s: %d of %d blocks of %d×%d pixels differ (%.2f%%, at most %.2f%% allowed); the picture and the diff are in the test's artifacts (-update accepts the new picture)",
			name, bad, total, blockSize, blockSize, 100*float64(bad)/float64(total), 100*blockShare)
	}
}

// compare returns the diff picture, how many blocks differ and how many
// there are. A block at the edge is smaller.
func compare(got, want *image.NRGBA) (diff *image.NRGBA, bad, total int) {
	b := want.Bounds()
	diff = image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y += blockSize {
		for x := b.Min.X; x < b.Max.X; x += blockSize {
			r := image.Rect(x, y, min(x+blockSize, b.Max.X), min(y+blockSize, b.Max.Y))
			total++
			differs := far(mean(got, r), mean(want, r))
			if differs {
				bad++
			}
			for py := r.Min.Y; py < r.Max.Y; py++ {
				for px := r.Min.X; px < r.Max.X; px++ {
					w := want.NRGBAAt(px, py)
					if differs {
						diff.SetNRGBA(px, py, color.NRGBA{255, 0, 0, 255})
					} else {
						diff.SetNRGBA(px, py, color.NRGBA{w.R/4 + 190, w.G/4 + 190, w.B/4 + 190, 255})
					}
				}
			}
		}
	}
	return diff, bad, total
}

// mean is the mean colour of the pixels of r.
func mean(img *image.NRGBA, r image.Rectangle) color.NRGBA {
	var sr, sg, sb, sa, n int
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			sr, sg, sb, sa, n = sr+int(c.R), sg+int(c.G), sb+int(c.B), sa+int(c.A), n+1
		}
	}
	return color.NRGBA{uint8(sr / n), uint8(sg / n), uint8(sb / n), uint8(sa / n)}
}

func far(a, b color.NRGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) > blockTolerance || d(a.G, b.G) > blockTolerance || d(a.B, b.B) > blockTolerance || d(a.A, b.A) > blockTolerance
}

func toNRGBA(img image.Image) *image.NRGBA {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

func save(t testing.TB, name string, got, diff image.Image) {
	t.Helper()
	dir := t.ArtifactDir()
	for suffix, img := range map[string]image.Image{".actual.png": got, ".diff.png": diff} {
		if img == nil {
			continue
		}
		var b bytes.Buffer
		if err := png.Encode(&b, img); err != nil {
			t.Error(err)
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name+suffix), b.Bytes(), 0o644); err != nil {
			t.Error(err)
		}
	}
}
