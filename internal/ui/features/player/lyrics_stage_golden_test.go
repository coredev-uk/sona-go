package player

import (
	"fmt"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/lyrics"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// sketchLines are the approved sketch's lines (invented words): quick lines,
// then one held for six seconds with a held note and background vocals.
func sketchLines() []services.LyricLine {
	word := func(begin, end int64, text string, bg bool) services.LyricWord {
		return services.LyricWord{BeginMs: begin, EndMs: end, Text: text, Background: bg}
	}
	line := func(begin, end int64, text string, words ...services.LyricWord) services.LyricLine {
		return services.LyricLine{BeginMs: begin, EndMs: end, Text: text, Agent: "v1", AgentType: "person", Words: words}
	}
	out := []services.LyricLine{
		line(1000, 3800, "Out past the breakwater", word(1000, 1500, "Out ", false), word(1500, 2000, "past ", false), word(2000, 2400, "the ", false), word(2400, 3800, "breakwater", false)),
		line(4000, 6200, "the radio was low", word(4000, 4500, "the ", false), word(4500, 5100, "radio ", false), word(5100, 5600, "was ", false), word(5600, 6200, "low", false)),
		line(6400, 9400, "We left the lights on", word(6400, 6900, "We ", false), word(6900, 7500, "left ", false), word(7500, 8000, "the ", false), word(8000, 8600, "lights ", false), word(8600, 9400, "on", false)),
		line(9600, 15600, "for the tide", word(9600, 10200, "for ", false), word(10200, 10800, "the ", false), word(10800, 15000, "tide", false), word(11200, 15200, "(for the tide)", true)),
		line(15800, 18000, "and it came back", word(15800, 16300, "and ", false), word(16300, 16700, "it ", false), word(16700, 17300, "came ", false), word(17300, 18000, "back", false)),
		line(18200, 21000, "slower than we thought", word(18200, 18800, "slower ", false), word(18800, 19400, "than ", false), word(19400, 20000, "we ", false), word(20000, 21000, "thought", false)),
		line(21200, 23400, "and the lights stayed on", word(21200, 21800, "and ", false), word(21800, 22300, "the ", false), word(22300, 22800, "lights ", false), word(22800, 23400, "stayed on", false)),
	}
	// More lines, held for different times, so the song's quickest and
	// longest are not just those on show.
	at := int64(23600)
	for i, d := range []int64{1500, 2600, 3400, 4200, 5000, 3000} {
		out = append(out, line(at, at+d, fmt.Sprintf("and again, line %d", i), word(at, at+d, fmt.Sprintf("and again, line %d", i), false)))
		at += d + 200
	}
	out[3].Background = "(for the tide)"
	return out
}

// goldenRig is Full-Screen Lyrics at 1280×720 over a cover's wash, with the
// strip not yet woken.
func goldenRig(t *testing.T, opts []uitest.Option, l services.Lyrics, lyrErr error, posMs int64) *npRig {
	t.Helper()
	r := newNPRigWith(t, opts, func(r *npRig) { r.width, r.height = 1280, 720 })
	loader := kit.NewArtLoaderFrom(backdropArt{}, r.h.Main.Post)
	t.Cleanup(loader.Close)
	r.h.Env.Art = loader
	r.lyr.set(l, lyrErr)
	r.play(posMs, func(s *services.PlaybackState) {
		s.Current.Backdrop = "/art/k1/backdrop.jpg"
		s.Current.Art.Small = "/art/k1/art.jpg"
		s.Current.Art.Large = "/art/k1/art.jpg"
	})
	r.p.ui.openRoom()
	r.tt.Until("the view", r.tt.Shows("Full-Screen Lyrics"))
	r.tt.Until("the wash to load", func() bool { return loader.Pending() == 0 })
	r.settle()
	r.advance(300 * time.Millisecond)
	r.advance(300 * time.Millisecond)
	r.tt.Settle()
	return r
}

// sketchGapLines are the sketch with 9 s of instrumental before its fifth
// line: a gap from 15.6 s to 24.8 s.
func sketchGapLines() []services.LyricLine {
	out := sketchLines()
	const shift = 9000
	for i := 4; i < len(out); i++ {
		out[i].BeginMs += shift
		out[i].EndMs += shift
		for k := range out[i].Words {
			out[i].Words[k].BeginMs += shift
			out[i].Words[k].EndMs += shift
		}
	}
	return out
}

// The view is dark whatever the theme, so the light and the dark window
// each show the same picture. Five states: a word-timed line mid-sweep, a
// duet, no lyrics, the strip shown, and an instrumental gap.
func TestFullScreenLyricsGoldens(t *testing.T) {
	for _, mode := range []struct {
		name string
		opts []uitest.Option
	}{{"light", nil}, {"dark", []uitest.Option{uitest.Dark()}}} {
		t.Run(mode.name+"/sweep", func(t *testing.T) {
			// Half way through the held "tide".
			r := goldenRig(t, mode.opts, services.Lyrics{Timing: "Word", Lines: sketchLines()}, nil, 12_900)
			uitest.Golden(t, r.tt.Image(), "full-screen-lyrics-sweep-"+mode.name)
		})
		t.Run(mode.name+"/duet", func(t *testing.T) {
			r := goldenRig(t, mode.opts, services.Lyrics{Timing: "Word", Lines: duetLines()}, nil, 5_300)
			uitest.Golden(t, r.tt.Image(), "full-screen-lyrics-duet-"+mode.name)
		})
		t.Run(mode.name+"/none", func(t *testing.T) {
			r := goldenRig(t, mode.opts, services.Lyrics{}, lyrics.ErrNotFound, 2_000)
			uitest.Golden(t, r.tt.Image(), "full-screen-lyrics-none-"+mode.name)
		})
		t.Run(mode.name+"/gap", func(t *testing.T) {
			// The gap runs 15.6 s to 24.8 s: at 20 s (the view is drawn 0.6 s
			// after it opens) the first dot is full and the second half way.
			r := goldenRig(t, mode.opts, services.Lyrics{Timing: "Word", Lines: sketchGapLines()}, nil, 19_400)
			uitest.Golden(t, r.tt.Image(), "full-screen-lyrics-gap-"+mode.name)
		})
		t.Run(mode.name+"/strip", func(t *testing.T) {
			r := goldenRig(t, mode.opts, services.Lyrics{Timing: "Word", Lines: sketchLines()}, nil, 12_900)
			r.wake()
			r.tt.Until("the strip", r.tt.Shows("Exit full screen"))
			r.advance(300 * time.Millisecond)
			uitest.Golden(t, r.tt.Image(), "full-screen-lyrics-strip-"+mode.name)
		})
	}
}
