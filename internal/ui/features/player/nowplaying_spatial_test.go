package player

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// spatialMissLine is the reason Now Playing gives for a song Apple keeps
// the Spatial Audio mix of.
const spatialMissLine = "Stereo · Apple keeps this song's Spatial Audio mix for its own apps"

// The line under the album says why the song is in stereo only while the
// engine reports a reason (Spatial Audio on, the song has a mix, and it
// plays stereo); it gives way to the Spatial tag when the song plays
// spatial, and it is not in the bar.
func TestNowPlayingSaysWhyASongIsNotSpatial(t *testing.T) {
	r := newNPRig(t)
	r.play(14_500)
	r.open("")
	has := func() bool { return r.tt.HasText(spatialMissLine) }
	tag := func() bool { _, ok := access.Read(t, r.tt.Tester).Find(ui.RoleText, "Spatial"); return ok }
	if has() || tag() {
		t.Fatalf("a song with nothing to say shows %q", r.tt.Texts())
	}

	r.h.Play(14_500, func(s *services.PlaybackState) { s.SpatialMiss = playback.SpatialAppleOnly })
	r.tt.Until("the reason", has)
	if tag() {
		t.Errorf("the Spatial tag shows beside the reason: %q", r.tt.Texts())
	}

	for code, want := range map[string]string{
		playback.SpatialNoMix:       "Stereo · Apple has no Spatial Audio mix of this song for Sona",
		playback.SpatialUnavailable: "Stereo · This song's Spatial Audio mix couldn't be loaded",
	} {
		r.h.Play(14_500, func(s *services.PlaybackState) { s.SpatialMiss = code })
		r.tt.Until(code, func() bool { return r.tt.HasText(want) })
	}

	// Playing spatial: the tag, no reason (even if a stale code came along).
	r.h.Play(14_500, func(s *services.PlaybackState) { s.Spatial, s.SpatialMiss = true, playback.SpatialAppleOnly })
	r.tt.Until("the tag", tag)
	for _, tx := range r.tt.Texts() {
		if strings.HasPrefix(tx, "Stereo ·") {
			t.Errorf("the reason shows beside the tag: %q", tx)
		}
	}

	// Back to nothing to say.
	r.h.Play(14_500)
	r.tt.Until("the line is gone", func() bool { return !has() && !tag() })
}

// The player bar adds nothing for it.
func TestBarSaysNothingOfASongThatIsNotSpatial(t *testing.T) {
	r := newRig(t, true)
	r.seed(func(s *services.PlaybackState) { s.SpatialMiss = playback.SpatialAppleOnly })
	for _, tx := range r.tt.Texts() {
		if strings.Contains(tx, "Spatial") || strings.Contains(tx, "Stereo") {
			t.Errorf("the bar shows %q", tx)
		}
	}
}

// Now Playing with the reason, light and dark.
func TestNowPlayingSpatialMissGolden(t *testing.T) {
	if err := theme.RegisterFonts(); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []struct {
		name string
		dark bool
	}{{"light", false}, {"dark", true}} {
		t.Run(mode.name, func(t *testing.T) {
			var opts []uitest.Option
			if mode.dark {
				opts = append(opts, uitest.Dark())
			}
			r := newNPRigWith(t, opts)
			r.play(14_500, func(s *services.PlaybackState) { s.SpatialMiss = playback.SpatialAppleOnly })
			r.open("")
			r.tt.Until("the reason", r.tt.Shows(spatialMissLine))
			r.tt.Settle()
			uitest.Golden(t, r.tt.Image(), "nowplaying-spatial-miss-"+mode.name)
		})
	}
}
