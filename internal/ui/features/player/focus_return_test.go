package player

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/lyricsync"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// focusRig is the bar under Now Playing and full-screen lyrics, as the shell
// lays them out, with the shell's Escape that steps a panel back.
type focusRig struct {
	p  *player
	tt *uitest.Session
	h  *uitest.Harness
}

func newFocusRig(t *testing.T) *focusRig {
	t.Helper()
	h := uitest.New(t, uitest.Bare())
	fake := newFake()
	queueOver(h.Env, fake)
	p := newPlayer(h.Env, fake, func() bool { return false })
	p.now = h.Clock.Now
	p.np.prefsInit = true
	lyr := &fakeLyrics{}
	lyr.set(services.Lyrics{Timing: "Line", Lines: timedLines(30, false)}, nil)
	p.np.lyrics = lyr
	lyricsync.Session().Clear()
	t.Cleanup(lyricsync.Session().Clear)
	r := &focusRig{p: p, h: h}
	slot := p.overlaySlot()
	r.tt = h.Tester(t, func(c *ui.Context) {
		c.DisableAnimations()
		p.env.Video.ReturnFocus(c) // the video feature's layer does this every frame
		if p.ui.overlay != overlayNone && c.Shortcut(0, ui.KeyEscape) {
			p.ui.closeOverlay() // the shell's player.close-overlay
		}
		ui.Column(c).Fill().Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinHeight(0).Children(func() {
				if slot.Open() {
					slot.Build(c)
				}
			})
			p.bar(c)
		})
		ui.Column(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).IgnoreInput(!p.roomOpen()).Children(func() { p.roomLayer(c) })
	}, 1132, 700)
	return r
}

// play shows a playing catalogue song, or a music video.
func (r *focusRig) play(video bool) {
	r.h.Play(0, func(s *services.PlaybackState) {
		s.Current.Album, s.Current.ArtistID, s.Current.AlbumID, s.Current.Video = "", "", "", video
	})
	r.settle()
}

func (r *focusRig) settle() {
	for range 4 {
		r.tt.Frame()
	}
}

func (r *focusRig) click(t *testing.T, label string) {
	t.Helper()
	r.tt.MustClick(label)
	r.settle()
}

func (r *focusRig) escape() {
	r.tt.Key(0, ui.KeyEscape)
	r.settle()
}

// focusedAny names the bar controls that have the focus, "" for none.
func (r *focusRig) focusedAny() string {
	for _, l := range []string{"Open now playing", "Close now playing", "Lyrics", "Queue", "Show video", "Next", "Play", "Pause"} {
		if r.tt.Focused(l) {
			return l
		}
	}
	return ""
}

func TestEscapeClosesNowPlayingBackOnTheCoverThatOpenedIt(t *testing.T) {
	r := newFocusRig(t)
	r.play(false)
	r.click(t, "Open now playing")
	if r.p.ui.overlay != overlayNowPlaying {
		t.Fatal("the cover did not open Now Playing")
	}
	if !r.p.np.hadFocus {
		t.Fatal("the focus did not go into the view")
	}
	r.escape()
	if r.p.ui.overlay != overlayNone {
		t.Fatal("Escape left Now Playing open")
	}
	if !r.tt.Focused("Open now playing") {
		t.Errorf("the focus did not return to the cover (on %q)", r.focusedAny())
	}
}

func TestCloseButtonReturnsTheFocusToTheLyricsButtonThatOpenedNowPlaying(t *testing.T) {
	r := newFocusRig(t)
	r.play(false)
	r.click(t, "Lyrics")
	if r.p.ui.overlay != overlayNowPlaying {
		t.Fatal("Lyrics did not open Now Playing")
	}
	r.click(t, "Close now playing (Escape)")
	if r.p.ui.overlay != overlayNone {
		t.Fatal("Close left Now Playing open")
	}
	if !r.tt.Focused("Lyrics") {
		t.Errorf("the focus did not return to Lyrics (on %q)", r.focusedAny())
	}
}

func TestTheCoverClosingNowPlayingKeepsTheFocusOnItself(t *testing.T) {
	r := newFocusRig(t)
	r.play(false)
	r.click(t, "Open now playing")
	r.click(t, "Close now playing")
	if r.p.ui.overlay != overlayNone {
		t.Fatal("the cover left Now Playing open")
	}
	if !r.tt.Focused("Open now playing") {
		t.Errorf("the focus is on %q, want the cover", r.focusedAny())
	}
}

func TestNowPlayingOpenedByAKeyLeavesTheFocusWithThePageOnClose(t *testing.T) {
	r := newFocusRig(t)
	r.play(false)
	r.p.toggleNowPlaying() // the N key: no bar control opened it
	r.settle()
	if r.p.ui.overlay != overlayNowPlaying {
		t.Fatal("N did not open Now Playing")
	}
	r.escape()
	if r.p.ui.overlay != overlayNone {
		t.Fatal("Escape left Now Playing open")
	}
	if got := r.focusedAny(); got != "" {
		t.Errorf("the focus went to the bar's %q, want the page (no control)", got)
	}
}

func TestNowPlayingClosedWhileTheFocusIsElsewhereTakesNothing(t *testing.T) {
	r := newFocusRig(t)
	r.play(false)
	r.click(t, "Open now playing")
	// The user went on to another control of the bar, then closed the view
	// from it (here: the shortcut that closes it).
	r.click(t, "Next")
	if !r.tt.Focused("Next") {
		t.Fatal("Next did not take the focus")
	}
	r.p.ui.closeOverlay()
	r.settle()
	if !r.tt.Focused("Next") {
		t.Errorf("the focus was taken from Next (on %q)", r.focusedAny())
	}
}

func TestAnOpenerThatIsGoneIsNotAskedAgain(t *testing.T) {
	r := newFocusRig(t)
	r.play(false)
	r.click(t, "Lyrics")
	// A music video takes over the bar: Lyrics gives way to Show video.
	r.play(true)
	r.escape()
	if r.p.ui.overlay != overlayNone {
		t.Fatal("Escape left Now Playing open")
	}
	if got := r.focusedAny(); got != "" {
		t.Fatalf("the focus went to %q with Lyrics gone", got)
	}
	// Lyrics comes back much later: it must not take the focus now.
	r.play(false)
	r.settle()
	if r.tt.Focused("Lyrics") {
		t.Error("Lyrics took the focus when it returned")
	}
}

func TestLeavingFullScreenLyricsPutsTheFocusOnNowPlayingsFullScreenButton(t *testing.T) {
	r := newFocusRig(t)
	r.play(false)
	r.click(t, "Open now playing")
	r.tt.Until("the lyrics tools", r.tt.Shows("Full screen"))
	r.click(t, "Full screen")
	if r.p.ui.overlay != overlayRoom {
		t.Fatal("Full screen did not open the room")
	}
	r.escape()
	if r.p.ui.overlay != overlayNowPlaying {
		t.Fatal("Escape did not step back to Now Playing")
	}
	if !r.tt.Focused("Full screen") {
		t.Error("the focus did not return to Now Playing's Full screen")
	}
	// And leaving Now Playing from there still returns to the cover.
	r.escape()
	if !r.tt.Focused("Open now playing") {
		t.Errorf("the focus did not return to the cover (on %q)", r.focusedAny())
	}
}

func TestTheShowVideoButtonIsTheVideoViewsOpener(t *testing.T) {
	r := newFocusRig(t)
	r.play(true)
	r.click(t, "Show video")
	if !r.h.Env.Video.IsOpen() {
		t.Fatal("Show video did not open the view")
	}
	r.h.Env.Video.Close()
	r.settle()
	if !r.tt.Focused("Show video") {
		t.Errorf("the focus did not return to Show video (on %q)", r.focusedAny())
	}
}
