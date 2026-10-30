package scrobble

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type lbRequest struct {
	ListenType string `json:"listen_type"`
	Payload    []struct {
		ListenedAt    *int64 `json:"listened_at"`
		TrackMetadata struct {
			ArtistName     string `json:"artist_name"`
			TrackName      string `json:"track_name"`
			ReleaseName    string `json:"release_name"`
			AdditionalInfo struct {
				DurationMS       int64  `json:"duration_ms"`
				SubmissionClient string `json:"submission_client"`
				MusicService     string `json:"music_service"`
			} `json:"additional_info"`
		} `json:"track_metadata"`
	} `json:"payload"`
}

// fakeListenBrainz accepts the token "good" and records submissions.
type fakeListenBrainz struct {
	t    *testing.T
	mu   sync.Mutex
	subs []lbRequest
	fail int // submit-listens answers 503 this many times
}

func (f *fakeListenBrainz) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ok := r.Header.Get("Authorization") == "Token good"
	switch r.URL.Path {
	case "/1/validate-token":
		if ok {
			_, _ = io.WriteString(w, `{"code":200,"message":"Token valid.","valid":true,"user_name":"rj"}`)
		} else {
			_, _ = io.WriteString(w, `{"code":200,"message":"Token invalid.","valid":false}`)
		}
	case "/1/submit-listens":
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":401,"error":"Invalid authorization token."}`)
			return
		}
		var req lbRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"code":400,"error":"bad json"}`)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.fail > 0 {
			f.fail--
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		f.subs = append(f.subs, req)
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fakeListenBrainz) submissions() []lbRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]lbRequest(nil), f.subs...)
}

func newListenBrainz(t *testing.T) (*ListenBrainz, *fakeListenBrainz) {
	f := &fakeListenBrainz{t: t}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return &ListenBrainz{API: srv.URL, HTTP: srv.Client(), Version: "1.2.3"}, f
}

func TestListenBrainzValidate(t *testing.T) {
	c, _ := newListenBrainz(t)
	if u, err := c.Validate(t.Context(), "good"); err != nil || u != "rj" {
		t.Fatalf("Validate(good) = %q, %v", u, err)
	}
	if _, err := c.Validate(t.Context(), "bad"); !errors.Is(err, ErrAuth) {
		t.Fatalf("Validate(bad) = %v, want ErrAuth", err)
	}
}

func TestListenBrainzListenTypes(t *testing.T) {
	c, f := newListenBrainz(t)
	ctx := t.Context()
	tr := Track{Artist: "Artist", Title: "Song", Album: "Album", Duration: 200 * time.Second}
	if err := c.NowPlaying(ctx, "good", tr); err != nil {
		t.Fatal(err)
	}
	if err := c.Submit(ctx, "good", []Listen{{Track: tr, ListenedAt: t0}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Submit(ctx, "good", []Listen{{Track: tr, ListenedAt: t0}, {Track: tr, ListenedAt: t0.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	subs := f.submissions()
	if len(subs) != 3 {
		t.Fatalf("%d submissions", len(subs))
	}
	if s := subs[0]; s.ListenType != "playing_now" || len(s.Payload) != 1 || s.Payload[0].ListenedAt != nil {
		t.Fatalf("playing_now = %+v", s)
	}
	md := subs[0].Payload[0].TrackMetadata
	if md.ArtistName != "Artist" || md.TrackName != "Song" || md.ReleaseName != "Album" ||
		md.AdditionalInfo.DurationMS != 200000 || md.AdditionalInfo.SubmissionClient != "Sona" || md.AdditionalInfo.MusicService != "music.apple.com" {
		t.Fatalf("metadata = %+v", md)
	}
	if s := subs[1]; s.ListenType != "single" || len(s.Payload) != 1 || *s.Payload[0].ListenedAt != t0.Unix() {
		t.Fatalf("single = %+v", s)
	}
	if s := subs[2]; s.ListenType != "import" || len(s.Payload) != 2 || *s.Payload[1].ListenedAt != t0.Unix()+3600 {
		t.Fatalf("import = %+v", s)
	}
	if err := c.Submit(ctx, "bad", []Listen{{Track: tr, ListenedAt: t0}}); !errors.Is(err, ErrAuth) {
		t.Fatalf("submit with a revoked token = %v, want ErrAuth", err)
	}
}
