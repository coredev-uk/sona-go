package lyrics

import (
	"bytes"
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
)

// MusixmatchBase is the API Musixmatch's own apps use. It wants an
// anonymous user token (token.get) on every request instead of a
// request signature.
const MusixmatchBase = "https://apic-appmobile.musixmatch.com/ws/1.1/"

// musixmatchApp is the app ID the anonymous token is issued for.
const musixmatchApp = "mac-ios-v2.0"

// musixmatchCaptcha is how long Sona leaves Musixmatch alone after it asks
// for a captcha (its sign of too many requests).
const musixmatchCaptcha = 15 * time.Minute

// musixmatchKeyringWait is how long a request waits for the keyring to
// give back the kept token before fetching one for this run only.
const musixmatchKeyringWait = 2 * time.Second

// Musixmatch is the Musixmatch catalogue: word-timed lyrics ("richsync")
// where it has them, else line-timed or plain, and its community
// translations into the requested language.
type Musixmatch struct {
	HTTP      *http.Client
	Base      string // default MusixmatchBase
	UserAgent string
	// Secret opens where the anonymous user token is kept (the keyring, or
	// its 0600 file fallback). It is called once, on its own goroutine at
	// the first request, since a keyring may take long to answer.
	Secret func() (store.Secret, error)

	once   sync.Once
	opened chan struct{} // closed once sec and stored are set
	sec    store.Secret  // nil without a keyring
	stored string        // the token kept from an earlier run

	mu    sync.Mutex // held while the token is read or fetched
	token string
	// keyringWait bounds the wait for the keyring (default
	// musixmatchKeyringWait); tests shorten it.
	keyringWait time.Duration
}

func (p *Musixmatch) ID() string      { return config.LyricsMusixmatch }
func (p *Musixmatch) Localized() bool { return true }

// mxmHeader is the status Musixmatch puts inside every 200 response.
type mxmHeader struct {
	StatusCode   int    `json:"status_code"`
	Hint         string `json:"hint"`
	Instrumental int    `json:"instrumental"`
}

// mxmEnvelope is one response, or one call of a macro. Body is "[]" or
// absent when empty.
type mxmEnvelope struct {
	Message struct {
		Header mxmHeader       `json:"header"`
		Body   json.RawMessage `json:"body"`
	} `json:"message"`
}

// body decodes the envelope's body into v; an empty body leaves v alone.
func (e mxmEnvelope) body(v any) error {
	b := bytes.TrimSpace(e.Message.Body)
	if len(b) == 0 || b[0] != '{' {
		return nil
	}
	return json.Unmarshal(b, v)
}

type mxmTrack struct {
	TrackID     int64  `json:"track_id"`
	TrackName   string `json:"track_name"`
	ArtistName  string `json:"artist_name"`
	AlbumName   string `json:"album_name"`
	TrackLength int64  `json:"track_length"` // seconds
	TrackISRC   string `json:"track_isrc"`
	// CommontrackISRCs are the ISRCs of every release of the song.
	CommontrackISRCs [][]string `json:"commontrack_isrcs"`
	Instrumental     int        `json:"instrumental"`
	HasRichsync      int        `json:"has_richsync"`
}

// hasISRC reports whether isrc is one of the track's.
func (t mxmTrack) hasISRC(isrc string) bool {
	if isrc == "" {
		return false
	}
	if strings.EqualFold(t.TrackISRC, isrc) {
		return true
	}
	for _, group := range t.CommontrackISRCs {
		for _, i := range group {
			if strings.EqualFold(i, isrc) {
				return true
			}
		}
	}
	return false
}

type mxmRichsync struct {
	Richsync struct {
		Body       string `json:"richsync_body"`
		Language   string `json:"richsync_language"`
		Language2  string `json:"richssync_language"` // sic: the field is misspelt in some responses
		Copyright  string `json:"lyrics_copyright"`
		Restricted int    `json:"restricted"`
	} `json:"richsync"`
}

type mxmSubtitles struct {
	List []struct {
		Subtitle struct {
			Body       string `json:"subtitle_body"`
			Language   string `json:"subtitle_language"`
			Copyright  string `json:"lyrics_copyright"`
			Restricted int    `json:"restricted"`
		} `json:"subtitle"`
	} `json:"subtitle_list"`
}

type mxmLyrics struct {
	Lyrics struct {
		Body         string `json:"lyrics_body"`
		Language     string `json:"lyrics_language"`
		Copyright    string `json:"lyrics_copyright"`
		Restricted   int    `json:"restricted"`
		Instrumental int    `json:"instrumental"`
	} `json:"lyrics"`
}

type mxmTranslations struct {
	List []struct {
		Translation struct {
			Description         string `json:"description"` // the translated line
			Snippet             string `json:"snippet"`
			MatchedLine         string `json:"matched_line"`
			SubtitleMatchedLine string `json:"subtitle_matched_line"`
		} `json:"translation"`
	} `json:"translations_list"`
}

// Fetch matches the song (by ISRC, else title, artist, album and length)
// and gets its richsync, subtitles and plain lyrics in one macro call,
// then the translation into req.Language when the lyrics are in another
// language.
func (p *Musixmatch) Fetch(ctx context.Context, req Request) (Lyrics, error) {
	if req.Title == "" || req.Artist == "" {
		return Lyrics{}, ErrNotFound
	}
	q := url.Values{
		"q_track": {req.Title}, "q_artist": {req.Artist},
		"namespace": {"lyrics_richsynched"}, "subtitle_format": {"mxm"}, "optional_calls": {"track.richsync"},
	}
	if req.Album != "" {
		q.Set("q_album", req.Album)
	}
	if req.ISRC != "" {
		q.Set("track_isrc", req.ISRC)
	}
	if req.Duration > 0 {
		secs := formatSeconds(req.Duration)
		q.Set("q_duration", secs)
		q.Set("f_subtitle_length", secs)
		q.Set("f_subtitle_length_max_deviation", strconv.Itoa(int(durationTolerance/time.Second)))
	}
	var macro struct {
		Calls map[string]mxmEnvelope `json:"macro_calls"`
	}
	if err := p.call(ctx, "macro.subtitles.get", q, &macro); err != nil {
		return Lyrics{}, err
	}
	var matched struct {
		Track mxmTrack `json:"track"`
	}
	m := macro.Calls["matcher.track.get"]
	if m.Message.Header.StatusCode != http.StatusOK {
		return Lyrics{}, ErrNotFound
	}
	if err := m.body(&matched); err != nil {
		return Lyrics{}, fmt.Errorf("lyrics: musixmatch: matcher: %w", err)
	}
	t := matched.Track
	sameISRC := t.hasISRC(req.ISRC)
	if t.TrackID == 0 || (!sameISRC && score(req.Track, candidate{Title: t.TrackName, Artist: t.ArtistName, Album: t.AlbumName, Duration: time.Duration(t.TrackLength) * time.Second}) < 0) {
		return Lyrics{}, ErrNotFound
	}
	if t.Instrumental == 1 {
		return Lyrics{}, ErrNotFound
	}

	out, err := p.lyrics(ctx, macro.Calls, t, req.Duration)
	if err != nil {
		return Lyrics{}, err
	}
	if lang := primary(req.Language); lang != "" && lang != primary(out.Language) {
		if err := p.translate(ctx, t.TrackID, lang, out.Lines); err != nil {
			if !errors.Is(err, ErrNotFound) {
				out.partial = true
			}
		} else {
			out.TranslationLanguage = lang
		}
	}
	return out, nil
}

// lyrics picks the best of the macro's answers: richsync (fetched on its
// own when the matched track has it but the macro left it out), then
// subtitles, then plain lyrics.
func (p *Musixmatch) lyrics(ctx context.Context, calls map[string]mxmEnvelope, t mxmTrack, songEnd time.Duration) (Lyrics, error) {
	var rs mxmRichsync
	if c := calls["track.richsync.get"]; c.Message.Header.StatusCode == http.StatusOK {
		_ = c.body(&rs)
	}
	if rs.Richsync.Body == "" && t.HasRichsync == 1 {
		err := p.call(ctx, "track.richsync.get", url.Values{"track_id": {strconv.FormatInt(t.TrackID, 10)}}, &rs)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return Lyrics{}, err
		}
	}
	if r := rs.Richsync; r.Body != "" && r.Restricted == 0 {
		if lines := richsyncLines(r.Body); len(lines) > 0 {
			return Lyrics{Provider: p.ID(), Timing: TimingWord, Language: cmp.Or(r.Language, r.Language2), Songwriters: writers(r.Copyright), Lines: lines}, nil
		}
	}
	var subs mxmSubtitles
	if c := calls["track.subtitles.get"]; c.Message.Header.StatusCode == http.StatusOK {
		_ = c.body(&subs)
	}
	if len(subs.List) > 0 {
		s := subs.List[0].Subtitle
		if s.Body != "" && s.Restricted == 0 {
			if lines, timing := subtitleLines(s.Body, songEnd); len(lines) > 0 {
				return Lyrics{Provider: p.ID(), Timing: timing, Language: s.Language, Songwriters: writers(s.Copyright), Lines: lines}, nil
			}
		}
	}
	var ly mxmLyrics
	if c := calls["track.lyrics.get"]; c.Message.Header.StatusCode == http.StatusOK {
		_ = c.body(&ly)
	}
	if l := ly.Lyrics; l.Body != "" && l.Restricted == 0 && l.Instrumental == 0 {
		if lines := plainLines(stripMxmFooter(l.Body)); len(lines) > 0 {
			return Lyrics{Provider: p.ID(), Timing: TimingNone, Language: l.Language, Songwriters: writers(l.Copyright), Lines: lines}, nil
		}
	}
	return Lyrics{}, ErrNotFound
}

// translate sets each line's Translation from the community translations
// into lang, matched by the original line's text.
func (p *Musixmatch) translate(ctx context.Context, trackID int64, lang string, lines []Line) error {
	var tr mxmTranslations
	q := url.Values{"track_id": {strconv.FormatInt(trackID, 10)}, "selected_language": {lang}, "comment_format": {"text"}, "part": {"user"}}
	if err := p.call(ctx, "crowd.track.translations.get", q, &tr); err != nil {
		return err
	}
	byLine := map[string]string{}
	for _, e := range tr.List {
		t := e.Translation
		if t.Description == "" {
			continue
		}
		for _, src := range []string{t.MatchedLine, t.SubtitleMatchedLine, t.Snippet} {
			if k := normalize(src); k != "" {
				byLine[k] = t.Description
			}
		}
	}
	found := false
	for i := range lines {
		if d, ok := byLine[normalize(lines[i].Text)]; ok {
			lines[i].Translation = &Text{Text: d}
			found = true
		}
	}
	if !found {
		return ErrNotFound
	}
	return nil
}

// call runs one API method with the user token and decodes the body into
// v. A token Musixmatch no longer accepts is replaced once.
func (p *Musixmatch) call(ctx context.Context, method string, q url.Values, v any) error {
	for attempt := 0; ; attempt++ {
		tok, err := p.userToken(ctx)
		if err != nil {
			return err
		}
		env, err := p.get(ctx, method, q, tok)
		if err != nil {
			return err
		}
		h := env.Message.Header
		switch {
		case h.StatusCode == http.StatusOK:
			return env.body(v)
		case h.StatusCode == http.StatusNotFound:
			return fmt.Errorf("%w: musixmatch %s", ErrNotFound, method)
		case h.StatusCode == http.StatusUnauthorized && h.Hint == "captcha":
			return &RateLimitError{Provider: p.ID(), RetryAfter: musixmatchCaptcha}
		case h.StatusCode == http.StatusUnauthorized && attempt == 0:
			p.dropToken(tok)
			continue
		case h.StatusCode == http.StatusPaymentRequired || h.StatusCode == http.StatusTooManyRequests:
			return &RateLimitError{Provider: p.ID()}
		}
		return fmt.Errorf("lyrics: musixmatch %s: status %d %s", method, h.StatusCode, h.Hint)
	}
}

// get sends one request; the token is added to the query here only, so
// it never appears in an error (url.Error carries the URL).
func (p *Musixmatch) get(ctx context.Context, method string, q url.Values, tok string) (mxmEnvelope, error) {
	base := p.Base
	if base == "" {
		base = MusixmatchBase
	}
	all := url.Values{"app_id": {musixmatchApp}, "format": {"json"}}
	for k, vs := range q {
		all[k] = vs
	}
	if tok != "" {
		all.Set("usertoken", tok)
	}
	var env mxmEnvelope
	err := getJSON(ctx, p.HTTP, p.ID(), base+method+"?"+all.Encode(), http.Header{"User-Agent": {p.UserAgent}}, &env)
	var ue *url.Error
	if errors.As(err, &ue) {
		// Keep the token out of the message: drop the URL.
		err = fmt.Errorf("lyrics: musixmatch %s: %w", method, ue.Err)
	}
	return env, err
}

// open reads the kept token on its own goroutine (once).
func (p *Musixmatch) open() {
	p.once.Do(func() {
		p.opened = make(chan struct{})
		go func() {
			defer close(p.opened)
			if p.Secret == nil {
				return
			}
			sec, err := p.Secret()
			if err != nil {
				return
			}
			if t, err := sec.Get(); err == nil {
				p.stored = t
			}
			p.sec = sec
		}()
	})
}

// keyring returns the opened secret store, waiting for it a short while
// (keyringWait, within ctx); nil when there is none, or when it hasn't
// answered yet: the token is then fetched and kept in memory only.
func (p *Musixmatch) keyring(ctx context.Context) store.Secret {
	p.open()
	wait := p.keyringWait
	if wait <= 0 {
		wait = musixmatchKeyringWait
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-p.opened:
		return p.sec
	case <-t.C:
	case <-ctx.Done():
	}
	return nil
}

// userToken returns the anonymous user token: the kept one, or a new one
// from token.get (then kept).
func (p *Musixmatch) userToken(ctx context.Context) (string, error) {
	sec := p.keyring(ctx)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" {
		return p.token, nil
	}
	if sec != nil && p.stored != "" {
		p.token = p.stored
		return p.token, nil
	}
	env, err := p.get(ctx, "token.get", url.Values{"guid": {newGUID()}, "timestamp": {time.Now().UTC().Format(time.RFC3339)}}, "")
	if err != nil {
		return "", err
	}
	if h := env.Message.Header; h.StatusCode != http.StatusOK {
		if h.Hint == "captcha" {
			return "", &RateLimitError{Provider: p.ID(), RetryAfter: musixmatchCaptcha}
		}
		return "", fmt.Errorf("lyrics: musixmatch token: status %d %s", h.StatusCode, h.Hint)
	}
	var body struct {
		UserToken string `json:"user_token"`
	}
	if err := env.body(&body); err != nil {
		return "", fmt.Errorf("lyrics: musixmatch token: %w", err)
	}
	if body.UserToken == "" || strings.HasPrefix(body.UserToken, "UpgradeOnly") {
		return "", errors.New("lyrics: musixmatch gave no usable token")
	}
	p.token = body.UserToken
	if sec != nil {
		_ = sec.Set(p.token)
	}
	return p.token, nil
}

// dropToken forgets tok (when it is still the current one), so the next
// call fetches a new one.
func (p *Musixmatch) dropToken(tok string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != tok {
		return
	}
	p.token = ""
	select {
	case <-p.opened:
		p.stored = ""
		if p.sec != nil {
			_ = p.sec.Delete()
		}
	default: // the keyring hasn't answered yet; its token gets refused once more
	}
}

// newGUID is a random version 4 UUID.
func newGUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// richsyncLines parses a richsync body: lines with their start and end
// ("ts", "te", seconds) and characters or words with offsets from the
// line's start ("l": {"c", "o"}); a space entry ends the word before it.
func richsyncLines(body string) []Line {
	var rows []struct {
		TS float64 `json:"ts"`
		TE float64 `json:"te"`
		L  []struct {
			C string  `json:"c"`
			O float64 `json:"o"`
		} `json:"l"`
		X string `json:"x"`
	}
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		return nil
	}
	var out []Line
	for _, r := range rows {
		text := strings.TrimSpace(r.X)
		if text == "" {
			continue
		}
		begin, end := seconds(r.TS), seconds(r.TE)
		var words []Word
		for _, c := range r.L {
			at := begin + seconds(c.O)
			if n := len(words); n > 0 && words[n-1].End == 0 {
				words[n-1].End = at
			}
			if strings.TrimSpace(c.C) == "" {
				if n := len(words); n > 0 {
					words[n-1].Text += c.C
				}
				continue
			}
			words = append(words, Word{Begin: at, Text: c.C})
		}
		if n := len(words); n > 0 && words[n-1].End == 0 {
			words[n-1].End = end
		}
		out = append(out, Line{Begin: begin, End: end, Text: text, Words: words})
	}
	return out
}

// subtitleLines parses a subtitle body in Musixmatch's "mxm" format.
func subtitleLines(body string, songEnd time.Duration) ([]Line, string) {
	var rows []struct {
		Text string `json:"text"`
		Time struct {
			Total float64 `json:"total"`
		} `json:"time"`
	}
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		return nil, TimingNone
	}
	in := make([]lrcLine, len(rows))
	for i, r := range rows {
		in[i] = lrcLine{At: seconds(r.Time.Total), Text: strings.TrimSpace(r.Text)}
	}
	return timedLines(in, songEnd)
}

// writers reads the songwriters from Musixmatch's copyright text
// ("Writer(s): A, B\nCopyright: …").
func writers(copyright string) []string {
	line, _, _ := strings.Cut(copyright, "\n")
	names, ok := strings.CutPrefix(strings.TrimSpace(line), "Writer(s):")
	if !ok {
		return nil
	}
	var out []string
	for _, n := range strings.Split(names, ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// stripMxmFooter removes the notice Musixmatch appends to plain lyrics
// ("******* This Lyrics is NOT for Commercial use *******" and an ID).
func stripMxmFooter(body string) string {
	if i := strings.Index(body, "*******"); i >= 0 {
		return body[:i]
	}
	return body
}

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second)).Round(time.Millisecond)
}
