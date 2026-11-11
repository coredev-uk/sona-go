//go:build linux && cgo

package media

import (
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"testing"
)

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
	// The video loops: rewinding plays the clip again, as often as asked,
	// without an error or a stall (Next gives up after 5 s), every frame again.
	for pass := 1; pass <= 3; pass++ {
		if err := d.Rewind(); err != nil {
			t.Fatal(err)
		}
		if n := count(); n != 12 {
			t.Fatalf("after rewind %d decoded %d frames, want 12", pass, n)
		}
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
