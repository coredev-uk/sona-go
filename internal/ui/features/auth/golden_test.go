package auth

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The open account dialog, signed out: the sign-in form with the Apple
// Account field focused. The images are generated with
// `go test ./internal/ui/features/auth -update` and live in
// testdata/golden.

func goldenDialog(t *testing.T, name string, opts ...uitest.Option) {
	t.Helper()
	r := newRig(t, signedOut, opts...)
	r.h.Env.Surfaces.OpenAccount()
	r.tt.Drain()
	// The dialog's 200 ms entrance, then a frame at rest.
	r.tt.Advance(500 * time.Millisecond)
	r.tt.Frame()
	r.shows("Sign in to Apple Music")
	uitest.Golden(t, r.tt.Image(), name)
}

func TestGoldenAccountDialogSignedOutLight(t *testing.T) {
	goldenDialog(t, "account-dialog-signed-out-light")
}

func TestGoldenAccountDialogSignedOutDark(t *testing.T) {
	goldenDialog(t, "account-dialog-signed-out-dark", uitest.Dark())
}

// The signed-out dialog on a Mac: "Sign in with Touch ID" (Safari's passkey).
func goldenPasskey(t *testing.T, name string, opts ...uitest.Option) {
	t.Helper()
	st := services.AuthStatus{State: "signed_out", PasskeySignIn: true}
	r := newRig(t, st, opts...)
	r.h.Env.Surfaces.OpenAccount()
	r.tt.Drain()
	r.tt.Advance(500 * time.Millisecond)
	r.tt.Frame()
	r.shows("Sign in with Touch ID")
	uitest.Golden(t, r.tt.Image(), name)
}

func TestGoldenAccountDialogPasskeyLight(t *testing.T) {
	goldenPasskey(t, "account-dialog-passkey-light")
}

func TestGoldenAccountDialogPasskeyDark(t *testing.T) {
	goldenPasskey(t, "account-dialog-passkey-dark", uitest.Dark())
}
