//go:build unix

package addons

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// install writes an add-on folder: its manifest and a script as its command.
func install(t *testing.T, root, dir, manifest string) string {
	t.Helper()
	d := filepath.Join(root, dir)
	if err := os.MkdirAll(filepath.Join(d, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, ManifestName), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "bin", "run"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return d
}

func manifestFor(id string) string {
	return strings.Replace(goodManifest, `"demo"`, `"`+id+`"`, 1)
}

func problemOf(t *testing.T, entries []Entry, dir string) string {
	t.Helper()
	for _, e := range entries {
		if e.Dir == dir {
			if e.Problem == nil {
				return ""
			}
			return e.Problem.Code
		}
	}
	t.Fatalf("no entry for %q in %+v", dir, entries)
	return ""
}

func scan(t *testing.T, root string) []Entry {
	t.Helper()
	es, err := Scan(root, ScanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return es
}

func TestScanListsValidAndRefusedAddons(t *testing.T) {
	root := t.TempDir()
	good := install(t, root, "demo", manifestFor("demo"))
	install(t, root, "other", manifestFor("demo")) // id is not the folder's
	install(t, root, "lastfm", manifestFor("lastfm"))
	install(t, root, "sona-tools", manifestFor("sona-tools"))
	install(t, root, "broken", `{"id":"broken","oops":1}`)
	install(t, root, "Mixed", manifestFor("mixed")) // folder name compares case-insensitively
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0o700); err != nil {
		t.Fatal(err)
	}
	es := scan(t, root)
	if len(es) != 6 {
		t.Fatalf("entries = %+v", es)
	}
	for dir, want := range map[string]string{
		"demo": "", "other": ProblemIDMismatch, "lastfm": ProblemReservedID,
		"sona-tools": ProblemReservedID, "broken": ProblemManifest, "Mixed": "",
	} {
		if got := problemOf(t, es, dir); got != want {
			t.Errorf("%s: problem %q, want %q", dir, got, want)
		}
	}
	for _, e := range es {
		if e.Dir == "demo" {
			real, _ := filepath.EvalSymlinks(good)
			if e.Exe != filepath.Join(real, "bin", "run") || e.Manifest.Name != "Demo" {
				t.Errorf("demo = %+v", e)
			}
		}
	}
	// Sona's own bundle may use the reserved IDs.
	bundled, err := Scan(root, ScanOptions{Bundled: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := problemOf(t, bundled, "lastfm"); got != "" {
		t.Errorf("bundled lastfm: %q", got)
	}
	if es, err := Scan(filepath.Join(root, "missing"), ScanOptions{}); err != nil || es != nil {
		t.Errorf("missing root = %v, %v", es, err)
	}
}

func TestScanRefusesExecutablesOutsideTheFolder(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		command string
		setup   func(d string)
		code    string
	}{
		"link out of the folder": {"bin/run", func(d string) {
			_ = os.Remove(filepath.Join(d, "bin", "run"))
			_ = os.Symlink(outside, filepath.Join(d, "bin", "run"))
		}, ProblemEscapes},
		"link to a folder out":  {"out/evil", func(d string) { _ = os.Symlink(filepath.Dir(outside), filepath.Join(d, "out")) }, ProblemEscapes},
		"missing":               {"bin/none", func(string) {}, ProblemNoExecutable},
		"a folder":              {"bin", func(string) {}, ProblemNoExecutable},
		"the manifest itself":   {"addon.json", func(string) {}, ProblemNoExecutable},
		"not executable":        {"bin/run", func(d string) { _ = os.Chmod(filepath.Join(d, "bin", "run"), 0o600) }, ProblemNoExecutable},
		"group writable":        {"bin/run", func(d string) { _ = os.Chmod(filepath.Join(d, "bin", "run"), 0o770) }, ProblemUnsafe},
		"world writable folder": {"bin/run", func(d string) { _ = os.Chmod(filepath.Join(d, "bin"), 0o777) }, ProblemUnsafe},
		"absolute":              {"/bin/sh", func(string) {}, ProblemCommand},
		"parent":                {"../demo/bin/run", func(string) {}, ProblemCommand},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			m := strings.Replace(manifestFor("demo"), "bin/run", tc.command, 1)
			d := install(t, dir, "demo", m)
			tc.setup(d)
			if err := os.Chmod(filepath.Join(d, "bin"), mustMode(t, filepath.Join(d, "bin"))); err != nil {
				t.Fatal(err)
			}
			es := scan(t, dir)
			if got := problemOf(t, es, "demo"); got != tc.code {
				t.Errorf("problem %q, want %q (%+v)", got, tc.code, es)
			}
			if es[0].Exe != "" {
				t.Errorf("refused add-on has an executable: %q", es[0].Exe)
			}
		})
	}
	_ = root
}

func mustMode(t *testing.T, p string) os.FileMode {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return fi.Mode().Perm()
}

// A link inside the folder that stays inside it is fine.
func TestScanAllowsALinkInsideTheFolder(t *testing.T) {
	root := t.TempDir()
	d := install(t, root, "demo", strings.Replace(manifestFor("demo"), "bin/run", "run", 1))
	if err := os.Symlink(filepath.Join("bin", "run"), filepath.Join(d, "run")); err != nil {
		t.Fatal(err)
	}
	if got := problemOf(t, scan(t, root), "demo"); got != "" {
		t.Errorf("problem %q", got)
	}
}

func TestScanRefusesAFolderThatIsALink(t *testing.T) {
	root := t.TempDir()
	elsewhere := install(t, t.TempDir(), "demo", manifestFor("demo"))
	if err := os.Symlink(elsewhere, filepath.Join(root, "demo")); err != nil {
		t.Fatal(err)
	}
	if got := problemOf(t, scan(t, root), "demo"); got != ProblemUnsafe {
		t.Errorf("problem %q", got)
	}
}

func TestScanRefusesFoldersOthersMayWrite(t *testing.T) {
	root := t.TempDir()
	d := install(t, root, "demo", manifestFor("demo"))
	if err := os.Chmod(d, 0o775); err != nil {
		t.Fatal(err)
	}
	if got := problemOf(t, scan(t, root), "demo"); got != ProblemUnsafe {
		t.Errorf("group-writable folder: %q", got)
	}
	if err := os.Chmod(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(d, ManifestName), 0o666); err != nil {
		t.Fatal(err)
	}
	if got := problemOf(t, scan(t, root), "demo"); got != ProblemUnsafe {
		t.Errorf("world-writable manifest: %q", got)
	}
	// An unsafe root refuses the whole scan.
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(root, ScanOptions{}); ProblemOf(err) != ProblemUnsafe {
		t.Errorf("world-writable root: %v", err)
	}
}

func TestScanRefusesAnotherUsersFolder(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root to hand a folder to another user")
	}
	root := t.TempDir()
	d := install(t, root, "demo", manifestFor("demo"))
	if err := os.Chown(d, 12345, 12345); err != nil {
		t.Fatal(err)
	}
	if got := problemOf(t, scan(t, root), "demo"); got != ProblemUnsafe {
		t.Errorf("problem %q", got)
	}
	_ = syscall.Getuid
}

func TestScanRefusesAnOversizeManifestWithoutReadingItAll(t *testing.T) {
	root := t.TempDir()
	d := install(t, root, "demo", manifestFor("demo"))
	big := append([]byte(`{"id":"demo","description":"`), []byte(strings.Repeat("x", 4*MaxManifestBytes))...)
	if err := os.WriteFile(filepath.Join(d, ManifestName), big, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := problemOf(t, scan(t, root), "demo"); got != ProblemTooLarge {
		t.Errorf("problem %q", got)
	}
}

func TestScanStopsAtTheLimit(t *testing.T) {
	root := t.TempDir()
	for i := range MaxScanned + 5 {
		if err := os.Mkdir(filepath.Join(root, "d"+strings.Repeat("a", 3)+string(rune('a'+i/26))+string(rune('a'+i%26))), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	es := scan(t, root)
	if len(es) != MaxScanned+1 || es[len(es)-1].Problem == nil || es[len(es)-1].Problem.Code != ProblemTooMany {
		t.Errorf("%d entries, last %+v", len(es), es[len(es)-1])
	}
}
