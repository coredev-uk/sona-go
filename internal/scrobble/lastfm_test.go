package scrobble

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// The worked examples from the Last.fm signature guide
// (lastfm-docs.github.io/api-docs/auth/signature).
func TestSignKnownExamples(t *testing.T) {
	for _, tc := range []struct {
		params url.Values
		want   string
	}{
		{url.Values{
			"method": {"auth.getSession"}, "api_key": {"YOUR_API_KEY"},
			"token": {"YOUR_REQUESTED_TOKEN"}, "format": {"json"},
		}, "94539006DE89B3C6B3C030BB1E52B9C4"},
		{url.Values{
			"method": {"track.love"}, "artist": {"KITANO REM"}, "track": {"RAINSICK"},
			"api_key": {"YOUR_API_KEY"}, "sk": {"YOUR_SESSION_KEY"}, "format": {"json"},
		}, "800B8884B00C9343D1D425ED271E0F42"},
	} {
		if got := Sign(tc.params, "YOUR_SECRET"); !strings.EqualFold(got, tc.want) {
			t.Errorf("Sign(%v) = %s, want %s", tc.params, got, tc.want)
		}
	}
}

// fakeLastFM checks every request's signature the way Last.fm does and
// records the calls.
type fakeLastFM struct {
	t *testing.T

	mu         sync.Mutex
	calls      []url.Values
	pending    int  // auth.getSession answers "not authorised" this many times
	fail       int  // track.scrobble answers 503 this many times
	badSession bool // every sk is refused (error 9)
}

const testKey, testSecret = "0123456789abcdef0123456789abcdef", "s3cret"

func (f *fakeLastFM) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		f.t.Error(err)
	}
	p := r.Form
	// Independent of Sign: sorted name+value pairs, then the secret.
	var names []string
	for k := range p {
		if k != "format" && k != "api_sig" {
			names = append(names, k)
		}
	}
	slices.Sort(names)
	var b strings.Builder
	for _, k := range names {
		b.WriteString(k + p.Get(k))
	}
	sum := md5.Sum([]byte(b.String() + testSecret))
	if p.Get("api_sig") != hex.EncodeToString(sum[:]) || p.Get("api_key") != testKey || p.Get("format") != "json" {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":13,"message":"Invalid method signature supplied"}`)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, p)
	m := p.Get("method")
	if strings.HasPrefix(m, "track.") && r.Method != http.MethodPost {
		f.t.Errorf("%s sent with %s", m, r.Method)
	}
	if p.Get("sk") != "" && f.badSession {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":9,"message":"Invalid session key - Please re-authenticate"}`)
		return
	}
	switch m {
	case "auth.getToken":
		_, _ = io.WriteString(w, `{"token":"tok123"}`)
	case "auth.getSession":
		if f.pending > 0 {
			f.pending--
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":14,"message":"Unauthorized Token - This token has not been authorized"}`)
			return
		}
		_, _ = io.WriteString(w, `{"session":{"name":"rj","key":"sk-abc","subscriber":0}}`)
	case "track.updateNowPlaying":
		_, _ = io.WriteString(w, `{"nowplaying":{}}`)
	case "track.scrobble":
		if f.fail > 0 {
			f.fail--
			f.calls = f.calls[:len(f.calls)-1] // only accepted scrobbles count
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "<html>busy</html>")
			return
		}
		_, _ = fmt.Fprintf(w, `{"scrobbles":{"@attr":{"accepted":%d,"ignored":0}}}`, countIndexed(p, "track"))
	default:
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":3,"message":"Invalid Method"}`)
	}
}

func countIndexed(p url.Values, field string) int {
	n := 0
	for p.Has(fmt.Sprintf("%s[%d]", field, n)) {
		n++
	}
	return n
}

func (f *fakeLastFM) methodCalls(method string) []url.Values {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []url.Values
	for _, c := range f.calls {
		if c.Get("method") == method {
			out = append(out, c)
		}
	}
	return out
}

func newLastFM(t *testing.T) (*LastFM, *fakeLastFM) {
	f := &fakeLastFM{t: t}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &LastFM{Key: testKey, Secret: testSecret, API: srv.URL + "/2.0/", AuthBase: "https://www.last.fm/api/auth/", HTTP: srv.Client()}, f
}

func TestLastFMDesktopAuth(t *testing.T) {
	c, f := newLastFM(t)
	f.pending = 2
	ctx := t.Context()
	tok, err := c.Token(ctx)
	if err != nil || tok != "tok123" {
		t.Fatalf("Token = %q, %v", tok, err)
	}
	u, _ := url.Parse(c.AuthURL(tok))
	if u.Host != "www.last.fm" || u.Query().Get("api_key") != testKey || u.Query().Get("token") != tok {
		t.Fatalf("AuthURL = %s", u)
	}
	if _, err := c.Session(ctx, tok); !errors.Is(err, ErrPending) {
		t.Fatalf("unapproved Session = %v, want ErrPending", err)
	}
	s, err := c.AwaitSession(ctx, tok, time.Millisecond)
	if err != nil || s != (Session{Name: "rj", Key: "sk-abc"}) {
		t.Fatalf("AwaitSession = %+v, %v", s, err)
	}
	if got := f.methodCalls("auth.getSession"); len(got) != 3 || got[2].Get("token") != tok {
		t.Fatalf("getSession calls = %v", got)
	}
}

func TestLastFMAwaitSessionStopsWithContext(t *testing.T) {
	c, f := newLastFM(t)
	f.pending = 1 << 30
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.AwaitSession(ctx, "tok123", time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v", err)
	}
}

func TestLastFMNowPlayingAndScrobbleFields(t *testing.T) {
	c, f := newLastFM(t)
	ctx := t.Context()
	tr := Track{Artist: "Björk", Title: "Jóga", Album: "Homogenic", Duration: 305*time.Second + 400*time.Millisecond}
	if err := c.NowPlaying(ctx, "sk-abc", tr); err != nil {
		t.Fatal(err)
	}
	np := f.methodCalls("track.updateNowPlaying")
	if len(np) != 1 || np[0].Get("artist") != "Björk" || np[0].Get("track") != "Jóga" ||
		np[0].Get("album") != "Homogenic" || np[0].Get("duration") != "305" || np[0].Get("sk") != "sk-abc" {
		t.Fatalf("updateNowPlaying = %v", np)
	}
	if err := c.Submit(ctx, "sk-abc", []Listen{{Track: tr, ListenedAt: t0}, {Track: Track{Artist: "A", Title: "B"}, ListenedAt: t0.Add(time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	sc := f.methodCalls("track.scrobble")[0]
	if sc.Get("artist[0]") != "Björk" || sc.Get("timestamp[0]") != fmt.Sprint(t0.Unix()) || sc.Get("duration[0]") != "305" ||
		sc.Get("track[1]") != "B" || sc.Get("timestamp[1]") != fmt.Sprint(t0.Unix()+60) || sc.Has("album[1]") || sc.Has("duration[1]") {
		t.Fatalf("scrobble = %v", sc)
	}
}

func TestLastFMErrorClasses(t *testing.T) {
	for code, want := range map[int]error{4: ErrAuth, 9: ErrAuth, 14: ErrPending, 15: ErrExpired, 6: ErrRejected, 13: ErrRejected} {
		if err := error(&LastFMError{Code: code}); !errors.Is(err, want) {
			t.Errorf("error %d is not %v", code, want)
		}
	}
	for _, code := range []int{8, 11, 16, 29} {
		err := error(&LastFMError{Code: code})
		if errors.Is(err, ErrAuth) || errors.Is(err, ErrRejected) {
			t.Errorf("transient error %d classified as permanent", code)
		}
	}
}
