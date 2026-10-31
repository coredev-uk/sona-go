package services

import (
	"regexp"
	"testing"
)

// The running Mac's version and build come back from sysctl, as sw_vers
// prints them ("macOS 27.2 (26B123)").
func TestOSNameReadsSysctl(t *testing.T) {
	got := osName()
	if !regexp.MustCompile(`^macOS \d+(\.\d+)* \([0-9A-Za-z]+\)$`).MatchString(got) {
		t.Fatalf("osName() = %q, want macOS <version> (<build>)", got)
	}
	t.Log(got)
}
