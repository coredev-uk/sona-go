//go:build unix

package addons

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func writeScript(t *testing.T, body string) (dir, exe string) {
	t.Helper()
	dir = t.TempDir()
	exe = filepath.Join(dir, "run")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

func startScript(t *testing.T, body string, env []string) (*Proc, string) {
	t.Helper()
	dir, exe := writeScript(t, body)
	p, err := Start(Spec{Exe: exe, Dir: dir, Env: env})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Stop(time.Second) })
	return p, dir
}

// alive reports whether pid runs: not gone, and not a zombie that nobody
// reaped (a container without an init process leaves orphans as zombies).
func alive(pid int) bool {
	if b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat"); err == nil {
		if i := strings.LastIndexByte(string(b), ')'); i > 0 && i+2 < len(b) && b[i+2] == 'Z' {
			return false
		}
	}
	return syscall.Kill(pid, 0) == nil
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// The child sees only the allowlist, whatever Sona's own environment holds.
func TestAddonRunsWithTheAllowlistedEnvironment(t *testing.T) {
	for _, kv := range []string{"MUSIC_USER_TOKEN=secret-token", "SONA_LASTFM_KEY=k", "SONA_UPDATE_KEY=u", "DBUS_SESSION_BUS_ADDRESS=unix:path=/x", "MYGO_AUTOMATION=1"} {
		name, val, _ := strings.Cut(kv, "=")
		t.Setenv(name, val)
	}
	p, dir := startScript(t, "env > env.out\n", Environment(os.Environ(), "demo"))
	<-p.Done()
	b, err := os.ReadFile(filepath.Join(dir, "env.out"))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"MUSIC_USER_TOKEN", "SONA_LASTFM_KEY", "SONA_UPDATE_KEY", "DBUS_", "MYGO_AUTOMATION", "secret-token"} {
		if strings.Contains(string(b), banned) {
			t.Errorf("the add-on's environment has %q:\n%s", banned, b)
		}
	}
	if !strings.Contains(string(b), "SONA_ADDON_ID=demo") || !strings.Contains(string(b), "PATH=") {
		t.Errorf("environment:\n%s", b)
	}
}

func TestAddonChannelCarriesLinesBothWays(t *testing.T) {
	p, _ := startScript(t, "read line\nprintf 'got:%s\\n' \"$line\"\n", Environment(os.Environ(), "demo"))
	c := p.Conn()
	if _, err := c.Write([]byte("ping\n")); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil || line != "got:ping\n" {
		t.Errorf("read %q, %v", line, err)
	}
	if err := p.Err(); err != nil {
		t.Errorf("exit: %v", err)
	}
}

func TestAddonRunsByAbsolutePathInItsFolder(t *testing.T) {
	// A PATH entry that holds a program of the same name must not be used.
	dir, exe := writeScript(t, "pwd > cwd.out\n")
	decoy := t.TempDir()
	if err := os.WriteFile(filepath.Join(decoy, "run"), []byte("#!/bin/sh\necho decoy > "+filepath.Join(dir, "decoy.out")+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	p, err := Start(Spec{Exe: exe, Dir: dir, Env: []string{"PATH=" + decoy}})
	if err != nil {
		t.Fatal(err)
	}
	<-p.Done()
	if _, err := os.Stat(filepath.Join(dir, "decoy.out")); err == nil {
		t.Error("the add-on was found through PATH")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "cwd.out"))
	real, _ := filepath.EvalSymlinks(dir)
	if strings.TrimSpace(string(b)) != real {
		t.Errorf("cwd = %q, want %q", b, real)
	}
}

// Stop ends the add-on and the children it left in its group; nothing is
// left running or unreaped.
func TestStopKillsTheWholeGroup(t *testing.T) {
	p, dir := startScript(t, "trap '' TERM\nsleep 300 &\necho $! > child.pid\nwait\n", Environment(os.Environ(), "demo"))
	var child int
	waitFor(t, "the child's pid", func() bool {
		b, err := os.ReadFile(filepath.Join(dir, "child.pid"))
		n, perr := strconv.Atoi(strings.TrimSpace(string(b)))
		child = n
		return err == nil && perr == nil
	})
	if !alive(child) {
		t.Fatal("the child isn't running")
	}
	start := time.Now()
	p.Stop(200 * time.Millisecond) // it ignores SIGTERM: the kill follows the grace
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("Stop took %v", d)
	}
	select {
	case <-p.Done():
	default:
		t.Error("the add-on was not reaped")
	}
	waitFor(t, "the child to die", func() bool { return !alive(child) })
}

func TestStopLetsAnAddonLeaveOnEndOfInput(t *testing.T) {
	p, dir := startScript(t, "read line\necho clean > bye.out\n", Environment(os.Environ(), "demo"))
	start := time.Now()
	p.Stop(5 * time.Second)
	if time.Since(start) > 3*time.Second {
		t.Error("Stop waited out the grace for an add-on that leaves at once")
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "bye.out")); strings.TrimSpace(string(b)) != "clean" {
		t.Errorf("the add-on did not see end of input: %q", b)
	}
}

// The kernel ends the add-on when Sona dies without quitting (Linux's
// parent-death signal). A helper process, this test binary run again, starts
// an add-on as the supervisor does and is then killed outright: the add-on dies
// with it.
func TestLinuxParentDeathKillsTheAddon(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("parent-death signals are Linux's")
	}
	dir, exe := writeScript(t, "echo $$ > pid.out\nsleep 300\n")
	helper := exec.Command(os.Args[0], "-test.run=^TestAddonSupervisorHelper$")
	helper.Env = append(os.Environ(), "SONA_ADDON_TEST_HELPER="+dir+"|"+exe)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = helper.Process.Kill(); _ = helper.Wait() })
	var pid int
	waitFor(t, "the add-on's pid", func() bool {
		b, err := os.ReadFile(filepath.Join(dir, "pid.out"))
		n, perr := strconv.Atoi(strings.TrimSpace(string(b)))
		pid = n
		return err == nil && perr == nil
	})
	if !alive(pid) {
		t.Fatal("the add-on isn't running")
	}
	if err := helper.Process.Kill(); err != nil { // a crash: no chance to clean up
		t.Fatal(err)
	}
	_ = helper.Wait()
	waitFor(t, "the add-on to die with its supervisor", func() bool { return !alive(pid) })
}

// TestAddonSupervisorHelper is the helper process of the test above: it
// starts an add-on on a locked thread, as the supervisor does, and waits to
// be killed. Run on its own it does nothing.
func TestAddonSupervisorHelper(t *testing.T) {
	spec := os.Getenv("SONA_ADDON_TEST_HELPER")
	if spec == "" {
		return
	}
	runtime.LockOSThread()
	dir, exe, _ := strings.Cut(spec, "|")
	if _, err := Start(Spec{Exe: exe, Dir: dir, Env: Environment(os.Environ(), "demo")}); err != nil {
		t.Fatal(err)
	}
	select {}
}
