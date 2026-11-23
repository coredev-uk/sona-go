package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestInstallHelper is the running "app" of TestInstallOverRunningExe: it
// sleeps until killed.
func TestInstallHelper(t *testing.T) {
	if os.Getenv("DEVLOOP_INSTALL_HELPER") != "1" {
		t.Skip("the helper process of TestInstallOverRunningExe")
	}
	time.Sleep(time.Minute)
}

// A new build replaces one that is still running, as when the app's playback
// host child outlives it; a plain rename failed with "Access is denied".
func TestInstallOverRunningExe(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "app.exe")
	copyFile(t, out)
	run := exec.Command(out, "-test.run=^TestInstallHelper$")
	run.Env = append(os.Environ(), "DEVLOOP_INSTALL_HELPER=1")
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = run.Process.Kill(); _ = run.Wait() })

	copyFile(t, out+".new")
	if err := os.Rename(out+".new", out); err == nil {
		t.Fatal("Windows let a running executable be replaced: the test no longer shows the problem")
	}
	if err := install(out+".new", out); err != nil {
		t.Fatalf("install over a running build: %v", err)
	}
	if _, err := os.Stat(out + ".new"); !os.IsNotExist(err) {
		t.Fatalf("the new build is still at out.new: %v", err)
	}

	// Once nothing runs the old build, the next install removes it.
	_ = run.Process.Kill()
	_ = run.Wait()
	copyFile(t, out+".new")
	if err := install(out+".new", out); err != nil {
		t.Fatal(err)
	}
	old, _ := filepath.Glob(out + ".old-*")
	if len(old) != 1 { // the one just moved aside
		t.Fatalf("old builds left: %v, want only the latest", old)
	}
}

// copyFile copies the test binary to dst.
func copyFile(t *testing.T, dst string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(self)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	f, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(f, in); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
