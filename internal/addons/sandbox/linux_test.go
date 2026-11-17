//go:build linux

package sandbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// The test binary is the add-on too: Start runs it confined, with a mode in
// its environment, and it reports what it could reach (probe).
func TestMain(m *testing.M) {
	Main()
	if mode := os.Getenv(envProbe); mode != "" {
		probe(mode)
		return
	}
	os.Exit(m.Run())
}

const (
	envProbe       = "SONA_SANDBOX_TEST_PROBE"
	envProbeAddr   = "SONA_SANDBOX_TEST_ADDR"   // a listener on the loopback
	envProbeSecret = "SONA_SANDBOX_TEST_SECRET" // a file outside every folder of the policy
	envProbeSocket = "SONA_SANDBOX_TEST_SOCKET" // a Unix socket of the user's
	envProbePID    = "SONA_SANDBOX_TEST_PID"    // a process outside the sandbox
	// envRequire turns a skip for a system that refuses the sandbox into a
	// failure (CI sets it).
	envRequire = "SONA_ADDON_REQUIRE_SANDBOX"
)

// results is what the probe add-on found out.
type results struct {
	Env        []string          `json:"env"`
	TCP        string            `json:"tcp"`       // "" when it connected, else the error
	Secret     string            `json:"secret"`    // reading the file outside the policy: "" when it could
	WriteData  string            `json:"writeData"` // "" when it wrote
	WriteDir   string            `json:"writeDir"`  // writing in its own folder
	WriteRoot  string            `json:"writeRoot"` // writing in /
	WriteTmp   string            `json:"writeTmp"`  // writing in /tmp
	ReadDir    string            `json:"readDir"`   // reading its own folder
	Home       string            `json:"home"`      // listing /home, /root, /run, /sys
	RunData    string            `json:"runData"`   // running a file from the data folder
	OpenTTY    string            `json:"openTty"`   // opening /dev/tty
	OpenPTS    string            `json:"openPts"`   // listing /dev/pts
	TTYFD      []int             `json:"ttyFd"`     // its descriptors that are terminals
	TTYNr      string            `json:"ttyNr"`     // the controlling terminal in /proc/self/stat
	SID        [2]int            `json:"sid"`       // its session and process IDs
	Procs      int               `json:"procs"`     // processes it can see
	Fds        []string          `json:"fds"`       // its open descriptors
	Unix       string            `json:"unix"`      // socket(AF_UNIX): "" when allowed
	Pair       string            `json:"pair"`      // socketpair(AF_UNIX)
	UnixPath   string            `json:"unixPath"`  // connecting to the user's socket
	Raw        string            `json:"raw"`       // socket(AF_NETLINK)
	Mounts     string            `json:"mounts"`    // its view of /proc/self/mountinfo
	Kill       string            `json:"kill"`      // signalling its parent
	Extra      map[string]string `json:"extra,omitempty"`
	Landlocked bool              `json:"landlocked"`
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// probe runs in the add-on and writes its findings to standard output.
func probe(mode string) {
	var r results
	r.Env = os.Environ()
	if addr := os.Getenv(envProbeAddr); addr != "" {
		c, err := net.DialTimeout("tcp", addr, 2*time.Second)
		r.TCP = errText(err)
		if c != nil {
			_ = c.Close()
		}
	}
	if p := os.Getenv(envProbeSecret); p != "" {
		_, err := os.ReadFile(p)
		r.Secret = errText(err)
	}
	data := os.Getenv("SONA_ADDON_DATA")
	r.WriteData = errText(os.WriteFile(filepath.Join(data, "ok"), []byte("x"), 0o600))
	r.WriteDir = errText(os.WriteFile("owned", []byte("x"), 0o600))
	r.WriteRoot = errText(os.WriteFile("/owned", []byte("x"), 0o600))
	r.WriteTmp = errText(os.WriteFile("/tmp/owned", []byte("x"), 0o600))
	_, err := os.ReadDir(".")
	r.ReadDir = errText(err)
	var listed []string
	for _, d := range []string{"/home", "/root", "/run", "/sys", "/mnt", "/var"} {
		if _, err := os.ReadDir(d); err == nil {
			listed = append(listed, d)
		}
	}
	r.Home = strings.Join(listed, ",")
	script := filepath.Join(data, "run.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o700)
	r.RunData = errText(execFile(script))
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	r.OpenTTY = errText(err)
	if f != nil {
		_ = f.Close()
	}
	_, err = os.ReadDir("/dev/pts")
	r.OpenPTS = errText(err)
	entries, _ := os.ReadDir("/proc/self/fd")
	for _, e := range entries {
		fd, _ := strconv.Atoi(e.Name())
		if _, err := unix.IoctlGetTermios(fd, unix.TCGETS); err == nil {
			r.TTYFD = append(r.TTYFD, fd)
		}
		if t, err := os.Readlink("/proc/self/fd/" + e.Name()); err == nil {
			r.Fds = append(r.Fds, e.Name()+"="+t)
		}
	}
	if b, err := os.ReadFile("/proc/self/stat"); err == nil {
		rest := string(b[strings.LastIndexByte(string(b), ')')+2:])
		fields := strings.Fields(rest)
		if len(fields) > 4 {
			r.TTYNr = fields[4]
		}
	}
	sid, _ := unix.Getsid(0)
	r.SID = [2]int{sid, os.Getpid()}
	procs, _ := os.ReadDir("/proc")
	for _, e := range procs {
		if _, err := strconv.Atoi(e.Name()); err == nil {
			r.Procs++
		}
	}
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	r.Unix = errText(err)
	if err == nil {
		_ = unix.Close(fd)
	}
	pair, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	r.Pair = errText(err)
	if err == nil {
		_ = unix.Close(pair[0])
		_ = unix.Close(pair[1])
	}
	if p := os.Getenv(envProbeSocket); p != "" {
		c, err := net.Dial("unix", p)
		r.UnixPath = errText(err)
		if c != nil {
			_ = c.Close()
		}
	}
	nl, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW, unix.NETLINK_ROUTE)
	r.Raw = errText(err)
	if err == nil {
		_ = unix.Close(nl)
	}
	mi, _ := os.ReadFile("/proc/self/mountinfo")
	r.Mounts = string(mi)
	if pid, err := strconv.Atoi(os.Getenv(envProbePID)); err == nil {
		r.Kill = errText(syscall.Kill(pid, 0))
	}
	b, _ := json.Marshal(r)
	fmt.Println(string(b))
}

// execFile runs a file and reports why it could not.
func execFile(path string) error {
	pid, err := syscall.ForkExec(path, []string{path}, &syscall.ProcAttr{Files: []uintptr{0, 1, 2}})
	if err != nil {
		return err
	}
	var ws syscall.WaitStatus
	_, err = syscall.Wait4(pid, &ws, 0, nil)
	return err
}

// run starts the probe add-on under policy p and returns what it found.
func run(t *testing.T, p Policy, extraEnv ...string) results {
	t.Helper()
	res, _ := runWith(t, p, extraEnv...)
	return res
}

func runWith(t *testing.T, p Policy, extraEnv ...string) (results, string) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if p.Dir == "" {
		p.Dir = t.TempDir()
	}
	if p.Data == "" {
		p.Data = t.TempDir()
	}
	// The probe is the test binary: its folder is readable, as a bundled
	// add-on's is.
	p.Read = append(p.Read, filepath.Dir(self))
	env := append([]string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + p.Data, "SONA_ADDON_DATA=" + p.Data, "TMPDIR=" + p.Data,
		envProbe + "=1",
	}, extraEnv...)
	var log bytes.Buffer
	c, err := Start(Spec{Policy: p, Exe: self, Env: env, Stderr: &log})
	if err != nil {
		skipIfRefused(t, err)
		t.Fatal(err)
	}
	out, _ := io.ReadAll(c.Out)
	select {
	case <-c.Done():
	case <-time.After(10 * time.Second):
		c.Kill()
		t.Fatal("the add-on did not exit")
	}
	var res results
	if err := json.Unmarshal(bytes.TrimSpace(out), &res); err != nil {
		t.Fatalf("the add-on's report %q: %v\nlog: %s", out, err, log.String())
	}
	return res, log.String()
}

func listener(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return l.Addr().String()
}

// outsideFile is a file the policy doesn't name: in the user's home when the
// tests run there, else in a temporary folder.
func outsideFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if home, err := os.UserHomeDir(); err == nil {
		if d, err := os.MkdirTemp(home, ".sona-sandbox-test-"); err == nil {
			t.Cleanup(func() { _ = os.RemoveAll(d) })
			dir = d
		}
	}
	p := filepath.Join(dir, "secret")
	if err := os.WriteFile(p, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNoNetworkWithoutThePermission(t *testing.T) {
	addr := listener(t)
	r := run(t, Policy{}, envProbeAddr+"="+addr)
	if r.TCP == "" {
		t.Error("an add-on without network connected to a TCP listener")
	}
	if r.Raw == "" {
		t.Error("an add-on opened a netlink socket")
	}
	if r.Unix == "" {
		t.Error("an add-on without a socket of the user's opened a Unix socket")
	}
	if r.Pair != "" {
		t.Errorf("socketpair: %s", r.Pair)
	}
}

func TestNetworkWithThePermission(t *testing.T) {
	addr := listener(t)
	r := run(t, Policy{Network: true}, envProbeAddr+"="+addr)
	if r.TCP != "" {
		t.Errorf("an add-on with network could not connect: %s", r.TCP)
	}
	if r.Unix == "" {
		t.Error("network gave an add-on Unix sockets")
	}
	if r.Raw == "" {
		t.Error("network gave an add-on a netlink socket")
	}
}

func TestFilesystemIsOnlyItsOwnFolders(t *testing.T) {
	secret := outsideFile(t)
	dir := t.TempDir()
	r := run(t, Policy{Dir: dir}, envProbeSecret+"="+secret)
	if r.Secret == "" {
		t.Error("the add-on read a file in the user's home outside its folders")
	}
	if r.WriteData != "" {
		t.Errorf("the add-on could not write to its data folder: %s", r.WriteData)
	}
	for name, got := range map[string]string{"its own folder": r.WriteDir, "/": r.WriteRoot, "/tmp": r.WriteTmp} {
		if got == "" {
			t.Errorf("the add-on wrote to %s", name)
		}
	}
	if r.ReadDir != "" {
		t.Errorf("the add-on could not read its own folder: %s", r.ReadDir)
	}
	if r.Home != "" {
		t.Errorf("the add-on listed %s", r.Home)
	}
	if r.RunData == "" {
		t.Error("the add-on ran a program from its data folder")
	}
}

func TestNoControllingTerminal(t *testing.T) {
	r := run(t, Policy{})
	if r.OpenTTY == "" {
		t.Error("the add-on opened /dev/tty")
	}
	if r.OpenPTS == "" {
		t.Error("the add-on listed /dev/pts")
	}
	if len(r.TTYFD) != 0 {
		t.Errorf("descriptors %v are terminals (%v)", r.TTYFD, r.Fds)
	}
	if r.TTYNr != "0" {
		t.Errorf("controlling terminal %q, want none (0)", r.TTYNr)
	}
}

func TestNoDescriptorsButItsChannel(t *testing.T) {
	r := run(t, Policy{})
	t.Logf("descriptors %v\nmounts:\n%s", r.Fds, r.Mounts)
	for _, fd := range r.Fds {
		n, _, _ := strings.Cut(fd, "=")
		// Go's own epoll and event descriptors are the probe's.
		if i, _ := strconv.Atoi(n); i > 2 && !strings.Contains(fd, "anon_inode:") {
			t.Errorf("an extra descriptor of Sona's: %s", fd)
		}
	}
	for _, kv := range r.Env {
		if strings.HasPrefix(kv, envRequest+"=") {
			t.Errorf("the sandbox's request is in the add-on's environment")
		}
	}
}

func TestOwnProcessesOnly(t *testing.T) {
	r := run(t, Policy{}, envProbePID+"="+strconv.Itoa(os.Getpid()))
	if r.Procs > 3 {
		t.Errorf("the add-on sees %d processes", r.Procs)
	}
	if r.Kill == "" {
		t.Error("the add-on could signal a process outside its sandbox")
	}
}

func TestUnixSocketOnlyWhenNamed(t *testing.T) {
	dir, err := os.MkdirTemp("", "sbx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	sock := filepath.Join(dir, "ipc")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	if r := run(t, Policy{}, envProbeSocket+"="+sock); r.UnixPath == "" {
		t.Error("an add-on connected to a socket it was not given")
	}
	if r := run(t, Policy{Sockets: []string{sock}}, envProbeSocket+"="+sock); r.UnixPath != "" {
		t.Errorf("an add-on could not connect to the socket it was given: %s", r.UnixPath)
	}
}

// The exception is a socket, not its folder: the sockets beside the one
// named (the session bus in the same runtime folder, another of Discord's)
// and in the folders below it stay out of reach. This is the Discord add-on's
// case (docs/security-review.md, "Add-ons").
func TestOnlyTheNamedSocketNotItsNeighbours(t *testing.T) {
	dir, err := os.MkdirTemp("", "sbx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err := os.MkdirAll(filepath.Join(dir, "app", "com.discordapp.Discord"), 0o700); err != nil {
		t.Fatal(err)
	}
	listen := func(rel string) string {
		p := filepath.Join(dir, rel)
		l, err := net.Listen("unix", p)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
		return p
	}
	given := listen("discord-ipc-0")
	neighbours := []string{
		listen("discord-ipc-1"), listen("bus"), listen("wayland-0"), listen("app/com.discordapp.Discord/discord-ipc-0"),
	}
	policy := Policy{Sockets: []string{given}}
	if r := run(t, policy, envProbeSocket+"="+given); r.UnixPath != "" {
		t.Errorf("the named socket is out of reach: %s", r.UnixPath)
	}
	for _, p := range neighbours {
		if r := run(t, policy, envProbeSocket+"="+p); r.UnixPath == "" {
			t.Errorf("an add-on named only %s reached %s", given, p)
		}
	}
	// The exception is no network: an add-on that can reach a socket still has none.
	if r := run(t, policy, envProbeAddr+"="+listener(t)); r.TCP == "" || r.Raw == "" {
		t.Errorf("a socket exception gave the add-on the network (tcp %q, netlink %q)", r.TCP, r.Raw)
	}
}

// A program that left the add-on's group (setsid) still dies with it: the PID
// namespace ends with its init.
func TestKillEndsEverythingInItsNamespace(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	const marker = "301.4159"
	script := filepath.Join(dir, "run")
	body := "#!/bin/sh\nsetsid sleep " + marker + " &\nexec sleep 300\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	c, err := Start(Spec{Policy: Policy{Dir: dir, Data: data}, Exe: script, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		skipIfRefused(t, err)
		t.Fatal(err)
	}
	found := func() bool {
		procs, _ := filepath.Glob("/proc/[0-9]*/cmdline")
		for _, p := range procs {
			if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte(marker)) {
				return true
			}
		}
		return false
	}
	deadline := time.Now().Add(5 * time.Second)
	for !found() {
		if time.Now().After(deadline) {
			c.Kill()
			t.Fatal("the add-on's child never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
	c.Kill()
	select {
	case <-c.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the add-on survived Kill")
	}
	for deadline := time.Now().Add(5 * time.Second); found(); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("a process of the add-on outlived it")
		}
	}
}

// skipIfRefused skips a test on a system that refuses the sandbox, unless CI
// says it must work.
func skipIfRefused(t *testing.T, err error) {
	t.Helper()
	if reason, ok := ReasonOf(err); ok && (reason == ReasonUserNamespaceRestricted || reason == ReasonNoUserNamespace) && os.Getenv(envRequire) != "1" {
		t.Skipf("this system refuses the sandbox: %v (see docs/testing.md)", err)
	}
}

// A program that doesn't start is not the sandbox's failure: the add-on's own
// problem, which the supervisor restarts.
func TestAProgramThatDoesNotStartIsNotASandboxFailure(t *testing.T) {
	_, err := Start(Spec{Policy: Policy{Dir: t.TempDir(), Data: t.TempDir()}, Exe: "/nonexistent/program"})
	if err == nil {
		t.Fatal("a program that doesn't exist started")
	}
	if reason, ok := ReasonOf(err); ok {
		skipIfRefused(t, err)
		t.Errorf("a missing program is the sandbox's failure (%s): %v", reason, err)
	}
}

func TestStartErrorsTellRefusedNamespacesFromOtherFailures(t *testing.T) {
	for _, e := range []error{unix.EPERM, unix.EACCES, unix.EINVAL, unix.ENOSPC} {
		reason, ok := ReasonOf(startError(e))
		if !ok || (reason != ReasonUserNamespaceRestricted && reason != ReasonNoUserNamespace) {
			t.Errorf("%v: reason %q, %v", e, reason, ok)
		}
	}
	if reason, _ := ReasonOf(startError(unix.ENOENT)); reason != ReasonSetup {
		t.Errorf("ENOENT: reason %q, want %q", reason, ReasonSetup)
	}
}

// The file that was checked runs, with its arguments, inside the sandbox.
func TestRunsTheOpenFileWithItsArguments(t *testing.T) {
	dir, data := t.TempDir(), t.TempDir()
	script := filepath.Join(dir, "run")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$1 $2\" > \"$SONA_ADDON_DATA/out\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(script)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	c, err := Start(Spec{
		Policy: Policy{Dir: dir, Data: data}, Exe: script, ExeFile: f, Args: []string{"--addon", "demo"},
		Env: []string{"PATH=" + os.Getenv("PATH"), "SONA_ADDON_DATA=" + data},
	})
	if err != nil {
		skipIfRefused(t, err)
		t.Fatal(err)
	}
	select {
	case <-c.Done():
	case <-time.After(10 * time.Second):
		c.Kill()
		t.Fatal("the add-on did not exit")
	}
	if b, err := os.ReadFile(filepath.Join(data, "out")); err != nil || strings.TrimSpace(string(b)) != "--addon demo" {
		t.Errorf("out = %q, %v (exit %v)", b, err, c.Err())
	}
}

// Landlock, where the kernel has it, is the second layer; where it hasn't, the
// add-on's log says so.
func TestLandlockIsReportedWhereItIsMissing(t *testing.T) {
	_, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	has := errno == 0
	dir, data := t.TempDir(), t.TempDir()
	script := filepath.Join(dir, "run")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c, err := Start(Spec{Policy: Policy{Dir: dir, Data: data}, Exe: script, Env: []string{"PATH=" + os.Getenv("PATH")}})
	if err != nil {
		skipIfRefused(t, err)
		t.Fatal(err)
	}
	<-c.Done()
	if has && len(c.Notes) != 0 {
		t.Errorf("the kernel has Landlock but the sandbox says: %q", c.Notes)
	}
	if !has && len(c.Notes) == 0 {
		t.Error("the kernel has no Landlock and the sandbox didn't say so")
	}
}
