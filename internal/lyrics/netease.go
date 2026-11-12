package lyrics

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
)

// NetEaseBase is NetEase Cloud Music's web API.
const NetEaseBase = "https://music.163.com/api/"

// NetEase is NetEase Cloud Music: line-timed lyrics, strong for Chinese,
// Japanese and Korean music, with a Chinese translation and a
// romanisation for some songs. Off by default.
type NetEase struct {
	HTTP      *http.Client
	Base      string // default NetEaseBase
	UserAgent string
}

func (p *NetEase) ID() string { return config.LyricsNetEase }

// Localized: the Chinese translation is only attached when Chinese is
// asked for.
func (p *NetEase) Localized() bool { return true }

type neteaseSearch struct {
	Code   int `json:"code"`
	Result struct {
		Songs []struct {
			ID       int64  `json:"id"`
			Name     string `json:"name"`
			Duration int64  `json:"duration"` // ms
			Artists  []struct {
				Name string `json:"name"`
			} `json:"artists"`
			Album struct {
				Name string `json:"name"`
			} `json:"album"`
		} `json:"songs"`
	} `json:"result"`
}

type neteaseLyric struct {
	Code        int  `json:"code"`
	NoLyric     bool `json:"nolyric"`
	Uncollected bool `json:"uncollected"`
	LRC         struct {
		Lyric string `json:"lyric"`
	} `json:"lrc"`
	TLyric struct {
		Lyric string `json:"lyric"`
	} `json:"tlyric"`
	RomaLRC struct {
		Lyric string `json:"lyric"`
	} `json:"romalrc"`
}

// Credit lines NetEase puts at the top of its LRC ("作词 : …").
var reNetEaseCredit = regexp.MustCompile(`^\s*(作词|作曲|编曲|制作人|监制|混音|母带|和声|吉他|贝斯|鼓|弦乐|录音|出品|发行|词|曲|Lyrics by|Composed by|Produced by|Arranged by)\s*[:：]`)

// neteaseInstrumental is NetEase's line for a song without words.
const neteaseInstrumental = "纯音乐，请欣赏"

func (p *NetEase) Fetch(ctx context.Context, req Request) (Lyrics, error) {
	if req.Title == "" || req.Artist == "" {
		return Lyrics{}, ErrNotFound
	}
	base := p.Base
	if base == "" {
		base = NetEaseBase
	}
	h := http.Header{"User-Agent": {p.UserAgent}, "Referer": {"https://music.163.com/"}}
	var s neteaseSearch
	q := url.Values{"s": {req.Artist + " " + req.Title}, "type": {"1"}, "limit": {"10"}, "offset": {"0"}}
	if err := getJSON(ctx, p.HTTP, p.ID(), base+"search/get?"+q.Encode(), h, &s); err != nil {
		return Lyrics{}, err
	}
	cs := make([]candidate, len(s.Result.Songs))
	for i, so := range s.Result.Songs {
		names := make([]string, len(so.Artists))
		for j, a := range so.Artists {
			names[j] = a.Name
		}
		cs[i] = candidate{Title: so.Name, Artist: strings.Join(names, ", "), Album: so.Album.Name, Duration: time.Duration(so.Duration) * time.Millisecond}
	}
	i := best(req.Track, cs, nil)
	if i < 0 {
		return Lyrics{}, ErrNotFound
	}
	var l neteaseLyric
	lq := url.Values{"id": {strconv.FormatInt(s.Result.Songs[i].ID, 10)}, "lv": {"1"}, "tv": {"-1"}, "rv": {"-1"}}
	if err := getJSON(ctx, p.HTTP, p.ID(), base+"song/lyric?"+lq.Encode(), h, &l); err != nil {
		return Lyrics{}, err
	}
	return neteaseLyrics(l, req.Language, req.Duration)
}

func neteaseLyrics(l neteaseLyric, lang string, songEnd time.Duration) (Lyrics, error) {
	if l.NoLyric || l.Uncollected || strings.Contains(l.LRC.Lyric, neteaseInstrumental) {
		return Lyrics{}, ErrNotFound
	}
	parsed := neteaseLines(l.LRC.Lyric)
	out := Lyrics{Provider: config.LyricsNetEase}
	out.Lines, out.Timing = timedLines(parsed, songEnd)
	if len(out.Lines) == 0 || out.empty() {
		return Lyrics{}, ErrNotFound
	}
	if primary(lang) == "zh" && attachByTime(out.Lines, neteaseLines(l.TLyric.Lyric), func(ln *Line, t string) { ln.Translation = &Text{Text: t} }) {
		out.TranslationLanguage = "zh-Hans"
	}
	if attachByTime(out.Lines, neteaseLines(l.RomaLRC.Lyric), func(ln *Line, t string) { ln.Transliteration = &Text{Text: t} }) {
		out.TransliterationLanguage = "und-Latn"
	}
	return out, nil
}

// neteaseLines parses NetEase LRC without its credit lines.
func neteaseLines(s string) []lrcLine {
	var out []lrcLine
	for _, l := range parseLRC(s) {
		if !reNetEaseCredit.MatchString(l.Text) {
			out = append(out, l)
		}
	}
	return out
}

// attachByTime sets, on each line, the text of the localized line with
// the same start (to 10 ms); it reports whether any line got one.
func attachByTime(lines []Line, loc []lrcLine, set func(*Line, string)) bool {
	byTime := map[time.Duration]string{}
	for _, l := range loc {
		if l.Text != "" {
			byTime[l.At.Round(10*time.Millisecond)] = l.Text
		}
	}
	found := false
	for i := range lines {
		if t, ok := byTime[lines[i].Begin.Round(10*time.Millisecond)]; ok {
			set(&lines[i], t)
			found = true
		}
	}
	return found
}

// primary is a language tag's primary subtag, lower-cased ("pt" for
// "pt-BR").
func primary(tag string) string {
	p, _, _ := strings.Cut(strings.ToLower(tag), "-")
	return p
}
