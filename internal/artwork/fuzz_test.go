package artwork

import (
	"bytes"
	"errors"
	"image/jpeg"
	"runtime"
	"testing"
)

// The seeds of these targets are in testdata/fuzz: small JPEG and PNG
// pictures made by this package's test helpers (opaque and transparent,
// with Exif orientations), and the pixel bombs of GHSA-v66p-rw4p-w8qq (PNG
// headers declaring 40000×40000 and past MaxSourcePixels).

// fuzzPixels is how large a picture the targets decode. The limit of
// MaxSourcePixels (40 million, 160 MB as RGBA) is the production bound; a
// fuzzer that runs a hundred thousand inputs a second does not need to
// allocate that for each header that claims it. Pictures between the two are
// accepted by the parser, and not decoded here.
const fuzzPixels = 1 << 22

// allocated returns what the process has allocated so far, in bytes.
func allocated() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc
}

// FuzzDecodeImage feeds decodeImage the bytes of an artwork source, from the
// CDN or the disk cache (testdata/fuzz/FuzzDecodeImage: JPEG and PNG
// pictures; a GIF; PNG headers of 40000×40000, 1×(MaxSourcePixels+1) and
// 0×0, GHSA-v66p-rw4p-w8qq).
//
// It checks that decodeImage does not panic, and that:
//   - what is neither a JPEG nor a PNG fails with ErrFormat, and a picture
//     whose header declares no area or more than MaxSourcePixels with
//     ErrTooManyPixels;
//   - those refusals happen on the header, before any pixel is allocated:
//     the process allocates under 1 MiB and eight times the input;
//   - a picture it decodes has the size its header declared, and can be
//     scaled: fit gives at most the requested size, at least 1×1, and
//     cover gives the square asked for.
func FuzzDecodeImage(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		cfg, cerr := decodeConfig(b)
		before := allocated()
		img, err := decodeImage(b)
		spent := allocated() - before
		switch {
		case cerr != nil:
			if err == nil || img != nil {
				t.Fatalf("decodeImage = %v, %v; its header fails with %v", img, err, cerr)
			}
			if !isJPEG(b) && !isPNG(b) && !errors.Is(err, ErrFormat) {
				t.Fatalf("a %d-byte input that is no JPEG or PNG fails with %v, want ErrFormat", len(b), err)
			}
		case checkPixels(cfg) != nil:
			if !errors.Is(err, ErrTooManyPixels) || img != nil {
				t.Fatalf("a %d×%d header decoded: %v, %v", cfg.Width, cfg.Height, img, err)
			}
			if spent > 1<<20+8*uint64(len(b)) {
				t.Fatalf("refusing a %d×%d header allocated %d bytes for %d bytes of input", cfg.Width, cfg.Height, spent, len(b))
			}
		case cfg.Width*cfg.Height > fuzzPixels:
			// Accepted by the parser; not decoded by the fuzzer (see fuzzPixels).
		default:
			if err != nil {
				return // a damaged picture
			}
			if bd := img.Bounds(); bd.Dx() != cfg.Width || bd.Dy() != cfg.Height {
				t.Fatalf("decoded %v, header %d×%d", bd, cfg.Width, cfg.Height)
			}
			if bd := fit(img, 48, 32).Bounds(); bd.Dx() < 1 || bd.Dy() < 1 || bd.Dx() > 48 || bd.Dy() > 32 {
				t.Fatalf("fit(%d×%d, 48×32) = %v", cfg.Width, cfg.Height, bd)
			}
			if bd := cover(img, 40).Bounds(); bd.Dx() != 40 || bd.Dy() != 40 {
				t.Fatalf("cover(%d×%d, 40) = %v", cfg.Width, cfg.Height, bd)
			}
		}
	})
}

// FuzzPrepareCover feeds prepareCover the bytes of an uploaded cover (the
// image check of Cache.SaveCover) (testdata/fuzz/FuzzPrepareCover: JPEGs and
// PNGs, opaque and with transparency, with Exif orientations, one too
// narrow, a 20000×20000 header).
//
// It checks that prepareCover does not panic, and that:
//   - what it refuses it refuses with ErrCoverImage, and returns nothing;
//   - what it accepts it returns as a JPEG that decodes, a square of
//     the smaller of CoverEdge and the picture's sides, which is at least
//     MinCoverEdge;
//   - the cover is itself a valid cover, and a square of the same edge
//     again (storing the same picture twice gives the same cover).
func FuzzPrepareCover(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		cfg, cerr := decodeConfig(b)
		if cerr == nil && cfg.Width*cfg.Height > fuzzPixels && cfg.Width*cfg.Height <= MaxCoverPixels {
			return // see fuzzPixels
		}
		out, err := prepareCover(b)
		if err != nil {
			if !errors.Is(err, ErrCoverImage) || out != nil {
				t.Fatalf("prepareCover = %d bytes, %v", len(out), err)
			}
			return
		}
		if cerr != nil {
			t.Fatalf("accepted a picture whose header fails with %v", cerr)
		}
		if !isJPEG(out) {
			t.Fatal("the cover is not a JPEG")
		}
		img, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatalf("the cover does not decode: %v", err)
		}
		want := min(CoverEdge, cfg.Width, cfg.Height)
		if bd := img.Bounds(); bd.Dx() != bd.Dy() || bd.Dx() != want || bd.Dx() < MinCoverEdge {
			t.Fatalf("a %d×%d picture became a cover of %v, want %d px square", cfg.Width, cfg.Height, bd, want)
		}
		again, err := prepareCover(out)
		if err != nil {
			t.Fatalf("the cover is not a cover: %v", err)
		}
		if bd, err := decodeConfig(again); err != nil || bd.Width != want || bd.Height != want {
			t.Fatalf("a cover of %d px became %+v, %v", want, bd, err)
		}
	})
}

// FuzzExifOrientation feeds exifOrientation the head of an uploaded JPEG, and
// tiffOrientation the TIFF block of an Exif header (testdata/fuzz/
// FuzzExifOrientation: a JPEG with each orientation, TIFF blocks of both
// byte orders).
//
// It checks that neither panics, that the orientation is 1 to 8, and that
// bytes that are not a JPEG are as stored (1).
func FuzzExifOrientation(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		if o := exifOrientation(b); o < 1 || o > 8 || o != 1 && !isJPEG(b) {
			t.Fatalf("exifOrientation = %d, JPEG %v", o, isJPEG(b))
		}
		if o := tiffOrientation(b); o < 1 || o > 8 {
			t.Fatalf("tiffOrientation = %d", o)
		}
	})
}
