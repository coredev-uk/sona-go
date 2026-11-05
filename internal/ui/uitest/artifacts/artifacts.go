// Package artifacts says where a failing test leaves its pictures. It is a
// leaf package (no Sona imports) because uitest imports kit and kit's own
// tests need it.
package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Env names the directory CI collects failed-golden pictures from
// (.github/ci/test.sh sets it).
const Env = "SONA_TEST_ARTIFACTS"

// Dir returns the directory for t's artifacts, created: $SONA_TEST_ARTIFACTS/
// <test name> when the variable is set, else t.ArtifactDir(). Call it only
// when a failure has something to leave: t.ArtifactDir makes go test treat
// the run as one that uses -artifacts, which no cached result survives, and
// the variable keeps a passing test out of that.
func Dir(t testing.TB) string {
	t.Helper()
	base := os.Getenv(Env)
	if base == "" {
		return t.ArtifactDir()
	}
	dir := filepath.Join(base, strings.NewReplacer("/", "_", `\`, "_").Replace(t.Name()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("artifacts: %v", err)
	}
	return dir
}
