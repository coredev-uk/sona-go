package app

import (
	"os"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/devinstance"
)

// BusName follows SONA_DEV_INSTANCE in a development build and never in a
// production one (run with the variable set, with and without -tags dev).
func TestBusNameInstance(t *testing.T) {
	want := "dev.coredev.Sona"
	if devinstance.Name() != "" {
		want += ".instance_" + devinstance.Name()
	}
	if BusName != want {
		t.Fatalf("BusName = %q, want %q (%s=%q)", BusName, want, devinstance.Env, os.Getenv(devinstance.Env))
	}
	if !strings.HasPrefix(BusName, "dev.coredev.Sona") || autostartFile != "dev.coredev.Sona.desktop" {
		t.Fatalf("base name or autostart file changed: %q %q", BusName, autostartFile)
	}
}
