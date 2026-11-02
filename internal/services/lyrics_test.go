package services

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/lyrics"
)

// TestLyricsServiceUsesSongDetails: the providers match the catalog
// song's title, artist, album, length and ISRC; Apple is asked in the
// chosen translation language; the answer maps to milliseconds with
// empty lists as [].
func TestLyricsServiceUsesSongDetails(t *testing.T) {
	var mu sync.Mutex
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		switch r.URL.Path {
		case "/v1/catalog/gb/songs/1":
			_, _ = w.Write([]byte(`{"data":[{"id":"1","type":"songs","attributes":{"name":"Placeholder Song","artistName":"Test Artist","albumName":"Test Album","durationInMillis":200400,"isrc":"ZZTST2600001"}}]}`))
		case "/v1/catalog/gb/songs/1/syllable-lyrics", "/v1/catalog/gb/songs/1/lyrics":
			_, _ = w.Write([]byte(`{"data":[]}`))
		case "/api/get":
			_, _ = w.Write([]byte(`{"id":1,"trackName":"Placeholder Song","artistName":"Test Artist","albumName":"Test Album","duration":200,"instrumental":false,"plainLyrics":"alpha","syncedLyrics":"[00:01.50]alpha\n[00:03.00]bravo"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	d, err := db.Open(t.Context(), filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return "mut" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	sf := func() string { return "gb" }
	settings := testSettings(t)
	if _, err := settings.Update(func(s *config.Settings) { s.Lyrics.TranslationLanguage = "fr" }); err != nil {
		t.Fatal(err)
	}
	e := &Env{Deps: Deps{DB: d, API: api, Catalog: catalog.New(d, api, sf, nil), Storefront: sf, Settings: settings, Logger: discard(), Now: time.Now}}
	s := newLyrics(e,
		&lyrics.Apple{API: api, Storefront: sf},
		&lyrics.LRCLIB{HTTP: srv.Client(), Base: srv.URL + "/api/", UserAgent: "Sona/test"},
	)

	l, err := s.Lyrics(t.Context(), "1")
	if err != nil {
		t.Fatal(err)
	}
	if l.Provider != config.LyricsLRCLIB || l.Timing != lyrics.TimingLine || len(l.Lines) != 2 ||
		l.Lines[0].BeginMs != 1500 || l.Lines[0].EndMs != 3000 || l.Lines[1].Text != "bravo" {
		t.Fatalf("lyrics = %+v", l)
	}
	b, _ := json.Marshal(l)
	if !strings.Contains(string(b), `"songwriters":[]`) || !strings.Contains(string(b), `"words":[]`) {
		t.Fatalf("json = %s", b)
	}
	mu.Lock()
	all := strings.Join(queries, "\n")
	mu.Unlock()
	if !strings.Contains(all, "/api/get?album_name=Test+Album&artist_name=Test+Artist&duration=200&track_name=Placeholder+Song") {
		t.Fatalf("LRCLIB not asked with the song's details:\n%s", all)
	}
	if !strings.Contains(all, "syllable-lyrics?extend=ttmlLocalizations&l%5Blyrics%5D=fr") {
		t.Fatalf("Apple not asked in the chosen language:\n%s", all)
	}

	if _, err := s.Lyrics(t.Context(), ""); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty id err = %v", err)
	}
}

func TestLyricsServiceNotFoundCode(t *testing.T) {
	if got := ErrorCode(lyrics.ErrNotFound); got != CodeNotFound {
		t.Fatalf("code = %q", got)
	}
}

func TestLyricsDTO(t *testing.T) {
	ms := time.Millisecond
	l := lyricsDTO(lyrics.Lyrics{Provider: "apple", Timing: lyrics.TimingWord, TranslationLanguage: "en-US", TransliterationLanguage: "ko-Latn", Lines: []lyrics.Line{{
		Begin: 1500 * ms, End: 3 * time.Second, Text: "hi there", Background: "(hey)", Agent: "v2", AgentType: "other", Part: "Chorus",
		Words:           []lyrics.Word{{Begin: 1500 * ms, End: 2 * time.Second, Text: "hi "}, {Begin: 2 * time.Second, End: 2500 * ms, Text: "(hey)", Background: true}},
		Translation:     &lyrics.Text{Text: "hello"},
		Transliteration: &lyrics.Text{Text: "an nyeong", Words: []lyrics.Word{{Begin: 1500 * ms, End: 1800 * ms, Text: "an "}}},
	}, {Text: "plain"}}})
	ln := l.Lines[0]
	if l.Provider != "apple" || l.Songwriters == nil || ln.BeginMs != 1500 || ln.EndMs != 3000 || ln.Background != "(hey)" || ln.Agent != "v2" || ln.AgentType != "other" ||
		ln.Words[1].EndMs != 2500 || !ln.Words[1].Background || ln.Translation.Text != "hello" || ln.Transliteration.Words[0].EndMs != 1800 {
		t.Fatalf("lyrics = %+v", l)
	}
	if l.TranslationLanguage != "en-US" || l.TransliterationLanguage != "ko-Latn" {
		t.Fatalf("languages = %+v", l)
	}
	if p := l.Lines[1]; p.Words == nil || p.Translation != nil || p.Transliteration != nil {
		t.Fatalf("plain line = %+v", p)
	}
}
