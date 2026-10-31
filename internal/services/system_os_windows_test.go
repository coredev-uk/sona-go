package services

import (
	"regexp"
	"testing"
)

// The running Windows' edition, version and build come back from the
// registry ("Windows 11 Pro 24H2 (build 26100.2605)").
func TestOSNameReadsRegistry(t *testing.T) {
	got := osName()
	if !regexp.MustCompile(`^Windows .*\(build \d+(\.\d+)?\)$`).MatchString(got) {
		t.Fatalf("osName() = %q, want Windows … (build <n>[.<ubr>])", got)
	}
	t.Log(got)
}
