package diag

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/logging"
)

// envCrashDir makes the test binary a crashing app: it captures crashes
// into the named directory and, with envCrashPanic set, panics on a
// background goroutine.
const (
	envCrashDir   = "DIAG_TEST_CRASH_DIR"
	envCrashPanic = "DIAG_TEST_PANIC"
)

func TestMain(m *testing.M) {
	// On Linux the test binary also doubles as the CDM host child
	// (diag_linux_test.go's init).
	if dir := os.Getenv(envCrashDir); dir != "" {
		crashingApp(dir)
	}
	os.Exit(m.Run())
}

func crashingApp(dir string) {
	log, closeLog, err := logging.New(logging.Options{File: filepath.Join(dir, "sona.log")})
	if err != nil {
		os.Exit(3)
	}
	if err := CaptureCrashes(dir, log); err != nil {
		os.Exit(4)
	}
	if msg := os.Getenv(envCrashPanic); msg != "" {
		done := make(chan struct{})
		go func() { panic(msg) }()
		<-done
	}
	_ = closeLog()
	os.Exit(0)
}

func runCrashingApp(t *testing.T, dir, panicMsg string) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), envCrashDir+"="+dir, envCrashPanic+"="+panicMsg)
	return cmd.Run()
}

// Fake secrets in every shape Sona handles.
var (
	fakeMUT = "AkT" + strings.Repeat("q9Zx/4Lm+7Rt2Wv8", 14) + "=="
	fakeJWT = "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6IldlYlBsYXkifQ." +
		"eyJpc3MiOiJBTVBXZWJQbGF5IiwiaWF0IjoxNzAwMDAwMDAwLCJleHAiOjE3OTk5OTk5OTl9." +
		"MEUCIQDsignaturesignaturesignature0123456789"
	fakeShortMUT = "shortMUTvalue0123456789abcdef"
	fakeOpaque   = "opaqueBearerCredential123456"
	fakeEmail    = "someone.private@example.com"
	home         = "/home/tester"
)

func secrets() []string {
	return []string{fakeMUT, fakeMUT[:40], fakeJWT, "eyJhbGciOiJFUzI1NiIs", fakeShortMUT, fakeOpaque, fakeEmail, home}
}

type fakeSettings struct {
	Theme          string `json:"theme"`
	CDMPath        string `json:"cdmPath"`
	MusicUserToken string `json:"musicUserToken"`
	Desktop        struct {
		LastRoute string `json:"lastRoute"`
	} `json:"desktop"`
	Scrobbling struct {
		Username string `json:"username"`
	} `json:"scrobbling"`
}

func TestBundleRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sona.log")
	lines := []string{
		`{"level":"INFO","msg":"startup: process start","version":"0.1.0"}`,
		`{"level":"DEBUG","msg":"request","media-user-token":"` + fakeShortMUT + `"}`,
		`{"level":"WARN","msg":"apple: 401 for Authorization: Bearer ` + fakeJWT + `"}`,
		`raw stderr: Media-User-Token: ` + fakeShortMUT + ` and mut=` + fakeShortMUT,
		`{"level":"INFO","msg":"sign-in for ` + fakeEmail + `","path":"` + home + `/.config/sona"}`,
		`curl -H "Authorization: Bearer ` + fakeOpaque + `" -H 'x-apple-music-user-token: ` + fakeShortMUT + `'`,
		`token dump ` + fakeMUT,
		`{"level":"ERROR","msg":"cdmhost: child exited unexpectedly","exit_code":2,"url":"Authorization: [REDACTED]","mut":"[REDACTED]"}`,
	}
	if err := os.WriteFile(logFile, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, PrevCrashFile), []byte("panic: request failed: Bearer "+fakeJWT+"\n\ngoroutine 1 [running]:\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var st fakeSettings
	st.Theme, st.CDMPath, st.MusicUserToken = "dark", home+"/.config/google-chrome/WidevineCdm", fakeMUT
	st.Desktop.LastRoute = "/playlist/p.private"
	st.Scrobbling.Username = "privatelistener"

	out := Bundle(BundleOptions{
		Now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
		Sections: []Section{
			{Title: "Diagnostics", Value: map[string]any{"cdmError": "licence: media-user-token: " + fakeShortMUT, "version": "0.1.0"}},
			{Title: "Settings", Value: st},
		},
		LogFile:  logFile,
		StateDir: dir,
		Home:     home,
	})
	// "[REDACTED]]" would mean a value already redacted by the log
	// handler was redacted again and mangled.
	for _, s := range append(secrets(), "p.private", "privatelistener", logging.Redacted+"]") {
		if strings.Contains(out, s) {
			t.Errorf("bundle contains %q", s)
		}
	}
	for _, want := range []string{
		"startup: process start", "cdmhost: child exited unexpectedly", "## Previous crash", "panic: request failed",
		`"theme": "dark"`, "~/.config/google-chrome/WidevineCdm", logging.Redacted, "## Log (last 8 lines)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bundle lacks %q", want)
		}
	}
	if t.Failed() {
		t.Log(out)
	}
}

// A log written before the log handler redacted build flags still holds
// a "Build Info:" record (a Go runtime's startup record) with the -ldflags the build used, which
// inject the Last.fm and Discord credentials; the bundle drops them.
func TestBundleScrubsBuildCredentialsFromOldLogs(t *testing.T) {
	const (
		key    = "fakeLastfmKey0123456789abcdef"
		secret = "fakeLastfmSecret0123456789abcd"
		client = "112233445566778899"
	)
	ldflags := "-X main.version=0.1.0 -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmKey=" + key +
		" -X github.com/coredev-uk/sona-go/internal/scrobble.lastfmSecret=" + secret +
		" -X github.com/coredev-uk/sona-go/internal/discord.clientID=" + client
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sona.log")
	lines := []string{
		`{"level":"INFO","msg":"startup: process start","version":"0.1.0"}`,
		`{"level":"INFO","msg":"Build Info:","Framework":"v1.0.0","-tags":"sqlite_fts5","-ldflags":"` + ldflags + `","CGO_ENABLED":"1"}`,
		`{"level":"INFO","msg":"Platform Info:","Name":"MacOS","Version":"27.2"}`,
	}
	if err := os.WriteFile(logFile+".1", []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logFile, []byte("time=2026-10-31T16:56:22Z level=INFO msg=\"Build Info:\" -ldflags=\""+ldflags+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Bundle(BundleOptions{LogFile: logFile, StateDir: dir})
	for _, s := range []string{key, secret, client} {
		if strings.Contains(out, s) {
			t.Errorf("bundle contains %q", s)
		}
	}
	for _, want := range []string{
		`"-ldflags":"` + logging.Redacted + `","CGO_ENABLED":"1"`, `-ldflags="` + logging.Redacted + `"`,
		`"-tags":"sqlite_fts5"`, `"msg":"Platform Info:","Name":"MacOS","Version":"27.2"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("bundle lacks %q", want)
		}
	}
	if t.Failed() {
		t.Log(out)
	}
}

func TestBundleLogTailSpansRotation(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sona.log")
	if err := os.WriteFile(logFile+".1", []byte("old1\nold2\nold3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logFile, []byte("new1\nnew2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := Bundle(BundleOptions{LogFile: logFile, LogLines: 4, StateDir: dir})
	if !strings.Contains(out, "## Log (last 4 lines)\nold2\nold3\nnew1\nnew2\n") {
		t.Fatalf("log tail:\n%s", out)
	}
}

// A panic on a background goroutine of a process that called
// CaptureCrashes lands in its crash file; the next start keeps it as the
// previous crash, logs it, and the bundle carries it.
func TestCaptureCrashes(t *testing.T) {
	dir := t.TempDir()
	if err := runCrashingApp(t, dir, "boom from a background goroutine"); err == nil {
		t.Fatal("the crashing app exited cleanly")
	}
	crash, _ := os.ReadFile(filepath.Join(dir, CrashFile))
	if !strings.Contains(string(crash), "panic: boom from a background goroutine") || !strings.Contains(string(crash), "goroutine ") {
		t.Fatalf("crash file:\n%s", crash)
	}

	if err := runCrashingApp(t, dir, ""); err != nil {
		t.Fatal("clean start:", err)
	}
	if fi, err := os.Stat(filepath.Join(dir, CrashFile)); err != nil || fi.Size() != 0 {
		t.Fatalf("this run's crash file: %v, %v", fi, err)
	}
	logData, _ := os.ReadFile(filepath.Join(dir, "sona.log"))
	if !strings.Contains(string(logData), "the previous run crashed") {
		t.Fatalf("log:\n%s", logData)
	}
	out := Bundle(BundleOptions{LogFile: filepath.Join(dir, "sona.log"), StateDir: dir})
	if !strings.Contains(out, "## Previous crash") || !strings.Contains(out, "panic: boom from a background goroutine") {
		t.Fatalf("bundle:\n%s", out)
	}
}
