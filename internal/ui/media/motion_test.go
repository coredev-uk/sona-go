package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/media/mediatest"
)

// fixtureClip is a generated H.264 clip, or a skip where nothing here plays
// motion artwork or the fixture cannot be made.
func fixtureClip(t *testing.T, frames int) string {
	t.Helper()
	if Codec() != "avc" {
		t.Skip("nothing plays H.264 motion artwork here")
	}
	return mediatest.Clip(t, frames)
}

// shown is a Tester that builds the video of p, 96×64: a player the system
// opens (macOS, Windows) loads only while a ui.Video builds it.
func shown(p *ui.VideoPlayer) *ui.Tester {
	return ui.NewTester(func(c *ui.Context) { ui.Video(c, p).Size(96, 64) }, 96, 64)
}

// until builds frames of tt until cond holds, which fails the test after d.
func until(t *testing.T, tt *ui.Tester, what string, d time.Duration, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(d); ; time.Sleep(3 * time.Millisecond) {
		tt.Frame()
		if cond() {
			return
		}
		if time.Now().After(end) {
			t.Fatalf("no %s within %v", what, d)
		}
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
	if !p.Loop() {
		t.Error("the player does not loop")
	}
	// Ready once the first picture came (at once with GStreamer's decoder,
	// after a build of its video with the system's).
	until(t, shown(p), "first picture", 20*time.Second, func() bool {
		if err := p.Err(); err != nil {
			t.Fatal(err)
		}
		return p.Ready()
	})
	if w, h := p.Size(); w != 96 || h != 64 {
		t.Errorf("size %d×%d, want 96×64", w, h)
	}
	if f.gotPath != "/motion/abc/720.mp4" || f.gotPx != 360 || f.gotCodec != "avc" {
		t.Errorf("asked the cache for %q at %d in %q", f.gotPath, f.gotPx, f.gotCodec)
	}
}

// TestOpenMotionOfAFileThatIsNotAVideo: GStreamer's decoder does not open on
// one (ErrDecode); the system's player opens at once and fails on its own,
// which its Err says, never becoming Ready.
func TestOpenMotionOfAFileThatIsNotAVideo(t *testing.T) {
	if Codec() == "" {
		t.Skip("nothing plays motion artwork here")
	}
	junk := filepath.Join(t.TempDir(), "junk.mp4")
	if err := os.WriteFile(junk, []byte("this is not a video"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := OpenMotion(context.Background(), &files{path: junk}, "/motion/abc/720.mp4", 360)
	if err != nil {
		if !errors.Is(err, ErrDecode) {
			t.Fatalf("a junk file: %v, want ErrDecode", err)
		}
	} else {
		defer p.Close()
		until(t, shown(p), "error of the junk file", 20*time.Second, func() bool { return p.Err() != nil })
		if p.Ready() {
			t.Error("a junk file is Ready")
		}
	}
	// A failure to get the file is not a decode failure.
	_, err = OpenMotion(context.Background(), failing{}, "/motion/abc/720.mp4", 360)
	if err == nil || errors.Is(err, ErrDecode) {
		t.Errorf("a failed download: %v", err)
	}
}

type failing struct{}

func (failing) Local(context.Context, string, int, string) (string, error) {
	return "", errors.New("offline")
}

func TestOpenMotionWithoutDecoder(t *testing.T) {
	if Codec() != "" {
		t.Skip("this platform plays motion artwork")
	}
	if _, err := OpenMotion(context.Background(), &files{}, "/motion/abc/720.mp4", 360); !errors.Is(err, ErrNoDecoder) || !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("err = %v", err)
	}
}

// TestMotionFileOfTheCache plays a real rendition of the motion cache when
// SONA_MOTION_FILE names one (a file of the cache, never committed).
func TestMotionFileOfTheCache(t *testing.T) {
	path := os.Getenv("SONA_MOTION_FILE")
	if path == "" || Codec() == "" {
		t.Skip("SONA_MOTION_FILE is not set, or nothing plays motion artwork here")
	}
	p, err := OpenMotion(context.Background(), &files{path: path}, "/motion/abc/720.mp4", 360)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	until(t, shown(p), "first picture", 20*time.Second, func() bool {
		if err := p.Err(); err != nil {
			t.Fatal(err)
		}
		return p.Ready()
	})
	if w, h := p.Size(); w < 16 || h < 16 {
		t.Errorf("first frame is %d×%d", w, h)
	}
}
