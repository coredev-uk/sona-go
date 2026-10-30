package applemusic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const wordTTML = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:itunes="http://music.apple.com/lyric-ttml-internal" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" itunes:timing="Word" xml:lang="en"><head><metadata><ttm:agent type="person" xml:id="v1"/><iTunesMetadata xmlns="http://music.apple.com/lyric-ttml-internal"><songwriters><songwriter>Ann Writer</songwriter><songwriter>Bo Writer</songwriter></songwriters></iTunesMetadata></metadata></head><body dur="3:21.570"><div begin="27.395" end="48.621" itunes:songPart="Verse"><p begin="27.395" end="28.960" itunes:key="L1" ttm:agent="v1"><span begin="27.395" end="27.549">Sing</span> <span begin="27.549" end="27.740">a</span> <span begin="27.740" end="28.077">lo</span><span begin="28.077" end="28.960">ng</span></p><p begin="1:02.000" end="1:05.5" ttm:agent="v2"><span begin="1:02.000" end="1:03.000">Echo</span> <span ttm:role="x-bg"><span begin="1:03.100" end="1:04.000">(back</span> <span begin="1:04.000" end="1:05.500">up)</span></span></p></div><div begin="1:10" end="1:20" itunes:songPart="Chorus"><p begin="1:10" end="1:12"><span begin="1:10" end="1:12">Hey</span></p></div></body></tt>`

const lineTTML = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:itunes="http://music.apple.com/lyric-ttml-internal" itunes:timing="Line" xml:lang="en-GB"><head><metadata/></head><body><div itunes:songPart="Verse"><p begin="8.481" end="11.6">First line here</p><p begin="1:00:01.25" end="1:00:03">Second &amp; last</p></div></body></tt>`

func TestParseTTMLWordTimed(t *testing.T) {
	l, err := ParseTTML(wordTTML)
	if err != nil {
		t.Fatal(err)
	}
	if l.Timing != "Word" || l.Language != "en" || strings.Join(l.Songwriters, ";") != "Ann Writer;Bo Writer" {
		t.Fatalf("meta = %q %q %v", l.Timing, l.Language, l.Songwriters)
	}
	if len(l.Lines) != 3 {
		t.Fatalf("lines = %d", len(l.Lines))
	}
	l1 := l.Lines[0]
	if l1.Text != "Sing a long" || l1.Part != "Verse" || l1.Agent != "v1" ||
		l1.Begin != 27395*time.Millisecond || l1.End != 28960*time.Millisecond {
		t.Fatalf("line 1 = %+v", l1)
	}
	var words []string
	for _, w := range l1.Words {
		words = append(words, w.Text)
	}
	// "lo"+"ng" are syllables of one word: no space between them.
	if strings.Join(words, "|") != "Sing |a |lo|ng" || l1.Words[2].Begin != 27740*time.Millisecond {
		t.Fatalf("words = %q", words)
	}
	l2 := l.Lines[1]
	if l2.Text != "Echo" || l2.BackgroundText != "(back up)" || l2.Agent != "v2" || l2.Begin != 62*time.Second {
		t.Fatalf("line 2 = %+v", l2)
	}
	if len(l2.Words) != 3 || l2.Words[0].Background || !l2.Words[1].Background || !l2.Words[2].Background {
		t.Fatalf("line 2 words = %+v", l2.Words)
	}
	if l.Lines[2].Part != "Chorus" || l.Lines[2].Begin != 70*time.Second {
		t.Fatalf("line 3 = %+v", l.Lines[2])
	}
	if l.TTML != wordTTML {
		t.Fatal("raw TTML not kept")
	}
}

func TestParseTTMLLineTimed(t *testing.T) {
	l, err := ParseTTML(lineTTML)
	if err != nil {
		t.Fatal(err)
	}
	if l.Timing != "Line" || l.Language != "en-GB" || len(l.Lines) != 2 {
		t.Fatalf("%+v", l)
	}
	if l.Lines[0].Text != "First line here" || l.Lines[0].End != 11600*time.Millisecond || len(l.Lines[0].Words) != 0 {
		t.Fatalf("line 1 = %+v", l.Lines[0])
	}
	if l.Lines[1].Text != "Second & last" || l.Lines[1].Begin != time.Hour+1250*time.Millisecond {
		t.Fatalf("line 2 = %+v", l.Lines[1])
	}
}

func TestParseTTMLInvalid(t *testing.T) {
	if _, err := ParseTTML("<tt><p>"); err == nil {
		t.Fatal("want error for truncated TTML")
	}
}

func TestLyricsFallsBackToLineTimed(t *testing.T) {
	var paths []string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if strings.HasSuffix(r.URL.Path, "/syllable-lyrics") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Write([]byte(`{"data":[{"id":"1","type":"lyrics","attributes":{"ttml":` + jsonString(lineTTML) + `}}]}`))
	}))
	l, err := lyricsAt(c, "1", true)
	if err != nil || len(l.Lines) != 2 {
		t.Fatalf("lyrics = %+v, %v", l, err)
	}
	if strings.Join(paths, " ") != "/v1/catalog/gb/songs/1/syllable-lyrics /v1/catalog/gb/songs/1/lyrics" {
		t.Fatalf("paths = %v", paths)
	}
}

func TestLyricsNone(t *testing.T) {
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":[]}`))
	}))
	if _, err := lyricsAt(c, "1", false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

// lyricsAt calls Lyrics with the AMP host pointed at the test server.
func lyricsAt(c *Client, id string, syllable bool) (Lyrics, error) {
	c.AMPBase = c.Base
	return c.Lyrics(context.Background(), "gb", id, LyricsOptions{Syllable: syllable})
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// localizedTTML is trimmed from a live syllable-lyrics response with
// extend=ttmlLocalizations (Korean lyrics, English translation, Latin
// transliteration).
const localizedTTML = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:itunes="http://music.apple.com/lyric-ttml-internal" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" itunes:timing="Word" xml:lang="ko"><head><metadata><ttm:agent type="person" xml:id="v1"><ttm:name type="full">방탄소년단</ttm:name></ttm:agent><iTunesMetadata xmlns="http://music.apple.com/lyric-ttml-internal">` +
	`<translations><translation type="subtitle" xml:lang="en-US"><text for="L2">I miss you <span xmlns:ttm="http://www.w3.org/ns/ttml#metadata" ttm:role="x-bg" xmlns="http://www.w3.org/ns/ttml">(I miss you)</span></text><text for="L1">I miss you</text></translation></translations>` +
	`<transliterations><transliteration xml:lang="ko-Latn"><text for="L1"><span begin="17.105" end="17.346" xmlns="http://www.w3.org/ns/ttml">bo</span> <span begin="17.346" end="18.566" xmlns="http://www.w3.org/ns/ttml">go</span></text>` +
	`<text for="L2"><span begin="3:47.401" end="3:48.157" xmlns="http://www.w3.org/ns/ttml">bo go</span>  <span xmlns:ttm="http://www.w3.org/ns/ttml#metadata" ttm:role="x-bg" xmlns="http://www.w3.org/ns/ttml"><span begin="3:49.576" end="3:50.408">(bo go)</span></span></text></transliteration></transliterations>` +
	`<songwriters><songwriter>RM</songwriter></songwriters></iTunesMetadata></metadata></head>` +
	`<body dur="4:34.097"><div begin="17.105" end="35.081" itunes:songPart="Verse"><p begin="17.105" end="18.566" itunes:key="L1" ttm:agent="v1"><span begin="17.105" end="17.346">보</span><span begin="17.346" end="18.566">고</span></p>` +
	`<p begin="3:47.401" end="3:51.863" itunes:key="L2" ttm:agent="v1"><span begin="3:47.401" end="3:48.157">보고</span> <span ttm:role="x-bg"><span begin="3:49.576" end="3:50.408">(보고)</span></span></p></div></body></tt>`

func TestParseTTMLLocalizations(t *testing.T) {
	l, err := ParseTTML(localizedTTML)
	if err != nil {
		t.Fatal(err)
	}
	if l.Language != "ko" || len(l.Lines) != 2 || strings.Join(l.Songwriters, ";") != "RM" {
		t.Fatalf("lyrics = %+v", l)
	}
	if l.Lines[0].Key != "L1" || l.Lines[0].Text != "보고" || l.Lines[1].Key != "L2" || l.Lines[1].BackgroundText != "(보고)" {
		t.Fatalf("lines = %+v", l.Lines)
	}
	if len(l.Localizations) != 2 {
		t.Fatalf("localizations = %+v", l.Localizations)
	}
	tr := l.Localization(LocalizationTranslation, "en-GB")
	if tr == nil || tr.Language != "en-US" || len(tr.Lines) != 2 {
		t.Fatalf("translation = %+v", tr)
	}
	// Untimed background vocals in a translation are kept apart.
	if ln := tr.Lines["L2"]; ln.Text != "I miss you" || ln.BackgroundText != "(I miss you)" || len(ln.Words) != 0 {
		t.Fatalf("translated L2 = %+v", ln)
	}
	tl := l.Localization(LocalizationTransliteration, "en-GB")
	if tl == nil || tl.Language != "ko-Latn" {
		t.Fatalf("transliteration = %+v", tl)
	}
	l1 := tl.Lines["L1"]
	if l1.Text != "bo go" || len(l1.Words) != 2 || l1.Words[0].Text != "bo " || l1.Words[1].Begin != 17346*time.Millisecond {
		t.Fatalf("transliterated L1 = %+v", l1)
	}
	l2 := tl.Lines["L2"]
	if l2.Text != "bo go" || l2.BackgroundText != "(bo go)" || len(l2.Words) != 2 || !l2.Words[1].Background {
		t.Fatalf("transliterated L2 = %+v", l2)
	}
	if l.Localization("translation", "") == nil || l.Localization("nope", "en") != nil {
		t.Fatal("Localization lookup")
	}
	// Plain lyrics have none.
	if p, _ := ParseTTML(lineTTML); len(p.Localizations) != 0 || p.Localization(LocalizationTranslation, "en") != nil {
		t.Fatalf("line lyrics localizations = %+v", p.Localizations)
	}
}

func TestLyricsRequestsLocalizations(t *testing.T) {
	var queries []string
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		// With extend=ttmlLocalizations Apple sends ttmlLocalizations instead of ttml.
		w.Write([]byte(`{"data":[{"id":"1","type":"syllable-lyrics","attributes":{"ttmlLocalizations":` + jsonString(localizedTTML) + `}}]}`))
	}))
	c.AMPBase = c.Base
	l, err := c.Lyrics(context.Background(), "gb", "1", LyricsOptions{Syllable: true, Language: "en-GB"})
	if err != nil || len(l.Localizations) != 2 {
		t.Fatalf("lyrics = %+v, %v", l.Localizations, err)
	}
	want := "/v1/catalog/gb/songs/1/syllable-lyrics?extend=ttmlLocalizations&l%5Blyrics%5D=en-gb&l%5Bscript%5D=en-Latn"
	if len(queries) != 1 || queries[0] != want {
		t.Fatalf("queries = %v\nwant %s", queries, want)
	}
}
