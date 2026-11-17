package sandbox

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestProgramReadsAreTheProgramsFolderAndWhatItRunsFrom(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(root, "app")
	bin := filepath.Join(app, "usr", "bin")
	libs := filepath.Join(root, "libs")
	for _, d := range []string{bin, libs} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(bin, "sona")
	got := ProgramReads(exe, []string{"APPDIR=" + app, "LD_LIBRARY_PATH=" + libs + string(os.PathListSeparator) + filepath.Join(root, "missing") + string(os.PathListSeparator) + "relative", "HOME=" + root})
	// The program's folder is inside APPDIR: one entry; the missing and the relative are left out.
	if !slices.Equal(got, []string{app, libs}) && !slices.Equal(got, []string{libs, app}) {
		t.Errorf("ProgramReads = %q, want %q and %q", got, app, libs)
	}
	if got := ProgramReads(exe, nil); !slices.Equal(got, []string{bin}) {
		t.Errorf("without APPDIR = %q, want %q", got, bin)
	}
}
