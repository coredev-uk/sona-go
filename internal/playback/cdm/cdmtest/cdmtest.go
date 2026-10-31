// Package cdmtest gives tests Google's real Widevine CDM through the same
// downloader Sona uses (internal/playback/cdm), so the real-CDM tests run
// wherever the network reaches Google, CI included.
package cdmtest

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/cdm"
)

// Environment variables the real-CDM tests read.
const (
	// EnvCache is the directory the CDM is installed into for tests
	// (CI caches it, keyed by the manifest's version); default
	// <user cache dir>/sona-tests/widevine, shared by every checkout.
	EnvCache = "SONA_CDM_CACHE"
	// EnvRequire, when set, fails a test that would skip for want of the
	// network (CI, which must run them).
	EnvRequire = "SONA_CDM_REQUIRE"
)

var (
	once sync.Once
	lib  string
	err  error
)

// Library returns the real CDM: the one installed in the test cache,
// downloaded and verified there first when missing. It skips the test, saying why,
// only when the download cannot reach the network; any other failure
// fails it.
func Library(t testing.TB) string {
	t.Helper()
	once.Do(func() { lib, err = find() })
	switch {
	case err == nil:
		return lib
	case cdm.CodeOf(err) == cdm.CodeNetwork && os.Getenv(EnvRequire) == "":
		t.Skipf("offline: the Widevine CDM can't be downloaded: %v", err)
	}
	t.Fatalf("Widevine CDM for tests: %v", err)
	return ""
}

func find() (string, error) {
	dir := os.Getenv(EnvCache)
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "sona-tests", "widevine")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	inst, err := cdm.Ensure(ctx, cdm.Options{Dir: dir})
	return inst.Path, err
}
