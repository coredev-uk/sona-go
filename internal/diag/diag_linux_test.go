//go:build linux

package diag

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/logging"
	"github.com/coredev-uk/sona-go/internal/playback/cdm/cdmtest"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

func init() {
	// The test binary doubles as the CDM host child; ChildMain returns at
	// once unless this process is one.
	cdmhost.ChildMain()
}

// A CDM host child killed by a signal is logged with its exit, signal
// report and stderr tail, and that record reaches the bundle without the
// tokens logged around it.
func TestChildCrashInBundle(t *testing.T) {
	lib := cdmtest.Library(t)
	dir := t.TempDir()
	logFile := filepath.Join(dir, "sona.log")
	log, closeLog, err := logging.New(logging.Options{File: logFile, Level: slog.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	log.Info("apple request", "media-user-token", fakeMUT, "url", "https://amp-api.music.apple.com/v1/me?x=1 Authorization: Bearer "+fakeJWT)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h, err := cdmhost.Start(ctx, cdmhost.Options{CDMPath: lib, Sink: "fakesink", Logger: log})
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(h.Info().PID, syscall.SIGABRT); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("child did not exit")
	}
	_ = closeLog()

	out := Bundle(BundleOptions{LogFile: logFile, StateDir: dir, Home: os.Getenv("HOME")})
	for _, want := range []string{"cdmhost: child exited unexpectedly", `"exit_code":2`, "SIGABRT: abort", "goroutine "} {
		if !strings.Contains(out, want) {
			t.Errorf("bundle lacks %q", want)
		}
	}
	for _, s := range append(secrets()[:4], logging.Redacted+"]") {
		if strings.Contains(out, s) {
			t.Errorf("bundle contains %q", s)
		}
	}
	if t.Failed() {
		t.Log(out)
	}
}
