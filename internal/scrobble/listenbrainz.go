package scrobble

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ListenBrainzAPI is the public ListenBrainz server.
const ListenBrainzAPI = "https://api.listenbrainz.org"

// ListenBrainz submits listens with a user token
// (https://listenbrainz.readthedocs.io/en/latest/users/api/core.html).
type ListenBrainz struct {
	API       string // default ListenBrainzAPI
	HTTP      *http.Client
	UserAgent string // default "Sona"
	Version   string // submission_client_version
}

// Name is the provider name.
func (c *ListenBrainz) Name() string { return "listenbrainz" }

// MaxBatch keeps import requests well under the server's size limits.
func (c *ListenBrainz) MaxBatch() int { return 100 }

// Validate checks token and returns its user name; an invalid token is
// ErrAuth.
func (c *ListenBrainz) Validate(ctx context.Context, token string) (string, error) {
	var out struct {
		Valid    bool   `json:"valid"`
		UserName string `json:"user_name"`
	}
	if err := c.do(ctx, http.MethodGet, "/1/validate-token", token, nil, &out); err != nil {
		return "", err
	}
	if !out.Valid || out.UserName == "" {
		return "", fmt.Errorf("%w: ListenBrainz does not accept this token", ErrAuth)
	}
	return out.UserName, nil
}

// NowPlaying announces t (listen_type playing_now).
func (c *ListenBrainz) NowPlaying(ctx context.Context, token string, t Track) error {
	return c.submit(ctx, token, "playing_now", []lbListen{{TrackMetadata: c.metadata(t)}})
}

// Submit sends listens: one as listen_type single, more as import.
func (c *ListenBrainz) Submit(ctx context.Context, token string, ls []Listen) error {
	if len(ls) == 0 {
		return nil
	}
	if len(ls) > c.MaxBatch() {
		return fmt.Errorf("listenbrainz: %d listens exceed the batch limit", len(ls))
	}
	payload := make([]lbListen, len(ls))
	for i, l := range ls {
		at := l.ListenedAt.Unix()
		payload[i] = lbListen{ListenedAt: &at, TrackMetadata: c.metadata(l.Track)}
	}
	kind := "import"
	if len(ls) == 1 {
		kind = "single"
	}
	return c.submit(ctx, token, kind, payload)
}

type lbListen struct {
	ListenedAt    *int64     `json:"listened_at,omitempty"`
	TrackMetadata lbMetadata `json:"track_metadata"`
}

type lbMetadata struct {
	ArtistName     string           `json:"artist_name"`
	TrackName      string           `json:"track_name"`
	ReleaseName    string           `json:"release_name,omitempty"`
	AdditionalInfo lbAdditionalInfo `json:"additional_info"`
}

type lbAdditionalInfo struct {
	DurationMS              int64  `json:"duration_ms,omitempty"`
	MediaPlayer             string `json:"media_player"`
	SubmissionClient        string `json:"submission_client"`
	SubmissionClientVersion string `json:"submission_client_version,omitempty"`
	MusicService            string `json:"music_service"`
}

func (c *ListenBrainz) metadata(t Track) lbMetadata {
	return lbMetadata{
		ArtistName:  t.Artist,
		TrackName:   t.Title,
		ReleaseName: t.Album,
		AdditionalInfo: lbAdditionalInfo{
			DurationMS:              t.Duration.Milliseconds(),
			MediaPlayer:             "Sona",
			SubmissionClient:        "Sona",
			SubmissionClientVersion: c.Version,
			MusicService:            "music.apple.com",
		},
	}
}

func (c *ListenBrainz) submit(ctx context.Context, token, kind string, payload []lbListen) error {
	body, err := json.Marshal(struct {
		ListenType string     `json:"listen_type"`
		Payload    []lbListen `json:"payload"`
	}{kind, payload})
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "/1/submit-listens", token, body, nil)
}

// do sends one request with the user token. 401 is ErrAuth, other 4xx
// (except 408 and 429) ErrRejected; the rest are retried.
func (c *ListenBrainz) do(ctx context.Context, method, path, token string, body []byte, out any) error {
	api := c.API
	if api == "" {
		api = ListenBrainzAPI
	}
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(api, "/")+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+token)
	req.Header.Set("User-Agent", agent(c.UserAgent))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient(c.HTTP).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(b, &e)
		se := &StatusError{Service: "listenbrainz", Code: resp.StatusCode}
		if e.Error != "" {
			return fmt.Errorf("%w: %s", se, e.Error)
		}
		return se
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return fmt.Errorf("listenbrainz: decode %s: %w", path, err)
		}
	}
	return nil
}
