package media

import (
	"context"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/coredev-uk/sona-go/internal/ui/media/mediatest"
)

// fixtureClip is a generated H.264 clip, or a skip where the GStreamer in
// use cannot make or decode one.
func fixtureClip(t *testing.T, frames int) string {
	t.Helper()
	if Codec() != "avc" {
		t.Skip("no H.264 decoder in this GStreamer")
	}
	return mediatest.Clip(t, frames)
}

func TestMotionDecoderPlaysAndLoops(t *testing.T) {
	path := fixtureClip(t, 12)
	d, err := NewMotionDecoder(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.(io.Closer).Close() })

	count := func() (n int) {
		for {
			f, err := d.Next()
			if errors.Is(err, io.EOF) {
				return n
			}
			if err != nil {
				t.Fatalf("frame %d: %v", n, err)
			}
			if b := f.Image.Bounds(); b.Dx() != 96 || b.Dy() != 64 {
				t.Fatalf("frame is %v, want 96×64", b)
			}
			if f.Duration <= 0 {
				t.Fatalf("frame %d lasts %v", n, f.Duration)
			}
			n++
		}
	}
	if n := count(); n != 12 {
		t.Fatalf("decoded %d frames, want 12", n)
	}
	// The video loops: rewinding plays every frame again.
	if err := d.Rewind(); err != nil {
		t.Fatal(err)
	}
	if n := count(); n != 12 {
		t.Fatalf("after a rewind decoded %d frames, want 12", n)
	}
}

func TestMotionDecoderPicturesAreNotBlank(t *testing.T) {
	d, err := NewMotionDecoder(fixtureClip(t, 3))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.(io.Closer).Close() })
	f, err := d.Next()
	if err != nil {
		t.Fatal(err)
	}
	// The ball pattern has a field and a ball: several colours, all opaque.
	img, ok := f.Image.(*image.RGBA)
	if !ok {
		t.Fatalf("frame is a %T, want *image.RGBA", f.Image)
	}
	seen := map[[3]byte]bool{}
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] != 255 {
			t.Fatalf("pixel %d is transparent", i/4)
		}
		seen[[3]byte{img.Pix[i], img.Pix[i+1], img.Pix[i+2]}] = true
	}
	if len(seen) < 3 {
		t.Errorf("the picture has %d colours", len(seen))
	}
}

func TestMotionDecoderErrors(t *testing.T) {
	if Codec() == "" {
		if _, err := NewMotionDecoder("whatever.mp4"); !errors.Is(err, errors.ErrUnsupported) && err == nil {
			t.Fatal("a decoder opened where there is none")
		}
		t.Skip("no motion decoder on this platform")
	}
	path := filepath.Join(t.TempDir(), "junk.mp4")
	if err := os.WriteFile(path, []byte("this is not a video"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := NewMotionDecoder(path)
	if err == nil {
		defer func() { _ = d.(io.Closer).Close() }()
		if _, err = d.Next(); err == nil || errors.Is(err, io.EOF) {
			t.Fatalf("a junk file decoded: %v", err)
		}
	}
	if d, err := NewMotionDecoder(filepath.Join(t.TempDir(), "missing.mp4")); err == nil {
		defer func() { _ = d.(io.Closer).Close() }()
		if _, err := d.Next(); err == nil || errors.Is(err, io.EOF) {
			t.Fatalf("a missing file decoded: %v", err)
		}
	}
}

func TestMotionDecoderClose(t *testing.T) {
	d, err := NewMotionDecoder(fixtureClip(t, 6))
	if err != nil {
		t.Fatal(err)
	}
	c := d.(io.Closer)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if _, err := d.Next(); err == nil {
		t.Error("Next after Close")
	}
}

// files is a MotionFiles over one local file.
type files struct {
	path     string
	gotPath  string
	gotPx    int
	gotCodec string
}

func (f *files) Local(_ context.Context, urlPath string, px int, codec string) (string, error) {
	f.gotPath, f.gotPx, f.gotCodec = urlPath, px, codec
	return f.path, nil
}

func TestOpenMotion(t *testing.T) {
	f := &files{path: fixtureClip(t, 8)}
	p, err := OpenMotion(context.Background(), f, "/motion/abc/720.mp4", 360)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if w, h := p.Size(); w != 96 || h != 64 {
		t.Errorf("size %d×%d, want 96×64", w, h)
	}
	if !p.Loop() {
		t.Error("the player does not loop")
	}
	if f.gotPath != "/motion/abc/720.mp4" || f.gotPx != 360 || f.gotCodec != "avc" {
		t.Errorf("asked the cache for %q at %d in %q", f.gotPath, f.gotPx, f.gotCodec)
	}
}

func TestOpenMotionWithoutDecoder(t *testing.T) {
	if Codec() != "" {
		t.Skip("this platform decodes motion artwork")
	}
	if _, err := OpenMotion(context.Background(), &files{}, "/motion/abc/720.mp4", 360); !errors.Is(err, ErrNoDecoder) || !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("err = %v", err)
	}
}

// TestMotionFileOfTheCache decodes a real rendition of the motion cache when
// SONA_MOTION_FILE names one (a file of the cache, never committed).
func TestMotionFileOfTheCache(t *testing.T) {
	path := os.Getenv("SONA_MOTION_FILE")
	if path == "" || Codec() == "" {
		t.Skip("SONA_MOTION_FILE is not set, or nothing decodes motion artwork here")
	}
	d, err := NewMotionDecoder(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.(io.Closer).Close() }()
	f, err := d.Next()
	if err != nil {
		t.Fatal(err)
	}
	if b := f.Image.Bounds(); b.Dx() < 16 || b.Dy() < 16 {
		t.Errorf("first frame is %v", b)
	}
}
