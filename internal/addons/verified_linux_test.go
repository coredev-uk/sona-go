//go:build linux

package addons

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scriptDir(t *testing.T, body string) (dir, exe string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	exe = filepath.Join(dir, "run")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

// The file that was checked is the file that runs: the path is swapped for
// another program after the check and the first one still runs.
func TestOpenVerifiedRunsTheFileThatWasChecked(t *testing.T) {
	dir, exe := scriptDir(t, "echo original >&2\n")
	f, err := OpenVerified(dir, exe)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	// After the check: the path now names something else.
	if err := os.Remove(exe); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("#!/bin/sh\necho swapped >&2\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	p, err := Start(Spec{Exe: exe, ExeFile: f, Dir: dir, Env: []string{"PATH=/usr/bin:/bin"}, Stderr: &out})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-p.Done():
	case <-time.After(10 * time.Second):
		p.Kill()
		t.Fatal("the add-on did not exit")
	}
	if got := strings.TrimSpace(out.String()); got != "original" {
		t.Errorf("the add-on printed %q, want the file that was checked", got)
	}
}

func TestOpenVerifiedRefusesWhatItWouldNotRun(t *testing.T) {
	dir, exe := scriptDir(t, "exit 0\n")
	other := t.TempDir()
	outside := filepath.Join(other, "elsewhere")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	groupW := filepath.Join(dir, "groupw")
	if err := os.WriteFile(groupW, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(groupW, 0o720); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, tc := range map[string]struct {
		dir, exe, code string
	}{
		"a link in the last place": {dir, link, ProblemNoExecutable},
		"writable by the group":    {dir, groupW, ProblemUnsafe},
		"not executable":           {dir, plain, ProblemNoExecutable},
		"a folder":                 {dir, sub, ProblemNoExecutable},
		"outside the folder":       {dir, outside, ProblemEscapes},
		"missing":                  {dir, filepath.Join(dir, "nope"), ProblemNoExecutable},
	} {
		t.Run(name, func(t *testing.T) {
			f, err := OpenVerified(tc.dir, tc.exe)
			if err == nil {
				_ = f.Close()
				t.Fatal("opened")
			}
			if p := asProblem(err); p.Code != tc.code {
				t.Errorf("problem %q, want %q (%v)", p.Code, tc.code, err)
			}
		})
	}
	if f, err := OpenVerified(dir, exe); err != nil {
		t.Errorf("the plain program is refused: %v", err)
	} else {
		_ = f.Close()
	}
}
