package lyrics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Track is the song lyrics are wanted for. CatalogID (Apple's) and ISRC
// match exactly where a provider knows them; the other providers match
// title, artist and album, with the duration within durationTolerance.
type Track struct {
	CatalogID string
	ISRC      string
	Title     string
	Artist    string
	Album     string
	Duration  time.Duration
}

// Key identifies the track in the cache: its catalog ID, or a hash of its
// metadata when it has none.
func (t Track) Key() string {
	if t.CatalogID != "" {
		return "am:" + t.CatalogID
	}
	h := sha256.Sum256([]byte(strings.Join([]string{
		normalize(t.Title), normalize(t.Artist), normalize(t.Album), strconv.FormatInt(int64(t.Duration/time.Second), 10),
	}, "\x00")))
	return "meta:" + hex.EncodeToString(h[:12])
}

// Request asks for one track's lyrics. Language is the BCP 47 tag
// translations are wanted in ("" for none).
type Request struct {
	Track
	Language string
}

// Provider is one source of lyrics.
type Provider interface {
	// ID is the provider's config.Lyrics* ID.
	ID() string
	// Localized reports whether the answer depends on Request.Language
	// (the provider returns translations), which keys its cache entries.
	Localized() bool
	// Fetch returns the track's lyrics, an error matching ErrNotFound when
	// the provider has none (or only for an instrumental), ErrUnavailable
	// when it can't be asked now, or a *RateLimitError.
	Fetch(ctx context.Context, req Request) (Lyrics, error)
}

var (
	// ErrNotFound means the provider has no lyrics for the track.
	ErrNotFound = errors.New("lyrics: not found")
	// ErrUnavailable means the provider can't be asked now (signed out,
	// or backing off after failures); nothing is cached.
	ErrUnavailable = errors.New("lyrics: provider unavailable")
)

// RateLimitError is a provider asking Sona to slow down (HTTP 429 or 503,
// a captcha); RetryAfter is how long it asked for (0 when it didn't say).
type RateLimitError struct {
	Provider   string
	RetryAfter time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("lyrics: %s is rate limiting (retry after %s)", e.Provider, e.RetryAfter)
}

// maxBody caps a provider response.
const maxBody = 8 << 20

// HTTPSOnly is the providers' redirect policy (http.Client.CheckRedirect):
// a redirect must stay on https, at most 5 times.
func HTTPSOnly(req *http.Request, via []*http.Request) error {
	if req.URL.Scheme != "https" {
		return fmt.Errorf("lyrics: refused a redirect to %s", req.URL.Scheme)
	}
	if len(via) >= 5 {
		return errors.New("lyrics: too many redirects")
	}
	return nil
}

// getJSON GETs u and decodes the JSON body into v. 404 is ErrNotFound,
// 429 and 503 a *RateLimitError.
func getJSON(ctx context.Context, c *http.Client, provider, u string, header http.Header, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	for k, vs := range header {
		req.Header[k] = vs
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("lyrics: %s: %w", provider, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s answered 404", ErrNotFound, provider)
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable:
		return &RateLimitError{Provider: provider, RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	case resp.StatusCode/100 != 2:
		return fmt.Errorf("lyrics: %s: HTTP %d", provider, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(v); err != nil {
		return fmt.Errorf("lyrics: %s: decode: %w", provider, err)
	}
	return nil
}

// retryAfter parses a Retry-After header in seconds (dates are rare
// enough from these providers to be read as "not said").
func retryAfter(h string) time.Duration {
	n, err := strconv.Atoi(strings.TrimSpace(h))
	if err != nil || n < 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}
