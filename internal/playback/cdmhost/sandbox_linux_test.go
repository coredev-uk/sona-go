package cdmhost

import (
	"bufio"
	"context"
	"crypto/rand"
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
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// initChild spawns a child and runs opInit with a CDM that does not exist:
// the child stays up (no real CDM needed) and reports its sandbox.
func initChild(t *testing.T) (*Host, Info) {
	t.Helper()
	h, err := spawn(Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := h.call(ctx, request{Op: opInit, CDMPath: "/nonexistent/libwidevinecdm.so", Sink: "fakesink"})
	if err == nil || r.Info == nil {
		t.Fatalf("init with a missing CDM: %v %+v", err, r.Info)
	}
	if !errors.Is(replyErr(r), ErrCDMLoad) {
		t.Fatalf("init with a missing CDM: %v (code %q), want ErrCDMLoad", err, r.Code)
	}
	info := *r.Info
	info.Sandbox = h.withSandbox(info.Sandbox)
	return h, info
}

func TestChildCannotOpenInetSockets(t *testing.T) {
	h, info := initChild(t)
	sb := info.Sandbox
	t.Logf("sandbox %+v", sb)
	// The child's own socket(AF_INET) and socket(AF_INET6) attempts failed
	// with EPERM/EACCES under its seccomp filter.
	if !sb.Seccomp || !sb.InetDenied {
		t.Fatalf("child can open inet sockets: %+v", sb)
	}
	pid := h.cmd.Process.Pid
	status, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(status), "Seccomp:\t2\n") {
		t.Fatalf("child is not in seccomp filter mode:\n%s", status)
	}
	if why := namespacesRefused(t); why != "" {
		// The child runs with the layers it could set up (Landlock and
		// seccomp at least), and it must say why the others are missing.
		if sb.Active() || sb.MountNS || sb.Reason == "" {
			t.Fatalf("fallback without a reason: %+v", sb)
		}
		t.Skip(why)
	}
	if !sb.NetNS {
		t.Fatalf("no network namespace where user namespaces are allowed: %+v", sb)
	}
	self, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	child, err := os.Readlink("/proc/" + strconv.Itoa(pid) + "/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	if child == self {
		t.Fatalf("child shares the parent's network namespace %s", self)
	}
	if !sb.MountNS {
		t.Fatalf("no mount namespace: %+v", sb)
	}
	if sb.Landlock == 0 {
		// Every other layer is in place; the kernel lacks Landlock.
		if sb.Active() || !strings.Contains(sb.Reason, "Landlock") {
			t.Fatalf("missing Landlock not reported: %+v", sb)
		}
		return
	}
	if !sb.Active() || sb.Reason != "" {
		t.Fatalf("sandbox %+v", sb)
	}
}

func TestSandboxFallsBackWithoutNamespaces(t *testing.T) {
	orig := namespaceAttr
	t.Cleanup(func() { namespaceAttr = orig })
	// Any clone failure takes the fallback; a chroot into a missing
	// directory fails the same way a disabled user namespace does.
	namespaceAttr = func() *syscall.SysProcAttr { return &syscall.SysProcAttr{Chroot: "/nonexistent-sona-chroot"} }
	h, info := initChild(t)
	sb := info.Sandbox
	if sb.NetNS || sb.Active() || !strings.Contains(sb.Reason, "no network namespace") {
		t.Fatalf("fallback sandbox %+v", sb)
	}
	if !sb.Seccomp || !sb.InetDenied {
		t.Fatalf("the fallback child must still refuse inet sockets: %+v", sb)
	}
	if err := h.SetVolume(context.Background(), 0.5); err != nil {
		t.Fatalf("fallback child does not serve requests: %v", err)
	}
}

// The Linux test child modes (envTestChild, cdmhost_test.go).
const (
	// probe: sockets it must not reach (a path list), the audio socket it
	// must reach, an abstract socket name it must not reach, and a
	// directory under $HOME it must not write.
	envProbeDeny     = "SONA_CDMHOST_PROBE_DENY"
	envProbeAudio    = "SONA_CDMHOST_PROBE_AUDIO"
	envProbeAbstract = "SONA_CDMHOST_PROBE_ABSTRACT"
	envProbeHome     = "SONA_CDMHOST_PROBE_HOME"
)

// osTestChildMode runs the Linux test child modes: "probe" and "userns".
func osTestChildMode(m string) {
	switch m {
	case "probe":
		probeChild()
	case "userns":
		userNSChild()
	}
}

// probeChild enters the sandbox like the real child, then tries what code
// in the CDM might, and prints each outcome ("ok": it worked) as JSON.
func probeChild() {
	sb := enterSandbox()
	res := map[string]string{"sandbox": fmt.Sprintf("%+v", sb)}
	outcome := func(err error) string {
		if err == nil {
			return "ok"
		}
		return err.Error()
	}
	dial := func(addr string) error {
		c, err := net.DialTimeout("unix", addr, 2*time.Second)
		if err == nil {
			_ = c.Close()
		}
		return err
	}
	for _, p := range filepath.SplitList(os.Getenv(envProbeDeny)) {
		res["connect "+p] = outcome(dial(p))
	}
	if p := os.Getenv(envProbeAudio); p != "" {
		res["audio"] = outcome(dial(p))
	}
	res["abstract"] = outcome(dial("@" + os.Getenv(envProbeAbstract)))
	f, err := os.CreateTemp(os.Getenv(envProbeHome), "write-")
	if err == nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}
	res["home write"] = outcome(err)
	if d := os.Getenv(envCDMDir); d != "" {
		_, err = os.ReadFile(filepath.Join(d, "libwidevinecdm.so"))
		res["cdm read"] = outcome(err)
		err = os.WriteFile(filepath.Join(d, "libwidevinecdm.so"), []byte("patched"), 0o644)
		res["cdm write"] = outcome(err)
	}
	for _, fam := range refusedFamilies {
		fd, err := unix.Socket(fam.family, fam.typ|unix.SOCK_CLOEXEC, fam.proto)
		if err == nil {
			_ = unix.Close(fd)
		}
		res[fam.name] = outcome(err)
	}
	ppid := os.Getppid()
	_, err = os.ReadFile("/proc/" + strconv.Itoa(ppid) + "/environ")
	res["parent environ"] = outcome(err)
	var b [8]byte
	local := []unix.Iovec{{Base: &b[0], Len: 8}}
	remote := []unix.RemoteIovec{{Base: uintptr(unsafe.Pointer(&b[0])), Len: 8}}
	_, err = unix.ProcessVMReadv(ppid, local, remote, 0)
	res["process_vm_readv"] = outcome(err)
	_ = json.NewEncoder(os.Stdout).Encode(res)
	os.Exit(0)
}

// probe runs probeChild in namespaces (ns) or without, and returns its
// outcomes.
func probe(t *testing.T, ns bool, extra ...string) map[string]string {
	t.Helper()
	env := append(childEnv(slog.New(slog.DiscardHandler)), envTestChild+"=probe")
	env = append(env, extra...)
	var attr *syscall.SysProcAttr
	if ns {
		env, attr = append(env, envSandboxNS+"=1"), namespaceAttr()
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	p, err := startChild([]string{exe}, env, attr)
	if err != nil {
		t.Fatal(err)
	}
	logged := make(chan struct{})
	go func() {
		defer close(logged)
		sc := bufio.NewScanner(p.stderr)
		for sc.Scan() {
			t.Log("child:", sc.Text())
		}
	}()
	var res map[string]string
	derr := json.NewDecoder(p.stdout).Decode(&res)
	_ = p.stdin.Close()
	<-logged
	_ = p.cmd.Wait()
	if derr != nil {
		t.Fatalf("probe child: %v", derr)
	}
	for _, k := range slices.Sorted(maps.Keys(res)) {
		t.Logf("%s: %s", k, res[k])
	}
	return res
}

// userNSChild runs in new user, network and mount namespaces
// (namespacesRefused) and prints the errno of making its mounts private,
// which takes CAP_SYS_ADMIN in its user namespace: 0 when it may.
func userNSChild() {
	err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, "")
	errno, _ := err.(syscall.Errno) // 0 when err is nil
	fmt.Println(int(errno))
	os.Exit(0)
}

// envRequireNamespaces=1 (set by CI) turns a refusal into a failure, so a
// runner that stops allowing namespaces cannot skip the sandbox tests
// unnoticed.
const envRequireNamespaces = "SONA_CDMHOST_REQUIRE_NAMESPACES"

// namespacesRefused reports why this system refuses the child's
// namespaces, or "" when it allows them. Refused means EPERM or EACCES from
// creating unprivileged user, network and mount namespaces (user namespaces
// disabled, Flatpak), or from changing mounts inside them (Ubuntu's
// AppArmor restriction, kernel.apparmor_restrict_unprivileged_userns=1,
// creates the namespaces but denies their capabilities). Any other failure
// fails the test. Its probe child is set up independently of namespaceAttr,
// so a broken sandbox is not taken for a refusing system.
func namespacesRefused(t *testing.T) string {
	t.Helper()
	refused := func(why string) string {
		if os.Getenv(envRequireNamespaces) == "1" {
			t.Fatalf("%s (%s=1: the sandbox tests must run here)", why, envRequireNamespaces)
		}
		return why
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), EnvChild+"=1", envTestChild+"=userns")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: 0, HostID: os.Getgid(), Size: 1}},
	}
	out, err := cmd.Output()
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return refused("user namespaces refused: " + err.Error())
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			t.Fatalf("namespace probe: %v\n%s", err, exit.Stderr)
		}
		t.Fatalf("namespace probe: %v", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatalf("namespace probe printed %q", out)
	}
	switch errno := syscall.Errno(n); errno {
	case 0:
		return ""
	case syscall.EPERM, syscall.EACCES:
		return refused("mount namespace refused: make mounts private: " + errno.Error())
	default:
		t.Fatalf("namespace probe: make mounts private: %v", errno)
		return ""
	}
}

// listenUnix serves a Unix socket at addr until the test ends.
func listenUnix(t *testing.T, addr string) {
	t.Helper()
	ln, err := net.Listen("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
}

// TestSandboxConfinesChild checks, from inside a sandboxed child, that the
// session bus and other Unix sockets of the session, the network and the
// home directory are out of reach, and the audio server is not.
func TestSandboxConfinesChild(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	homeDir, err := os.MkdirTemp(home, ".sona-sandbox-probe-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(homeDir) })
	deny := []string{filepath.Join(t.TempDir(), "agent.sock")}
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir != "" {
		// A session service's socket, like the bus or an agent.
		s := filepath.Join(runtimeDir, "sona-probe-"+rand.Text()[:8])
		deny = append(deny, s)
	}
	for _, p := range deny {
		listenUnix(t, p)
	}
	for _, p := range []string{filepath.Join(runtimeDir, "bus"), "/run/dbus/system_bus_socket", "/run/systemd/resolve/io.systemd.Resolve"} {
		if fi, err := os.Stat(p); err == nil && fi.Mode().Type() == os.ModeSocket {
			deny = append(deny, p)
		}
	}
	abstract := "sona-probe-" + rand.Text()[:8]
	listenUnix(t, "@"+abstract)
	var audio string
	if s := audioSockets(runtimeDir); len(s) > 0 {
		audio = s[0]
	}
	// The CDM's directory, as the parent passes it (spawn): under the
	// test's temporary directory, which the namespaced child's private
	// /tmp covers, so it must be bound back, read-only.
	cdmDir := filepath.Join(t.TempDir(), "widevine", "4.10.0.0", "linux_x64")
	if err := os.MkdirAll(cdmDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cdmDir, "libwidevinecdm.so"), []byte("not a CDM"), 0o644); err != nil {
		t.Fatal(err)
	}
	extra := []string{
		envProbeDeny + "=" + strings.Join(deny, string(filepath.ListSeparator)),
		envProbeAudio + "=" + audio,
		envProbeAbstract + "=" + abstract,
		envProbeHome + "=" + homeDir,
		envCDMDir + "=" + cdmDir,
	}
	refused := func(t *testing.T, res map[string]string, what string) {
		t.Helper()
		if got, ok := res[what]; !ok || got == "ok" {
			t.Errorf("%s: %q, want refused", what, got)
		}
	}
	reached := func(t *testing.T, res map[string]string, what string) {
		t.Helper()
		if got := res[what]; got != "ok" {
			t.Errorf("%s: %q, want ok", what, got)
		}
	}
	landlockABI := func(res map[string]string) int {
		_, v, _ := strings.Cut(res["sandbox"], "Landlock:")
		n, _ := strconv.Atoi(strings.Fields(v)[0])
		return n
	}

	t.Run("namespaces", func(t *testing.T) {
		if why := namespacesRefused(t); why != "" {
			t.Skip(why)
		}
		res := probe(t, true, extra...)
		if !strings.Contains(res["sandbox"], "MountNS:true") {
			t.Fatalf("no mount namespace: %s", res["sandbox"])
		}
		for _, p := range deny {
			refused(t, res, "connect "+p)
		}
		for _, w := range []string{"abstract", "home write", "AF_INET", "AF_INET6", "AF_NETLINK", "process_vm_readv", "cdm write"} {
			refused(t, res, w)
		}
		reached(t, res, "cdm read")
		if landlockABI(res) > 0 {
			refused(t, res, "parent environ")
		}
		if audio != "" {
			reached(t, res, "audio")
		}
	})
	t.Run("landlock only", func(t *testing.T) {
		res := probe(t, false, extra...)
		for _, w := range []string{"AF_INET", "AF_INET6", "AF_NETLINK", "process_vm_readv"} {
			refused(t, res, w)
		}
		abi := landlockABI(res)
		if abi == 0 {
			t.Skip("no Landlock here; seccomp is the only layer without namespaces")
		}
		refused(t, res, "home write")
		refused(t, res, "cdm write")
		reached(t, res, "cdm read")
		refused(t, res, "parent environ")
		if abi >= 6 {
			refused(t, res, "abstract")
		}
		if abi >= 9 {
			for _, p := range deny {
				refused(t, res, "connect "+p)
			}
		}
		if audio != "" {
			reached(t, res, "audio")
		}
	})
}

// The child drops the session's addresses and anything named like a
// credential, and keeps what GStreamer and PipeWire need. An inherited
// CDM directory is dropped too: only spawn names it.
func TestChildEnvDropsSessionAndSecrets(t *testing.T) {
	drop := []string{
		"DBUS_SESSION_BUS_ADDRESS", "DBUS_SYSTEM_BUS_ADDRESS", "DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY",
		"SSH_AUTH_SOCK", "GPG_AGENT_INFO", "SESSION_MANAGER", "MUSIC_USER_TOKEN", "LASTFM_API_KEY", "GITHUB_TOKEN",
		"SOME_SECRET", "DB_PASSWORD", envSandboxStage, envCDMDir,
	}
	keep := []string{"HOME", "PATH", "XDG_RUNTIME_DIR", "GST_PLUGIN_SYSTEM_PATH_1_0", "PIPEWIRE_RUNTIME_DIR", "LANG"}
	for _, k := range slices.Concat(drop, keep) {
		t.Setenv(k, "x")
	}
	got := map[string]bool{}
	for _, kv := range childEnv(slog.New(slog.DiscardHandler)) {
		k, _, _ := strings.Cut(kv, "=")
		got[k] = true
	}
	for _, k := range drop {
		if got[k] {
			t.Errorf("child gets %s", k)
		}
	}
	for _, k := range keep {
		if !got[k] {
			t.Errorf("child lacks %s", k)
		}
	}
}

// F3: without the seccomp filter the CDM is never loaded, and the parent
// gets ErrSandbox through the reply code.
func TestCDMRefusedWithoutSeccomp(t *testing.T) {
	cdm := filepath.Join(t.TempDir(), "libwidevinecdm.so")
	if err := os.WriteFile(cdm, []byte("not a CDM"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, sb := range []Sandbox{{Reason: "seccomp filter not installed: test"}, {Seccomp: true}, {InetDenied: true}} {
		s := &server{log: slog.New(slog.DiscardHandler), sandbox: sb}
		info, err := s.open(cdm)
		if !errors.Is(err, ErrSandbox) || s.cdm != nil {
			t.Fatalf("%+v: open = %v", sb, err)
		}
		if info.CDMPath != cdm {
			t.Errorf("%+v: CDM path not reported: %+v", sb, info)
		}
		rep := fail(1, err)
		if got := replyErr(rep); rep.Code != codeSandbox || !errors.Is(got, ErrSandbox) || strings.Count(got.Error(), ErrSandbox.Error()) != 1 {
			t.Errorf("%+v: reply %+v → %v", sb, rep, got)
		}
	}
}
