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

// The signed-in dialog with the Touch ID row: the switch where the build can
// keep the token behind Touch ID, the requirement where a Mac build can't.

func goldenSignedIn(t *testing.T, name, goos string, st services.AuthStatus, want string, opts ...uitest.Option) {
	t.Helper()
	r := newRigOS(t, st, goos, opts...)
	r.h.Env.Surfaces.OpenAccount()
	r.tt.Drain()
	r.tt.Advance(500 * time.Millisecond)
	r.tt.Frame()
	r.shows(want)
	uitest.Golden(t, r.tt.Image(), name)
}

func touchIDAvailable() services.AuthStatus {
	st := signedIn
	st.BiometricAvailable = true
	return st
}

func TestGoldenAccountDialogTouchIDLight(t *testing.T) {
	goldenSignedIn(t, "account-dialog-touch-id-light", "darwin", touchIDAvailable(), "Use Touch ID to unlock Sona")
}

func TestGoldenAccountDialogTouchIDDark(t *testing.T) {
	goldenSignedIn(t, "account-dialog-touch-id-dark", "darwin", touchIDAvailable(), "Use Touch ID to unlock Sona", uitest.Dark())
}

func TestGoldenAccountDialogTouchIDRequirementLight(t *testing.T) {
	goldenSignedIn(t, "account-dialog-touch-id-requirement-light", "darwin", signedIn, "Touch ID unlock isn't available in this build")
}

func TestGoldenAccountDialogTouchIDRequirementDark(t *testing.T) {
	goldenSignedIn(t, "account-dialog-touch-id-requirement-dark", "darwin", signedIn, "Touch ID unlock isn't available in this build", uitest.Dark())
}
