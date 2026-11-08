package update

import (
	"os"
	"path/filepath"
	"testing"
)

// writePlist gives a macOS bundle the Info.plist that build/darwin/Info.plist.in
// makes, for version v (nothing on Windows, which has no bundle).
func writePlist(t *testing.T, g Guard, v string) {
	t.Helper()
	if g.GOOS != "darwin" {
		return
	}
	p := filepath.Join(g.Root, "Contents", "Info.plist")
	_ = os.Remove(p) // as an update does: files are replaced, never written through
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>Sona</string>
	<key>CFBundleShortVersionString</key>
	<string>` + v + `</string>
	<key>CFBundleVersion</key>
	<string>` + v + `</string>
</dict>
</plist>
`
	if err := os.WriteFile(p, []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
}

// updated is an install of 0.3.0-rc.4 that replaced 0.2.0, as an update
// leaves it.
func updated(t *testing.T, goos string) Guard {
	t.Helper()
	g := layout(t, goos, "0.2.0")
	writePlist(t, g, "0.2.0")
	install(t, g, "0.3.0-rc.4")
	writePlist(t, g, "0.3.0-rc.4")
	return g
}

func TestThePreviousVersionIsKnown(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			g := layout(t, goos, "0.2.0")
			if v, ok := g.HasPrevious(); ok || v != "" {
				t.Fatalf("before any update: %q, %v", v, ok)
			}
			g = updated(t, goos)
			if v, ok := g.HasPrevious(); !ok || v != "0.2.0" {
				t.Fatalf("after an update: %q, %v", v, ok)
			}
		})
	}
}

// A snapshot made by a Sona that recorded no version is still there to go
// back to, without a number.
func TestAPreviousVersionWithoutANumberIsStillKept(t *testing.T) {
	g := updated(t, "windows")
	if err := os.Remove(filepath.Join(g.State, previousFile)); err != nil {
		t.Fatal(err)
	}
	if v, ok := g.HasPrevious(); !ok || v != "" {
		t.Fatalf("HasPrevious = %q, %v", v, ok)
	}
	g = updated(t, "darwin")
	if err := os.Remove(filepath.Join(g.Previous, "Contents", "Info.plist")); err != nil {
		t.Fatal(err)
	}
	if v, ok := g.HasPrevious(); !ok || v != "" {
		t.Fatalf("darwin HasPrevious = %q, %v", v, ok)
	}
}

func TestABinaryPlistHasNoReadableVersion(t *testing.T) {
	g := updated(t, "darwin")
	if err := os.WriteFile(filepath.Join(g.Previous, "Contents", "Info.plist"), []byte("bplist00\x00\x01"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, ok := g.HasPrevious(); !ok || v != "" {
		t.Fatalf("HasPrevious = %q, %v", v, ok)
	}
}

func TestAnEarlierSnapshotIsReplacedWithItsVersion(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			g := updated(t, goos)
			// The next update keeps 0.3.0-rc.4.
			if err := g.Snapshot("0.3.0-rc.4"); err != nil {
				t.Fatal(err)
			}
			if v, ok := g.HasPrevious(); !ok || v != "0.3.0-rc.4" {
				t.Fatalf("HasPrevious = %q, %v", v, ok)
			}
			if read(t, filepath.Join(g.Previous, filepath.FromSlash(map[string]string{
				"darwin": "Contents/MacOS/sona", "windows": "sona.exe",
			}[goos]))) != "0.3.0-rc.4" {
				t.Fatal("the snapshot isn't the running version")
			}
		})
	}
}

func TestRestoringByChoicePutsThePreviousVersionBack(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			g := updated(t, goos)
			// The update is still on trial, as right after Install.
			if err := g.Mark("0.2.0", "0.3.0-rc.4"); err != nil {
				t.Fatal(err)
			}
			if err := g.RestorePrevious("0.3.0-rc.4"); err != nil {
				t.Fatal(err)
			}
			if read(t, exeOf(g)) != "0.2.0" {
				t.Fatal("the old version isn't back")
			}
			if g.Pending() != nil {
				t.Fatal("the marker stays")
			}
			if _, ok := g.HasPrevious(); ok {
				t.Fatal("a previous version is still offered")
			}
			if _, err := os.Stat(g.Previous); !os.IsNotExist(err) {
				t.Fatalf("the snapshot stays: %v", err)
			}
			if _, err := os.Stat(filepath.Join(g.State, previousFile)); !os.IsNotExist(err) {
				t.Fatalf("the recorded version stays: %v", err)
			}
			r := g.LastRollback()
			if r == nil || !r.Manual || r.From != "0.3.0-rc.4" || r.To != "0.2.0" || r.Error != "" || r.At.IsZero() {
				t.Fatalf("report = %+v", r)
			}
			// The restored version starts normally, not as a trial.
			if act, _ := g.Begin("0.2.0"); act != Normal {
				t.Fatalf("after the restore: %v", act)
			}
		})
	}
}

func TestRestoringByChoiceWithoutAPreviousVersionSaysSo(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			g := layout(t, goos, "0.3.0")
			if err := g.RestorePrevious("0.3.0"); err == nil {
				t.Fatal("restored nothing without error")
			}
			if read(t, exeOf(g)) != "0.3.0" {
				t.Fatal("the running version changed")
			}
			if r := g.LastRollback(); r == nil || !r.Manual || r.Error == "" {
				t.Fatalf("report = %+v", r)
			}
		})
	}
}

// The guard's own rollback of a failed update is not a manual one.
func TestTheGuardsRollbackIsNotManual(t *testing.T) {
	g := updated(t, "darwin")
	if err := g.Rollback(); err != nil {
		t.Fatal(err)
	}
	if r := g.LastRollback(); r == nil || r.Manual || r.From != "0.3.0-rc.4" || r.To != "0.2.0" {
		t.Fatalf("report = %+v", r)
	}
}
