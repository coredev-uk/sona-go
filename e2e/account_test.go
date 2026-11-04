//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/e2e/automation"
)

// TestAccount is the account entry of the top bar end to end on the
// signed-out fixture profile: the avatar's menu offers Sign in, which opens
// the account dialog with the native sign-in form and its fallbacks; Escape
// closes it. (Signing in itself needs Apple's endpoints: the probe build has
// no seam for fake ones, see docs/ui-architecture.md, "Account and
// sign-in". The same flow runs against fakes in internal/ui/account.)
func TestAccount(t *testing.T) {
	a := startApp(t)
	c := a.client
	if _, err := c.Wait(startTimeout, "the Albums page", func(tr *automation.Tree) bool { return tr.HasText("24 albums") }); err != nil {
		t.Fatal(err)
	}

	if err := c.Press(10*time.Second, "Account"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Wait(10*time.Second, "the account menu", func(tr *automation.Tree) bool {
		return tr.HasText("Not signed in") && tr.HasText("Sign in")
	}); err != nil {
		t.Fatal(err)
	}
	// The menu's row is not a button: the last node of that label is the row
	// (the menu is the topmost overlay).
	menu, err := c.Tree()
	if err != nil {
		t.Fatal(err)
	}
	var row automation.Node
	for _, n := range menu.Nodes {
		if n.Label == "Sign in" {
			row = n
		}
	}
	x, y := row.Bounds.Centre()
	if err := c.Input(automation.Click(x, y)); err != nil {
		t.Fatal(err)
	}
	tree, err := c.Wait(10*time.Second, "the account dialog's sign-in form", func(tr *automation.Tree) bool {
		return tr.HasText("Sign in to Apple Music") && tr.HasText("Apple Account") && tr.HasText("Password")
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Sign in with your browser", "Paste a Music User Token"} {
		if !tree.HasText(want) {
			t.Errorf("the dialog does not offer %q", want)
		}
	}
	if err := c.Idle(5 * time.Second); err != nil {
		t.Log("idle:", err)
	}
	if _, data, err := c.Capture(); err == nil {
		_ = os.WriteFile(filepath.Join(a.out, "account-dialog.png"), data, 0o644)
	}

	if err := c.Input(automation.Key("Escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Wait(10*time.Second, "the dialog to close", func(tr *automation.Tree) bool {
		return !tr.HasText("Sign in to Apple Music") && tr.HasText("24 albums")
	}); err != nil {
		t.Fatal(err)
	}
}

// TestFirstRun is the first start end to end: a profile whose setup was
// never finished opens on the setup layer over Home; Continue walks the
// steps, the Apple Music step offers the sign-in, and Skip setup leaves it
// for good.
func TestFirstRun(t *testing.T) {
	a := startAppWith(t, "-first-run")
	c := a.client
	if _, err := c.Wait(startTimeout, "the first-run setup", func(tr *automation.Tree) bool {
		return tr.HasText("Set up Sona") && tr.HasText("Welcome to Sona")
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Idle(5 * time.Second); err != nil {
		t.Log("idle:", err)
	}
	if _, data, err := c.Capture(); err == nil {
		_ = os.WriteFile(filepath.Join(a.out, "setup-welcome.png"), data, 0o644)
	}

	if err := c.Press(10*time.Second, "Get started"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Wait(10*time.Second, "the Apple Music step", func(tr *automation.Tree) bool {
		return tr.HasText("Sign in to Apple Music") || tr.HasText("Apple Account")
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.Idle(5 * time.Second); err != nil {
		t.Log("idle:", err)
	}
	if _, data, err := c.Capture(); err == nil {
		_ = os.WriteFile(filepath.Join(a.out, "setup-account.png"), data, 0o644)
	}

	if err := c.Press(10*time.Second, "Skip setup"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Wait(10*time.Second, "the setup to close over Home", func(tr *automation.Tree) bool {
		return !tr.HasText("Set up Sona") && tr.HasText("Recommendations need Apple Music")
	}); err != nil {
		t.Fatal(err)
	}
}
