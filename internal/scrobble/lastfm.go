package scrobble

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Last.fm endpoints.
const (
	LastFMAPI  = "https://ws.audioscrobbler.com/2.0/"
	LastFMAuth = "https://www.last.fm/api/auth/"
)

// Sona's Last.fm API account, injected at build time:
//
//	-ldflags "-X github.com/coredev-uk/sona-go/internal/scrobble.lastfmKey=… -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmSecret=…"
//
// (production builds of build/linux/Taskfile.yml pass SONA_LASTFM_KEY and
// SONA_LASTFM_SECRET, with -trimpath so -ldflags stays out of the build
// info). A build without them, development builds included, reads the same
// variables from the environment.
var lastfmKey, lastfmSecret string

// LastFMAccount returns the API key and secret built in (or from the
// environment); ok is false when either is missing.
func LastFMAccount() (key, secret string, ok bool) {
	key, secret = lastfmKey, lastfmSecret
	if key == "" || secret == "" {
		key, secret = os.Getenv("SONA_LASTFM_KEY"), os.Getenv("SONA_LASTFM_SECRET")
	}
	return key, secret, key != "" && secret != ""
}

// LastFM is a Last.fm API 2.0 client for desktop authentication and
// scrobbling. Every call is signed.
type LastFM struct {
	Key, Secret string
	API         string // default LastFMAPI
	AuthBase    string // default LastFMAuth
	HTTP        *http.Client
	UserAgent   string // default "Sona"
}

// Session is an authorised Last.fm session.
type Session struct {
	Name string `json:"name"` // the user's Last.fm username
	Key  string `json:"key"`  // the session key; valid until the user revokes it
}

// LastFMError is an error response from the API.
type LastFMError struct {
	Code    int
	Message string
}

func (e *LastFMError) Error() string { return fmt.Sprintf("last.fm error %d: %s", e.Code, e.Message) }

// Is classifies API errors (https://www.last.fm/api/errorcodes):
// authentication and API-key failures are ErrAuth, an unapproved token is
// ErrPending, an expired one ErrExpired, and bad parameters or signatures
// are ErrRejected. Service-side failures (8, 11, 16, 29) match none and are
// retried.
func (e *LastFMError) Is(target error) bool {
	switch target {
	case ErrAuth:
		return e.Code == 4 || e.Code == 9 || e.Code == 10 || e.Code == 26
	case ErrPending:
		return e.Code == 14
	case ErrExpired:
		return e.Code == 15
	case ErrRejected:
		switch e.Code {
		case 4, 8, 9, 10, 11, 14, 15, 16, 26, 29:
			return false
		}
		return true
	}
	return false
}

// Name is the provider name.
func (c *LastFM) Name() string { return "lastfm" }

// MaxBatch is track.scrobble's limit.
func (c *LastFM) MaxBatch() int { return 50 }

// Sign returns api_sig for params: every name and value (except format,
// callback and api_sig) concatenated in name order, then the secret, MD5 in
// hex.
func Sign(params url.Values, secret string) string {
	names := make([]string, 0, len(params))
	for k := range params {
		if k != "format" && k != "callback" && k != "api_sig" {
			names = append(names, k)
		}
	}
	sort.Strings(names)
	h := md5.New()
	for _, k := range names {
		_, _ = io.WriteString(h, k)
		_, _ = io.WriteString(h, params.Get(k))
	}
	_, _ = io.WriteString(h, secret)
	return hex.EncodeToString(h.Sum(nil))
}

// Token starts desktop authentication (auth.getToken).
func (c *LastFM) Token(ctx context.Context) (string, error) {
	var out struct {
		Token string `json:"token"`
	}
	if err := c.call(ctx, http.MethodGet, "auth.getToken", url.Values{}, &out); err != nil {
		return "", err
	}
	if out.Token == "" {
		return "", errors.New("last.fm: auth.getToken returned no token")
	}
	return out.Token, nil
}

// AuthURL is the page where the user approves token for Sona.
func (c *LastFM) AuthURL(token string) string {
	base := c.AuthBase
	if base == "" {
		base = LastFMAuth
	}
	return base + "?" + url.Values{"api_key": {c.Key}, "token": {token}}.Encode()
}

// Session exchanges an approved token for a session (auth.getSession). It
// returns an error matching ErrPending while the user has not approved it.
func (c *LastFM) Session(ctx context.Context, token string) (Session, error) {
	var out struct {
		Session Session `json:"session"`
	}
	if err := c.call(ctx, http.MethodGet, "auth.getSession", url.Values{"token": {token}}, &out); err != nil {
		return Session{}, err
	}
	if out.Session.Key == "" {
		return Session{}, errors.New("last.fm: auth.getSession returned no session")
	}
	return out.Session, nil
}

// AwaitSession polls Session every interval until the user approves token,
// the token expires or ctx ends.
func (c *LastFM) AwaitSession(ctx context.Context, token string, interval time.Duration) (Session, error) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s, err := c.Session(ctx, token)
		if !errors.Is(err, ErrPending) {
			return s, err
		}
		select {
		case <-ctx.Done():
			return Session{}, ctx.Err()
		case <-t.C:
		}
	}
}

// NowPlaying announces t (track.updateNowPlaying).
func (c *LastFM) NowPlaying(ctx context.Context, sk string, t Track) error {
	p := url.Values{"sk": {sk}, "artist": {t.Artist}, "track": {t.Title}}
	if t.Album != "" {
		p.Set("album", t.Album)
	}
	if s := int(t.Duration / time.Second); s > 0 {
		p.Set("duration", strconv.Itoa(s))
	}
	return c.call(ctx, http.MethodPost, "track.updateNowPlaying", p, nil)
}

// Submit scrobbles up to MaxBatch listens (track.scrobble). Listens the
// service ignores (too old, filtered) are not an error.
func (c *LastFM) Submit(ctx context.Context, sk string, ls []Listen) error {
	if len(ls) == 0 {
		return nil
	}
	if len(ls) > c.MaxBatch() {
		return fmt.Errorf("last.fm: %d listens exceed the batch limit", len(ls))
	}
	p := url.Values{"sk": {sk}}
	for i, l := range ls {
		n := "[" + strconv.Itoa(i) + "]"
		p.Set("artist"+n, l.Artist)
		p.Set("track"+n, l.Title)
		p.Set("timestamp"+n, strconv.FormatInt(l.ListenedAt.Unix(), 10))
		if l.Album != "" {
			p.Set("album"+n, l.Album)
		}
		if s := int(l.Duration / time.Second); s > 0 {
			p.Set("duration"+n, strconv.Itoa(s))
		}
	}
	return c.call(ctx, http.MethodPost, "track.scrobble", p, nil)
}

// call signs and sends one API method and decodes the JSON reply into out.
func (c *LastFM) call(ctx context.Context, method, name string, p url.Values, out any) error {
	p.Set("method", name)
	p.Set("api_key", c.Key)
	p.Set("api_sig", Sign(p, c.Secret))
	p.Set("format", "json")
	api := c.API
	if api == "" {
		api = LastFMAPI
	}
	var (
		req *http.Request
		err error
	)
	if method == http.MethodGet {
		req, err = http.NewRequestWithContext(ctx, method, api+"?"+p.Encode(), nil)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, api, strings.NewReader(p.Encode()))
	}
	if err != nil {
		return err
	}
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	req.Header.Set("User-Agent", agent(c.UserAgent))
	resp, err := httpClient(c.HTTP).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var e struct {
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &e) != nil {
		return &StatusError{Service: "last.fm", Code: resp.StatusCode}
	}
	if e.Error != 0 {
		return &LastFMError{Code: e.Error, Message: e.Message}
	}
	if resp.StatusCode/100 != 2 {
		return &StatusError{Service: "last.fm", Code: resp.StatusCode}
	}
	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("last.fm: decode %s: %w", name, err)
		}
	}
	return nil
}

// StatusError is an HTTP failure without a usable API error. 4xx statuses
// other than 401, 403, 408 and 429 match ErrRejected; the rest are retried.
type StatusError struct {
	Service string
	Code    int
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s: HTTP %d", e.Service, e.Code) }

// Is classifies the status (see StatusError).
func (e *StatusError) Is(target error) bool {
	switch target {
	case ErrAuth:
		return e.Code == http.StatusUnauthorized || e.Code == http.StatusForbidden
	case ErrRejected:
		return e.Code >= 400 && e.Code < 500 && e.Code != http.StatusUnauthorized &&
			e.Code != http.StatusForbidden && e.Code != http.StatusRequestTimeout && e.Code != http.StatusTooManyRequests
	}
	return false
}

func agent(ua string) string {
	if ua == "" {
		return "Sona"
	}
	return ua
}

func httpClient(c *http.Client) *http.Client {
	if c == nil {
		return http.DefaultClient
	}
	return c
}
