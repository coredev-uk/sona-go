package artwork

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// coverPrefix starts a template for a cover the user chose from a file:
// the prefix, then 32 hex digits naming a JPEG in Options.CoverDir. A
// cover is addressed, cached and served like any other template, but it is
// the only copy there is: the cache may evict its variants, never the file.
const coverPrefix = "cover:"

// Limits of an uploaded cover, shown to the user beside the chooser.
const (
	// MaxCoverBytes is the largest file SaveCover reads.
	MaxCoverBytes = 20 << 20
	// MinCoverEdge is the least width and height of a usable image, in px.
	MinCoverEdge = 300
	// CoverEdge is the edge of the stored square, in px: the largest size
	// Sona draws a playlist cover at is below it.
	CoverEdge = 1024
	// MaxCoverPixels bounds what is decoded, so a small file that claims a
	// huge picture cannot take the memory of the machine.
	MaxCoverPixels = 40_000_000
	coverQuality   = 90
)

// ErrCoverImage reports an image that cannot be a cover: not a JPEG or
// PNG, damaged, over MaxCoverBytes or MaxCoverPixels, or under MinCoverEdge
// on a side.
var ErrCoverImage = errors.New("artwork: unusable cover image")

// ErrNoCoverDir reports a Cache built without Options.CoverDir.
var ErrNoCoverDir = errors.New("artwork: covers are not stored here")

// IsCover reports whether tmpl is a valid template of a stored cover.
func IsCover(tmpl string) bool {
	rest, ok := strings.CutPrefix(tmpl, coverPrefix)
	return ok && validKey(rest)
}

// parseCover validates the part of a cover template after its prefix.
func parseCover(rest string) (string, string, error) {
	if !validKey(rest) {
		return "", "", errors.New("artwork: invalid cover template")
	}
	tmpl := coverPrefix + rest
	return tmpl, keyOf(tmpl), nil
}

func coverFile(dir, tmpl string) string {
	return filepath.Join(dir, strings.TrimPrefix(tmpl, coverPrefix)+".jpg")
}

// SaveCover reads an image from r (a JPEG or PNG of at most MaxCoverBytes,
// at least MinCoverEdge px on each side), crops its centred square, scales
// it to at most CoverEdge px and stores it as a JPEG in CoverDir. A JPEG's
// orientation is honoured and transparency is flattened onto white. It
// returns the cover's template; storing the same picture again gives the
// same one. The work is CPU-bound: call it off the main thread.
func (c *Cache) SaveCover(r io.Reader) (string, error) {
	if c.covers == "" {
		return "", ErrNoCoverDir
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxCoverBytes+1))
	if err != nil {
		return "", fmt.Errorf("artwork: read cover: %w", err)
	}
	if len(data) > MaxCoverBytes {
		return "", fmt.Errorf("%w: over %d MiB", ErrCoverImage, MaxCoverBytes>>20)
	}
	jpg, err := prepareCover(data)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(jpg)
	tmpl := coverPrefix + hex.EncodeToString(sum[:16])
	if err := os.MkdirAll(c.covers, 0o700); err != nil {
		return "", fmt.Errorf("artwork: %w", err)
	}
	path := coverFile(c.covers, tmpl)
	if _, err := os.Stat(path); err == nil {
		return tmpl, nil
	}
	f, err := os.CreateTemp(c.covers, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("artwork: %w", err)
	}
	_, werr := f.Write(jpg)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(f.Name(), path)
	}
	if werr != nil {
		_ = os.Remove(f.Name())
		return "", fmt.Errorf("artwork: store cover: %w", werr)
	}
	return tmpl, nil
}

// prepareCover turns the bytes of an uploaded image into the stored JPEG.
func prepareCover(data []byte) ([]byte, error) {
	cfg, err := decodeConfig(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCoverImage, err)
	}
	if min(cfg.Width, cfg.Height) < MinCoverEdge {
		return nil, fmt.Errorf("%w: %d×%d px is under %d px", ErrCoverImage, cfg.Width, cfg.Height, MinCoverEdge)
	}
	if cfg.Width*cfg.Height > MaxCoverPixels {
		return nil, fmt.Errorf("%w: %d×%d px is too large", ErrCoverImage, cfg.Width, cfg.Height)
	}
	img, err := decodeImage(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCoverImage, err)
	}
	if !opaque(img) {
		flat := image.NewRGBA(img.Bounds())
		draw.Draw(flat, flat.Rect, image.NewUniform(color.White), image.Point{}, draw.Src)
		draw.Draw(flat, flat.Rect, img, img.Bounds().Min, draw.Over)
		img = flat
	}
	out := orient(cover(img, min(CoverEdge, cfg.Width, cfg.Height)), exifOrientation(data))
	return encodeJPEG(out, coverQuality)
}

// opaque reports whether img has no transparency to flatten: every
// format Go decodes without an alpha channel.
func opaque(img image.Image) bool {
	switch img.(type) {
	case *image.YCbCr, *image.Gray, *image.Gray16, *image.CMYK:
		return true
	}
	o, ok := img.(interface{ Opaque() bool })
	return ok && o.Opaque()
}

// exifOrientation reads the orientation (1–8, 1 meaning as stored) a JPEG's
// Exif header asks for; 1 for a PNG or a JPEG that has none.
func exifOrientation(b []byte) int {
	if !isJPEG(b) {
		return 1
	}
	for i := 2; i+4 <= len(b) && b[i] == 0xFF; {
		marker, n := b[i+1], int(b[i+2])<<8|int(b[i+3])
		if marker == 0xDA || marker == 0xD9 || n < 2 || i+2+n > len(b) {
			break
		}
		if seg := b[i+4 : i+2+n]; marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}
		i += 2 + n
	}
	return 1
}

// tiffOrientation finds the Orientation tag (0x0112) in the first IFD of
// an Exif TIFF block.
func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var u16 func([]byte) int
	var u32 func([]byte) int
	switch string(t[:2]) {
	case "II":
		u16 = func(p []byte) int { return int(p[0]) | int(p[1])<<8 }
		u32 = func(p []byte) int { return int(p[0]) | int(p[1])<<8 | int(p[2])<<16 | int(p[3])<<24 }
	case "MM":
		u16 = func(p []byte) int { return int(p[0])<<8 | int(p[1]) }
		u32 = func(p []byte) int { return int(p[0])<<24 | int(p[1])<<16 | int(p[2])<<8 | int(p[3]) }
	default:
		return 1
	}
	off := u32(t[4:8])
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := u16(t[off:])
	for i := range n {
		e := off + 2 + i*12
		if e+12 > len(t) {
			break
		}
		if u16(t[e:]) == 0x0112 {
			if v := u16(t[e+8:]); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// orient applies an Exif orientation to a square image: the transformed
// copy, or img itself for 1.
func orient(img *image.RGBA, o int) *image.RGBA {
	if o <= 1 || o > 8 {
		return img
	}
	n := img.Rect.Dx()
	out := image.NewRGBA(img.Rect)
	for y := range n {
		for x := range n {
			// Where the stored pixel (x, y) lands, by orientation.
			var dx, dy int
			switch o {
			case 2:
				dx, dy = n-1-x, y
			case 3:
				dx, dy = n-1-x, n-1-y
			case 4:
				dx, dy = x, n-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = n-1-y, x
			case 7:
				dx, dy = n-1-y, n-1-x
			case 8:
				dx, dy = y, n-1-x
			}
			copy(out.Pix[dy*out.Stride+dx*4:][:4], img.Pix[y*img.Stride+x*4:][:4])
		}
	}
	return out
}

// renderCover draws a stored cover as a side×side JPEG.
func (c *Cache) renderCover(ctx context.Context, tmpl string, side int) ([]byte, error) {
	if c.covers == "" {
		return nil, ErrNoCoverDir
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(coverFile(c.covers, tmpl))
	if err != nil {
		return nil, fmt.Errorf("artwork: read cover: %w", err)
	}
	c.workSem <- struct{}{}
	defer func() { <-c.workSem }()
	img, err := decodeImage(data)
	if err != nil {
		return nil, fmt.Errorf("artwork: decode cover: %w", err)
	}
	return encodeJPEG(cover(img, side), variantQuality)
}

// RemoveCovers deletes the stored covers whose templates are not in keep
// and the cached artwork drawn from them, and returns how many files went.
// Callers keep what the library still refers to; sign-out keeps nothing.
func (c *Cache) RemoveCovers(keep []string) (int, error) {
	if c.covers == "" {
		return 0, nil
	}
	des, err := os.ReadDir(c.covers)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("artwork: list covers: %w", err)
	}
	kept := make(map[string]bool, len(keep))
	for _, k := range keep {
		kept[k] = true
	}
	var gone []string
	var errs []error
	for _, de := range des {
		name, ok := strings.CutSuffix(de.Name(), ".jpg")
		if !ok || !validKey(name) || kept[coverPrefix+name] {
			if strings.HasPrefix(de.Name(), ".tmp-") {
				_ = os.Remove(filepath.Join(c.covers, de.Name()))
			}
			continue
		}
		if err := os.Remove(filepath.Join(c.covers, de.Name())); err != nil {
			errs = append(errs, fmt.Errorf("artwork: remove cover: %w", err))
			continue
		}
		gone = append(gone, coverPrefix+name)
	}
	_, err = c.Remove(gone)
	return len(gone), errors.Join(append(errs, err)...)
}
