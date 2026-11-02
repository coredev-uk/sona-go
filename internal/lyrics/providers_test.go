package lyrics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth/devtoken"
	"github.com/coredev-uk/sona-go/internal/auth/store"
)

// The testdata fixtures keep the shape of real responses (recorded from
// each service) with every lyric replaced by placeholder words.

var song = Track{CatalogID: "1", ISRC: "ZZTST2600001", Title: "Placeholder Song", Artist: "Test Artist", Album: "Test Album", Duration: 200 * time.Second}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeServer answers by path with handler and records each request.
type fakeServer struct {
	*httptest.Server
	mu   sync.Mutex
	reqs []*url.URL
	hdrs []http.Header
}

func newFake(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *fakeServer {
	f := &fakeServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.reqs = append(f.reqs, r.URL)
		f.hdrs = append(f.hdrs, r.Header.Clone())
		f.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServer) paths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.reqs))
	for i, u := range f.reqs {
		out[i] = u.Path
	}
	return out
}

type memSecret struct {
	mu sync.Mutex
	v  string
}

func (m *memSecret) Get() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.v == "" {
		return "", store.ErrNotFound
	}
	return m.v, nil
}
func (m *memSecret) Set(v string) error     { m.mu.Lock(); m.v = v; m.mu.Unlock(); return nil }
func (m *memSecret) Delete() error          { m.mu.Lock(); m.v = ""; m.mu.Unlock(); return nil }
func (m *memSecret) Backend() store.Backend { return store.BackendFile }

func musixmatch(f *fakeServer, sec *memSecret) *Musixmatch {
	return &Musixmatch{HTTP: f.Client(), Base: f.URL + "/ws/1.1/", UserAgent: "Sona/test", Secret: func() (store.Secret, error) { return sec, nil }}
}

func TestMusixmatchRichsyncAndTranslation(t *testing.T) {
	macro, tok, tr := fixture(t, "musixmatch_macro.json"), fixture(t, "musixmatch_token.json"), fixture(t, "musixmatch_translations.json")
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/1.1/token.get":
			_, _ = w.Write(tok)
		case "/ws/1.1/macro.subtitles.get":
			_, _ = w.Write(macro)
		case "/ws/1.1/crowd.track.translations.get":
			_, _ = w.Write(tr)
		default:
			http.NotFound(w, r)
		}
	})
	sec := &memSecret{}
	l, err := musixmatch(f, sec).Fetch(context.Background(), Request{Track: song, Language: "es-ES"})
	if err != nil {
		t.Fatal(err)
	}
	if l.Timing != TimingWord || l.Language != "en" || len(l.Lines) != 4 || strings.Join(l.Songwriters, ";") != "Ann Writer;Bo Writer" {
		t.Fatalf("lyrics = %+v", l)
	}
	ln := l.Lines[1]
	if ln.Text != "bravo charlie delta echo" || ln.Begin != 27170*ms || ln.End != 28268*ms || len(ln.Words) != 4 ||
		ln.Words[0].Text != "bravo " || ln.Words[0].Begin != 27209*ms || ln.Words[0].End != 27580*ms || ln.Words[3].End != 28268*ms {
		t.Fatalf("line = %+v", ln)
	}
	if l.TranslationLanguage != "es" || ln.Translation == nil || ln.Translation.Text != "traducido bravo" || l.Lines[3].Translation != nil {
		t.Fatalf("translation = %q %+v %+v", l.TranslationLanguage, ln.Translation, l.Lines[3].Translation)
	}
	// The token was fetched once and kept; it goes on every call.
	if sec.v != "fake-user-token-0001" {
		t.Fatalf("stored token = %q", sec.v)
	}
	paths := f.paths()
	if strings.Join(paths, " ") != "/ws/1.1/token.get /ws/1.1/macro.subtitles.get /ws/1.1/crowd.track.translations.get" {
		t.Fatalf("paths = %v", paths)
	}
	q := f.reqs[1].Query()
	if q.Get("usertoken") != "fake-user-token-0001" || q.Get("app_id") != musixmatchApp || q.Get("track_isrc") != "ZZTST2600001" ||
		q.Get("q_duration") != "200" || q.Get("q_track") != "Placeholder Song" || q.Get("optional_calls") != "track.richsync" {
		t.Fatalf("macro query = %v", q)
	}
	if f.reqs[2].Query().Get("selected_language") != "es" || f.reqs[2].Query().Get("track_id") != "1001" {
		t.Fatalf("translation query = %v", f.reqs[2].Query())
	}
	if ua := f.hdrs[1].Get("User-Agent"); ua != "Sona/test" {
		t.Fatalf("user agent = %q", ua)
	}
}

func TestMusixmatchMatchesByISRC(t *testing.T) {
	macro := fixture(t, "musixmatch_macro.json")
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(macro) })
	// The title differs (a localized name) but the ISRC is the song's.
	req := Request{Track: Track{ISRC: "zztst2600001", Title: "Другое название", Artist: "Test Artist", Duration: 200 * time.Second}}
	if _, err := musixmatch(f, &memSecret{v: "stored"}).Fetch(context.Background(), req); err != nil {
		t.Fatalf("ISRC match: %v", err)
	}
	// Without the ISRC the title decides, and doesn't match.
	req.ISRC = ""
	if _, err := musixmatch(f, &memSecret{v: "stored"}).Fetch(context.Background(), req); !errors.Is(err, ErrNotFound) {
		t.Fatalf("title mismatch err = %v", err)
	}
}

func TestMusixmatchFallsBackToSubtitles(t *testing.T) {
	// The same answer with the richsync restricted (not shown outside
	// Musixmatch's own apps).
	var doc map[string]any
	if err := json.Unmarshal(fixture(t, "musixmatch_macro.json"), &doc); err != nil {
		t.Fatal(err)
	}
	calls := doc["message"].(map[string]any)["body"].(map[string]any)["macro_calls"].(map[string]any)
	calls["track.richsync.get"].(map[string]any)["message"].(map[string]any)["body"].(map[string]any)["richsync"].(map[string]any)["restricted"] = 1
	macro, _ := json.Marshal(doc)
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(macro)) })
	l, err := musixmatch(f, &memSecret{v: "stored"}).Fetch(context.Background(), Request{Track: song})
	if err != nil {
		t.Fatal(err)
	}
	// The empty subtitle line ends the first line (a pause).
	if l.Timing != TimingLine || len(l.Lines) != 4 || l.Lines[0].End != 16550*ms || l.Lines[1].Begin != 27170*ms || l.Lines[1].Words != nil {
		t.Fatalf("lyrics = %+v", l)
	}
}

func TestMusixmatchNoLyrics(t *testing.T) {
	macro := fixture(t, "musixmatch_macro_none.json")
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(macro) })
	req := Request{Track: Track{ISRC: "ZZTST2600002", Title: "Quiet Song", Artist: "Test Artist", Duration: 318 * time.Second}}
	if _, err := musixmatch(f, &memSecret{v: "stored"}).Fetch(context.Background(), req); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestMusixmatchCaptchaAndTokenRenewal(t *testing.T) {
	var captcha, rejected bool
	macro, tok := fixture(t, "musixmatch_macro.json"), fixture(t, "musixmatch_token.json")
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case captcha:
			_, _ = w.Write([]byte(`{"message":{"header":{"status_code":401,"execute_time":0.01,"hint":"captcha"}}}`))
		case r.URL.Path == "/ws/1.1/token.get":
			_, _ = w.Write(tok)
		case rejected && r.URL.Query().Get("usertoken") == "expired":
			_, _ = w.Write([]byte(`{"message":{"header":{"status_code":401,"execute_time":0.01,"hint":"renew"}}}`))
		default:
			_, _ = w.Write(macro)
		}
	})
	captcha = true
	_, err := musixmatch(f, &memSecret{v: "stored"}).Fetch(context.Background(), Request{Track: song})
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter != musixmatchCaptcha {
		t.Fatalf("captcha err = %v", err)
	}
	// A token Musixmatch no longer takes is replaced once, and the new one kept.
	captcha, rejected = false, true
	sec := &memSecret{v: "expired"}
	if _, err := musixmatch(f, sec).Fetch(context.Background(), Request{Track: song}); err != nil {
		t.Fatal(err)
	}
	if sec.v != "fake-user-token-0001" {
		t.Fatalf("token = %q", sec.v)
	}
}

func TestMusixmatchKeepsTokenOutOfErrors(t *testing.T) {
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		hj, _ := w.(http.Hijacker)
		c, _, _ := hj.Hijack()
		_ = c.Close()
	})
	_, err := musixmatch(f, &memSecret{v: "secret-token-value"}).Fetch(context.Background(), Request{Track: song})
	if err == nil || strings.Contains(err.Error(), "secret-token-value") {
		t.Fatalf("err = %v", err)
	}
}

func TestLRCLIB(t *testing.T) {
	get := fixture(t, "lrclib_get.json")
	var exact bool
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/get":
			if !exact {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"code":404,"name":"TrackNotFound","message":"Failed to find specified track"}`))
				return
			}
			_, _ = w.Write(get)
		case "/api/search":
			// An instrumental, a plain-only entry, another song, and the
			// synced entry (preferred).
			rec := strings.TrimSuffix(strings.TrimSpace(string(get)), "}")
			_, _ = w.Write([]byte(`[{"id":1,"trackName":"Placeholder Song","artistName":"Test Artist","duration":200,"instrumental":true},` +
				`{"id":2,"trackName":"Placeholder Song","artistName":"Test Artist","duration":201,"plainLyrics":"golf\nhotel","syncedLyrics":null},` +
				`{"id":3,"trackName":"Other Song","artistName":"Test Artist","duration":200,"syncedLyrics":"[00:01.00]x"},` + rec + `}]`))
		}
	})
	p := &LRCLIB{HTTP: f.Client(), Base: f.URL + "/api/", UserAgent: "Sona/test"}
	exact = true
	l, err := p.Fetch(context.Background(), Request{Track: song})
	if err != nil {
		t.Fatal(err)
	}
	if l.Timing != TimingLine || len(l.Lines) != 4 || l.Lines[3].Text != "november oscar papa alpha bravo charlie delta echo foxtrot" || l.Lines[3].End != 40*time.Second {
		t.Fatalf("lyrics = %+v", l)
	}
	q := f.reqs[0].Query()
	if q.Get("track_name") != "Placeholder Song" || q.Get("artist_name") != "Test Artist" || q.Get("album_name") != "Test Album" || q.Get("duration") != "200" {
		t.Fatalf("query = %v", q)
	}
	if f.hdrs[0].Get("User-Agent") != "Sona/test" {
		t.Fatal("no user agent")
	}

	exact = false
	if l, err = p.Fetch(context.Background(), Request{Track: song}); err != nil || l.Timing != TimingLine {
		t.Fatalf("search: %+v, %v", l, err)
	}
	if got := strings.Join(f.paths()[1:], " "); got != "/api/get /api/search" {
		t.Fatalf("paths = %s", got)
	}

	if _, err := lrclibLyrics(lrclibRecord{Instrumental: true, PlainLyrics: "x"}, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("instrumental err = %v", err)
	}
	if l, err := lrclibLyrics(lrclibRecord{PlainLyrics: "golf\n\nhotel"}, 0); err != nil || l.Timing != TimingNone || len(l.Lines) != 2 {
		t.Fatalf("plain = %+v, %v", l, err)
	}
}

func TestNetEase(t *testing.T) {
	search, lyric := fixture(t, "netease_search.json"), fixture(t, "netease_lyric.json")
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/search/get":
			_, _ = w.Write(search)
		case "/api/song/lyric":
			_, _ = w.Write(lyric)
		}
	})
	p := &NetEase{HTTP: f.Client(), Base: f.URL + "/api/", UserAgent: "Sona/test"}
	l, err := p.Fetch(context.Background(), Request{Track: song, Language: "en-GB"})
	if err != nil {
		t.Fatal(err)
	}
	// The studio version within the length tolerance; credit lines dropped.
	if f.reqs[1].Query().Get("id") != "11002" {
		t.Fatalf("lyric query = %v", f.reqs[1].Query())
	}
	if l.Timing != TimingLine || len(l.Lines) != 4 || l.Lines[0].Text != "alpha" || l.Lines[0].Begin != 13120*ms {
		t.Fatalf("lyrics = %+v", l)
	}
	// The Chinese translation is only attached when Chinese is asked for.
	if l.TranslationLanguage != "" || l.Lines[0].Translation != nil {
		t.Fatalf("translation for en = %+v", l.Lines[0].Translation)
	}
	l, err = p.Fetch(context.Background(), Request{Track: song, Language: "zh-Hans-CN"})
	if err != nil || l.TranslationLanguage != "zh-Hans" || l.Lines[1].Translation == nil || l.Lines[1].Translation.Text != "译 bravo" {
		t.Fatalf("zh = %+v, %v", l, err)
	}
	if f.hdrs[0].Get("Referer") != "https://music.163.com/" {
		t.Fatal("no referer")
	}

	if _, err := neteaseLyrics(neteaseLyric{NoLyric: true}, "", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no lyric err = %v", err)
	}
	var instrumental neteaseLyric
	instrumental.LRC.Lyric = "[00:00.00] 纯音乐，请欣赏\n"
	if _, err := neteaseLyrics(instrumental, "", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("instrumental err = %v", err)
	}
}

type staticDev struct{}

func (staticDev) Token(context.Context) (devtoken.Token, error) {
	return devtoken.Token{JWT: "dev", Expires: time.Now().Add(time.Hour)}, nil
}
func (staticDev) Invalidate() {}

const appleTTML = `<tt xmlns="http://www.w3.org/ns/ttml" xmlns:itunes="http://music.apple.com/lyric-ttml-internal" xmlns:ttm="http://www.w3.org/ns/ttml#metadata" itunes:timing="Word" xml:lang="ko"><head><metadata><iTunesMetadata xmlns="http://music.apple.com/lyric-ttml-internal">` +
	`<translations><translation type="subtitle" xml:lang="en-US"><text for="L1">alpha bravo</text></translation></translations>` +
	`<transliterations><transliteration xml:lang="ko-Latn"><text for="L1"><span begin="1" end="1.5">al</span> <span begin="1.5" end="2">pha</span></text></transliteration></transliterations>` +
	`<songwriters><songwriter>Ann Writer</songwriter></songwriters></iTunesMetadata></metadata></head>` +
	`<body><div itunes:songPart="Verse"><p begin="1" end="2" itunes:key="L1" ttm:agent="v1"><span begin="1" end="1.5">가</span> <span begin="1.5" end="2">나</span><span ttm:role="x-bg"><span begin="2" end="2.5">(다)</span></span></p></div></body></tt>`

func TestApple(t *testing.T) {
	var signedIn bool
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/syllable-lyrics") {
			_, _ = w.Write([]byte(`{"data":[{"id":"1","type":"syllable-lyrics","attributes":{"ttml":` + jsonString(appleTTML) + `}}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	api := applemusic.New(f.Client(), staticDev{}, func() string {
		if signedIn {
			return "mut"
		}
		return ""
	})
	api.Base, api.AMPBase = f.URL, f.URL
	p := &Apple{API: api, Storefront: func() string { return "gb" }}
	if _, err := p.Fetch(context.Background(), Request{Track: song}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("signed out err = %v", err)
	}
	if _, err := p.Fetch(context.Background(), Request{Track: Track{Title: "x"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no catalog ID err = %v", err)
	}
	signedIn = true
	l, err := p.Fetch(context.Background(), Request{Track: song, Language: "en-GB"})
	if err != nil {
		t.Fatal(err)
	}
	ln := l.Lines[0]
	if l.Timing != TimingWord || l.Language != "ko" || ln.Text != "가 나" || ln.Background != "(다)" || ln.Agent != "v1" || ln.Part != "Verse" ||
		len(ln.Words) != 3 || !ln.Words[2].Background {
		t.Fatalf("lyrics = %+v", l)
	}
	if l.TranslationLanguage != "en-US" || ln.Translation.Text != "alpha bravo" || l.TransliterationLanguage != "ko-Latn" || len(ln.Transliteration.Words) != 2 {
		t.Fatalf("localizations = %+v", l)
	}
	if q := f.reqs[0].Query(); q.Get("l[lyrics]") != "en-gb" {
		t.Fatalf("query = %v", q)
	}
}

func jsonString(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}

func TestHTTPSOnly(t *testing.T) {
	r := func(u string) *http.Request {
		req, _ := http.NewRequest(http.MethodGet, u, nil)
		return req
	}
	if err := HTTPSOnly(r("https://lrclib.net/api/get"), nil); err != nil {
		t.Fatalf("https: %v", err)
	}
	if err := HTTPSOnly(r("http://lrclib.net/api/get"), nil); err == nil {
		t.Fatal("followed a redirect to http")
	}
	if err := HTTPSOnly(r("https://lrclib.net/"), make([]*http.Request, 5)); err == nil {
		t.Fatal("followed a sixth redirect")
	}
}

func TestMusixmatchDoesNotWaitForASilentKeyring(t *testing.T) {
	macro, tok := fixture(t, "musixmatch_macro.json"), fixture(t, "musixmatch_token.json")
	f := newFake(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ws/1.1/token.get" {
			_, _ = w.Write(tok)
			return
		}
		_, _ = w.Write(macro)
	})
	unlock := make(chan struct{})
	t.Cleanup(func() { close(unlock) })
	p := musixmatch(f, nil)
	p.keyringWait = 50 * time.Millisecond
	p.Secret = func() (store.Secret, error) {
		<-unlock // an unlock prompt nobody answers
		return &memSecret{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if _, err := p.Fetch(ctx, Request{Track: song}); err != nil {
		t.Fatalf("err = %v", err)
	}
}
