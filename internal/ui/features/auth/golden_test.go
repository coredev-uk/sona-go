package auth

import (
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// The open account dialog: signed out (the sign-in form with the Apple
// Account field focused), signed in, and stacked in a window too narrow for
// two panels; and the account menu with everything worth knowing in it. The
// images are generated with `go test ./internal/ui/features/auth -update` and
// live in testdata/golden.

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

func TestGoldenAccountDialogSignedInLight(t *testing.T) {
	r := newRig(t, signedIn)
	r.h.Env.Surfaces.OpenAccount()
	r.tt.Drain()
	r.tt.Advance(500 * time.Millisecond)
	r.tt.Frame()
	r.shows("Signed in to Apple Music", "Storefront: United Kingdom · British English")
	uitest.Golden(t, r.tt.Image(), "account-dialog-signed-in-light")
}

func TestGoldenAccountDialogStackedDark(t *testing.T) {
	r := newRig(t, signedOut, uitest.Dark())
	r.tt.SetSize(600, 700)
	r.h.Env.Surfaces.OpenAccount()
	r.tt.Drain()
	r.tt.Advance(500 * time.Millisecond)
	r.tt.Frame()
	r.shows("Sign in to Apple Music")
	uitest.Golden(t, r.tt.Image(), "account-dialog-stacked-dark")
}

func TestGoldenAccountMenuLight(t *testing.T) {
	st := signedIn
	st.Restricted, st.Offline = true, true
	r := newRig(t, st)
	r.openMenu()
	r.tt.Advance(500 * time.Millisecond)
	r.tt.Frame()
	r.shows("Signed in · United Kingdom")
	uitest.Golden(t, r.tt.Image(), "account-menu-light")
}

func TestGoldenAccountDialogSignedOutLight(t *testing.T) {
	goldenDialog(t, "account-dialog-signed-out-light")
}

func TestGoldenAccountDialogSignedOutDark(t *testing.T) {
	goldenDialog(t, "account-dialog-signed-out-dark", uitest.Dark())
}
