package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// HiddenFlag starts Sona with no window: it plays and sits in the tray. The
// autostart entry passes it when the user chose to start hidden at login.
const HiddenFlag = "--hidden"

// Hidden reports whether the launch asked for no window.
func (a Activation) Hidden() bool { return slices.Contains(a.Args, HiddenFlag) }

// autostartFile is the entry's name in the autostart directory: the
// application ID, like the installed desktop file.
const autostartFile = BusName + ".desktop"

// AutostartPath is where Sona's XDG autostart entry lives:
// $XDG_CONFIG_HOME/autostart/dev.coredev.Sona.desktop, with
// XDG_CONFIG_HOME defaulting to ~/.config.
func AutostartPath() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" || !filepath.IsAbs(dir) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("app: autostart: %w", err)
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", autostartFile), nil
}

// LaunchCommand is the command that starts this build of Sona again: the
// AppImage itself when running from one (its mount point changes every
// run), else this executable.
func LaunchCommand() (string, error) {
	if p := os.Getenv("APPIMAGE"); p != "" && filepath.IsAbs(p) {
		return p, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("app: locate executable: %w", err)
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	return exe, nil
}

// AutostartEntry renders the autostart desktop entry that runs argv.
func AutostartEntry(argv []string) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("app: autostart: empty command")
	}
	words := make([]string, len(argv))
	for i, a := range argv {
		w, err := execWord(a)
		if err != nil {
			return "", err
		}
		words[i] = w
	}
	return "[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=Sona\n" +
		"Comment=Apple Music for the Linux desktop\n" +
		"Exec=" + strings.Join(words, " ") + "\n" +
		"Icon=sona\n" +
		"Terminal=false\n" +
		"X-GNOME-Autostart-enabled=true\n", nil
}

// execWord quotes one Exec argument (Desktop Entry spec, "The Exec key"):
// an argument with a reserved character is double-quoted with ", `, $ and
// \ backslash-escaped; then the string-value escape doubles every
// backslash, and % becomes %% so it is not a field code. Control
// characters cannot be represented and are rejected.
func execWord(s string) (string, error) {
	if s == "" {
		return `""`, nil
	}
	if strings.ContainsFunc(s, func(c rune) bool { return c < 0x20 || c == 0x7f }) {
		return "", fmt.Errorf("app: autostart: control character in %q", s)
	}
	w := s
	if strings.ContainsAny(s, " \t\"'\\><~|&;$*?#()`") {
		var b strings.Builder
		b.WriteByte('"')
		for _, c := range s {
			if strings.ContainsRune("\"`$\\", c) {
				b.WriteByte('\\')
			}
			b.WriteRune(c)
		}
		b.WriteByte('"')
		w = b.String()
	}
	w = strings.ReplaceAll(w, `\`, `\\`)
	return strings.ReplaceAll(w, "%", "%%"), nil
}

// WriteAutostart writes the entry that runs argv at login to path,
// creating the autostart directory.
func WriteAutostart(path string, argv []string) error {
	body, err := AutostartEntry(argv)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("app: autostart: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sona-autostart-*")
	if err != nil {
		return fmt.Errorf("app: autostart: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op after the rename
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("app: autostart: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("app: autostart: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("app: autostart: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("app: autostart: %w", err)
	}
	return nil
}

// RemoveAutostart deletes the entry at path; a missing entry is not an
// error.
func RemoveAutostart(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("app: autostart: %w", err)
	}
	return nil
}

// AutostartExists reports whether an entry is at path. A desktop's own
// "startup applications" settings may disable an entry instead of deleting
// it (Hidden=true, or X-GNOME-Autostart-enabled=false); that counts as
// absent.
func AutostartExists(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "Hidden":
			if strings.TrimSpace(v) == "true" {
				return false
			}
		case "X-GNOME-Autostart-enabled":
			if strings.TrimSpace(v) == "false" {
				return false
			}
		}
	}
	return true
}
