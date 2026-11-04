package mpris

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/devinstance"
)

// BusName follows SONA_DEV_INSTANCE in a development build and never in a
// production one (run with the variable set, with and without -tags dev).
func TestBusNameInstance(t *testing.T) {
	want := "org.mpris.MediaPlayer2.sona"
	if devinstance.Name() != "" {
		want += ".instance_" + devinstance.Name()
	}
	if BusName != want {
		t.Fatalf("BusName = %q, want %q", BusName, want)
	}
}
