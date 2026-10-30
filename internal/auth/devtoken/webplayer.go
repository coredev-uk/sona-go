// Package devtoken provides Apple Music developer tokens.
package devtoken

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

// WebPlayerOrigin is the origin Apple requires on every request made with the
// web player token (its JWT carries root_https_origin: ["apple.com"]).
const WebPlayerOrigin = "https://music.apple.com"

// Token is a developer token and its expiry.
type Token struct {
	JWT     string
	Expires time.Time
}

// String keeps the JWT out of logs.
func (t Token) String() string { return "devtoken(exp=" + t.Expires.UTC().Format(time.RFC3339) + ")" }

// Claims is the subset of JWT claims Sona inspects.
type Claims struct {
	Issuer          string   `json:"iss"`
	IssuedAt        int64    `json:"iat"`
	ExpiresAt       int64    `json:"exp"`
	RootHTTPSOrigin []string `json:"root_https_origin"`
}

var (
	entryScriptRe = regexp.MustCompile(`src="(/assets/index~[A-Za-z0-9]+\.js)"`)
	jwtRe         = regexp.MustCompile(`"(eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+)"`)
)

// WebPlayer extracts the token embedded in the music.apple.com web player bundle.
// It caches the token and refetches it a week before expiry or after Invalidate.
type WebPlayer struct {
	HTTP     *http.Client
	PageURL  string // defaults to https://music.apple.com/us/browse
	UA       string
	mu       sync.Mutex
	cached   Token
	minValid time.Duration
}

// NewWebPlayer returns a provider using client (http.DefaultClient if nil).
func NewWebPlayer(client *http.Client) *WebPlayer {
	if client == nil {
		client = http.DefaultClient
	}
	return &WebPlayer{
		HTTP:     client,
		PageURL:  WebPlayerOrigin + "/us/browse",
		UA:       "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/156.0.0.0 Safari/537.36",
		minValid: 7 * 24 * time.Hour,
	}
}

// ID identifies the provider; stored alongside the Music User Token.
func (p *WebPlayer) ID() string { return "webplayer" }

// Invalidate drops the cached token so the next Token call refetches it.
func (p *WebPlayer) Invalidate() {
	p.mu.Lock()
	p.cached = Token{}
	p.mu.Unlock()
}

// Token returns a cached token or extracts a fresh one.
func (p *WebPlayer) Token(ctx context.Context) (Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cached.JWT != "" && time.Until(p.cached.Expires) > p.minValid {
		return p.cached, nil
	}
	tok, err := p.fetch(ctx)
	if err != nil {
		return Token{}, err
	}
	p.cached = tok
	return tok, nil
}

func (p *WebPlayer) fetch(ctx context.Context) (Token, error) {
	page, err := p.get(ctx, p.PageURL)
	if err != nil {
		return Token{}, fmt.Errorf("webplayer page: %w", err)
	}
	m := entryScriptRe.FindStringSubmatch(page)
	if m == nil {
		return Token{}, errors.New("webplayer: entry script not found in page")
	}
	js, err := p.get(ctx, WebPlayerOrigin+m[1])
	if err != nil {
		return Token{}, fmt.Errorf("webplayer bundle: %w", err)
	}
	for _, cand := range jwtRe.FindAllStringSubmatch(js, -1) {
		c, err := ParseClaims(cand[1])
		if err != nil || c.Issuer != "AMPWebPlay" {
			continue
		}
		return Token{JWT: cand[1], Expires: time.Unix(c.ExpiresAt, 0)}, nil
	}
	return Token{}, errors.New("webplayer: no AMPWebPlay token in bundle")
}

func (p *WebPlayer) get(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", p.UA)
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return string(b), err
}

// ParseClaims decodes (without verifying) the payload of a JWT.
func ParseClaims(jwt string) (Claims, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return Claims{}, errors.New("jwt: want 3 segments")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, fmt.Errorf("jwt payload: %w", err)
	}
	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return Claims{}, fmt.Errorf("jwt payload: %w", err)
	}
	return c, nil
}
