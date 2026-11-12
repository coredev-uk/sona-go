package kit

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"

	"github.com/coredev-uk/sona-go/internal/ui/media"
	"github.com/coredev-uk/sona-go/internal/ui/media/mediatest"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// systemOpen opens path with ui/video, as media.OpenMotion does on macOS and
// Windows: at once, muted and looping, not Ready until a built ui.Video loads
// its first picture (MyGo's engine players are the only ones that behave so;
// a decoder's are Ready from the start). It skips where the system has no
// engine.
func systemOpen(t *testing.T, path string) func(context.Context, string, int) (*ui.VideoPlayer, error) {
	t.Helper()
	return func(context.Context, string, int) (*ui.VideoPlayer, error) {
		p, err := video.Open(path)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", media.ErrDecode, err)
		}
		return p.SetLoop(true).SetMuted(true), nil
	}
}

// mediaWaitFor builds frames of tt and runs what posted until cond holds.
func mediaWaitFor(t *testing.T, tt *ui.Tester, e *mediaMotionEnv, what string, cond func() bool) {
	t.Helper()
	for end := time.Now().Add(20 * time.Second); !cond(); time.Sleep(3 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("no %s", what)
		}
		tt.Frame()
		e.mt.run(t, 0)
	}
}

// TestMotionCoverFadesInOnlyOnceTheSystemPlayerIsReady: a player that opens
// at once and loads in the background is not shown, and the slot is not
// ready, until it has its first picture, which it gets only because the cover
// builds its video, transparent, meanwhile.
func TestMotionCoverFadesInOnlyOnceTheSystemPlayerIsReady(t *testing.T) {
	clip := mediatest.Clip(t, 90)
	e := newMediaMotionEnv(t)
	e.p.open = systemOpen(t, clip)
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")

	// The player has arrived and nothing has built its video yet: opening.
	var s *motionSlot
	mediaWaitFor(t, tt, e, "player", func() bool {
		s = mediaOnlySlot(e.p)
		return s != nil && s.state != slotLoading
	})
	if s.state != slotOpening {
		t.Fatalf("state %v with a player that was never built, want opening (%v)", s.state, slotOpening)
	}
	if s.player.Ready() {
		t.Fatal("the player is Ready before any video was built")
	}
	if r := e.p.Report(); r.Last != "" {
		t.Errorf("last = %q before the first picture: nothing played yet", r.Last)
	}

	// Builds load it: transparent until ready, then the fade.
	hover := theme.C("hover")
	still := func() bool {
		got := at(tt, 60, 60)
		return !near(got, mediaGifRed) && !near(got, mediaGifBlue) && (near(got, color.RGBA{220, 30, 30, 255}) || near(got, color.RGBA{hover.R, hover.G, hover.B, 255}))
	}
	for s.state == slotOpening {
		tt.Frame()
		if s.state == slotOpening && !still() {
			t.Fatalf("the cover shows something else than its still before the video is ready: %v", at(tt, 60, 60))
		}
		e.mt.run(t, 0)
		time.Sleep(3 * time.Millisecond)
		if s.state == slotFailed {
			t.Fatalf("the video failed: %v", s.err)
		}
	}
	if s.state != slotReady || !s.player.Ready() {
		t.Fatalf("state %v, ready %v", s.state, s.player.Ready())
	}
	if r := e.p.Report(); r.Last != "playing" || r.Playing != 1 {
		t.Errorf("report %+v, want playing once ready", r)
	}
	tt.Frame()
	tt.Advance(motionFade + 100*time.Millisecond)
	tt.Frame()
	if still() {
		t.Errorf("the video is not drawn once faded in: %v", at(tt, 60, 60))
	}
}

// TestMotionCoverKeepsTheStillWhenTheSystemPlayerFails: a file the system
// cannot play opens, never becomes Ready, and fails on its own: the slot says
// so (Diagnostics' last video), closes the player and the still stays.
func TestMotionCoverKeepsTheStillWhenTheSystemPlayerFails(t *testing.T) {
	junk := filepath.Join(t.TempDir(), "junk.mp4")
	if err := os.WriteFile(junk, []byte("this is not a video"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := newMediaMotionEnv(t)
	e.p.open = systemOpen(t, junk)
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	var s *motionSlot
	mediaWaitFor(t, tt, e, "failure", func() bool {
		s = mediaOnlySlot(e.p)
		return s != nil && s.state == slotFailed
	})
	if s.player != nil || s.err == nil {
		t.Errorf("player %v, err %v after the failure", s.player, s.err)
	}
	if r := e.p.Report(); !strings.HasPrefix(r.Last, "media-error: ") || r.Playing != 0 {
		t.Errorf("report %+v, want a media-error and nothing playing", r)
	}
	tt.Advance(time.Second)
	tt.Frame()
	if mediaIsMoving(tt) {
		t.Error("a video is drawn after the player failed")
	}
	got := at(tt, 60, 60)
	if near(got, mediaGifRed) || near(got, mediaGifBlue) {
		t.Errorf("a video is drawn: %v", got)
	}
}

// TestMotionPolicyStopsOfferingMotionWhereTheSystemHasNoVideo: a Windows N
// edition without the Media Feature Pack opens no player. Every other cover
// would fail the same, so motion is off, with the reason in the report.
func TestMotionPolicyStopsOfferingMotionWhereTheSystemHasNoVideo(t *testing.T) {
	e := newMediaMotionEnv(t)
	e.f.fail = fmt.Errorf("%w: %w", media.ErrNoDecoder, errors.New("video: no media engine"))
	_, tt := newMediaCoverApp(t, e, "/motion/k1/720.mp4")
	mediaSettle(t, tt, e, 1)
	if e.p.Allowed() {
		t.Error("motion is still allowed")
	}
	r := e.p.Report()
	if !strings.HasPrefix(r.Last, "media-error: ") || r.H264 != "" || r.HEVC != "" || r.Allowed {
		t.Errorf("report %+v", r)
	}
	for range 3 {
		mediaFrames(t, tt, e.mt)
	}
	if n := len(e.f.opened()); n != 1 {
		t.Errorf("%d attempts: a system with no video is asked once", n)
	}
}
