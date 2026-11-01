// Package mut obtains an Apple Music User Token through Apple's no-opener
// redirect flow and a single-use loopback HTTP server.
//
// authorize.music.apple.com, when it has no window.opener, stores
// btoa(JSON{cid, itre, musicUserToken}) and navigates to
// `${referrer}#${data}`. The referrer is document.referrer, refined by the
// `referrer` query parameter when both share an origin. Sona points it at
// http://127.0.0.1:<port>/auth/<nonce>/callback; the callback page reads the
// fragment and POSTs it back to this server. See docs/auth.md §4.
//
// The nonce travels in the URL Sona hands to the browser, so it is not a
// secret from other local users. Two controls bind the attempt to the
// browser Sona opened: each start link works once and sets a cookie with a
// random value that never appears in a URL, which /complete requires; and
// on Linux the listener drops connections from sockets another user owns
// (ownerOnly). See docs/auth.md §4.5.
package mut

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"math"
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
	// ErrStartReused means a start link was loaded a second time. Each link
	// works once, and Sona's pages never load one twice, so the link has
	// reached someone else; the attempt ends and the user starts again.
	ErrStartReused = errors.New("the sign-in link was opened twice; start the sign-in again")
)

// startCookie names the cookie /start sets and /complete requires.
const startCookie = "sona_signin"

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
	key      string    // value of the cookie /start gives each browser
	origin   string    // http://127.0.0.1:<port>
	deadline time.Time // the attempt ends here (Options.Timeout from Start)
	srv      *http.Server
	ln       net.Listener

	mu      sync.Mutex
	tickets map[string]bool // start tickets StartURL issued; true once used
	ended   bool            // a result was delivered or the attempt failed
	result  chan Result
	failed  chan error
	closed  chan struct{}
	close   sync.Once
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
	var kb [32]byte
	if _, err := rand.Read(nb[:]); err != nil {
		return nil, err
	}
	if _, err := rand.Read(kb[:]); err != nil {
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
		key:      base64.RawURLEncoding.EncodeToString(kb[:]),
		origin:   "http://" + ln.Addr().String(),
		deadline: time.Now().Add(opts.Timeout),
		ln:       ln,
		tickets:  make(map[string]bool),
		result:   make(chan Result, 1),
		failed:   make(chan error, 1),
		closed:   make(chan struct{}),
	}
	s.srv = &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() { _ = s.srv.Serve(ownerOnly(ln)) }()
	return s, nil
}

// StartURL returns a new start link for this attempt. Each link carries its
// own random ticket and works once: Sona opens one in the browser, and the
// UI asks for another to open the page again or copy it to a different
// browser.
func (s *Session) StartURL() string {
	ticket := rand.Text()
	s.mu.Lock()
	s.tickets[ticket] = false
	s.mu.Unlock()
	return s.origin + "/auth/" + s.nonce + "/start?t=" + ticket
}

func (s *Session) callbackURL() string { return s.origin + "/auth/" + s.nonce + "/callback" }

// Wait blocks until sign-in completes, fails, reaches the deadline, ctx
// ends, or Close is called. It always ends the attempt: afterwards every
// endpoint refuses, cookie or not.
func (s *Session) Wait(ctx context.Context) (Result, error) {
	defer s.Close()
	timer := time.NewTimer(time.Until(s.deadline))
	defer timer.Stop()
	select {
	case r := <-s.result:
		return r, nil
	case err := <-s.failed:
		return Result{}, err
	case <-timer.C:
		return Result{}, ErrTimeout
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-s.closed:
		return Result{}, ErrCanceled
	}
}

// Close ends the attempt and stops the server; a pending Wait returns
// ErrCanceled.
func (s *Session) Close() {
	s.close.Do(func() {
		s.mu.Lock()
		s.ended = true
		s.mu.Unlock()
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

// overLocked reports whether the attempt has ended. s.mu must be held.
func (s *Session) overLocked() bool { return s.ended || !time.Now().Before(s.deadline) }

// cookie is the attempt's cookie with value v and lifetime maxAge seconds
// (negative deletes it). It is host-only (no Domain), scoped to this
// attempt's paths, hidden from scripts, and SameSite=Strict: the only
// request that needs it is the callback page's same-origin POST to
// /complete. Apple's cross-site redirect to /callback arrives without it,
// which is fine because /callback receives nothing (the token stays in
// the fragment). There is no Secure attribute or __Host- prefix, as both
// need https.
func (s *Session) cookie(v string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     startCookie,
		Value:    v,
		Path:     "/auth/" + s.nonce + "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

// serveStart redeems a start ticket: it hands the browser the attempt's
// cookie and sends it on to Apple. Each ticket works once. A ticket used a
// second time ends the attempt (ErrStartReused): Sona's own pages never
// load one twice (the start page replaces itself), so a repeat means the
// link reached someone else, who must get nothing to complete with.
func (s *Session) serveStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ticket := r.URL.Query().Get("t")
	s.mu.Lock()
	over := s.overLocked()
	used, known := s.tickets[ticket]
	if known && !over {
		s.tickets[ticket] = true
		s.ended = used
	}
	s.mu.Unlock()
	switch {
	case over:
		http.Error(w, "This sign-in attempt has ended. Return to Sona and start again.", http.StatusGone)
		return
	case !known:
		http.NotFound(w, r)
		return
	case used:
		select {
		case s.failed <- ErrStartReused:
		default:
		}
		http.Error(w, "This sign-in link has already been used. Return to Sona and start again.", http.StatusGone)
		return
	}
	http.SetCookie(w, s.cookie(s.key, max(1, int(math.Ceil(time.Until(s.deadline).Seconds())))))
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

// hasKey reports whether r carries the attempt's cookie (constant-time).
func (s *Session) hasKey(r *http.Request) bool {
	for _, c := range r.CookiesNamed(startCookie) {
		if subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.key)) == 1 {
			return true
		}
	}
	return false
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
	s.mu.Lock()
	over := s.overLocked()
	s.mu.Unlock()
	if over {
		http.Error(w, "already completed", http.StatusGone)
		return
	}
	if !s.hasKey(r) {
		http.Error(w, "this browser did not start the sign-in", http.StatusForbidden)
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
	s.mu.Lock()
	over = s.overLocked()
	s.ended = true
	s.mu.Unlock()
	if over {
		http.Error(w, "already completed", http.StatusGone)
		return
	}
	s.result <- res
	http.SetCookie(w, s.cookie("", -1))
	w.WriteHeader(http.StatusNoContent)
}
