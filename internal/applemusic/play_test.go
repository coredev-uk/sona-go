package applemusic

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// rewrite sends every request to srv, keeping the path, so the absolute
// play.itunes.apple.com URLs reach the fixture.
type rewrite struct{ srv *httptest.Server }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(r.srv.URL)
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = u.Scheme, u.Host
	return http.DefaultTransport.RoundTrip(req)
}

type playRequest struct {
	path   string
	header http.Header
	body   map[string]any
}

func playClient(t *testing.T, status int, response string) (*Client, *[]playRequest) {
	t.Helper()
	var seen []playRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		pr := playRequest{path: r.URL.Path, header: r.Header.Clone()}
		if len(b) > 0 {
			if err := json.Unmarshal(b, &pr.body); err != nil {
				t.Errorf("request body %q: %v", b, err)
			}
		}
		seen = append(seen, pr)
		w.WriteHeader(status)
		io.WriteString(w, response)
	}))
	t.Cleanup(srv.Close)
	return New(&http.Client{Transport: rewrite{srv}}, &fakeDev{}, func() string { return "mut" }), &seen
}

const songList = `{"songList":[{"songId":1109715168,
 "hls-key-server-url":"https://play.itunes.apple.com/WebObjects/MZPlay.woa/wa/acquireWebPlaybackLicense",
 "widevine-cert-url":"https://play.itunes.apple.com/WebObjects/MZPlay.woa/wa/widevineCert",
 "assets":[{"flavor":"30:cbcp256","URL":"https://aod/c.m3u8"},{"flavor":"34:cbcp64","URL":"https://aod/cl.m3u8"},
 %s]}],"status":0}`

func TestWebPlaybackSelectsWidevineFlavor(t *testing.T) {
	c, seen := playClient(t, 200, fmt.Sprintf(songList, `{"flavor":"32:ctrp64","URL":"https://aod/r64.m3u8"},{"flavor":"28:ctrp256","URL":"https://aod/r256.m3u8"}`))
	info, err := c.WebPlayback(context.Background(), "1109715168", Quality256)
	if err != nil {
		t.Fatal(err)
	}
	if info.SongID != "1109715168" || info.Flavor != "28:ctrp256" || info.PlaylistURL != "https://aod/r256.m3u8" || !info.Protected() {
		t.Fatalf("info %+v", info)
	}
	if info.LicenseURL != LicenseURL || info.CertURL != WidevineCertURL || len(info.Assets) != 4 {
		t.Fatalf("urls %+v", info)
	}
	r := (*seen)[0]
	if r.path != "/WebObjects/MZPlay.woa/wa/webPlayback" || r.body["salableAdamId"] != "1109715168" {
		t.Fatalf("request %+v", r)
	}
	if r.header.Get(headerAppleMUT) != "mut" || r.header.Get("Media-User-Token") != "mut" || r.header.Get("Origin") != "https://music.apple.com" ||
		r.header.Get("Authorization") != "Bearer dev-0" {
		t.Fatalf("headers %v", r.header)
	}
}

func TestWebPlaybackHonoursQuality(t *testing.T) {
	both := fmt.Sprintf(songList, `{"flavor":"28:ctrp256","URL":"https://aod/r256.m3u8"},{"flavor":"32:ctrp64","URL":"https://aod/r64.m3u8"}`)
	c, _ := playClient(t, 200, both)
	info, err := c.WebPlayback(context.Background(), "1", Quality64)
	if err != nil || info.Flavor != "32:ctrp64" || info.PlaylistURL != "https://aod/r64.m3u8" {
		t.Fatalf("64 kbps: %+v %v", info, err)
	}
	// The other bitrate stands in when the requested one is missing.
	c, _ = playClient(t, 200, fmt.Sprintf(songList, `{"flavor":"32:ctrp64","URL":"https://aod/r64.m3u8"}`))
	info, err = c.WebPlayback(context.Background(), "1", Quality256)
	if err != nil || info.Flavor != "32:ctrp64" || info.PlaylistURL != "https://aod/r64.m3u8" {
		t.Fatalf("256 falls back to 64: %+v %v", info, err)
	}
	c, _ = playClient(t, 200, fmt.Sprintf(songList, `{"flavor":"28:ctrp256","URL":"https://aod/r256.m3u8"}`))
	info, err = c.WebPlayback(context.Background(), "1", Quality64)
	if err != nil || info.Flavor != "28:ctrp256" {
		t.Fatalf("64 falls back to 256: %+v %v", info, err)
	}
}

func TestWebPlaybackNotPlayable(t *testing.T) {
	c, _ := playClient(t, 200, fmt.Sprintf(songList, `{"flavor":"37:ibhp256","URL":"https://aod/i.m3u8"}`))
	if _, err := c.WebPlayback(context.Background(), "1", Quality256); !errors.Is(err, ErrNotPlayable) {
		t.Fatalf("FairPlay-only song: %v", err)
	}
	c, _ = playClient(t, 200, `{"failureType":"3076","customerMessage":"This item is not available."}`)
	_, err := c.WebPlayback(context.Background(), "1", Quality256)
	var wpe *WebPlaybackError
	if !errors.As(err, &wpe) || wpe.FailureType != "3076" || !errors.Is(err, ErrNotPlayable) {
		t.Fatalf("failure answer: %v", err)
	}
}

func TestWebPlaybackLibraryUpload(t *testing.T) {
	c, seen := playClient(t, 200, `{"songList":[{"songId":-1,"assets":[{"flavor":"","URL":"https://upload/file.m4a"}]}]}`)
	info, err := c.WebPlayback(context.Background(), "i.abc", Quality256)
	if err != nil {
		t.Fatal(err)
	}
	if info.Protected() || !info.IsLibrary || info.PlaylistURL != "https://upload/file.m4a" {
		t.Fatalf("info %+v", info)
	}
	if b := (*seen)[0].body; b["universalLibraryId"] != "i.abc" || b["isLibrary"] != true || b["salableAdamId"] != nil {
		t.Fatalf("body %v", b)
	}
}

func TestWebPlaybackForbidden(t *testing.T) {
	c, _ := playClient(t, http.StatusForbidden, `{"errors":[]}`)
	_, err := c.WebPlayback(context.Background(), "1", Quality256)
	var se *StatusError
	if !errors.Is(err, ErrUnauthorized) || !errors.As(err, &se) || se.Code != 403 {
		t.Fatalf("403: %v", err)
	}
}

func TestAcquireLicense(t *testing.T) {
	lic := []byte{1, 2, 3, 4}
	c, seen := playClient(t, 200, `{"license":"`+base64.StdEncoding.EncodeToString(lic)+`","status":0,"stkn":"slot-1"}`)
	got, err := c.AcquireLicense(context.Background(), LicenseRequest{Challenge: []byte("challenge"), KeyURI: "data:;base64,AAAA", AdamID: "1109715168"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Data, lic) || got.SlotToken != "slot-1" {
		t.Fatalf("licence %+v", got)
	}
	r := (*seen)[0]
	if r.path != "/WebObjects/MZPlay.woa/wa/acquireWebPlaybackLicense" || r.header.Get(headerAppleRenewal) != "true" || r.header.Get(headerAppleMUT) != "mut" {
		t.Fatalf("request %+v", r)
	}
	want := map[string]any{
		"challenge": base64.StdEncoding.EncodeToString([]byte("challenge")), "uri": "data:;base64,AAAA",
		"key-system": "com.widevine.alpha", "adamId": "1109715168", "isLibrary": false, "user-initiated": true,
	}
	for k, v := range want {
		if r.body[k] != v {
			t.Errorf("body[%s] = %v, want %v", k, r.body[k], v)
		}
	}
	if _, ok := r.body["stkn"]; ok {
		t.Error("first request must not send a slot token")
	}

	if _, err := c.AcquireLicense(context.Background(), LicenseRequest{Challenge: []byte("renew"), KeyURI: "k", AdamID: "1", Renewal: true, SlotToken: "slot-1"}); err != nil {
		t.Fatal(err)
	}
	if b := (*seen)[1].body; b["stkn"] != "slot-1" || b["user-initiated"] != false {
		t.Fatalf("renewal body %v", b)
	}
}

func TestAcquireLicenseRejected(t *testing.T) {
	c, _ := playClient(t, 200, `{"status":-1,"errorCode":-1017}`)
	_, err := c.AcquireLicense(context.Background(), LicenseRequest{Challenge: []byte("x")})
	var le *LicenseError
	if !errors.As(err, &le) || le.Status != LicenseStatusGeoBlock {
		t.Fatalf("rejected licence: %v", err)
	}
	c, _ = playClient(t, 200, `{"status":0,"license":"!!"}`)
	if _, err := c.AcquireLicense(context.Background(), LicenseRequest{Challenge: []byte("x")}); err == nil {
		t.Fatal("bad base64 licence accepted")
	}
	c, _ = playClient(t, http.StatusUnauthorized, `{}`)
	if _, err := c.AcquireLicense(context.Background(), LicenseRequest{Challenge: []byte("x")}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401: %v", err)
	}
}

func TestWidevineCert(t *testing.T) {
	c, seen := playClient(t, 200, "\x08\x05cert-bytes")
	cert, err := c.WidevineCert(context.Background(), "")
	if err != nil || string(cert) != "\x08\x05cert-bytes" {
		t.Fatalf("%q %v", cert, err)
	}
	if r := (*seen)[0]; r.path != "/WebObjects/MZPlay.woa/wa/widevineCert" || r.header.Get("Accept") != "application/octet-stream" {
		t.Fatalf("request %+v", r)
	}
	c, _ = playClient(t, 200, "")
	if _, err := c.WidevineCert(context.Background(), ""); err == nil {
		t.Fatal("empty certificate accepted")
	}
}
