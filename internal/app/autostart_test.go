package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutostartEntryQuotesExec(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"/usr/bin/sona", HiddenFlag}, "Exec=/usr/bin/sona --hidden"},
		{[]string{"/home/me/My Apps/Sona.AppImage"}, `Exec="/home/me/My Apps/Sona.AppImage"`},
		// Spec: quoting escapes $ and \, then the string escape doubles every backslash.
		{[]string{`/opt/a$b\c`}, `Exec="/opt/a\\$b\\\\c"`},
		// % would start a field code.
		{[]string{"/opt/100%/sona"}, "Exec=/opt/100%%/sona"},
	} {
		got, err := AutostartEntry(tc.argv)
		if err != nil {
			t.Fatalf("%q: %v", tc.argv, err)
		}
		if !strings.Contains(got, "\n"+tc.want+"\n") {
			t.Errorf("%q: entry has no %q:\n%s", tc.argv, tc.want, got)
		}
	}
	if _, err := AutostartEntry([]string{"/opt/sona\nHidden=true"}); err == nil {
		t.Error("a newline in the command must be rejected (it would inject keys)")
	}
}

func TestAutostartPathFollowsXDGConfigHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	got, err := AutostartPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "autostart", "dev.coredev.Sona.desktop"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	// A relative XDG_CONFIG_HOME is invalid per the base directory spec.
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	t.Setenv("HOME", dir)
	got, err = AutostartPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, ".config", "autostart", "dev.coredev.Sona.desktop"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestAutostartWriteExistsRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "autostart", autostartFile)
	if AutostartExists(path) {
		t.Fatal("exists before writing")
	}
	if err := WriteAutostart(path, []string{"/usr/bin/sona", HiddenFlag}); err != nil {
		t.Fatal(err)
	}
	if !AutostartExists(path) {
		t.Fatal("missing after writing")
	}
	if err := RemoveAutostart(path); err != nil {
		t.Fatal(err)
	}
	if AutostartExists(path) {
		t.Fatal("exists after removing")
	}
	if err := RemoveAutostart(path); err != nil {
		t.Fatalf("removing twice: %v", err)
	}
}

func TestAutostartDisabledByTheDesktopCountsAsAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), autostartFile)
	for _, line := range []string{"Hidden=true", "X-GNOME-Autostart-enabled=false"} {
		body, err := AutostartEntry([]string{"/usr/bin/sona"})
		if err != nil {
			t.Fatal(err)
		}
		body = strings.Replace(body, "X-GNOME-Autostart-enabled=true\n", line+"\n", 1)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if AutostartExists(path) {
			t.Errorf("%s: counted as enabled", line)
		}
	}
}
