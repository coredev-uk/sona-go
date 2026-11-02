package lyrics

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// lrcLine is one timed line of an LRC file.
type lrcLine struct {
	At    time.Duration
	Text  string
	Words []Word // enhanced LRC (<mm:ss.xx> word tags) only
}

var (
	reLRCTime  = regexp.MustCompile(`^\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)
	reLRCTag   = regexp.MustCompile(`^\[([a-zA-Z#]+):([^\]]*)\]\s*$`)
	reLRCWord  = regexp.MustCompile(`<(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?>`)
	reLRCVoice = regexp.MustCompile(`^(v\d+|bg):\s*`)
)

// parseLRC parses LRC lyrics, including lines with several timestamps,
// the [offset:] tag and enhanced LRC word times, in time order. Lines
// without a timestamp are dropped.
func parseLRC(s string) []lrcLine {
	var out []lrcLine
	var offset time.Duration
	for _, raw := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		raw = strings.TrimSpace(raw)
		if m := reLRCTag.FindStringSubmatch(raw); m != nil {
			if strings.EqualFold(m[1], "offset") {
				// A positive offset shows the lyrics earlier.
				if ms, err := strconv.Atoi(strings.TrimSpace(m[2])); err == nil {
					offset = time.Duration(ms) * time.Millisecond
				}
			}
			continue
		}
		var stamps []time.Duration
		for {
			m := reLRCTime.FindStringSubmatch(raw)
			if m == nil {
				break
			}
			stamps = append(stamps, clock(m[1], m[2], m[3]))
			raw = raw[len(m[0]):]
		}
		if len(stamps) == 0 {
			continue
		}
		text, words := lrcWords(raw)
		for _, at := range stamps {
			out = append(out, lrcLine{At: at, Text: text, Words: words})
		}
	}
	for i := range out {
		out[i].At = max(out[i].At-offset, 0)
		for j := range out[i].Words {
			out[i].Words[j].Begin = max(out[i].Words[j].Begin-offset, 0)
			out[i].Words[j].End = max(out[i].Words[j].End-offset, 0)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out
}

// lrcWords splits an enhanced LRC line ("<00:01.00>Hel<00:01.30>lo
// <00:01.60>") into its text and words; a plain line has no words.
func lrcWords(s string) (string, []Word) {
	s = reLRCVoice.ReplaceAllString(s, "")
	locs := reLRCWord.FindAllStringSubmatchIndex(s, -1)
	if len(locs) == 0 {
		return strings.TrimSpace(s), nil
	}
	var words []Word
	var text strings.Builder
	text.WriteString(s[:locs[0][0]])
	for i, loc := range locs {
		at := clock(s[loc[2]:loc[3]], s[loc[4]:loc[5]], sub(s, loc[6], loc[7]))
		end := len(s)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		w := s[loc[1]:end]
		text.WriteString(w)
		if n := len(words); n > 0 && words[n-1].End == 0 {
			words[n-1].End = at
		}
		if strings.TrimSpace(w) == "" {
			// A closing time tag, or one before a space only.
			if n := len(words); n > 0 {
				words[n-1].Text += w
			}
			continue
		}
		words = append(words, Word{Begin: at, Text: w})
	}
	if len(words) == 0 {
		return strings.TrimSpace(text.String()), nil
	}
	words[len(words)-1].Text = strings.TrimRight(words[len(words)-1].Text, " ")
	return strings.TrimSpace(text.String()), words
}

func sub(s string, a, b int) string {
	if a < 0 {
		return ""
	}
	return s[a:b]
}

// clock reads an LRC time from its minutes, seconds and fraction digits
// ("12" is 120 ms, "123" 123 ms).
func clock(min, sec, frac string) time.Duration {
	m, _ := strconv.Atoi(min)
	s, _ := strconv.Atoi(sec)
	d := time.Duration(m)*time.Minute + time.Duration(s)*time.Second
	if frac != "" {
		f, _ := strconv.Atoi(frac)
		for i := len(frac); i < 3; i++ {
			f *= 10
		}
		d += time.Duration(f) * time.Millisecond
	}
	return d
}

// timedLines turns timed lines into Lines: an empty line ends the line
// before it (a pause) and is dropped. Timing is word when every line has
// words, line otherwise.
func timedLines(in []lrcLine, songEnd time.Duration) ([]Line, string) {
	var out []Line
	words := true
	for _, l := range in {
		if l.Text == "" {
			if n := len(out); n > 0 && out[n-1].End == 0 {
				out[n-1].End = l.At
			}
			continue
		}
		if len(l.Words) == 0 {
			words = false
		}
		ln := Line{Begin: l.At, Text: l.Text, Words: l.Words}
		if len(l.Words) > 0 {
			ln.End = l.Words[len(l.Words)-1].End
		}
		out = append(out, ln)
	}
	endLines(out, songEnd)
	for i := range out {
		if ws := out[i].Words; len(ws) > 0 && ws[len(ws)-1].End == 0 {
			ws[len(ws)-1].End = out[i].End
		}
	}
	if len(out) == 0 {
		return nil, TimingNone
	}
	if words {
		return out, TimingWord
	}
	return out, TimingLine
}

// plainLines turns plain text into untimed Lines, skipping blank lines.
func plainLines(s string) []Line {
	var out []Line
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, Line{Text: l})
		}
	}
	return out
}
