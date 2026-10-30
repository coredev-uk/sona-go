package applemusic

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestNextTracksAndContinuous(t *testing.T) {
	var bodies []map[string]any
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/me/stations/next-tracks/ra.cp-1":
			if r.Method != http.MethodPost || r.URL.Query().Get("limit") != "10" || r.URL.Query().Get("relate[songs]") != "albums,artists" {
				t.Errorf("next-tracks %s %s", r.Method, r.URL.RawQuery)
			}
			w.Write([]byte(`{"data":[{"id":"7","type":"songs","attributes":{"name":"Seven","playParams":{"id":"7","kind":"song"}},
				"relationships":{"albums":{"data":[{"id":"70","type":"albums"}]},"artists":{"data":[{"id":"700","type":"artists"}]}}}]}`))
		case "/v1/me/stations/continuous":
			b, _ := io.ReadAll(r.Body)
			var m map[string]any
			_ = json.Unmarshal(b, &m)
			bodies = append(bodies, m)
			w.Write([]byte(`{"results":{"station":{"id":"ra.cp-1","type":"stations","attributes":{"name":"One Station","kind":"songSeeded"}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	c.AMPBase = c.Base
	st, err := c.ContinuousStation(t.Context(), []StationSeed{{ID: "1", Container: "pl.x"}, {ID: "i.2", Library: true}})
	if err != nil || st.ID != "ra.cp-1" || st.Attributes.Name != "One Station" {
		t.Fatalf("continuous %+v %v", st, err)
	}
	want := `{"data":[{"id":"1","meta":{"container":{"id":"pl.x"}},"type":"songs"},{"id":"i.2","type":"library-songs"}]}`
	if got, _ := json.Marshal(bodies[0]); string(got) != want {
		t.Errorf("body %s\nwant %s", got, want)
	}
	songs, err := c.NextTracks(t.Context(), "ra.cp-1", 10)
	if err != nil || len(songs) != 1 || songs[0].CatalogID() != "7" || songs[0].Relationships.Albums.Data[0].ID != "70" {
		t.Fatalf("next tracks %+v %v", songs, err)
	}
	if _, err := c.ContinuousStation(t.Context(), nil); err == nil {
		t.Error("empty seeds accepted")
	}
}

func TestRadioAssetsAndLicense(t *testing.T) {
	var lic map[string]any
	c, _, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/play/assets":
			q := r.URL.Query()
			if q.Get("id") != "ra.9" || q.Get("kind") != "radioStation" || q.Get("hasDrm") != "true" || q.Get("keyFormat") != "web" || q.Has("stationHash") {
				t.Errorf("assets query %s", r.URL.RawQuery)
			}
			w.Write([]byte(`{"results":{"assets":[{"url":"https://x/index.m3u8","keyServerUrl":"https://k/streaming-key-delivery","widevineKeyCertificateUrl":"https://c"}]}}`))
		case "/key":
			if r.Header.Get("X-Apple-Renewal") != "true" {
				t.Errorf("renewal header %v", r.Header)
			}
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &lic)
			if lic["stkn"] == "bad" {
				w.Write([]byte(`{"status":-1021,"message":"nope"}`))
				return
			}
			w.Write([]byte(`{"status":0,"license":"` + base64.StdEncoding.EncodeToString([]byte("LIC")) + `","stkn":"t2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	c.AMPBase = c.Base
	a, err := c.RadioAssets(t.Context(), StationPlayParams{ID: "ra.9", HasDRM: true})
	if err != nil || a.URL != "https://x/index.m3u8" || a.KeyServerURL == "" || a.CertURL != "https://c" {
		t.Fatalf("assets %+v %v", a, err)
	}
	l, err := c.AcquireRadioLicense(t.Context(), RadioLicenseRequest{URL: c.Base + "/key", Challenge: []byte{1, 2}, KeyURI: "data:text/plain;base64,AA=="})
	if err != nil || string(l.Data) != "LIC" || l.SlotToken != "t2" {
		t.Fatalf("licence %+v %v", l, err)
	}
	// MusicKit's live-station body: event instead of adamId/isLibrary.
	if lic["event"] != "beats1" || lic["key-system"] != WidevineKeySystem || lic["uri"] != "data:text/plain;base64,AA==" || lic["adamId"] != nil {
		t.Errorf("licence body %v", lic)
	}
	_, err = c.AcquireRadioLicense(t.Context(), RadioLicenseRequest{URL: c.Base + "/key", Challenge: []byte{1}, SlotToken: "bad"})
	var le *LicenseError
	if !errors.As(err, &le) || le.Status != -1021 {
		t.Fatalf("rejected licence: %v", err)
	}
}

func TestStationFormat(t *testing.T) {
	var s Station
	if s.Format() != StationFormatTracks {
		t.Error("default format")
	}
	s.Attributes.PlayParams = &StationPlayParams{Format: StationFormatStream}
	if s.Format() != StationFormatStream {
		t.Error("stream format")
	}
}
