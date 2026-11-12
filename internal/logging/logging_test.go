package logging

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fake secrets with the real shapes; never real credentials.
var (
	fakeJWT = "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IldlYlBsYXlLaWQifQ." +
		"eyJpc3MiOiJBTVBXZWJQbGF5IiwiaWF0IjoxNzYxNjAwMDAwLCJleHAiOjE3Njc4MDAwMDB9." +
		"c2lnbmF0dXJlLWJ5dGVzLW5vdC1yZWFsLXNpZ25hdHVyZS1ieXRlcw"
	fakeMUT = "A" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("not-a-real-music-user-token/"), 9))
)

func logTo(buf *bytes.Buffer) *slog.Logger {
	return slog.New(NewRedactHandler(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
}

func assertClean(t *testing.T, out string) {
	t.Helper()
	for _, secret := range []string{fakeJWT, fakeMUT, fakeMUT[:60], "eyJhbGciOiJFUzI1NiIs"} {
		if strings.Contains(out, secret) {
			t.Fatalf("secret leaked into log output:\n%s", out)
		}
	}
}

func TestRedactFreeText(t *testing.T) {
	cases := []struct{ in, keep string }{
		{"dev token " + fakeJWT + " fetched", "dev token "},
		{"Media-User-Token: " + fakeMUT, "Media-User-Token: " + Redacted},
		{"media-user-token=" + fakeMUT + "&x=1", "&x=1"},
		{"Authorization: Bearer " + fakeJWT, "Authorization: " + Redacted},
		{"req with bearer " + fakeJWT, "bearer"},
		{`{"musicUserToken":"` + fakeMUT + `"}`, `"musicUserToken":"` + Redacted},
		{"https://example.test/cb?token=abc123secret&state=ok", "state=ok"},
		{"raw mut " + fakeMUT, "raw mut " + Redacted},
	}
	for _, c := range cases {
		got := Redact(c.in)
		assertClean(t, got)
		if strings.Contains(got, "abc123secret") {
			t.Errorf("Redact(%q) kept query token: %q", c.in, got)
		}
		if !strings.Contains(got, c.keep) {
			t.Errorf("Redact(%q) = %q, want it to keep %q", c.in, got, c.keep)
		}
	}
}

func TestRedactLeavesOrdinaryTextAlone(t *testing.T) {
	for _, s := range []string{
		"token expired, refetching",
		"GET /v1/me/library/songs?limit=100&offset=200 200 OK",
		"album 1440857781 has 10 tracks",
		"press Ctrl-X to cut; -X alone means nothing",
		`{"msg":"Build Info:","-tags":"sqlite_fts5","-gcflags":"all=-l","GOOS":"linux"}`,
	} {
		if got := Redact(s); got != s {
			t.Errorf("Redact(%q) = %q, want unchanged", s, got)
		}
	}
}

// mayHoldSecret may only say false for text the regexps leave alone.
func TestMayHoldSecretIsConservative(t *testing.T) {
	for _, s := range []string{
		"", "starting", "playback ready", "bearer", "BEARER x", "Bearer abc",
		"token expired", "album 1440857781 has 10 tracks", "eyJ", "-", "a=b", "k: v",
		strings.Repeat("x", 99), strings.Repeat("A", 100), "BeArEr  abcdefgh12345",
	} {
		if !mayHoldSecret(s) && scrub(s) != s {
			t.Errorf("mayHoldSecret(%q) = false, but scrub changes it to %q", s, scrub(s))
		}
	}
	for _, s := range []string{
		"starting", "playback ready", "album 1440857781 has 10 tracks", "queue changed",
		strings.Repeat("word ", 19),
	} {
		if mayHoldSecret(s) {
			t.Errorf("mayHoldSecret(%q) = true, want the regexps skipped", s)
		}
	}
	for _, s := range []string{fakeJWT, fakeMUT, "Bearer abcdefgh12345", "bEaReR abcdefgh12345"} {
		if !mayHoldSecret(s) {
			t.Errorf("mayHoldSecret(%q) = false for a secret", s)
		}
	}
}

type creds struct {
	Storefront string
	Token      string
}

func TestHandlerRedactsRecords(t *testing.T) {
	var buf bytes.Buffer
	log := logTo(&buf)

	log.Info("sign-in ok with "+fakeJWT,
		"token", fakeJWT,
		"mut", fakeMUT,
		"Authorization", "Bearer "+fakeJWT,
		"musicUserToken", 12345, // non-string under a secret key
		"header", "Media-User-Token: "+fakeMUT,
		"err", errors.New("apple: 401 for token="+fakeMUT),
		"creds", creds{Storefront: "gb", Token: fakeMUT},
		slog.Group("req", "url", "/v1/me/storefront", "devToken", fakeJWT),
		"storefront", "gb",
	)
	out := buf.String()
	assertClean(t, out)

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output is not one JSON record: %v\n%s", err, out)
	}
	for _, k := range []string{"token", "mut", "Authorization", "musicUserToken"} {
		if rec[k] != Redacted {
			t.Errorf("%s = %v, want %s", k, rec[k], Redacted)
		}
	}
	if rec["storefront"] != "gb" {
		t.Errorf("non-secret attribute altered: %v", rec["storefront"])
	}
	req, _ := rec["req"].(map[string]any)
	if req["devToken"] != Redacted || req["url"] != "/v1/me/storefront" {
		t.Errorf("group not redacted correctly: %v", req)
	}
	if s, _ := rec["creds"].(string); !strings.Contains(s, "Storefront:gb") {
		t.Errorf("struct context lost: %v", rec["creds"])
	}
}

func TestHandlerRedactsWithAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := logTo(&buf).With("authorization", "Bearer "+fakeJWT, "note", "jwt "+fakeJWT).WithGroup("g")
	log.Debug("hello", "ok", true)
	out := buf.String()
	assertClean(t, out)
	if !strings.Contains(out, `"ok":true`) {
		t.Fatalf("grouped attribute missing: %s", out)
	}
}

func TestNewWritesRedactedJSONFileAndDevText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sona.log")
	var stderr bytes.Buffer
	log, closeFn, err := New(Options{File: path, Dev: true, Stderr: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	log.Info("configured", "mut", fakeMUT, "storefront", "us")
	log.Debug("debug only on stderr")
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertClean(t, string(b))
	assertClean(t, stderr.String())
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 1 {
		t.Fatalf("file should hold only the info record, got %d lines:\n%s", len(lines), b)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil || rec["storefront"] != "us" {
		t.Fatalf("bad JSON record %q: %v", lines[0], err)
	}
	if !strings.Contains(stderr.String(), "debug only on stderr") || !strings.Contains(stderr.String(), "storefront=us") {
		t.Fatalf("dev text output missing records:\n%s", stderr.String())
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("log file mode %v, want 0600", fi.Mode().Perm())
	}
}

// Fake build credentials, injected the way build/linux/Taskfile.yml does.
const (
	fakeLastFMKey    = "fakeLastfmKey0123456789abcdef"
	fakeLastFMSecret = "fakeLastfmSecret0123456789abcd"
	fakeDiscordID    = "112233445566778899"
	fakeLdflags      = "-X main.version=0.1.0" +
		" -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmKey=" + fakeLastFMKey +
		" -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmSecret=" + fakeLastFMSecret +
		" -X github.com/coredev-uk/sona-go/internal/discord.clientID=" + fakeDiscordID
)

func assertNoBuildCredentials(t *testing.T, out string) {
	t.Helper()
	for _, secret := range []string{fakeLastFMKey, fakeLastFMSecret, fakeDiscordID} {
		if strings.Contains(out, secret) {
			t.Fatalf("build credential %q leaked:\n%s", secret, out)
		}
	}
}

// A "Build Info:" record carries every debug.BuildInfo setting as an
// attribute, -ldflags included. Neither the file nor the dev stderr text may carry the
// credentials the linker flags inject; the other settings stay.
func TestNewKeepsBuildCredentialsOutOfBuildInfoRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sona.log")
	var stderr bytes.Buffer
	log, closeFn, err := New(Options{File: path, Dev: true, Stderr: &stderr})
	if err != nil {
		t.Fatal(err)
	}
	log.Info("Build Info:", "Framework", "v1.0.0", "Compiler", "go1.26.10", "-buildmode", "exe",
		"-tags", "sqlite_fts5", "-gcflags", "all=-l", "-ldflags", fakeLdflags, "GOOS", "linux", "CGO_ENABLED", "1")
	if err := closeFn(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertNoBuildCredentials(t, string(b))
	assertNoBuildCredentials(t, stderr.String())
	var rec map[string]any
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatalf("output is not one JSON record: %v\n%s", err, b)
	}
	if rec["-ldflags"] != Redacted {
		t.Errorf("-ldflags = %v, want %s", rec["-ldflags"], Redacted)
	}
	if rec["msg"] != "Build Info:" || rec["-tags"] != "sqlite_fts5" || rec["GOOS"] != "linux" || rec["Compiler"] != "go1.26.10" {
		t.Errorf("other build settings altered: %v", rec)
	}
	if !strings.Contains(stderr.String(), "-ldflags="+Redacted) {
		t.Errorf("dev text lacks the redacted -ldflags:\n%s", stderr.String())
	}
}

// The same flags in free text: the log shapes of older builds (read back by
// the diagnostics bundle), command lines and quoted values.
func TestRedactBuildFlagsInText(t *testing.T) {
	cases := []struct{ name, in, keep string }{
		{
			"json record", `{"msg":"Build Info:","-tags":"sqlite_fts5","-ldflags":"` + fakeLdflags + `","GOOS":"darwin"}`,
			`"-ldflags":"` + Redacted + `","GOOS":"darwin"`,
		},
		{"text record", `msg="Build Info:" -ldflags="` + fakeLdflags + `" GOOS=linux`, `-ldflags="` + Redacted + `" GOOS=linux`},
		{"bare value", "-ldflags=-X=main.secret=" + fakeLastFMSecret + " next", "-ldflags=" + `"` + Redacted + `" next`},
		{"command line", "go build -ldflags '" + fakeLdflags + "' -o bin/sona", "-o bin/sona"},
		{
			"assignment", "build with -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmSecret=" + fakeLastFMSecret + " done",
			"scrobble.lastfmSecret=" + Redacted + " done",
		},
		{"equals form", "-X=github.com/coredev-uk/sona-go/internal/discord.clientID=" + fakeDiscordID, "discord.clientID=" + Redacted},
		{"quoted space value", `-X 'github.com/coredev-uk/sona-go/internal/scrobble.lastfmKey=` + fakeLastFMKey + ` x'`, "lastfmKey=" + Redacted},
	}
	for _, c := range cases {
		got := Redact(c.in)
		assertNoBuildCredentials(t, got)
		if !strings.Contains(got, c.keep) {
			t.Errorf("%s: Redact(%q) = %q, want it to keep %q", c.name, c.in, got, c.keep)
		}
		if again := Redact(got); again != got {
			t.Errorf("%s: redacting twice changed %q to %q", c.name, got, again)
		}
	}
}

func TestRotatingFileRotatesBySize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sona.log")
	f, err := OpenRotatingFile(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	line := []byte(strings.Repeat("x", 59) + "\n") // 60 bytes: one line per file
	for i := range 5 {
		line[0] = byte('a' + i)
		if _, err := f.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	want := map[string]byte{path: 'e', path + ".1": 'd', path + ".2": 'c'}
	for p, first := range want {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) != 60 || b[0] != first {
			t.Errorf("%s: %d bytes starting %q, want 60 starting %q", filepath.Base(p), len(b), b[0], first)
		}
	}
	if _, err := os.Stat(path + ".3"); !os.IsNotExist(err) {
		t.Errorf("backup beyond limit kept: %v", err)
	}
	if _, err := f.Write(line); !errors.Is(err, os.ErrClosed) {
		t.Errorf("write after close: %v, want os.ErrClosed", err)
	}
}

func TestRotatingFileAppendsAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sona.log")
	for range 2 {
		f, err := OpenRotatingFile(path, 1000, 1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte("line\n")); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	b, _ := os.ReadFile(path)
	if string(b) != "line\nline\n" {
		t.Fatalf("got %q", b)
	}
}
