package applemusic

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Playback endpoints on play.itunes.apple.com (MZPlay). webPlayback answers
// with the licence and certificate URLs to use; the constants are fallbacks.
const (
	WebPlaybackURL     = "https://play.itunes.apple.com/WebObjects/MZPlay.woa/wa/webPlayback"
	LicenseURL         = "https://play.itunes.apple.com/WebObjects/MZPlay.woa/wa/acquireWebPlaybackLicense"
	WidevineCertURL    = "https://play.itunes.apple.com/WebObjects/MZPlay.woa/wa/widevineCert"
	WidevineKeySystem  = "com.widevine.alpha"
	headerAppleMUT     = "X-Apple-Music-User-Token"
	headerAppleRenewal = "X-Apple-Renewal"
)

// Widevine HLS flavors, best first: CENC AES-CTR fMP4 AAC at 256 and 64 kbps.
// The cbcp/ibhp flavors are FairPlay and unusable with Widevine.
var widevineFlavors = []string{"28:ctrp256", "32:ctrp64"}

// ErrNotPlayable reports a webPlayback answer without a usable Widevine
// asset (region restriction, no subscription, or a FairPlay-only item).
var ErrNotPlayable = errors.New("apple music: no playable widevine asset")

// Asset is one encoding of a song in a webPlayback answer.
type Asset struct {
	Flavor string `json:"flavor"`
	URL    string `json:"URL"`
}

// PlaybackInfo is the webPlayback answer for one song, with the asset Sona
// plays already selected.
type PlaybackInfo struct {
	SongID      string  // catalog ID the assets belong to
	Flavor      string  // selected flavor, e.g. "28:ctrp256"; "" for a plain library upload
	PlaylistURL string  // HLS media playlist of the selected flavor, or the upload's file URL
	LicenseURL  string  // Widevine licence server (hls-key-server-url)
	CertURL     string  // Widevine service certificate (widevine-cert-url)
	IsLibrary   bool    // requested by library ID
	Assets      []Asset // every asset Apple offered
}

// Protected reports whether the selected asset is Widevine-encrypted HLS.
func (p PlaybackInfo) Protected() bool { return p.Flavor != "" }

// WebPlaybackError is a 2xx webPlayback answer that carries a failure
// instead of a song list.
type WebPlaybackError struct {
	FailureType     string
	CustomerMessage string
}

func (e *WebPlaybackError) Error() string {
	return fmt.Sprintf("apple music: webPlayback failure %s: %s", e.FailureType, e.CustomerMessage)
}

// Unwrap makes errors.Is(err, ErrNotPlayable) match.
func (e *WebPlaybackError) Unwrap() error { return ErrNotPlayable }

type webPlaybackResponse struct {
	SongList []struct {
		SongID          json.RawMessage `json:"songId"`
		Assets          []Asset         `json:"assets"`
		KeyServerURL    string          `json:"hls-key-server-url"`
		WidevineCertURL string          `json:"widevine-cert-url"`
	} `json:"songList"`
	FailureType     string `json:"failureType"`
	CustomerMessage string `json:"customerMessage"`
}

// WebPlayback fetches playback metadata for id: a catalog song ID
// ("1440…") or a library ID ("i.…", sent as universalLibraryId). It selects
// the 256 kbps Widevine flavor, falling back to 64 kbps. A library upload
// without a catalog song has a single flavorless, unencrypted asset, which is
// returned with Flavor "". Missing assets wrap ErrNotPlayable.
func (c *Client) WebPlayback(ctx context.Context, id string) (PlaybackInfo, error) {
	library := strings.HasPrefix(id, "i.")
	var body any = map[string]string{"salableAdamId": id}
	if library {
		body = map[string]any{"universalLibraryId": id, "isLibrary": true}
	}
	var resp webPlaybackResponse
	if err := c.Do(ctx, Request{Method: http.MethodPost, URL: WebPlaybackURL, Body: body, Header: c.playHeader()}, &resp); err != nil {
		return PlaybackInfo{}, fmt.Errorf("webPlayback %s: %w", id, err)
	}
	if len(resp.SongList) == 0 {
		if resp.FailureType != "" {
			return PlaybackInfo{}, &WebPlaybackError{FailureType: resp.FailureType, CustomerMessage: resp.CustomerMessage}
		}
		return PlaybackInfo{}, fmt.Errorf("webPlayback %s: empty song list: %w", id, ErrNotPlayable)
	}
	s := resp.SongList[0]
	info := PlaybackInfo{
		SongID:     strings.Trim(string(s.SongID), `"`),
		LicenseURL: s.KeyServerURL,
		CertURL:    s.WidevineCertURL,
		IsLibrary:  library,
		Assets:     s.Assets,
	}
	if info.LicenseURL == "" {
		info.LicenseURL = LicenseURL
	}
	if info.CertURL == "" {
		info.CertURL = WidevineCertURL
	}
	for _, want := range widevineFlavors {
		for _, a := range s.Assets {
			if a.Flavor == want && a.URL != "" {
				info.Flavor, info.PlaylistURL = a.Flavor, a.URL
				return info, nil
			}
		}
	}
	if library && len(s.Assets) == 1 && s.Assets[0].Flavor == "" && s.Assets[0].URL != "" {
		info.PlaylistURL = s.Assets[0].URL
		return info, nil
	}
	flavors := make([]string, 0, len(s.Assets))
	for _, a := range s.Assets {
		flavors = append(flavors, a.Flavor)
	}
	return PlaybackInfo{}, fmt.Errorf("webPlayback %s: flavors %v: %w", id, flavors, ErrNotPlayable)
}

// WidevineCert downloads the Widevine service certificate at url
// (WidevineCertURL when empty).
func (c *Client) WidevineCert(ctx context.Context, url string) ([]byte, error) {
	if url == "" {
		url = WidevineCertURL
	}
	var cert []byte
	h := c.playHeader()
	h.Set("Accept", "application/octet-stream")
	if err := c.Do(ctx, Request{URL: url, Header: h}, &cert); err != nil {
		return nil, fmt.Errorf("widevine certificate: %w", err)
	}
	if len(cert) == 0 {
		return nil, errors.New("widevine certificate: empty body")
	}
	return cert, nil
}

// LicenseRequest is one Widevine licence exchange.
type LicenseRequest struct {
	URL       string // licence server (PlaybackInfo.LicenseURL); LicenseURL when empty
	Challenge []byte // CDM licence request message
	KeyURI    string // the playlist's EXT-X-KEY URI ("data:;base64,…")
	AdamID    string // catalog song ID (PlaybackInfo.SongID)
	IsLibrary bool
	Renewal   bool   // the challenge is a CDM renewal message
	SlotToken string // License.SlotToken from the previous exchange for this key, if any
}

// License is Apple's answer to a LicenseRequest.
type License struct {
	Data      []byte // CDM licence response
	SlotToken string // "stkn"; send back on renewals
}

// LicenseError is a licence answer with a non-zero status.
type LicenseError struct {
	Status  int
	Message string
}

// Licence status codes MusicKit names.
const (
	LicenseStatusDeviceLimit = -1004
	LicenseStatusGeoBlock    = -1017
)

func (e *LicenseError) Error() string {
	return fmt.Sprintf("apple music: licence rejected: status %d %s", e.Status, e.Message)
}

type licenseResponse struct {
	License   string `json:"license"`
	Status    int    `json:"status"`
	ErrorCode int    `json:"errorCode"`
	Message   string `json:"message"`
	SlotToken string `json:"stkn"`
}

// AcquireLicense posts a Widevine challenge to Apple's licence server, in
// the shape MusicKit JS uses for catalog songs (challenge, uri, key-system,
// adamId, isLibrary, user-initiated, stkn). HTTP 401/403 wrap
// ErrUnauthorized; a non-zero licence status is a *LicenseError.
func (c *Client) AcquireLicense(ctx context.Context, r LicenseRequest) (License, error) {
	if len(r.Challenge) == 0 {
		return License{}, errors.New("licence: empty challenge")
	}
	url := r.URL
	if url == "" {
		url = LicenseURL
	}
	body := map[string]any{
		"challenge":      base64.StdEncoding.EncodeToString(r.Challenge),
		"uri":            r.KeyURI,
		"key-system":     WidevineKeySystem,
		"adamId":         r.AdamID,
		"isLibrary":      r.IsLibrary,
		"user-initiated": !r.Renewal,
	}
	if r.SlotToken != "" {
		body["stkn"] = r.SlotToken
	}
	h := c.playHeader()
	// MusicKit sends this on every Widevine licence request, initial or not.
	h.Set(headerAppleRenewal, "true")
	var resp licenseResponse
	if err := c.Do(ctx, Request{Method: http.MethodPost, URL: url, Body: body, Header: h}, &resp); err != nil {
		return License{}, fmt.Errorf("licence %s: %w", r.AdamID, err)
	}
	if resp.ErrorCode != 0 {
		resp.Status = resp.ErrorCode
	}
	if resp.Status != 0 {
		return License{}, &LicenseError{Status: resp.Status, Message: resp.Message}
	}
	data, err := base64.StdEncoding.DecodeString(resp.License)
	if err != nil || len(data) == 0 {
		return License{}, fmt.Errorf("licence %s: bad licence payload (%d chars): %v", r.AdamID, len(resp.License), err)
	}
	return License{Data: data, SlotToken: resp.SlotToken}, nil
}

// playHeader carries the Music User Token the way MusicKit sends it to MZPlay.
func (c *Client) playHeader() http.Header {
	h := http.Header{"Accept": {"application/json"}}
	if c.UserToken != nil {
		if mut := c.UserToken(); mut != "" {
			h.Set(headerAppleMUT, mut)
		}
	}
	return h
}
