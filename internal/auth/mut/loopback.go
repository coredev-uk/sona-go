// Package mut obtains an Apple Music User Token through Apple's no-opener
// redirect flow and a single-use loopback HTTP server.
//
// authorize.music.apple.com, when it has no window.opener, stores
// btoa(JSON{cid, itre, musicUserToken}) and navigates to
// `${referrer}#${data}`. The referrer is document.referrer, refined by the
// `referrer` query parameter when both share an origin. Sona points it at
// http://127.0.0.1:<port>/auth/<nonce>/callback; the callback page reads the
// fragment and POSTs it back to this server. See docs/auth.md §4.
package mut

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AuthorizeBase is Apple's MusicKit authorization endpoint.
const AuthorizeBase = "https://authorize.music.apple.com/woa"

// Result is what Apple hands back after sign-in.
type Result struct {
	MusicUserToken string
	CID            string // consumer ID
	Restricted     bool   // itre: explicit content restricted
}

// String keeps the token out of logs.
func (r Result) String() string {
	return fmt.Sprintf("mut(len=%d restricted=%v)", len(r.MusicUserToken), r.Restricted)
}

// Errors reported by Session.Wait.
var (
	ErrTimeout  = errors.New("sign-in timed out")
	ErrCanceled = errors.New("sign-in canceled")
)

// fragmentRe matches the hash format MusicKit JS itself parses
// (_processLocationHash in musickit.js v3).
var fragmentRe = regexp.MustCompile(`^#?([A-Za-z0-9+/]{200,}={0,2})$`)

// DecodeFragment decodes the URL fragment Apple appends to the referrer.
func DecodeFragment(fragment string) (Result, error) {
	m := fragmentRe.FindStringSubmatch(strings.TrimSpace(fragment))
	if m == nil {
		return Result{}, errors.New("fragment: not an Apple authorization payload")
	}
	raw, err := base64.StdEncoding.DecodeString(m[1])
	if err != nil {
		return Result{}, fmt.Errorf("fragment: %w", err)
	}
	var p struct {
		CID            json.RawMessage `json:"cid"`
		ITRE           json.RawMessage `json:"itre"`
		MusicUserToken string          `json:"musicUserToken"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return Result{}, fmt.Errorf("fragment: %w", err)
	}
	if p.MusicUserToken == "" {
		return Result{}, errors.New("fragment: no musicUserToken")
	}
	return Result{
		MusicUserToken: p.MusicUserToken,
		CID:            scalar(p.CID),
		Restricted:     truthy(p.ITRE),
	}, nil
}

// scalar renders a JSON string or number as text.
func scalar(v json.RawMessage) string {
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	if string(v) == "null" {
		return ""
	}
	return string(v)
}

// truthy accepts "1", 1 and true (Apple uses the string "1").
func truthy(v json.RawMessage) bool {
	switch strings.Trim(string(v), `"`) {
	case "1", "true":
		return true
	}
	return false
}

// AuthorizeURL builds the authorize URL MusicKit JS would open.
func AuthorizeURL(devToken, appName, iconURL, referrer string) string {
	info, _ := json.Marshal(map[string]any{
		"thirdPartyIconURL":    iconURL,
		"thirdPartyName":       appName,
		"thirdPartyParameters": map[string]any{},
		"thirdPartyToken":      devToken,
	})
	q := url.Values{
		"a":        {base64.StdEncoding.EncodeToString(info)},
		"referrer": {referrer},
		"app":      {"music"},
		"p":        {"subscribe"},
	}
	return AuthorizeBase + "?" + q.Encode()
}

// Options configures a sign-in Session.
type Options struct {
	AppName string        // shown on Apple's consent screen; default "Sona"
	IconURL string        // optional
	Timeout time.Duration // default 10 minutes
}

// Session is one sign-in attempt served on 127.0.0.1:<random port>.
type Session struct {
	devToken string
	opts     Options
	nonce    string
	origin   string // http://127.0.0.1:<port>
	srv      *http.Server
	ln       net.Listener

	once   sync.Once
	result chan Result
	closed chan struct{}
	close  sync.Once
}

// Start listens on a random loopback port and returns the session.
func Start(devToken string, opts Options) (*Session, error) {
	if devToken == "" {
		return nil, errors.New("mut: developer token required")
	}
	if opts.AppName == "" {
		opts.AppName = "Sona"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 10 * time.Minute
	}
	var nb [16]byte
	if _, err := rand.Read(nb[:]); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("mut: listen: %w", err)
	}
	s := &Session{
		devToken: devToken,
		opts:     opts,
		nonce:    hex.EncodeToString(nb[:]),
		origin:   "http://" + ln.Addr().String(),
		ln:       ln,
		result:   make(chan Result, 1),
		closed:   make(chan struct{}),
	}
	s.srv = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = s.srv.Serve(ln) }()
	return s, nil
}

// StartURL is the page to open in a browser to begin sign-in.
func (s *Session) StartURL() string { return s.origin + "/auth/" + s.nonce + "/start" }

func (s *Session) callbackURL() string { return s.origin + "/auth/" + s.nonce + "/callback" }

// Wait blocks until sign-in completes, times out, ctx ends, or Close is called.
func (s *Session) Wait(ctx context.Context) (Result, error) {
	defer s.Close()
	timer := time.NewTimer(s.opts.Timeout)
	defer timer.Stop()
	select {
	case r := <-s.result:
		return r, nil
	case <-timer.C:
		return Result{}, ErrTimeout
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-s.closed:
		return Result{}, ErrCanceled
	}
}

// Close stops the server; a pending Wait returns ErrCanceled.
func (s *Session) Close() {
	s.close.Do(func() {
		close(s.closed)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = s.srv.Shutdown(ctx)
	})
}

// Handler serves the start, callback and complete endpoints.
func (s *Session) Handler() http.Handler {
	prefix := "/auth/" + s.nonce + "/"
	host := strings.TrimPrefix(s.origin, "http://")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// DNS-rebinding defense: only answer to the literal loopback host.
		if r.Host != host {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		if !strings.HasPrefix(r.URL.Path, prefix) {
			http.NotFound(w, r)
			return
		}
		switch strings.TrimPrefix(r.URL.Path, prefix) {
		case "start":
			s.serveStart(w, r)
		case "callback":
			s.serveCallback(w, r)
		case "complete":
			s.serveComplete(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

var startTmpl = template.Must(template.New("start").Parse(`<!doctype html>
<meta charset="utf-8"><meta name="referrer" content="unsafe-url">
<title>Signing in to Apple Music…</title>
<p>Redirecting to Apple…</p>
<script nonce="{{.Nonce}}">location.replace({{.URL}});</script>`))

var callbackTmpl = template.Must(template.New("callback").Parse(`<!doctype html>
<meta charset="utf-8"><title>Sona sign-in</title>
<p id="m">Finishing sign-in…</p>
<script nonce="{{.Nonce}}">
(async () => {
  const m = document.getElementById("m");
  const fragment = location.hash;
  history.replaceState(null, "", location.pathname);
  if (!fragment) { m.textContent = "Sign-in did not return a token. Return to Sona and try again."; return; }
  try {
    const r = await fetch({{.Complete}}, {method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({fragment})});
    m.textContent = r.ok ? "Signed in. You can close this window and return to Sona." : "Sign-in failed: " + await r.text();
    if (r.ok) setTimeout(() => window.close(), 800);
  } catch (e) { m.textContent = "Sign-in failed: " + e; }
})();
</script>`))

func (s *Session) render(w http.ResponseWriter, t *template.Template, data map[string]any) {
	var nb [12]byte
	_, _ = rand.Read(nb[:])
	nonce := base64.RawURLEncoding.EncodeToString(nb[:])
	data["Nonce"] = nonce
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+nonce+"'; connect-src 'self'")
	_ = t.Execute(w, data)
}

func (s *Session) serveStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Referrer-Policy", "unsafe-url")
	s.render(w, startTmpl, map[string]any{
		"URL": AuthorizeURL(s.devToken, s.opts.AppName, s.opts.IconURL, s.callbackURL()),
	})
}

func (s *Session) serveCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Referrer-Policy", "no-referrer")
	s.render(w, callbackTmpl, map[string]any{"Complete": "/auth/" + s.nonce + "/complete"})
}

func (s *Session) serveComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Origin") != s.origin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "unsupported media type", http.StatusUnsupportedMediaType)
		return
	}
	var body struct {
		Fragment string `json:"fragment"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&body); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	res, err := DecodeFragment(body.Fragment)
	if err != nil {
		http.Error(w, "invalid authorization payload", http.StatusBadRequest)
		return
	}
	delivered := false
	s.once.Do(func() {
		s.result <- res
		delivered = true
	})
	if !delivered {
		http.Error(w, "already completed", http.StatusGone)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
