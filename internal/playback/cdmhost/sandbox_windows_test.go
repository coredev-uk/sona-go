//go:build cgo

package cdmhost

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdm/cdmtest"
	"github.com/coredev-uk/sona-go/internal/playback/widevine"
)

// envRequireSandbox, set (CI), turns a test that skips because the machine
// refuses AppContainers into a failure.
const envRequireSandbox = "SONA_CDMHOST_REQUIRE_SANDBOX"

// The Windows test child modes' inputs (envTestChild, cdmhost_test.go).
const (
	envProbeFile     = "SONA_CDMHOST_PROBE_FILE"     // a file outside every granted folder
	envProbeListener = "SONA_CDMHOST_PROBE_LISTENER" // the address of a listener on the loopback
	envProbeCDM      = "SONA_CDMHOST_PROBE_CDM"      // the CDM library
)

// The sandboxed child keeps its own plugin registry (profile.registryFile).
func init() {
	childRegistry = func(reg string) string {
		if prof, err := appContainer(); err == nil {
			return prof.registryFile(reg)
		}
		return reg
	}
}

// osTestChildMode runs the Windows test child modes: "probe" and "idle".
func osTestChildMode(m string) {
	switch m {
	case "probe":
		probeChild()
	case "idle":
		enterSandbox()
		select {}
	}
}

// probeResult is what the probe child reports.
type probeResult struct {
	Sandbox Sandbox
	// Outcomes maps each attempt to "ok" (it worked) or its error.
	Outcomes map[string]string
}

// probeChild enters the sandbox like the real child, tries what code in the
// CDM might, and prints each outcome as JSON.
func probeChild() {
	res := probeResult{Sandbox: enterSandbox(), Outcomes: map[string]string{}}
	outcome := func(name string, err error) {
		if err == nil {
			res.Outcomes[name] = "ok"
		} else {
			res.Outcomes[name] = err.Error()
		}
	}
	dial := func(network, addr string) error {
		c, err := net.DialTimeout(network, addr, 2*time.Second)
		if err == nil {
			_ = c.Close()
		}
		return err
	}
	outcome("tcp connect to the parent's listener", dial("tcp", os.Getenv(envProbeListener)))
	outcome("tcp connect to a public address", dial("tcp", "192.0.2.1:9"))
	outcome("udp connect to a public address", dial("udp", "192.0.2.1:9"))
	own := func() error {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		defer func() { _ = ln.Close() }()
		go func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
		return dial("tcp", ln.Addr().String())
	}
	outcome("tcp connect to its own listener", own())
	_, err := os.ReadFile(os.Getenv(envProbeFile))
	outcome("read outside the granted folders", err)
	probeFile := filepath.Join(os.Getenv("USERPROFILE"), "sona-sandbox-probe.tmp")
	f, err := os.OpenFile(probeFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err == nil {
		_ = f.Close()
		_ = os.Remove(probeFile)
	}
	outcome("write under the user profile", err)
	_, err = os.ReadDir(filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local"))
	outcome("list the local app data folder", err)
	cdmPath := os.Getenv(envProbeCDM)
	b, err := os.ReadFile(cdmPath)
	if err == nil && (len(b) < 2 || b[0] != 'M' || b[1] != 'Z') {
		err = fmt.Errorf("%s is not a DLL", cdmPath)
	}
	outcome("read the CDM", err)
	c, err := widevine.Open(cdmPath, slog.New(slog.DiscardHandler))
	if err == nil {
		err = c.Close()
	}
	outcome("load the CDM", err)
	outcome("write in the temp folder", os.WriteFile(filepath.Join(os.Getenv("TEMP"), "probe.txt"), []byte("x"), 0o600))
	outcome("write beside the GStreamer registry", os.WriteFile(filepath.Join(filepath.Dir(os.Getenv("GST_REGISTRY_1_0")), "probe.txt"), []byte("x"), 0o600))
	// Its standard handles are given, as opening NUL is refused too.
	ps := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"), "/c", "exit")
	ps.Stdin, ps.Stdout, ps.Stderr = os.Stdin, os.Stderr, os.Stderr
	outcome("start a process", ps.Run())
	_ = json.NewEncoder(os.Stdout).Encode(res)
	os.Exit(0)
}

// sandboxedChild starts a test child (mode, extra env) through
// startSandboxed. A machine that refuses the AppContainer skips the test,
// or fails it when envRequireSandbox is set.
func sandboxedChild(t *testing.T, mode string, extra ...string) proc {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(childEnv(slog.New(slog.DiscardHandler)), envTestChild+"="+mode)
	p, ps, err := startSandboxed(exe, append(env, extra...), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("start the child: %v", err)
	}
	if ps.gap != "" {
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
		if os.Getenv(envRequireSandbox) != "" {
			t.Fatalf("the sandbox is required, but the child runs unconfined: %s", ps.reason)
		}
		t.Skipf("no AppContainer here: %s", ps.reason)
	}
	go func() {
		sc := bufio.NewScanner(p.stderr)
		for sc.Scan() {
			t.Log("child:", sc.Text())
		}
	}()
	return p
}

// TestAppContainerConfinesChild starts a probe child in the AppContainer
// and checks what it could and could not do: no network, not even to a
// listener on the loopback; no reading outside the folders granted to it, no
// writing to the user's profile, no new process; but loading the CDM and
// writing its own folders.
func TestAppContainerConfinesChild(t *testing.T) {
	lib := cdmtest.Library(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("not for the child"), 0o600); err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var accepted atomic.Bool
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepted.Store(true)
			_ = c.Close()
		}
	}()

	p := sandboxedChild(t, "probe",
		envCDMDir+"="+filepath.Dir(lib), envProbeCDM+"="+lib, envProbeFile+"="+outside, envProbeListener+"="+ln.Addr().String())
	timer := time.AfterFunc(2*time.Minute, func() { _ = p.kill() })
	defer timer.Stop()
	var res probeResult
	derr := json.NewDecoder(p.stdout).Decode(&res)
	_ = p.stdin.Close()
	_ = p.cmd.Wait()
	if derr != nil {
		t.Fatalf("probe child: %v", derr)
	}
	t.Logf("sandbox %+v", res.Sandbox)
	for _, k := range slices.Sorted(maps.Keys(res.Outcomes)) {
		t.Logf("%s: %s", k, res.Outcomes[k])
	}

	sb := res.Sandbox.withParent(parentSandbox{})
	if !sb.Active() || sb.Kind() != playback.SandboxAppContainer || len(sb.Gaps()) != 0 || !sb.loadsCDM() {
		t.Errorf("the child reports its sandbox is not in place: %+v", sb)
	}
	denied := func(name string) {
		t.Helper()
		if got := res.Outcomes[name]; got == "" || got == "ok" {
			t.Errorf("%s: %q, want an error", name, got)
		}
	}
	allowed := func(name string) {
		t.Helper()
		if got := res.Outcomes[name]; got != "ok" {
			t.Errorf("%s: %q, want ok", name, got)
		}
	}
	// The parent's listener never answers the AppContainer; an address off
	// the machine is refused at once. (Its own listener and a UDP connect()
	// are logged above: the first connects, the second succeeds, and the
	// self-check relies on neither.)
	denied("tcp connect to the parent's listener")
	denied("tcp connect to a public address")
	if accepted.Load() {
		t.Error("the listener on the loopback got a connection from the child")
	}
	denied("read outside the granted folders")
	denied("write under the user profile")
	denied("list the local app data folder")
	denied("start a process")
	allowed("read the CDM")
	allowed("load the CDM")
	allowed("write in the temp folder")
	allowed("write beside the GStreamer registry")
	if _, err := os.Stat(filepath.Join(os.Getenv("USERPROFILE"), "sona-sandbox-probe.tmp")); err == nil {
		t.Error("the child created a file in the user profile")
		_ = os.Remove(filepath.Join(os.Getenv("USERPROFILE"), "sona-sandbox-probe.tmp"))
	}
}

// TestAppContainerKill ends a child with the kill the Host uses: Go's own
// handle on an AppContainer child cannot terminate it.
func TestAppContainerKill(t *testing.T) {
	p := sandboxedChild(t, "idle")
	pid := p.cmd.Process.Pid
	time.Sleep(500 * time.Millisecond)
	if err := p.kill(); err != nil {
		t.Fatalf("kill pid %d: %v", pid, err)
	}
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 1 {
			t.Errorf("wait after kill: %v, want exit status 1", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the child did not end after the kill")
	}
	_ = p.stdin.Close()
}

// TestSandboxedHostReports starts a real host: it reports an AppContainer
// that is fully in place.
func TestSandboxedHostReports(t *testing.T) {
	h := startHost(t)
	sb := h.Info().Sandbox
	t.Logf("sandbox %+v", sb)
	if os.Getenv(envAllowUnsandboxed) == "1" {
		t.Skip("the test escape is on")
	}
	if !sb.Active() || sb.Kind() != playback.SandboxAppContainer || len(sb.Gaps()) != 0 {
		t.Errorf("the host is not fully sandboxed: %+v (gaps %v)", sb, sb.Gaps())
	}
}

func TestSandboxLoadsCDM(t *testing.T) {
	for _, c := range []struct {
		sb   Sandbox
		want bool
	}{
		{Sandbox{AppContainer: true, NetworkDenied: true}, true},
		{Sandbox{AppContainer: true, NetworkDenied: true, FilesHidden: true}, true},
		{Sandbox{AppContainer: true}, false},
		{Sandbox{NetworkDenied: true}, false},
		{Sandbox{}, false},
	} {
		if got := c.sb.loadsCDM(); got != c.want {
			t.Errorf("%+v loadsCDM = %v, want %v", c.sb, got, c.want)
		}
	}
	// A test binary may load the CDM outside the sandbox, when told to.
	t.Setenv(envAllowUnsandboxed, "1")
	if !(Sandbox{}).loadsCDM() {
		t.Error("the test escape does not work")
	}
}

func TestSandboxGaps(t *testing.T) {
	if got := (Sandbox{}).withParent(parentSandbox{gap: playback.SandboxGapNoAppContainer, reason: "no Windows sandbox: x"}); got.Kind() != playback.SandboxNone ||
		!slices.Equal(got.Gaps(), []playback.SandboxGap{playback.SandboxGapNoAppContainer}) || got.Reason != "no Windows sandbox: x" || got.Active() {
		t.Errorf("unconfined: %+v", got)
	}
	if got := (Sandbox{AppContainer: true}).Gaps(); !slices.Equal(got, []playback.SandboxGap{playback.SandboxGapNetwork, playback.SandboxGapUserFiles}) {
		t.Errorf("AppContainer without self-checks: %v", got)
	}
}

// TestGrantAccess grants the AppContainer's SID read access to a folder and
// finds the entry; a second grant changes nothing, and a drive root is
// refused.
func TestGrantAccess(t *testing.T) {
	prof, err := appContainer()
	if err != nil {
		if os.Getenv(envRequireSandbox) != "" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	dir := t.TempDir()
	dacl := func() *windows.ACL {
		t.Helper()
		sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		acl, _, err := sd.DACL()
		if err != nil {
			t.Fatal(err)
		}
		return acl
	}
	who := []*windows.SID{prof.sid}
	if aclAllows(dacl(), who, accessRead, true) {
		t.Fatal("the new folder already grants the AppContainer read access")
	}
	for range 2 {
		if err := grantAccess(prof.sid, dir, accessRead, true); err != nil {
			t.Fatal(err)
		}
	}
	if !aclAllows(dacl(), who, accessRead, true) {
		t.Error("no read entry for the AppContainer after the grant")
	}
	if aclAllows(dacl(), who, accessModify, true) {
		t.Error("the AppContainer was given more than read access")
	}
	if err := grantAccess(prof.sid, `C:\`, accessRead, false); err == nil || !strings.Contains(err.Error(), "drive root") {
		t.Errorf("grant on a drive root: %v", err)
	}
}

func TestReadFolders(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cdmDir := filepath.Join(root, "widevine", "1", "win_x64")
	other := filepath.Join(root, "other")
	for _, d := range []string{cdmDir, other} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(root, "sona.exe")
	got := readFolders(exe, []string{
		envCDMDir + "=" + cdmDir,
		"SONA_CDM_SHIM=" + filepath.Join(other, "sona_cdm_shim.dll"),
		"GST_PLUGIN_PATH=" + filepath.Join(root, "missing") + ";" + other,
	})
	// The program's folder holds the others in this test (and GStreamer's
	// folders, which the test binary is linked to, come besides).
	if !slices.Contains(got, root) || slices.Contains(got, cdmDir) || slices.Contains(got, other) {
		t.Errorf("readFolders = %q, want %q but neither %q nor %q", got, root, cdmDir, other)
	}
	got = readFolders(filepath.Join(other, "sona.exe"), []string{envCDMDir + "=" + cdmDir})
	if !slices.Contains(got, other) || !slices.Contains(got, cdmDir) || slices.Contains(got, root) {
		t.Errorf("readFolders = %q, want %q and %q", got, other, cdmDir)
	}
}

func TestEnvHelpers(t *testing.T) {
	env := withEnv([]string{"Path=a", "TEMP=old", "x=1"}, "TEMP=new", "tmp=new")
	if envValue(env, "temp") != "new" || envValue(env, "PATH") != "a" || envValue(env, "X") != "1" || len(env) != 4 {
		t.Errorf("withEnv: %q", env)
	}
	block, err := envBlock([]string{"b=2", "A=1"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := windows.UTF16FromString("A=1")
	b, _ := windows.UTF16FromString("b=2")
	want := append(append(a, b...), 0)
	if !slices.Equal(block, want) {
		t.Errorf("envBlock = %v, want %v", block, want)
	}
	if _, err := envBlock([]string{"a\x00b=1"}); err == nil {
		t.Error("envBlock accepted a NUL")
	}
}
