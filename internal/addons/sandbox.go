package addons

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/coredev-uk/sona-go/internal/addons/sandbox"
)

// TempName is the folder inside an add-on's data folder that TMPDIR names.
const TempName = "tmp"

// The reasons the sandbox can't be set up (Settings → Add-ons words each).
const (
	SandboxUserNamespaceRestricted = string(sandbox.ReasonUserNamespaceRestricted)
	SandboxNoUserNamespace         = string(sandbox.ReasonNoUserNamespace)
	SandboxNoSeccomp               = string(sandbox.ReasonNoSeccomp)
	SandboxNoSeatbelt              = string(sandbox.ReasonNoSeatbelt)
	SandboxNoAppContainer          = string(sandbox.ReasonNoAppContainer)
	SandboxUnsupported             = string(sandbox.ReasonUnsupported)
	SandboxSetup                   = string(sandbox.ReasonSetup)
)

// SandboxReason is the reason err gives for the sandbox failing to start,
// false when err is no sandbox failure.
func SandboxReason(err error) (string, bool) {
	r, ok := sandbox.ReasonOf(err)
	return string(r), ok
}

// SandboxText is a reason in plain words, for an add-on's log. The reasons
// are worded again, translated, in Settings → Add-ons.
func SandboxText(reason string) string {
	switch reason {
	case SandboxUserNamespaceRestricted:
		return "this system takes away the rights of user namespaces from programs it doesn't know (AppArmor's kernel.apparmor_restrict_unprivileged_userns); Sona's sandbox needs them"
	case SandboxNoUserNamespace:
		return "this system doesn't let Sona create user namespaces, which its sandbox needs (a kernel setting, a container or a Flatpak)"
	case SandboxNoSeccomp:
		return "the system-call filter of the sandbox can't be installed here"
	case SandboxNoSeatbelt:
		return "macOS's sandbox-exec is missing or refused Sona's profile"
	case SandboxNoAppContainer:
		return "Windows could not create or start the AppContainer of the sandbox"
	case SandboxUnsupported:
		return "this operating system has no sandbox for add-ons"
	}
	return "the sandbox could not be set up"
}

// EnsureData makes the add-on's private folder (SONA_ADDON_DATA) and its
// temporary folder, owner-only where the OS has modes. It is never removed by
// Sona: not when the add-on is switched off, nor when its folder is deleted.
// A link in the place of the folder itself is refused: the folder is what the
// sandbox lets the add-on write to.
func EnsureData(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("addons: data folder: %w", err)
	}
	if fi, err := os.Lstat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("addons: %s is not a folder", dir)
	}
	// The temporary folder is the add-on's to fill, and so to replace: where it
	// left a file or a link in its place, Sona removes that (never following
	// it) and makes the folder again. Sona opens nothing else in the folder.
	tmp := filepath.Join(dir, TempName)
	if fi, err := os.Lstat(tmp); err == nil && !fi.IsDir() {
		if err := os.Remove(tmp); err != nil {
			return fmt.Errorf("addons: data folder: %w", err)
		}
	}
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		return fmt.Errorf("addons: data folder: %w", err)
	}
	return nil
}
