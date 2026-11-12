//go:build darwin || (windows && (amd64 || arm64))

package media

import (
	"context"
	"hash/crc32"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/media/mediatest"
)

// checksum is a checksum of what tt draws, which changes with every new frame
// of a clip that moves.
func checksum(tt *ui.Tester) uint32 { return crc32.ChecksumIEEE(tt.Image().Pix) }

// TestNativePlaysAndLoops plays a generated H.264 clip through the system's
// video framework (ui/video: AVFoundation, Media Foundation), the way
// MotionCover does: it becomes Ready, shows a moving picture, and loops (its
// position wraps, it never Ends). It also measures what the PR reports: how
// long the player takes to open, to be Ready, and the gap at the loop point
// against the gap between any two frames.
func TestNativePlaysAndLoops(t *testing.T) {
	const frames = 45 // 1.5 s at 30 fps
	f := &files{path: fixtureClip(t, frames)}
	t0 := time.Now()
	p, err := OpenMotion(context.Background(), f, "/motion/abc/720.mp4", 192)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	opened := time.Since(t0)
	if p.Ready() {
		t.Error("Ready before any ui.Video built the player")
	}
	if !p.Loop() || !p.Muted() {
		t.Errorf("loop %v, muted %v: want a muted loop", p.Loop(), p.Muted())
	}
	tt := shown(p)
	until(t, tt, "first picture", 20*time.Second, func() bool {
		if err := p.Err(); err != nil {
			t.Fatal(err)
		}
		return p.Ready()
	})
	ready := time.Since(t0)
	t.Logf("opened in %v, Ready after %v", opened.Round(100*time.Microsecond), ready.Round(time.Millisecond))
	if w, h := p.Size(); w != 96 || h != 64 {
		t.Fatalf("size %d×%d, want 96×64", w, h)
	}
	if d := p.Duration(); d < time.Second || d > 2*time.Second {
		t.Errorf("duration %v, want 1.5 s", d)
	}
	p.Play()

	// The ball pattern has a field and a ball: several colours, all opaque.
	seen := map[[3]byte]bool{}
	pix := tt.Image().Pix
	for i := 0; i+3 < len(pix); i += 4 {
		seen[[3]byte{pix[i], pix[i+1], pix[i+2]}] = true
	}
	if len(seen) < 3 {
		t.Errorf("the picture has %d colours", len(seen))
	}

	// Frames come as the video plays; a loop is the position going back.
	// gaps are the times between pictures that changed, seams the gaps of
	// the first picture after each wrap of the position.
	var gaps, seams []time.Duration
	last, lastAt, lastPos := checksum(tt), time.Now(), p.Position()
	wraps, afterWrap := 0, false
	deadline := time.Now().Add(20 * time.Second)
	for wraps < 3 || afterWrap {
		if time.Now().After(deadline) {
			t.Fatalf("the clip looped %d times in 20 s, want 3", wraps)
		}
		time.Sleep(2 * time.Millisecond)
		tt.Frame()
		if err := p.Err(); err != nil {
			t.Fatal(err)
		}
		if p.Ended() {
			t.Fatal("a looping video ended")
		}
		pos := p.Position()
		if pos < lastPos-500*time.Millisecond {
			wraps++
			afterWrap = true
		}
		lastPos = pos
		if sum := checksum(tt); sum != last {
			now := time.Now()
			gaps = append(gaps, now.Sub(lastAt))
			if afterWrap {
				seams = append(seams, now.Sub(lastAt))
				afterWrap = false
			}
			last, lastAt = sum, now
		}
	}
	if len(gaps) < 3*frames/2 {
		t.Fatalf("only %d pictures over %d loops of %d frames", len(gaps), wraps, frames)
	}
	sorted := slices.Clone(gaps)
	slices.Sort(sorted)
	median, longest := sorted[len(sorted)/2], sorted[len(sorted)-1]
	t.Logf("%d pictures over %d loops: median gap %v, longest gap %v; gap at the loop point %v (a frame is 33 ms)",
		len(gaps), wraps, median.Round(time.Millisecond), longest.Round(time.Millisecond), seams)
	for _, s := range seams {
		if s > 400*time.Millisecond {
			t.Errorf("a gap of %v at the loop point: the loop stalls", s)
		}
	}
}

// TestNativeOpenTime opens and closes a 486×486 clip, Apple's smallest
// motion rendition, as five hover dwells do,
// one after another, and logs what each costs: the Open call itself (on
// Windows it makes a Direct3D 11 device for the player), and the time until
// the first picture. Nothing is asserted but that each becomes Ready.
func TestNativeOpenTime(t *testing.T) {
	if Codec() != "avc" {
		t.Skip("nothing plays H.264 motion artwork here")
	}
	f := &files{path: mediatest.ClipSize(t, 60, "ball", 486, 486)}
	var opens, readies []time.Duration
	for i := range 5 {
		t0 := time.Now()
		p, err := OpenMotion(context.Background(), f, "/motion/abc/720.mp4", 192)
		if err != nil {
			t.Fatal(err)
		}
		opens = append(opens, time.Since(t0))
		tt := shown(p)
		until(t, tt, "first picture", 20*time.Second, func() bool {
			if err := p.Err(); err != nil {
				t.Fatal(err)
			}
			return p.Ready()
		})
		readies = append(readies, time.Since(t0))
		t.Logf("dwell %d: Open %v, Ready %v", i+1, opens[i].Round(100*time.Microsecond), readies[i].Round(time.Millisecond))
		p.Close()
	}
	slices.Sort(opens)
	slices.Sort(readies)
	t.Logf("Open: median %v, longest %v; Ready: median %v, longest %v",
		opens[2].Round(100*time.Microsecond), opens[4].Round(100*time.Microsecond), readies[2].Round(time.Millisecond), readies[4].Round(time.Millisecond))
}
