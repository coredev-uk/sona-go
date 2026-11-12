package lyricsync

import (
	"reflect"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

func sp(b, e int64) Span { return Span{b, e} }

func begins(bs ...int64) []Span {
	out := make([]Span, len(bs))
	for i, b := range bs {
		out[i] = Span{BeginMs: b}
	}
	return out
}

func TestActiveLine(t *testing.T) {
	lines := begins(1000, 4000, 4000, 9500)
	for _, c := range []struct {
		at   int64
		want int
	}{{0, -1}, {999, -1}, {1000, 0}, {3999, 0}, {4000, 2}, {60000, 3}} {
		if got := ActiveLine(lines, c.at); got != c.want {
			t.Errorf("ActiveLine(%d) = %d, want %d", c.at, got, c.want)
		}
	}
	if got := ActiveLine(nil, 5000); got != -1 {
		t.Errorf("no lines: %d", got)
	}
}

func TestNudgeOffsetMovesInHalfSecondStepsAndStopsAtTheLimit(t *testing.T) {
	for _, c := range []struct {
		from int64
		dir  int
		want int64
	}{{0, 1, 500}, {500, -1, 0}, {0, -1, -500}, {740, 1, 1000}} {
		if got := NudgeOffset(c.from, c.dir); got != c.want {
			t.Errorf("NudgeOffset(%d, %d) = %d, want %d", c.from, c.dir, got, c.want)
		}
	}
	o := int64(0)
	for range 100 {
		o = NudgeOffset(o, 1)
	}
	if o != MaxOffsetMs {
		t.Errorf("up: %d", o)
	}
	for range 100 {
		o = NudgeOffset(o, -1)
	}
	if o != -MaxOffsetMs {
		t.Errorf("down: %d", o)
	}
}

func TestOffsetShiftsWhichLineIsSungAndWhereAClickSeeks(t *testing.T) {
	lines := begins(10_000, 12_000)
	at := func(pos, off int64) int { return ActiveLine(lines, LyricsClock(pos, off)) }
	if at(9600, 0) != -1 || at(9750, 0) != 0 {
		t.Error("the 250 ms lead shows the first line from 9.75 s")
	}
	if at(9300, -500) != 0 {
		t.Error("lyrics earlier")
	}
	if at(12_000, 1000) != 0 || at(12_750, 1000) != 1 {
		t.Error("lyrics later")
	}
	if WordClock(10_000, 0) != 10_000 || WordClock(10_000, 500) != 9500 || WordClock(10_000, -500) != 10_500 {
		t.Error("the words' clock has no lead")
	}
	for _, off := range []int64{0, 500, -1500} {
		if got := at(SeekTarget(12_000, off), off); got != 1 {
			t.Errorf("a click under offset %d lands on line %d", off, got)
		}
	}
	if SeekTarget(200, -1000) != 0 {
		t.Error("a seek never goes below 0")
	}
}

func TestSessionOffsetsAreKeptPerSong(t *testing.T) {
	o := &Offsets{}
	changed := 0
	stop := o.OnChange(func() { changed++ })
	o.Nudge("a", 1)
	o.Nudge("a", 1)
	o.Nudge("b", -1)
	if o.Get("a") != 1000 || o.Get("b") != -500 || o.Get("c") != 0 {
		t.Errorf("offsets %d %d %d", o.Get("a"), o.Get("b"), o.Get("c"))
	}
	o.Reset("a")
	if o.Get("a") != 0 || o.Get("b") != -500 {
		t.Error("reset touches only its song")
	}
	stop()
	o.Nudge("a", 1)
	if changed != 4 {
		t.Errorf("subscriber called %d times, want 4", changed)
	}
}

func word(b, e int64, bg bool) services.LyricWord {
	return services.LyricWord{BeginMs: b, EndMs: e, Text: "lorem ", Background: bg}
}

func TestLineSpan(t *testing.T) {
	for _, c := range []struct {
		name string
		l    services.LyricLine
		want Span
	}{
		{"last word", services.LyricLine{BeginMs: 1000, EndMs: 3000, Words: []services.LyricWord{word(1000, 2000, false), word(2000, 3000, false)}}, sp(1000, 3000)},
		{"background vocals run on", services.LyricLine{BeginMs: 1000, EndMs: 3000, Words: []services.LyricWord{word(1000, 3000, false), word(2500, 4200, true)}}, sp(1000, 4200)},
		{"an earlier word", services.LyricLine{BeginMs: 1000, EndMs: 2000, Words: []services.LyricWord{word(900, 2000, false)}}, sp(900, 2000)},
		{"no words", services.LyricLine{BeginMs: 5, EndMs: 9}, sp(5, 9)},
	} {
		if got := LineSpan(&c.l); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSingingLines(t *testing.T) {
	// Line 1's background vocals run until 4.5 s, under line 2.
	spans := []Span{sp(0, 2000), sp(2000, 4500), sp(4000, 6000), sp(8000, 9000)}
	for _, c := range []struct {
		at   int64
		want []int
	}{
		{-1, nil},
		{0, []int{0}},
		{1999, []int{0}},
		{2000, []int{1}},
		{4000, []int{1, 2}},
		{4499, []int{1, 2}},
		{4500, []int{2}},
		{7000, []int{2}},
		{8000, []int{3}},
		{9499, []int{3}},
		{9500, nil},
		{8500, []int{3}},
		{500, []int{0}},
		{4200, []int{1, 2}},
	} {
		if got := SingingLines(spans, c.at); !reflect.DeepEqual(got, c.want) {
			t.Errorf("SingingLines(%d) = %v, want %v", c.at, got, c.want)
		}
	}
	if SingingLines(nil, 100) != nil {
		t.Error("no lines")
	}
	brk := []Span{sp(0, 2000), sp(5000, 6000)}
	for _, c := range []struct {
		at   int64
		want []int
	}{{2499, []int{0}}, {2500, nil}, {5000, []int{1}}} {
		if got := SingingLines(brk, c.at); !reflect.DeepEqual(got, c.want) {
			t.Errorf("break: SingingLines(%d) = %v, want %v", c.at, got, c.want)
		}
	}
}

func TestNextChange(t *testing.T) {
	spans := []Span{sp(0, 2000), sp(5000, 6000)}
	words := []Span{sp(0, 800), sp(1000, 2000)}
	for _, c := range []struct {
		singing []int
		words   []Span
		at      int64
		want    int64
	}{
		{[]int{0}, words, 100, 800},
		{[]int{0}, words, 800, 1000},
		{[]int{0}, words, 1000, 2000},
		{[]int{0}, words[:1], 2500, 5000},
		{[]int{1}, []Span{sp(5000, 6000)}, 7000, Never},
		{[]int{0}, words[:1], 2100, 2500},
	} {
		if got := NextChange(spans, c.singing, c.words, c.at); got != c.want {
			t.Errorf("NextChange at %d = %d, want %d", c.at, got, c.want)
		}
	}
}

func TestRestFrom(t *testing.T) {
	spans := []Span{sp(0, 2000), sp(4999, 6000), sp(9000, 10000)}
	if RestFrom(spans, 0) != Never || RestFrom(spans, 1) != 6500 || RestFrom(spans, 2) != 10500 {
		t.Errorf("%d %d %d", RestFrom(spans, 0), RestFrom(spans, 1), RestFrom(spans, 2))
	}
	if RestFrom([]Span{sp(1000, 0)}, 0) != Never {
		t.Error("a line without an end never rests")
	}
}

func TestSungLine(t *testing.T) {
	spans := []Span{sp(1000, 2000), sp(2100, 3000), sp(8000, 9000)}
	for _, c := range []struct {
		at   int64
		want Sheet
	}{
		{0, Sheet{-1, 0, 1000}},
		{1500, Sheet{0, 0, 2100}},
		{3400, Sheet{1, 1, 3500}},
		{3500, Sheet{-1, 2, 8000}},
		{8000, Sheet{2, 2, 9500}},
		{9500, Sheet{-1, 2, Never}},
	} {
		if got := SungLine(spans, c.at); got != c.want {
			t.Errorf("SungLine(%d) = %v, want %v", c.at, got, c.want)
		}
	}
	if got := SungLine(nil, 0); got != (Sheet{-1, -1, Never}) {
		t.Errorf("no lines: %v", got)
	}
}

func TestVerseStarts(t *testing.T) {
	spans := []Span{sp(0, 1000), sp(1200, 2000), sp(5000, 6000), sp(6100, 7000)}
	line := func(part string) services.LyricLine { return services.LyricLine{Part: part} }
	got := VerseStarts([]services.LyricLine{line(""), line(""), line(""), line("")}, spans)
	if !reflect.DeepEqual(got, []bool{false, false, true, false}) {
		t.Errorf("breaks: %v", got)
	}
	got = VerseStarts([]services.LyricLine{line("Verse"), line("Chorus"), line("Chorus"), line("Chorus")}, spans)
	if !reflect.DeepEqual(got, []bool{false, true, true, false}) {
		t.Errorf("parts: %v", got)
	}
	got = VerseStarts([]services.LyricLine{line(""), line("")}, []Span{sp(0, 0), sp(9000, 0)})
	if !reflect.DeepEqual(got, []bool{false, false}) {
		t.Errorf("no ends: %v", got)
	}
}

func TestVersesGroupLinesAndPlainLyricsAreOneVerse(t *testing.T) {
	lines := make([]services.LyricLine, 4)
	spans := []Span{sp(0, 1000), sp(1200, 2000), sp(5000, 6000), sp(6100, 7000)}
	if got := Verses(lines, spans, true); !reflect.DeepEqual(got, [][]int{{0, 1}, {2, 3}}) {
		t.Errorf("synced: %v", got)
	}
	if got := Verses(lines, spans, false); !reflect.DeepEqual(got, [][]int{{0, 1, 2, 3}}) {
		t.Errorf("plain: %v", got)
	}
	if got := Verses(nil, nil, true); len(got) != 0 {
		t.Errorf("none: %v", got)
	}
}

func TestLineVoices(t *testing.T) {
	l := func(agent, typ string) services.LyricLine { return services.LyricLine{Agent: agent, AgentType: typ} }
	for _, c := range []struct {
		name  string
		lines []services.LyricLine
		want  []Voice
	}{
		{"duet", []services.LyricLine{l("v1", "person"), l("v2", "other"), l("v1000", "group"), l("v1", "person")}, []Voice{VoiceOne, VoiceTwo, VoiceAll, VoiceOne}},
		{"first to sing leads", []services.LyricLine{l("v2", "person"), l("v1", "person"), l("v3", "person")}, []Voice{VoiceOne, VoiceTwo, VoiceTwo}},
		{"one voice", []services.LyricLine{l("v1", "person"), l("", "person"), l("v1", "person")}, []Voice{VoiceOne, VoiceOne, VoiceOne}},
		{"group and lead", []services.LyricLine{l("v1000", "group"), l("v1", "person")}, []Voice{VoiceAll, VoiceOne}},
	} {
		if got := LineVoices(c.lines); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	if len(LineVoices(nil)) != 0 {
		t.Error("no lines")
	}
}

func TestHasWordTiming(t *testing.T) {
	withWords := &services.Lyrics{Timing: "Word", Lines: []services.LyricLine{{Words: []services.LyricWord{word(0, 1, false)}}}}
	for name, c := range map[string]struct {
		l    *services.Lyrics
		want bool
	}{
		"nil":         {nil, false},
		"line timing": {&services.Lyrics{Timing: "Line", Lines: []services.LyricLine{{}}}, false},
		"no words":    {&services.Lyrics{Timing: "Word", Lines: []services.LyricLine{{}}}, false},
		"word timing": {withWords, true},
	} {
		if got := HasWordTiming(c.l); got != c.want {
			t.Errorf("%s: %v", name, got)
		}
	}
}

func TestIsSyncedAndExtras(t *testing.T) {
	if IsSynced(nil) || IsSynced(&services.Lyrics{Timing: "None"}) || !IsSynced(&services.Lyrics{Timing: "Line"}) || !IsSynced(&services.Lyrics{Timing: "Word"}) {
		t.Error("only Line and Word are synced")
	}
	if tr, pr := Extras(nil); tr || pr {
		t.Error("nil")
	}
	if tr, pr := Extras(&services.Lyrics{TranslationLanguage: "en-US"}); !tr || pr {
		t.Error("translation only")
	}
}

func TestShown(t *testing.T) {
	w := func(text string, bg bool) services.LyricWord {
		return services.LyricWord{EndMs: 1, Text: text, Background: bg}
	}
	line := services.LyricLine{
		EndMs: 1, Text: "lorem ipsum", Background: "(dolor)", Agent: "v1", AgentType: "person",
		Words:           []services.LyricWord{w("lorem ", false), w("ipsum", false), w("(dolor)", true)},
		Translation:     &services.LyricText{Text: "alpha bravo", Background: "(charlie)"},
		Transliteration: &services.LyricText{Text: "lo rem ip sum", Background: "(do lor)", Words: []services.LyricWord{w("lo ", false), w("rem ", false)}},
	}
	s := Shown(&line, Prefs{})
	if s.Lead.Text != "lorem ipsum" || len(s.Lead.Words) != 2 || s.Background.Text != "(dolor)" || len(s.Background.Words) != 1 || s.Pronunciation != nil || s.Translation != "" {
		t.Errorf("plain: %+v", s)
	}
	s = Shown(&line, Prefs{Translation: true, Pronunciation: true})
	if s.Pronunciation == nil || s.Pronunciation.Text != "lo rem ip sum (do lor)" || len(s.Pronunciation.Words) != 2 || s.Translation != "alpha bravo (charlie)" {
		t.Errorf("localized: %+v", s)
	}
	bare := line
	bare.Translation, bare.Transliteration = nil, nil
	s = Shown(&bare, Prefs{Translation: true, Pronunciation: true})
	if s.Pronunciation != nil || s.Translation != "" {
		t.Errorf("bare: %+v", s)
	}
	plain := bare
	plain.Words = nil
	s = Shown(&plain, Prefs{})
	if s.Lead.Text != "lorem ipsum" || len(s.Lead.Words) != 0 || s.Background.Text != "(dolor)" {
		t.Errorf("line-timed: %+v", s)
	}
}
