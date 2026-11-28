package lyrics

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// The fuzz targets below feed the lyrics parsers what a provider can send:
// Apple's TTML, LRC text, and the JSON of LRCLIB, Musixmatch and NetEase.
// The seeds are in testdata/fuzz/<target> (the scrubbed fixtures of
// testdata and small made-up documents); go test runs them as ordinary tests.

// checkLyrics holds what every parser promises about the lyrics it returns,
// whatever the input: a known timing, no negative or absurd time, and output
// that is bounded by the input (a line needs at least minBytes of input).
func checkLyrics(t *testing.T, l Lyrics, inputLen, minBytes int) {
	t.Helper()
	switch l.Timing {
	case TimingWord, TimingLine, TimingNone:
	default:
		if len(l.Lines) > 0 {
			t.Fatalf("timing %q", l.Timing)
		}
	}
	if len(l.Lines) > inputLen/minBytes+1 {
		t.Fatalf("%d lines from %d bytes", len(l.Lines), inputLen)
	}
	// endLines may add a few seconds to a time at the limit.
	okTime := func(d time.Duration) bool { return d >= 0 && d <= maxTime+time.Minute }
	words := func(ws []Word) {
		if len(ws) > inputLen/minBytes+1 {
			t.Fatalf("%d words from %d bytes", len(ws), inputLen)
		}
		for _, w := range ws {
			if !okTime(w.Begin) || !okTime(w.End) {
				t.Fatalf("word %+v has a time out of range", w)
			}
		}
	}
	for _, ln := range l.Lines {
		if !okTime(ln.Begin) || !okTime(ln.End) {
			t.Fatalf("line %+v has a time out of range", ln)
		}
		words(ln.Words)
		for _, loc := range []*Text{ln.Translation, ln.Transliteration} {
			if loc != nil {
				words(loc.Words)
			}
		}
	}
}

// FuzzParseLRC feeds parseLRC, timedLines and plainLines arbitrary text.
//
// Invariants: no panic; lines come in time order; no time is negative or
// beyond a day; a line needs a "[m:s]" stamp (5 bytes), so lines and words
// are bounded by the input's length; timed lyrics that claim word timing
// have words on every line.
func FuzzParseLRC(f *testing.F) {
	f.Fuzz(func(t *testing.T, s string) {
		in := parseLRC(s)
		for i, l := range in {
			if i > 0 && l.At < in[i-1].At {
				t.Fatalf("line %d at %v after %v", i, l.At, in[i-1].At)
			}
		}
		lines, timing := timedLines(in, 3*time.Minute)
		checkLyrics(t, Lyrics{Timing: timing, Lines: lines}, len(s), 5)
		if timing == TimingWord {
			for _, ln := range lines {
				if len(ln.Words) == 0 {
					t.Fatalf("word timing, line %+v has no words", ln)
				}
			}
		}
		checkLyrics(t, Lyrics{Timing: TimingNone, Lines: plainLines(s)}, len(s), 1)
	})
}

// FuzzFromAppleTTML parses Apple's lyrics TTML and maps it for the UI.
//
// Invariants: no panic; the mapped lyrics have a known timing and no
// negative or absurd time; lines and words are bounded by the document's
// size; the document is kept as it came; and a TTML that parses maps to as
// many lines as it has.
func FuzzFromAppleTTML(f *testing.F) {
	f.Fuzz(func(t *testing.T, ttml string) {
		l, err := applemusic.ParseTTML(ttml)
		if err != nil {
			return
		}
		if l.TTML != ttml {
			t.Fatal("ParseTTML dropped the document")
		}
		out := fromApple(l, "en")
		if len(out.Lines) != len(l.Lines) {
			t.Fatalf("%d lines mapped to %d", len(l.Lines), len(out.Lines))
		}
		switch out.Timing {
		case TimingWord, TimingLine, TimingNone:
		default:
			t.Fatalf("timing %q", out.Timing)
		}
		// A <p> is three bytes at least.
		checkLyrics(t, out, len(ttml), 3)
	})
}

// FuzzLRCLIBRecord decodes an LRCLIB entry (get and search answers share it)
// and builds lyrics of it.
//
// Invariants: no panic; a nil error means lyrics with some text; the lines
// are bounded by the synced or plain text they come from.
func FuzzLRCLIBRecord(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		var rec lrclibRecord
		if json.Unmarshal(b, &rec) != nil {
			return
		}
		_ = rec.candidate()
		l, err := lrclibLyrics(rec, time.Duration(rec.Duration*float64(time.Second)))
		if err != nil {
			return
		}
		if l.empty() {
			t.Fatal("lyrics without text and no error")
		}
		checkLyrics(t, l, len(b), 1)
	})
}

// FuzzNetEaseLyric decodes NetEase's lyric answer and builds lyrics of it,
// translations included.
//
// Invariants: no panic; a nil error means lyrics with some text, a known
// timing and no credit line left in; no negative or absurd time; output
// bounded by the input.
func FuzzNetEaseLyric(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		var l neteaseLyric
		if json.Unmarshal(b, &l) != nil {
			return
		}
		for _, lang := range []string{"", "zh-Hans", "en"} {
			out, err := neteaseLyrics(l, lang, 3*time.Minute)
			if err != nil {
				continue
			}
			if out.empty() {
				t.Fatal("lyrics without text and no error")
			}
			checkLyrics(t, out, len(b), 5)
		}
	})
}

// FuzzMusixmatchMacro decodes Musixmatch's macro answer the way Fetch does
// (the envelope, the track, then richsync, subtitles and lyrics bodies) and
// parses the bodies.
//
// Invariants: no panic; every line has a known timing, no negative or absurd
// time, and the lines are bounded by the input.
func FuzzMusixmatchMacro(f *testing.F) {
	f.Fuzz(func(t *testing.T, b []byte) {
		var macro struct {
			Calls map[string]mxmEnvelope `json:"macro_calls"`
		}
		var top mxmEnvelope
		if json.Unmarshal(b, &top) != nil {
			return
		}
		if top.body(&macro) != nil {
			return
		}
		var rs mxmRichsync
		if c, ok := macro.Calls["track.richsync.get"]; ok && c.body(&rs) == nil {
			if lines := richsyncLines(rs.Richsync.Body); len(lines) > 0 {
				checkLyrics(t, Lyrics{Timing: TimingWord, Lines: lines}, len(b), 1)
			}
			_ = writers(rs.Richsync.Copyright)
		}
		var subs mxmSubtitles
		if c, ok := macro.Calls["track.subtitles.get"]; ok && c.body(&subs) == nil && len(subs.List) > 0 {
			lines, timing := subtitleLines(subs.List[0].Subtitle.Body, 3*time.Minute)
			checkLyrics(t, Lyrics{Timing: timing, Lines: lines}, len(b), 1)
		}
		var ly mxmLyrics
		if c, ok := macro.Calls["track.lyrics.get"]; ok && c.body(&ly) == nil {
			checkLyrics(t, Lyrics{Timing: TimingNone, Lines: plainLines(stripMxmFooter(ly.Lyrics.Body))}, len(b), 1)
		}
		var tr mxmTranslations
		if c, ok := macro.Calls["crowd.track.translations.get"]; ok {
			_ = c.body(&tr)
		}
	})
}

// The stamps of one enhanced line share its words, which the offset must
// shift once, not once per stamp.
func TestParseLRCShiftsSharedWordsOnce(t *testing.T) {
	got := parseLRC("[offset:500]\n[00:10.00][00:20.00]<00:10.00>one <00:11.00>two <00:12.00>")
	if len(got) != 2 {
		t.Fatalf("lines = %+v", got)
	}
	if b := got[0].Words[0].Begin; b != 9500*time.Millisecond {
		t.Errorf("first word begins at %v, want 9.5s", b)
	}
}

// An offset or a time too large for a Duration is clamped, not wrapped.
func TestLyricTimesAreClamped(t *testing.T) {
	got := parseLRC("[offset:9223372036854775807]\n[00:10.00]x\n[offset:-9223372036854775808]\n")
	if len(got) != 1 || got[0].At != maxTime {
		t.Errorf("lines = %+v", got)
	}
	for _, s := range []float64{1e300, -1, 1e19} {
		if d := seconds(s); d < 0 || d > maxTime {
			t.Errorf("seconds(%v) = %v", s, d)
		}
	}
}
