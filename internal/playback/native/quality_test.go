//go:build linux || (cgo && (darwin || windows))

package native

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// A quality change applies from the next item: the playing item keeps its
// stream, and an item prepared at the old bitrate is prepared again.
func TestQualityAppliesFromNextItem(t *testing.T) {
	h := newHarness(t, -1, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	if err := h.e.Preload(ctx(t), "B"); err != nil {
		t.Fatal(err)
	}
	h.e.SetQuality(applemusic.Quality64)
	// Seeking the current item reuses its 256 kbps stream.
	if err := h.e.Seek(ctx(t), 0); err != nil {
		t.Fatal(err)
	}
	if n := h.fx.requests("/A/64/list.m3u8", ""); n != 0 {
		t.Fatalf("current item switched bitrate: %d 64 kbps playlist requests", n)
	}
	// B was preloaded at 256 kbps; playing it now fetches the 64 kbps flavor.
	if err := h.e.Play(ctx(t), "B", 0); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]int{"/A/list.m3u8": 1, "/B/list.m3u8": 1, "/B/64/list.m3u8": 1, "/A/64/list.m3u8": 0} {
		if n := h.fx.requests(path, ""); n != want {
			t.Errorf("%s fetched %d times, want %d", path, n, want)
		}
	}
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	if n := h.fx.requests("/A/64/list.m3u8", ""); n != 1 {
		t.Fatalf("replayed item kept the old bitrate: %d 64 kbps playlist requests", n)
	}
}
