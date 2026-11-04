package account

import (
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

func TestIsSignedOutNotWhileGoReadsTheKeyringOrBeforeItReported(t *testing.T) {
	for _, state := range []string{"", "restoring"} {
		if IsSignedOut(state) {
			t.Errorf("IsSignedOut(%q)", state)
		}
	}
}

func TestIsSignedOutKeepsTheLibraryWhenAppleOnlyAsksToSignInAgain(t *testing.T) {
	for _, state := range []string{"needs_reauth", "signed_in", "validating"} {
		if IsSignedOut(state) {
			t.Errorf("IsSignedOut(%q)", state)
		}
	}
}

func TestIsSignedOutWithoutATokenAndWhileASignInRuns(t *testing.T) {
	for _, state := range []string{"signed_out", "signing_in", "error"} {
		if !IsSignedOut(state) {
			t.Errorf("IsSignedOut(%q) is false", state)
		}
	}
}

func TestHasAccountIsSignedInAndValidatingOnly(t *testing.T) {
	for state, want := range map[string]bool{
		"signed_in": true, "validating": true,
		"": false, "restoring": false, "signed_out": false, "signing_in": false, "needs_reauth": false, "error": false,
	} {
		if got := HasAccount(state); got != want {
			t.Errorf("HasAccount(%q) = %v", state, got)
		}
	}
}

func TestNeedsAttention(t *testing.T) {
	for name, tc := range map[string]struct {
		st   services.AuthStatus
		want bool
	}{
		"unknown":                {services.AuthStatus{}, false},
		"needs reauth":           {services.AuthStatus{State: "needs_reauth"}, true},
		"error":                  {services.AuthStatus{State: "error"}, true},
		"signed out, no keyring": {services.AuthStatus{State: "signed_out", Keyring: "unavailable"}, true},
		"signed out":             {services.AuthStatus{State: "signed_out"}, false},
		"signed in":              {services.AuthStatus{State: "signed_in", Keyring: "unavailable"}, false},
		"restoring":              {services.AuthStatus{State: "restoring", Keyring: "waiting"}, false},
		"offline":                {services.AuthStatus{State: "signed_in", Offline: true}, false},
	} {
		if got := NeedsAttention(tc.st); got != tc.want {
			t.Errorf("%s: NeedsAttention = %v", name, got)
		}
	}
}
