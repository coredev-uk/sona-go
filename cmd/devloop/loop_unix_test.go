//go:build unix

package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// output is what devloop and the app print, readable while they run.
type output struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.Write(p)
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buf.String()
}

// waitFor waits until the output has n matches of re, and returns the last
// one's groups.
func (o *output) waitFor(t *testing.T, re string, n int) []string {
	t.Helper()
	rx := regexp.MustCompile(re)
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if m := rx.FindAllStringSubmatch(o.String(), -1); len(m) >= n {
			return m[n-1]
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no %d× %q in the output:\n%s", n, re, o.String())
	return nil
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

const appSource = `package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	fmt.Fprintf(os.Stderr, "app VERSION pid=%d\n", os.Getpid())
	fmt.Fprintln(os.Stderr, "startup: first paint")
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, os.Interrupt)
	<-ch
}
`

// A save that builds restarts the app; a save that does not build prints the
// compiler's errors and leaves the running app alone; the fix restarts it.
func TestFailedBuildKeepsTheApp(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go programs")
	}
	dir := t.TempDir()
	loopBin := filepath.Join(dir, "devloop")
	if out, err := exec.Command("go", "build", "-o", loopBin, ".").CombinedOutput(); err != nil {
		t.Fatalf("building devloop: %v\n%s", err, out)
	}

	mod := filepath.Join(dir, "mod")
	src := filepath.Join(mod, "cmd", "app", "main.go")
	write := func(content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module example.test/app\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	write(strings.Replace(appSource, "VERSION", "v1", 1))

	var out output
	cmd := exec.Command(loopBin, "-root", mod, "-target", "./cmd/app", "-tags", "")
	cmd.Env = append(os.Environ(), "GOFLAGS=")
	cmd.Stdout, cmd.Stderr = &out, &out
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		if m := regexp.MustCompile(`pid=(\d+)`).FindAllStringSubmatch(out.String(), -1); len(m) > 0 {
			pid, _ := strconv.Atoi(m[len(m)-1][1])
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	})

	pidOf := func(n int) int {
		pid, err := strconv.Atoi(out.waitFor(t, `app v\d pid=(\d+)`, n)[1])
		if err != nil {
			t.Fatal(err)
		}
		return pid
	}
	first := pidOf(1)
	out.waitFor(t, `window painted`, 1)
	// The app has a process group of its own: a Ctrl-C reaches devloop only.
	if pg, err := syscall.Getpgid(first); err != nil || pg == syscall.Getpgrp() {
		t.Fatalf("the app shares devloop's process group: %d %v", pg, err)
	}

	// A broken save: the errors are printed, the app lives on.
	write("package main\n\nfunc main() { undefinedCall() }\n")
	out.waitFor(t, `build failed`, 1)
	if !strings.Contains(out.String(), "undefinedCall") {
		t.Errorf("the compiler's error is not printed:\n%s", out.String())
	}
	if !alive(first) {
		t.Fatal("a failed build stopped the running app")
	}

	// The fix: the app is replaced by a new one.
	write(strings.Replace(appSource, "VERSION", "v2", 1))
	second := pidOf(2)
	out.waitFor(t, `window painted`, 2)
	if second == first {
		t.Fatal("the fixed build did not start a new app")
	}
	if alive(first) {
		t.Error("the old app is still running after the restart")
	}

	// q quits the app and the loop.
	if _, err := io.WriteString(stdin, "q"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		t.Logf("devloop exited: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatalf("devloop did not quit on q:\n%s", out.String())
	}
	if alive(second) {
		t.Error("the app outlived devloop")
	}
}
