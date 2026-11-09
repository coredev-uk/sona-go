//go:build linux || (cgo && (darwin || windows))

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// licenceTap watches the licence exchanges of the Apple client (its
// http.RoundTripper) so playspike can say whether Apple granted or refused
// a licence, and why: the HTTP status, Apple's status and message, and the
// size of the licence. It never keeps or prints the request (it carries
// the Music User Token) or the licence bytes.
type licenceTap struct {
	next http.RoundTripper

	mu        sync.Mutex
	exchanges []licenceExchange
	// drm counts requests for full-track playback: webPlayback and the
	// Widevine service certificate (a preview run must make none).
	drm int
}

// licenceExchange is one answer from Apple's licence server.
type licenceExchange struct {
	HTTPStatus  int    // 0 when the request failed before an answer
	HTTPText    string // e.g. "403 Forbidden"
	AppleStatus int    // "status" (or "errorCode") in Apple's JSON; 0 = success
	Message     string // Apple's "message"
	Licence     int    // length of the base64 "license"; 0 when absent
	Body        string // a non-JSON or error body, trimmed to bodySummary bytes
	Err         string // transport error
}

const bodySummary = 200

func newLicenceTap() *licenceTap { return &licenceTap{next: http.DefaultTransport} }

// isLicence reports whether r goes to a licence endpoint
// (acquireWebPlaybackLicense, or the URL webPlayback names).
func isLicence(r *http.Request) bool {
	return r.Method == http.MethodPost && strings.Contains(strings.ToLower(r.URL.Path), "license")
}

func (t *licenceTap) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.next.RoundTrip(r)
	if p := strings.ToLower(r.URL.Path); strings.Contains(p, "webplayback") || strings.Contains(p, "widevine") {
		t.mu.Lock()
		t.drm++
		t.mu.Unlock()
	}
	if !isLicence(r) {
		return resp, err
	}
	var x licenceExchange
	if err != nil {
		x.Err = err.Error()
		t.record(x)
		return resp, err
	}
	body, rerr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	resp.Body = io.NopCloser(bytes.NewReader(body))
	x.HTTPStatus, x.HTTPText = resp.StatusCode, resp.Status
	var j struct {
		Status    int    `json:"status"`
		ErrorCode int    `json:"errorCode"`
		Message   string `json:"message"`
		License   string `json:"license"`
	}
	if json.Unmarshal(body, &j) == nil {
		x.AppleStatus, x.Message, x.Licence = j.Status, j.Message, len(j.License)
		if j.ErrorCode != 0 {
			x.AppleStatus = j.ErrorCode
		}
	} else {
		x.Body = summarize(body)
	}
	if rerr != nil {
		x.Err = rerr.Error()
	}
	t.record(x)
	return resp, nil
}

func (t *licenceTap) record(x licenceExchange) {
	t.mu.Lock()
	t.exchanges = append(t.exchanges, x)
	n := len(t.exchanges)
	t.mu.Unlock()
	kv := []any{"n", n, "http", x.HTTPStatus, "apple_status", x.AppleStatus, "licence_chars", x.Licence}
	if x.Message != "" {
		kv = append(kv, "message", fmt.Sprintf("%q", x.Message))
	}
	if x.Body != "" {
		kv = append(kv, "body", fmt.Sprintf("%q", x.Body))
	}
	if x.Err != "" {
		kv = append(kv, "err", fmt.Sprintf("%q", x.Err))
	}
	step("licence exchange", kv...)
}

// requests returns how many licence exchanges and other full-track
// playback requests (drm) went out.
func (t *licenceTap) requests() (licences, drm int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.exchanges), t.drm
}

// summarize collapses whitespace and keeps the first bodySummary bytes.
func summarize(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > bodySummary {
		s = s[:bodySummary] + "…"
	}
	return s
}

// granted reports whether the exchange delivered a licence.
func (x licenceExchange) granted() bool {
	return x.Err == "" && x.HTTPStatus/100 == 2 && x.AppleStatus == 0 && x.Licence > 0
}

// refusal says why Apple did not deliver a licence.
func (x licenceExchange) refusal() string {
	switch {
	case x.Err != "":
		return "no answer from Apple's licence server: " + x.Err
	case x.HTTPStatus/100 != 2:
		s := "HTTP " + x.HTTPText
		if x.Message != "" {
			s += ": " + x.Message
		} else if x.Body != "" {
			s += ": " + x.Body
		}
		return s
	case x.AppleStatus != 0:
		return fmt.Sprintf("Apple status %d: %s", x.AppleStatus, x.Message)
	}
	return "Apple's answer held no licence"
}

// verdict is playspike's last line: whether Apple licensed this host
// ("licence: granted"), refused it ("licence: refused (<why>)"), or the
// run never got to ask ("licence: not requested (<why>)"). err is the
// run's error.
func (t *licenceTap) verdict(err error) string {
	t.mu.Lock()
	xs := append([]licenceExchange(nil), t.exchanges...)
	t.mu.Unlock()
	for _, x := range xs {
		if !x.granted() {
			return "licence: refused (" + x.refusal() + ")"
		}
	}
	switch {
	case len(xs) == 0 && err != nil:
		return "licence: not requested (" + err.Error() + ")"
	case len(xs) == 0:
		return "licence: not requested"
	case errors.Is(err, cdmhost.ErrNoUsableKey):
		// Apple answered with a licence, but it unlocked nothing.
		return "licence: refused (Apple sent a licence, but the CDM made no key usable: " + err.Error() + ")"
	case err != nil && strings.Contains(err.Error(), "UpdateSession rejected"):
		return "licence: refused (the CDM rejected Apple's licence: " + err.Error() + ")"
	case err != nil:
		return "licence: granted (playback failed after it: " + err.Error() + ")"
	}
	return "licence: granted"
}
