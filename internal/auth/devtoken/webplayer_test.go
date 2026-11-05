package devtoken

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func body(s string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(s)), Header: http.Header{}}
}

// The web player's script is megabytes long and the token is a few hundred
// bytes of it: the cached token must be a copy, or the process holds the whole
// script for as long as it holds the token (it read 3 MB of it on every
// start).
func TestTokenDoesNotKeepTheBundleItWasFoundIn(t *testing.T) {
	enc := base64.RawURLEncoding.EncodeToString
	exp := time.Now().Add(30 * 24 * time.Hour).Unix()
	jwt := enc([]byte(`{"alg":"ES256"}`)) + "." + enc([]byte(fmt.Sprintf(`{"iss":"AMPWebPlay","exp":%d}`, exp))) + "." + enc([]byte("signature"))
	// Large enough to tell from anything else the test allocates.
	script := strings.Repeat("var a=1;", 1<<20) + `token:"` + jwt + `";` + strings.Repeat("var b=2;", 1<<20)
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if strings.HasSuffix(r.URL.Path, ".js") {
			return body(script), nil
		}
		return body(`<script src="/assets/index~abc123.js"></script>`), nil
	})}
	p := NewWebPlayer(client)

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	tok, err := p.Token(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if tok.JWT != jwt {
		t.Fatalf("token %q, want %q", tok.JWT, jwt)
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	if grew := int64(after.HeapAlloc) - int64(before.HeapAlloc); grew > 4<<20 {
		t.Errorf("the heap grew by %d bytes holding a %d-byte token: it keeps the 16 MB bundle alive", grew, len(tok.JWT))
	}
	runtime.KeepAlive(p)
}
