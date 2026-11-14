package update

import (
	"os"
	"path/filepath"
	"testing"
)

// previousExe is the executable inside the kept previous version.
func previousExe(g Guard) string {
	if g.GOOS == "darwin" {
		return filepath.Join(g.Previous, "Contents", "MacOS", "sona")
	}
	return filepath.Join(g.Previous, "sona.exe")
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// An install that fails drops the copy it staged. The version kept before the
// attempt is still the previous version: the field report of a Windows tester
// whose failed installs left a copy of the running version as "previous".
func TestAFailedInstallLeavesThePreviousVersionKept(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			g := updated(t, goos) // running 0.3.0-rc.4, previous 0.2.0
			if err := g.Stage("0.3.0-rc.4"); err != nil {
				t.Fatal(err)
			}
			if !exists(g.staging()) {
				t.Fatal("nothing staged")
			}
			if v, ok := g.HasPrevious(); !ok || v != "0.2.0" {
				t.Fatalf("while staged: previous = %q, %v", v, ok)
			}
			g.Abandon()
			if exists(g.staging()) || exists(g.stagedVersion()) {
				t.Error("the staged copy stays after Abandon")
			}
			if v, ok := g.HasPrevious(); !ok || v != "0.2.0" || read(t, previousExe(g)) != "0.2.0" {
				t.Fatalf("after Abandon: previous = %q, %v", v, ok)
			}
		})
	}
}

// On Windows the staged copy shares no file with the install: the installer,
// or a zip extracted over the folder, writes files in place, and a hard link
// would carry that write into the version kept. (A hard link to the running
// executable also can't be deleted by an install that fails and drops its
// staged copy: wine's sharing checks left `previous.staging\sona.exe`.)
func TestWindowsStagedCopyIsNotLinkedToTheInstall(t *testing.T) {
	g := layout(t, "windows", "0.2.0")
	if err := g.Stage("0.2.0"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exeOf(g), []byte("overwritten in place"), 0o755); err != nil { // truncates the same file
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(g.staging(), "sona.exe")); got != "0.2.0" {
		t.Fatalf("the staged copy followed a write to the install: %q", got)
	}
}

func TestCommitKeepsTheStagedVersionAndLeavesNothingBehind(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			g := updated(t, goos)
			if err := g.Stage("0.3.0-rc.4"); err != nil {
				t.Fatal(err)
			}
			if err := g.Commit(); err != nil {
				t.Fatal(err)
			}
			if v, ok := g.HasPrevious(); !ok || v != "0.3.0-rc.4" || read(t, previousExe(g)) != "0.3.0-rc.4" {
				t.Fatalf("previous = %q, %v", v, ok)
			}
			for _, p := range []string{g.staging(), g.stagedVersion(), g.Previous + ".old"} {
				if exists(p) {
					t.Errorf("%s is left behind", p)
				}
			}
		})
	}
}

// Staging a second time does not copy the first copy (or previous\) into the
// new one: on Windows they sit in the folder being copied.
func TestStageSkipsTheGuardsOwnFolders(t *testing.T) {
	g := updated(t, "windows")
	for range 2 {
		if err := g.Stage("0.3.0-rc.4"); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"previous", "previous.staging", "previous.old"} {
		if exists(filepath.Join(g.staging(), name)) {
			t.Errorf("the staged copy holds %s", name)
		}
	}
}

func TestCommitWithNothingStagedChangesNothing(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			g := updated(t, goos)
			if err := g.Commit(); err == nil {
				t.Fatal("committed a copy that was never staged")
			}
			if v, ok := g.HasPrevious(); !ok || v != "0.2.0" || read(t, previousExe(g)) != "0.2.0" {
				t.Fatalf("previous = %q, %v", v, ok)
			}
		})
	}
}

// A process that dies between the swap and Commit leaves the staged copy: the
// next start settles it by which version runs.
func TestAStartSettlesAStagedCopyLeftByADeadProcess(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			t.Run("the update went in", func(t *testing.T) {
				g := layout(t, goos, "0.2.0")
				writePlist(t, g, "0.2.0")
				if err := g.Stage("0.2.0"); err != nil {
					t.Fatal(err)
				}
				if err := g.Mark("0.2.0", "0.3.0"); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(exeOf(g)); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(exeOf(g), []byte("0.3.0"), 0o755); err != nil {
					t.Fatal(err)
				}
				// The process dies here: no Commit.
				if a, err := g.Begin("0.3.0"); err != nil || a != Counted {
					t.Fatalf("Begin = %v, %v", a, err)
				}
				if v, ok := g.HasPrevious(); !ok || v != "0.2.0" || read(t, previousExe(g)) != "0.2.0" {
					t.Fatalf("previous = %q, %v", v, ok)
				}
				if exists(g.staging()) {
					t.Error("the staged copy stays")
				}
			})
			t.Run("the update never went in", func(t *testing.T) {
				g := layout(t, goos, "0.2.0")
				if err := g.Stage("0.2.0"); err != nil {
					t.Fatal(err)
				}
				if err := g.Mark("0.2.0", "0.3.0"); err != nil {
					t.Fatal(err)
				}
				if a, err := g.Begin("0.2.0"); err != nil || a != Normal {
					t.Fatalf("Begin = %v, %v", a, err)
				}
				if _, ok := g.HasPrevious(); ok || exists(g.staging()) || exists(g.stagedVersion()) {
					t.Error("a copy of the running version is kept as the previous one")
				}
			})
		})
	}
}
