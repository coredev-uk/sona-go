package lyrics

import (
	"strings"
	"testing"
	"time"
)

const ms = time.Millisecond

func TestParseLRC(t *testing.T) {
	got := parseLRC("[ar:Test Artist]\n[offset:+500]\r\n[00:12.00][01:02.50]alpha bravo\n\n[00:05.1]charlie\nno time here\n[00:20.123]  \n")
	want := []lrcLine{
		{At: 4600 * ms, Text: "charlie"},
		{At: 11500 * ms, Text: "alpha bravo"},
		{At: 19623 * ms, Text: ""},
		{At: 62 * time.Second, Text: "alpha bravo"},
	}
	if len(got) != len(want) {
		t.Fatalf("lines = %+v", got)
	}
	for i := range want {
		if got[i].At != want[i].At || got[i].Text != want[i].Text {
			t.Errorf("line %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseEnhancedLRC(t *testing.T) {
	got := parseLRC("[00:01.00]v1: <00:01.00>al<00:01.30>pha <00:01.60>bravo<00:02.40>\n[00:03.00]charlie")
	if len(got) != 2 || got[0].Text != "alpha bravo" {
		t.Fatalf("lines = %+v", got)
	}
	w := got[0].Words
	if len(w) != 3 || w[0].Text != "al" || w[1].Text != "pha " || w[2].Text != "bravo" ||
		w[0].Begin != time.Second || w[0].End != 1300*ms || w[1].End != 1600*ms || w[2].End != 2400*ms {
		t.Fatalf("words = %+v", w)
	}
	lines, timing := timedLines(got, 0)
	// One plain line makes the whole result line-timed.
	if timing != TimingLine || lines[0].End != 2400*ms || lines[1].End != 8*time.Second {
		t.Fatalf("timed = %s %+v", timing, lines)
	}
}

func TestTimedLinesPauses(t *testing.T) {
	lines, timing := timedLines([]lrcLine{
		{At: time.Second, Text: "alpha"},
		{At: 4 * time.Second, Text: ""}, // the singer stops here
		{At: 9 * time.Second, Text: "bravo"},
	}, 30*time.Second)
	if timing != TimingLine || len(lines) != 2 || lines[0].End != 4*time.Second || lines[1].End != 30*time.Second {
		t.Fatalf("lines = %s %+v", timing, lines)
	}
	if l, timing := timedLines(nil, 0); l != nil || timing != TimingNone {
		t.Fatalf("empty = %v %s", l, timing)
	}
}

func TestPlainLines(t *testing.T) {
	l := plainLines("alpha\r\n\r\n  bravo  \n")
	if len(l) != 2 || l[0].Text != "alpha" || l[1].Text != "bravo" || l[1].Begin != 0 {
		t.Fatalf("lines = %+v", l)
	}
}

func TestRichsyncLines(t *testing.T) {
	l := richsyncLines(`[{"ts":10.5,"te":12.25,"l":[{"c":"alpha","o":0},{"c":" ","o":0.4},{"c":"bravo","o":0.5}],"x":"alpha bravo"},{"ts":13,"te":14,"l":[],"x":"  "}]`)
	if len(l) != 1 || l[0].Text != "alpha bravo" || l[0].Begin != 10500*ms || l[0].End != 12250*ms {
		t.Fatalf("lines = %+v", l)
	}
	w := l[0].Words
	if len(w) != 2 || w[0].Text != "alpha " || w[0].Begin != 10500*ms || w[0].End != 10900*ms ||
		w[1].Text != "bravo" || w[1].Begin != 11*time.Second || w[1].End != 12250*ms {
		t.Fatalf("words = %+v", w)
	}
	if richsyncLines("not json") != nil {
		t.Fatal("bad body parsed")
	}
}

func TestWriters(t *testing.T) {
	if w := writers("Writer(s): Ann Writer, Bo Writer\nCopyright: Test Publishing"); strings.Join(w, ";") != "Ann Writer;Bo Writer" {
		t.Fatalf("writers = %q", w)
	}
	if w := writers("Lyrics powered by www.musixmatch.com"); w != nil {
		t.Fatalf("writers = %q", w)
	}
}

func TestMatching(t *testing.T) {
	song := Track{Title: "Placeholder Song (feat. Guest)", Artist: "Test Artist & Guest", Album: "Test Album (Deluxe)", Duration: 200 * time.Second}
	for _, tc := range []struct {
		name string
		c    candidate
		ok   bool
	}{
		{"exact", candidate{Title: "Placeholder Song (feat. Guest)", Artist: "Test Artist & Guest", Duration: 200 * time.Second}, true},
		{"version and featured artist dropped", candidate{Title: "Placeholder Song - Remastered 2011", Artist: "Test Artist", Duration: 201 * time.Second}, true},
		{"diacritics and case", candidate{Title: "PLACEHOLDER SÖNG", Artist: "test artist", Duration: 199 * time.Second}, true},
		{"length unknown", candidate{Title: "Placeholder Song", Artist: "Guest"}, true},
		{"length at the tolerance", candidate{Title: "Placeholder Song", Artist: "Test Artist", Duration: 203 * time.Second}, true},
		{"length past the tolerance", candidate{Title: "Placeholder Song", Artist: "Test Artist", Duration: 203500 * ms}, false},
		{"another artist", candidate{Title: "Placeholder Song", Artist: "Someone Else", Duration: 200 * time.Second}, false},
		{"another title", candidate{Title: "Placeholder Song Two", Artist: "Test Artist", Duration: 200 * time.Second}, false},
	} {
		if got := score(song, tc.c) >= 0; got != tc.ok {
			t.Errorf("%s: match = %v, want %v", tc.name, got, tc.ok)
		}
	}
	// "Simon & Garfunkel"-style names match as a whole.
	if !artistMatches("Ann & Bo", "Ann and Bo") {
		t.Error("whole-name match failed")
	}
	// The same album and a closer length win.
	cs := []candidate{
		{Title: "Placeholder Song", Artist: "Test Artist", Album: "Live", Duration: 202 * time.Second},
		{Title: "Placeholder Song", Artist: "Test Artist", Album: "Test Album", Duration: 200500 * ms},
		{Title: "Placeholder Song", Artist: "Other", Album: "Test Album", Duration: 200 * time.Second},
	}
	if i := best(Track{Title: "Placeholder Song", Artist: "Test Artist", Album: "Test Album", Duration: 200 * time.Second}, cs); i != 1 {
		t.Fatalf("best = %d", i)
	}
}

func TestTrackKey(t *testing.T) {
	if k := (Track{CatalogID: "123", Title: "x"}).Key(); k != "am:123" {
		t.Fatalf("key = %q", k)
	}
	a := Track{Title: "Placeholder Song", Artist: "Test Artist", Duration: 200 * time.Second}
	b := Track{Title: "placeholder  song", Artist: "TEST ARTIST", Duration: 200400 * ms}
	c := Track{Title: "Placeholder Song", Artist: "Test Artist", Duration: 201 * time.Second}
	if a.Key() != b.Key() || a.Key() == c.Key() || !strings.HasPrefix(a.Key(), "meta:") {
		t.Fatalf("keys = %q %q %q", a.Key(), b.Key(), c.Key())
	}
}
