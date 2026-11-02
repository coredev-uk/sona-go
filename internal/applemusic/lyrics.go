package applemusic

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Lyrics is a song's TTML lyrics: the raw document plus a parsed model.
type Lyrics struct {
	TTML        string      `json:"ttml"`
	Timing      string      `json:"timing"` // itunes:timing: "Word", "Line" or "None"
	Language    string      `json:"language"`
	Songwriters []string    `json:"songwriters"`
	Lines       []LyricLine `json:"lines"`
	// Localizations are the translations and transliterations Apple sent
	// (extend=ttmlLocalizations), in document order.
	Localizations []LyricLocalization `json:"localizations,omitempty"`
}

// LyricLocalization is one <translation> or <transliteration> of the
// lyrics. Its lines are matched to Lyrics.Lines by key (itunes:key).
// Transliterations are word-timed like the original; translations carry
// text only.
type LyricLocalization struct {
	Kind     string               `json:"kind"`     // LocalizationTranslation or LocalizationTransliteration
	Language string               `json:"language"` // e.g. "en-US", "ko-Latn"
	Lines    map[string]LyricLine `json:"lines"`    // by LyricLine.Key
}

// LyricLocalization kinds.
const (
	LocalizationTranslation     = "translation"
	LocalizationTransliteration = "transliteration"
)

// Localization returns the first localization of kind, preferring one
// whose language starts with lang's primary subtag ("en" for "en-GB");
// nil when there is none.
func (l Lyrics) Localization(kind, lang string) *LyricLocalization {
	base, _, _ := strings.Cut(strings.ToLower(lang), "-")
	var found *LyricLocalization
	for i := range l.Localizations {
		loc := &l.Localizations[i]
		if loc.Kind != kind {
			continue
		}
		if base != "" && strings.HasPrefix(strings.ToLower(loc.Language), base) {
			return loc
		}
		if found == nil {
			found = loc
		}
	}
	return found
}

// LyricLine is one <p> (or one <text> of a localization). Text is the main
// vocal line; background vocals (ttm:role="x-bg") are only in Words,
// flagged Background, and in BackgroundText.
type LyricLine struct {
	Begin          time.Duration `json:"begin"`
	End            time.Duration `json:"end"`
	Key            string        `json:"key,omitempty"` // itunes:key ("L12"), links localized lines
	Text           string        `json:"text"`
	BackgroundText string        `json:"backgroundText,omitempty"`
	Part           string        `json:"part,omitempty"`  // itunes:songPart, e.g. "Verse"
	Agent          string        `json:"agent,omitempty"` // ttm:agent, e.g. "v1"; v2 marks a duet voice
	// AgentType is how the head declares Agent: "person", "group" (every
	// voice together) or "other"; "" when it isn't declared.
	AgentType string      `json:"agentType,omitempty"`
	Words     []LyricWord `json:"words,omitempty"` // word-timed lyrics only
}

// LyricWord is one timed <span> (a word or syllable). Text keeps a trailing
// space when one follows in the source, so concatenating the Texts of a
// line rebuilds it.
type LyricWord struct {
	Begin      time.Duration `json:"begin"`
	End        time.Duration `json:"end"`
	Text       string        `json:"text"`
	Background bool          `json:"background,omitempty"`
}

// ParseTTML parses Apple's lyrics TTML (line- or word-timed), including
// the translations and transliterations in its head.
func ParseTTML(ttml string) (Lyrics, error) {
	l := Lyrics{TTML: ttml}
	dec := xml.NewDecoder(strings.NewReader(ttml))
	var (
		part       string
		line       *LyricLine
		lineText   strings.Builder // text directly inside <p> or <text>
		bgText     strings.Builder // text inside an untimed background <span>
		spans      []spanFrame     // open <span> elements inside the line
		songwriter *strings.Builder
		loc        *LyricLocalization    // the open <translation>/<transliteration>
		agentTypes = map[string]string{} // ttm:agent declarations by xml:id
	)
	startLine := func(ln *LyricLine) {
		line = ln
		lineText.Reset()
		bgText.Reset()
		spans = spans[:0]
	}
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Lyrics{}, fmt.Errorf("lyrics ttml: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "tt":
				l.Timing = attr(t, "timing")
				l.Language = attr(t, "lang")
			case "agent":
				agentTypes[attr(t, "id")] = attr(t, "type")
			case "songwriter":
				songwriter = &strings.Builder{}
			case LocalizationTranslation, LocalizationTransliteration:
				loc = &LyricLocalization{Kind: t.Name.Local, Language: attr(t, "lang"), Lines: map[string]LyricLine{}}
			case "text":
				if loc != nil {
					startLine(&LyricLine{Key: attr(t, "for")})
				}
			case "div":
				part = attr(t, "songPart")
			case "p":
				begin, _ := parseClock(attr(t, "begin"))
				end, _ := parseClock(attr(t, "end"))
				agent := attr(t, "agent")
				startLine(&LyricLine{Begin: begin, End: end, Key: attr(t, "key"), Part: part, Agent: agent, AgentType: agentTypes[agent]})
			case "span":
				if line == nil {
					continue
				}
				bg := attr(t, "role") == "x-bg" || (len(spans) > 0 && spans[len(spans)-1].bg)
				f := spanFrame{bg: bg, word: -1}
				if b := attr(t, "begin"); b != "" {
					begin, _ := parseClock(b)
					end, _ := parseClock(attr(t, "end"))
					line.Words = append(line.Words, LyricWord{Begin: begin, End: end, Background: bg})
					f.word = len(line.Words) - 1
				}
				spans = append(spans, f)
			}
		case xml.EndElement:
			switch t.Name.Local {
			case "songwriter":
				if songwriter != nil {
					if s := strings.TrimSpace(songwriter.String()); s != "" {
						l.Songwriters = append(l.Songwriters, s)
					}
					songwriter = nil
				}
			case "span":
				if len(spans) > 0 {
					spans = spans[:len(spans)-1]
				}
			case "p":
				if line != nil {
					finishLine(line, lineText.String(), bgText.String())
					l.Lines = append(l.Lines, *line)
					line = nil
				}
			case "text":
				if line != nil && loc != nil {
					finishLine(line, lineText.String(), bgText.String())
					if line.Key != "" {
						loc.Lines[line.Key] = *line
					}
					line = nil
				}
			case LocalizationTranslation, LocalizationTransliteration:
				if loc != nil && len(loc.Lines) > 0 {
					l.Localizations = append(l.Localizations, *loc)
				}
				loc = nil
			case "div":
				part = ""
			}
		case xml.CharData:
			switch {
			case songwriter != nil:
				songwriter.Write(t)
			case line == nil:
			case len(spans) > 0 && spans[len(spans)-1].word >= 0:
				w := &line.Words[spans[len(spans)-1].word]
				w.Text += string(t)
			case len(line.Words) > 0:
				// Whitespace between spans separates words.
				if strings.TrimSpace(string(t)) == "" {
					w := &line.Words[len(line.Words)-1]
					if !strings.HasSuffix(w.Text, " ") {
						w.Text += " "
					}
				}
			case len(spans) > 0 && spans[len(spans)-1].bg:
				bgText.Write(t)
			default:
				lineText.Write(t)
			}
		}
	}
	return l, nil
}

type spanFrame struct {
	bg   bool
	word int // index into LyricLine.Words, -1 for an untimed span
}

// finishLine sets the line's texts: from its words when it is word-timed,
// else from the text directly inside it (direct) and inside untimed
// background spans (bg).
func finishLine(line *LyricLine, direct, bg string) {
	if len(line.Words) == 0 {
		line.Text = strings.Join(strings.Fields(direct), " ")
		line.BackgroundText = strings.Join(strings.Fields(bg), " ")
		return
	}
	var main, back strings.Builder
	for i := range line.Words {
		w := &line.Words[i]
		if w.Background {
			back.WriteString(w.Text)
		} else {
			main.WriteString(w.Text)
		}
	}
	// A trailing space on the last word of each voice is a separator, not text.
	line.Text = strings.TrimSpace(main.String())
	line.BackgroundText = strings.TrimSpace(back.String())
	if n := len(line.Words); n > 0 {
		line.Words[n-1].Text = strings.TrimRight(line.Words[n-1].Text, " ")
	}
}

func attr(e xml.StartElement, local string) string {
	for _, a := range e.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

// parseClock parses TTML clock values: "12.3", "1:02.345", "1:02:03.4",
// with an optional "s" or "ms" suffix.
func parseClock(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty time")
	}
	if v, ok := strings.CutSuffix(s, "ms"); ok {
		f, err := strconv.ParseFloat(v, 64)
		return time.Duration(f * float64(time.Millisecond)), err
	}
	s = strings.TrimSuffix(s, "s")
	var total float64
	for _, part := range strings.Split(s, ":") {
		f, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0, fmt.Errorf("bad time %q", s)
		}
		total = total*60 + f
	}
	return time.Duration(total*float64(time.Second) + 0.5), nil
}
