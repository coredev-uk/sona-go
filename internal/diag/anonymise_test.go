package diag

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/logging"
)

// Everything below is made up: no ID, title, term or path is a real one.

// pseudonymRe is a pseudonym as the bundle writes it.
var pseudonymRe = regexp.MustCompile(`\b[a-z0-9]+~[0-9a-f]{6}\b`)

// bundleOf writes lines as the log file and builds a bundle of it.
func bundleOf(t *testing.T, lines ...string) string {
	t.Helper()
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sona.log")
	if err := os.WriteFile(logFile, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return Bundle(BundleOptions{Now: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC), LogFile: logFile, StateDir: dir, Home: home})
}

func mustNotContain(t *testing.T, out string, values ...string) {
	t.Helper()
	for _, v := range values {
		if strings.Contains(out, v) {
			t.Errorf("bundle contains %q", v)
		}
	}
}

func mustContain(t *testing.T, out string, values ...string) {
	t.Helper()
	for _, v := range values {
		if !strings.Contains(out, v) {
			t.Errorf("bundle lacks %q", v)
		}
	}
}

// A log tail with every kind of thing a person did: the bundle holds none
// of the original values.
func TestBundleAnonymisesTheLogTail(t *testing.T) {
	const (
		songA   = "1700000001"
		songB   = "1700000002"
		libSong = "i.FakeLibSong7"
		list    = "pl.u-FakePlaylist1"
		libList = "p.FakeLibList9"
		station = "ra.900001"
		shelf   = "7-fakeShelf99"
		cacheK  = "fake-cache-key-0123456789abcdef"
		term    = "fake secret band"
		route   = "/playlist/p.FakeRoute77"
		mail    = "fake.person@example.org"
		device  = "alsa_output.usb-Fake_DAC_SERIAL99-00.analog-stereo"
		title   = "Fake Song Title"
	)
	media := home + "/Music/Fake Artist/Fake Song.m4a"
	out := bundleOf(t,
		`{"time":"2026-11-01T10:00:00Z","level":"INFO","msg":"prepared","id":"`+songA+`","playlist":"`+list+`","took":1234567,"bytes":482113}`,
		`{"time":"2026-11-01T10:00:01Z","level":"INFO","msg":"prepared","id":"`+songA+`","station":"`+station+`","took":2000000}`,
		`{"level":"ERROR","msg":"playback failed","id":"`+libSong+`","err":"decode failed: code 7","exit_code":2}`,
		`{"level":"WARN","msg":"coordinator: skipping failed item","id":"`+songB+`","playlist":"`+libList+`"}`,
		`{"level":"WARN","msg":"spatial audio unavailable; playing the stereo stream","id":"`+songB+`","why":"no-other-mix"}`,
		`{"level":"INFO","msg":"playback: command","cmd":"play","station":"`+station+`","from":"cli"}`,
		`{"level":"WARN","msg":"home shelf refresh","shelf":"`+shelf+`","err":"Get \"https://amp-api.music.apple.com/v1/me/recommendations/`+shelf+`?l=en-GB&platform=web&with=library\": context deadline exceeded"}`,
		`{"level":"DEBUG","msg":"motion: fetched","key":"`+cacheK+`","size":2048,"took_ms":15}`,
		`{"level":"DEBUG","msg":"discover cache: write","key":"home:`+shelf+`"}`,
		`{"level":"INFO","msg":"search","term":"`+term+`","query":"`+term+`","results":12}`,
		`{"level":"INFO","msg":"on air","id":"`+songA+`","title":"`+title+`","artist":"Fake Artist"}`,
		`{"level":"WARN","msg":"remember route","route":"`+route+`?song=4","err":"saving"}`,
		`{"level":"INFO","msg":"sign-in for `+mail+`"}`,
		`{"level":"DEBUG","msg":"request","musicUserToken":"`+fakeShortMUT+`"}`,
		`{"level":"INFO","msg":"prepared file","path":"`+media+`","size":9000}`,
		`{"level":"ERROR","msg":"read `+media+`: permission denied"}`,
		`{"level":"INFO","msg":"playback host started","device":"`+device+`","sink":"`+device+`","pid":4242}`,
		`{"level":"INFO","msg":"x","req":{"songId":"`+songA+`","count":3},"ids":["`+songA+`","`+songB+`"]}`,
		`{"level":"INFO","msg":"nested","album":{"label":"Fake Label Text","count":3,"inner":{"note":"Fake Inner Note"}}}`,
		`{"level":"INFO","msg":"artwork: fetched","url":"https://is1-ssl.mzstatic.com/image/thumb/Music126/v4/ab/cd/ef/0123abcd-0000-4000-8000-feedbeef0001/source/600x600bb.jpg?token=fake"}`,
		`{"level":"INFO","msg":"lyrics","url":"https://lyrics.example.net/api/get?artist_name=Fake+Artist&track_name=Fake+Song"}`,
	)
	mustNotContain(t, out,
		songA, songB, libSong, list, libList, station, shelf, cacheK, term, route, mail, media, device, title, "Fake Artist", "Fake Song",
		fakeShortMUT, home, "SERIAL99", "Fake_DAC", "FakeRoute77", "0123abcd-0000", "artist_name", "l=en-GB", "token=fake",
		"900001", "Music126", "Fake Label Text", "Fake Inner Note",
	)
	mustContain(t, out,
		// What stays: counts, sizes, durations, error codes and the shape of what was called.
		`"took":1234567`, `"bytes":482113`, `"took":2000000`, `"exit_code":2`, `"size":2048`, `"took_ms":15`, `"size":9000`,
		`"pid":4242`, `"results":12`, `"count":3`, "decode failed: code 7", `"why":"no-other-mix"`, `"cmd":"play"`, `"from":"cli"`,
		"https://amp-api.music.apple.com/v1/me/recommendations/{id}",
		"https://is1-ssl.mzstatic.com/image/thumb/{id}/600x600bb.jpg",
		"https://lyrics.example.net/api/…",
		`"route":"/playlist/{id}"`,
		"~/Music/{path}", `"path":"~/Music/{path}"`,
		`"msg":"sign-in for [REDACTED]"`, `"musicUserToken":"[REDACTED]"`,
	)
	if t.Failed() {
		t.Log(out)
	}
}

// The same value has the same pseudonym within a bundle, whatever its key
// or line, so "the same song failed twice" can be read; the next bundle has
// other pseudonyms, so two bundles cannot be matched.
func TestPseudonymsAreStableWithinABundleAndDifferBetweenBundles(t *testing.T) {
	lines := []string{
		`{"level":"INFO","msg":"prepared","id":"1700000001"}`,
		`{"level":"ERROR","msg":"playback failed","id":"1700000001"}`,
		`{"level":"ERROR","msg":"playback failed","id":"1700000002"}`,
		`{"level":"WARN","msg":"failed for song 1700000001"}`,
	}
	ids := func(out string) []string { return pseudonymRe.FindAllString(out, -1) }
	first, second := ids(bundleOf(t, lines...)), ids(bundleOf(t, lines...))
	if len(first) != 4 || len(second) != 4 {
		t.Fatalf("pseudonyms: %v, %v", first, second)
	}
	if first[0] != first[1] || first[0] != first[3] || first[0] == first[2] {
		t.Errorf("one song must keep one pseudonym and another song another: %v", first)
	}
	if first[0] == second[0] || first[2] == second[2] {
		t.Errorf("two bundles share pseudonyms: %v and %v", first, second)
	}
	for _, p := range first {
		if !strings.HasPrefix(p, "id~") {
			t.Errorf("pseudonym %q does not say what it stands for", p)
		}
	}
}

// Numbers, booleans and codes are not pseudonymised even under a key that
// is: a "playlist" duration is not a playlist.
func TestKeptValuesStayUnderIdentifyingKeys(t *testing.T) {
	out := bundleOf(t,
		`{"level":"INFO","msg":"prepared","playlist":1234567,"song":2345678,"id":7,"key":true,"items":3,"err":"apple: status 403"}`,
	)
	mustContain(t, out, `"playlist":1234567`, `"song":2345678`, `"id":7`, `"key":true`, `"items":3`, "apple: status 403")
}

// A line an older build wrote, or any text that is not a JSON record, goes
// through the string pass.
func TestNonJSONLogLinesGetTheStringPass(t *testing.T) {
	out := bundleOf(t,
		"time=2026-11-01T10:00:00Z level=INFO msg=prepared id=1700000001 path="+home+"/Music/Fake Artist/Fake Song.m4a",
		"raw stderr: GET https://amp-api.music.apple.com/v1/catalog/gb/songs/1700000001?include=albums failed for fake.person@example.org",
		"playing i.FakeLibSong7 from pl.u-FakePlaylist1 and ra.900001 and mv.fakevideo42",
		"Authorization: Bearer "+fakeOpaque,
		`{"level":"INFO","msg":"a truncated record","id":"1700000001`,
	)
	mustNotContain(t, out, "1700000001", "Fake Artist", "Fake Song", "fake.person", "FakeLibSong7", "FakePlaylist1", "900001", "fakevideo42", "include=albums", fakeOpaque, home)
	mustContain(t, out, "https://amp-api.music.apple.com/v1/catalog/gb/songs/{id}", "level=INFO msg=prepared", "~/Music/{path}")
	if t.Failed() {
		t.Log(out)
	}
}

// Crash text is scrubbed the same way: it can carry a panic message with a
// title, an ID, a path and an address.
func TestCrashTextIsAnonymised(t *testing.T) {
	dir := t.TempDir()
	crash := "panic: open " + home + "/Music/Fake Artist/Fake Song.m4a: no such file for fake.person@example.org\n" +
		"panic: item i.FakeLibSong7 not found in https://amp-api.music.apple.com/v1/me/library/songs/i.FakeLibSong7?x=1\n\n" +
		"goroutine 1 [running]:\nmain.run(0x1)\n\t" + home + "/code/sona-go/internal/x.go:12 +0x1d\n" +
		"net/http.(*Client).do(r.go:4)\n"
	if err := os.WriteFile(filepath.Join(dir, CrashFile), []byte(crash), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Bundle(BundleOptions{StateDir: dir, Home: home})
	mustNotContain(t, out, "Fake Artist", "Fake Song", "fake.person", "FakeLibSong7", "?x=1", home)
	mustContain(t, out, "## Crash in this run", "panic: ", "goroutine 1 [running]:", "~/code/sona-go/internal/x.go:12 +0x1d",
		"https://amp-api.music.apple.com/v1/me/library/songs/{id}", "r.go:4")
	if t.Failed() {
		t.Log(out)
	}
}

// The Settings and Diagnostics sections keep what a bug report needs, and
// the settings' routes keep their page type only.
func TestSectionsKeepCountsStorefrontAndAddonsAndMaskRoutes(t *testing.T) {
	out := Bundle(BundleOptions{
		Sections: []Section{
			{Title: "Diagnostics", Value: map[string]any{
				"database":   map[string]any{"path": home + "/.local/share/sona/library.db", "counts": map[string]any{"songs": 4321, "albums": 321, "playlists": 21}},
				"storefront": "gb",
				"seen":       "failed on i.FakeLibSong7 at https://amp-api.music.apple.com/v1/me/recommendations/7-fakeShelf99?with=library",
			}},
			{Title: "Add-ons", Value: []map[string]any{{"id": "lastfm", "version": "1.0.0", "enabled": true}, {"id": "fake-community-addon", "version": "0.2.0"}}},
			{Title: "Settings", Value: map[string]any{
				"desktop": map[string]any{"startPage": "/playlist/p.FakeStart1", "lastRoute": "/album/1700000003", "closeAction": "background"},
				"theme":   "dark",
			}},
		},
		Home: home,
	})
	mustNotContain(t, out, "FakeStart1", "1700000003", "FakeLibSong7", "fakeShelf99", "with=library", home)
	mustContain(t, out,
		`"songs": 4321`, `"albums": 321`, `"playlists": 21`, `"storefront": "gb"`, `"id": "lastfm"`, `"id": "fake-community-addon"`,
		`"startPage": "/playlist/{id}"`, `"closeAction": "background"`, `"theme": "dark"`, "~/.local/share/sona/library.db",
		"https://amp-api.music.apple.com/v1/me/recommendations/{id}",
	)
	if strings.Contains(out, "lastRoute") {
		t.Error("lastRoute is dropped from sections")
	}
	if t.Failed() {
		t.Log(out)
	}
}

// What the string pass does with each shape it is meant to catch, and with
// what only looks like one.
func TestStringPass(t *testing.T) {
	a := newAnonymiser(home)
	gone := []struct{ in, secret string }{
		{"song i.AbCdEf123456 failed", "AbCdEf123456"},
		{"in l.AbCdEf12 and p.vQDQYvBsRK", "AbCdEf12"},
		{"playlist pl.u-FakeFake01 / pl.f4d106fed2bd4114", "FakeFake01"},
		{"video mv.fake12345, station ra.987654321, spatial sp.fakeSp12, rec r.fakeRec99", "fake12345"},
		{"adamId=1234567890 and id: 2345678901", "1234567890"},
		{"GET /v1/catalog/gb/albums/1234567890/tracks?limit=100 failed", "1234567890"},
		{"opened /album/1234567890", "1234567890"},
		{"webPlayback 1700000001: empty song list", "1700000001"},
		{"fetch /rooms/12345678/contents failed", "12345678"},
		{"ids=1234567,2345678,3456789", "2345678"},
		{"https://music.apple.com/gb/album/a-fake-album-title/1234567890?i=2345678901", "fake-album-title"},
		{"https://itunes.apple.com/lookup?id=1234567890&term=fake+band", "fake+band"},
		{"https://user:fakepass@example.com/path/a?token=abc&q=fake", "fakepass"},
		{"read " + home + "/Music/Fake Artist/Fake Song.flac: denied", "Fake Artist"},
		{"read /mnt/media/Fake Artist/Fake Song.flac: denied", "Fake Artist"},
		{`read C:\Fake\Fake Artist\Fake Song.mp3: denied`, "Fake Artist"},
		{`read ` + home + `/Documents/fake notes.txt`, "fake notes"},
		{"/home/other/Music/fake.txt", "other"},
		{`C:\Users\other\Music\fake.txt`, "other"},
	}
	for _, c := range gone {
		if got := a.text(c.in); strings.Contains(got, c.secret) {
			t.Errorf("text(%q) = %q, which holds %q", c.in, got, c.secret)
		}
	}
	same := []string{
		"internal/services/r.go:12 +0x1d", "i.e. the host", "p.Name() failed", "r.Body.Close()", "l.go", "see pid 1234567 and uid=1000000",
		"version 1.2.3 took 1234567ms", "https://api.github.com/repos", "code 7: apple: status 403", "id~3fa2ab", "~/.config/sona/sona.log",
		"https://amp-api.music.apple.com/v1/me/recommendations/{id}", "~/Music/{path}",
	}
	for _, s := range same {
		if got := a.text(s); got != s {
			t.Errorf("text(%q) changed to %q", s, got)
		}
	}
	// The pass is idempotent: a line scrubbed twice is the line scrubbed once.
	for _, c := range gone {
		once := a.text(c.in)
		if twice := a.text(once); twice != once {
			t.Errorf("text is not idempotent: %q then %q", once, twice)
		}
	}
}

func TestReduceURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://amp-api.music.apple.com/v1/me/recommendations/7-fakeShelf99?l=en":      "https://amp-api.music.apple.com/v1/me/recommendations/{id}",
		"https://amp-api.music.apple.com/v1/catalog/gb/search?term=fake":                "https://amp-api.music.apple.com/v1/catalog/gb/search",
		"https://amp-api.music.apple.com/v1/me/library/playlists/p.FakeLib/tracks?x=1":  "https://amp-api.music.apple.com/v1/me/library/playlists/{id}/tracks",
		"https://amp-api.music.apple.com/v1/catalog/gb/artists/1234567/view/top-songs":  "https://amp-api.music.apple.com/v1/catalog/gb/artists/{id}/view/top-songs",
		"https://music.apple.com/gb/album/fake-title/1234567":                           "https://music.apple.com/gb/album/{id}",
		"https://is1-ssl.mzstatic.com/image/thumb/Music1/v4/ab/cd/uuid-1/{w}x{h}bb.jpg": "https://is1-ssl.mzstatic.com/image/thumb/{id}/{w}x{h}bb.jpg",
		"https://example.com":                      "https://example.com",
		"https://example.com/stream":               "https://example.com/stream",
		"https://example.com/a/b/c?q=1#frag":       "https://example.com/a/…",
		"https://example.com/9f3a/x":               "https://example.com/…",
		"https://u:p@Example.com:8443/api/get?x=1": "https://example.com:8443/api/…",
	} {
		if got := reduceURL(in); got != want {
			t.Errorf("reduceURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskRoute(t *testing.T) {
	for in, want := range map[string]string{
		"/":                          "/",
		"/charts":                    "/charts",
		"/library/recently-added":    "/library/recently-added",
		"/playlist/p.FakeStart1":     "/playlist/{id}",
		"/album/1700000003?song=4":   "/album/{id}",
		"/artist/1700000003/songs":   "/artist/{id}/songs",
		"/library/albums/l.FakeAlb9": "/library/albums/{id}",
		"/charts/most-played":        "/charts/most-played",
	} {
		if got := maskRoute(in); got != want {
			t.Errorf("maskRoute(%q) = %q, want %q", in, got, want)
		}
	}
}

// A log line the bundle rewrites is still the JSON record it was, in the
// same key order.
func TestJSONLinesStayJSONInOrder(t *testing.T) {
	a := newAnonymiser(home)
	line := `{"time":"2026-11-01T10:00:00Z","level":"INFO","msg":"a <b> & c","id":"1700000001","n":1.5e3,"ok":false,"none":null,"list":[1,"x",{"title":"t"}],"nested":{"key":"k"}}`
	out := a.logLine(line)
	if !json.Valid([]byte(out)) {
		t.Fatalf("not JSON: %s", out)
	}
	var keys []string
	dec := json.NewDecoder(strings.NewReader(out))
	_, _ = dec.Token()
	for dec.More() {
		k, _ := dec.Token()
		keys = append(keys, k.(string))
		var v json.RawMessage
		_ = dec.Decode(&v)
	}
	if got := strings.Join(keys, ","); got != "time,level,msg,id,n,ok,none,list,nested" {
		t.Errorf("keys %s", got)
	}
	mustNotContain(t, out, "1700000001", `"t"`, `"k"`, `\u003c`)
	mustContain(t, out, `"msg":"a <b> & c"`, `"n":1.5e3`, `"ok":false`, `"none":null`)
	if strings.Contains(out, logging.Redacted) {
		t.Errorf("nothing here is a secret: %s", out)
	}
}

// The unit of the audit: what classify says about the keys of the
// assignment's list, by any spelling.
func TestClassify(t *testing.T) {
	for _, k := range []string{
		"id", "ids", "song", "album", "artist", "playlist", "station", "shelf", "key", "catalogID", "catalog_song", "libraryId", "library-album",
		"title", "name", "term", "query", "device", "sink", "songID", "Name", "ID",
	} {
		if classify(k) != pseudo {
			t.Errorf("classify(%q) = %v, want pseudo", k, classify(k))
		}
	}
	for k, want := range map[string]kind{"route": route, "startPage": route, "lastRoute": route, "url": link, "href": link, "imageURL": link, "path": file, "cdm_path": file, "took": plain, "bytes": plain, "err": plain, "msg": plain} {
		if classify(k) != want {
			t.Errorf("classify(%q) = %v, want %v", k, classify(k), want)
		}
	}
}

// Lines written by Sona's own log handler (what the bundle reads in
// practice): durations are numbers and stay, a slice of IDs is one string
// and goes, an error that wraps an Apple URL loses its query and IDs.
func TestBundleOfLinesFromTheLogHandler(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sona.log")
	log, closeLog, err := logging.New(logging.Options{File: logFile, Level: slog.LevelDebug})
	if err != nil {
		t.Fatal(err)
	}
	err = errors.New(`Get "https://amp-api.music.apple.com/v1/me/recommendations/7-fakeShelf99?with=library": EOF`)
	log.Debug("prepared", "id", "1700000001", "playlist", "pl.u-FakePlaylist1", "took", 1500*time.Millisecond, "bytes", 4096)
	log.Warn("home shelf refresh", "shelf", "7-fakeShelf99", "err", err)
	log.Info("library: remote check done", "ids", []string{"i.FakeLib1234", "i.FakeLib5678"}, "changed", 2)
	log.Info("on air", slog.Group("track", slog.String("title", "Fake Title"), slog.Int("n", 3)))
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	out := Bundle(BundleOptions{LogFile: logFile, StateDir: dir, Home: home})
	mustNotContain(t, out, "1700000001", "FakePlaylist1", "fakeShelf99", "with=library", "FakeLib1234", "FakeLib5678", "Fake Title")
	mustContain(t, out, `"took":1500000000`, `"bytes":4096`, `"changed":2`, `"n":3`, "https://amp-api.music.apple.com/v1/me/recommendations/{id}", "EOF")
	if t.Failed() {
		t.Log(out)
	}
}
