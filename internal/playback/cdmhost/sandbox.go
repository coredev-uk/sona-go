package cdmhost

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Sandbox describes how the CDM host child is confined. The layers, set
// up before the CDM or GStreamer load:
//
//   - Namespaces (spawn): its own user, network and mount namespace. The
//     network namespace has no interfaces besides a loopback that is
//     down, which also isolates abstract Unix sockets.
//   - Filesystem (sandbox_fs.go, first stage): in the mount namespace
//     /run, /tmp, /var/tmp and $XDG_RUNTIME_DIR are fresh, empty tmpfs
//     (the session and system D-Bus, systemd, resolved, X11, Wayland and
//     agent sockets are gone) except for the PipeWire (or PulseAudio)
//     socket and the program's own files; the home directory is read-only
//     and Sona's config and state, ~/.ssh, ~/.gnupg and the keyrings are
//     hidden.
//   - Landlock, where the kernel has it: no file writes outside /dev (and
//     the private /tmp), no TCP/UDP, no connect() to a path Unix socket
//     but the audio server's (ABI 9), no abstract Unix sockets or signals
//     outside the sandbox (ABI 6), and no ptrace or /proc/<pid>/mem of any
//     process outside it.
//   - Seccomp (confine): socket() and socketpair() only for AF_UNIX, and no
//     ptrace, process_vm_*, io_uring, bpf, perf events, keyrings,
//     namespaces or mounts.
//
// Without unprivileged user namespaces (Flatpak, Ubuntu's AppArmor
// restriction) the child runs with Landlock and seccomp only. The CDM is
// loaded only under the seccomp filter (opInit, F3 in
// docs/security-review.md).
type Sandbox struct {
	NetNS      bool   // runs in its own network namespace
	MountNS    bool   // runs in its own mount namespace with the filesystem view above
	Landlock   int    // Landlock ABI version enforced; 0 when the kernel has none
	Seccomp    bool   // the seccomp filter is installed
	InetDenied bool   // the child's own socket(AF_INET, AF_INET6, AF_NETLINK) attempts were refused
	Reason     string // why a layer is missing; "" when Active
}

// Active reports whether every layer is in place.
func (s Sandbox) Active() bool {
	return s.NetNS && s.MountNS && s.Landlock > 0 && s.Seccomp && s.InetDenied
}

// Environment between the parent and the child's two stages. The parent
// sets envSandboxNS when it started the child in namespaces; the first
// stage sets envSandboxStage to its report (stageReport) when it
// re-executes into the second.
const (
	envSandboxNS    = "SONA_CDMHOST_NS"
	envSandboxStage = "SONA_CDMHOST_STAGE"
)

// namespaceAttr runs the child in a new user namespace that maps the
// caller's uid and gid onto themselves (files and the PipeWire socket keep
// their owner), a new, empty network namespace and a new mount namespace.
// CAP_SYS_ADMIN (in the new user namespace only) is kept across exec as an
// ambient capability so the first stage can build its filesystem view; it
// is gone after the first stage re-executes. Tests replace it.
var namespaceAttr = func() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET | syscall.CLONE_NEWNS,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: os.Getuid(), HostID: os.Getuid(), Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: os.Getgid(), HostID: os.Getgid(), Size: 1}},
		AmbientCaps: []uintptr{unix.CAP_SYS_ADMIN},
	}
}

// stageReport is what the first stage set up, passed to the second in
// envSandboxStage as "<mount 0|1>;<landlock abi>;<reason>".
type stageReport struct {
	mount    bool
	landlock int
	reasons  []string
}

func (r stageReport) String() string {
	m := 0
	if r.mount {
		m = 1
	}
	return fmt.Sprintf("%d;%d;%s", m, r.landlock, strings.Join(r.reasons, "; "))
}

func parseStageReport(s string) stageReport {
	f := strings.SplitN(s, ";", 3)
	var r stageReport
	if len(f) != 3 {
		r.reasons = append(r.reasons, "malformed sandbox stage report")
		return r
	}
	r.mount = f[0] == "1"
	r.landlock, _ = strconv.Atoi(f[1])
	if f[2] != "" {
		r.reasons = append(r.reasons, f[2])
	}
	return r
}

// enterSandbox runs in the child before anything else. The first stage
// (no envSandboxStage) builds the filesystem view and the Landlock domain
// and re-executes the binary, so the second stage starts with no
// capabilities and every thread inside the domain; it does not return. The
// second stage installs the seccomp filter (confine) and returns the
// complete report.
func enterSandbox() Sandbox {
	st, ok := os.LookupEnv(envSandboxStage)
	if !ok {
		sandboxStage1()
	}
	rep := parseStageReport(st)
	sb := confine()
	sb.MountNS, sb.Landlock = rep.mount, rep.landlock
	if sb.Reason != "" {
		rep.reasons = append(rep.reasons, sb.Reason)
	}
	sb.Reason = strings.Join(rep.reasons, "; ")
	return sb
}

// auditArch is the seccomp architecture of this build; 0 when the filter
// does not support it.
var auditArch = map[string]uint32{
	"amd64": unix.AUDIT_ARCH_X86_64,
	"arm64": unix.AUDIT_ARCH_AARCH64,
}[runtime.GOARCH]

// x32Bit marks x32 ABI syscall numbers on amd64; nothing in the child uses
// that ABI, so it is refused rather than filtered a second time.
const x32Bit = 0x40000000

// deniedSyscalls fail with EPERM: debugging or reading other processes,
// io_uring (its IORING_OP_SOCKET would bypass the socket check), kernel
// attack surface the child has no use for, and namespace and mount changes.
var deniedSyscalls = []uint32{
	unix.SYS_PTRACE, unix.SYS_PROCESS_VM_READV, unix.SYS_PROCESS_VM_WRITEV, unix.SYS_PIDFD_GETFD, unix.SYS_KCMP,
	unix.SYS_IO_URING_SETUP, unix.SYS_IO_URING_ENTER, unix.SYS_IO_URING_REGISTER,
	unix.SYS_BPF, unix.SYS_PERF_EVENT_OPEN, unix.SYS_USERFAULTFD,
	unix.SYS_KEYCTL, unix.SYS_ADD_KEY, unix.SYS_REQUEST_KEY,
	unix.SYS_UNSHARE, unix.SYS_SETNS,
	unix.SYS_MOUNT, unix.SYS_UMOUNT2, unix.SYS_PIVOT_ROOT, unix.SYS_CHROOT,
	unix.SYS_OPEN_TREE, unix.SYS_MOVE_MOUNT, unix.SYS_FSOPEN, unix.SYS_FSCONFIG, unix.SYS_FSMOUNT, unix.SYS_FSPICK,
	unix.SYS_MOUNT_SETATTR,
}

// namespaceFlags are the clone flags that create namespaces.
const namespaceFlags = unix.CLONE_NEWUSER | unix.CLONE_NEWNS | unix.CLONE_NEWNET | unix.CLONE_NEWPID |
	unix.CLONE_NEWIPC | unix.CLONE_NEWUTS | unix.CLONE_NEWCGROUP

// bpfAsm assembles a classic BPF program whose jumps name labels.
type bpfAsm struct {
	ins    []unix.SockFilter
	jumps  []bpfJump
	labels map[string]int
}

type bpfJump struct {
	at     int
	jt, jf string // "" falls through
}

func (a *bpfAsm) stmt(code uint16, k uint32) {
	a.ins = append(a.ins, unix.SockFilter{Code: code, K: k})
}

func (a *bpfAsm) jump(code uint16, k uint32, jt, jf string) {
	a.jumps = append(a.jumps, bpfJump{len(a.ins), jt, jf})
	a.stmt(code, k)
}

func (a *bpfAsm) label(name string) {
	if a.labels == nil {
		a.labels = map[string]int{}
	}
	a.labels[name] = len(a.ins)
}

func (a *bpfAsm) assemble() []unix.SockFilter {
	off := func(from int, to string) uint8 {
		if to == "" {
			return 0
		}
		d := a.labels[to] - from - 1
		if d < 0 || d > 255 {
			panic("cdmhost: seccomp jump out of range: " + to)
		}
		return uint8(d)
	}
	for _, j := range a.jumps {
		a.ins[j.at].Jt, a.ins[j.at].Jf = off(j.at, j.jt), off(j.at, j.jf)
	}
	return a.ins
}

// seccompFilter refuses syscalls of a foreign ABI (i386 int 0x80, x32) and
// clone3, whose flags it cannot inspect, with ENOSYS (glibc then uses
// clone), fails deniedSyscalls, clone with namespace flags and socket or
// socketpair of any family but AF_UNIX with EPERM, and allows everything
// else.
func seccompFilter() []unix.SockFilter {
	const (
		ld   = unix.BPF_LD | unix.BPF_W | unix.BPF_ABS
		jeq  = unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K
		jge  = unix.BPF_JMP | unix.BPF_JGE | unix.BPF_K
		jset = unix.BPF_JMP | unix.BPF_JSET | unix.BPF_K
		ret  = unix.BPF_RET | unix.BPF_K
		// struct seccomp_data offsets: nr, arch, args[0] (low word, little endian).
		offNr, offArch, offArg0 = 0, 4, 16
	)
	var a bpfAsm
	a.stmt(ld, offArch)
	a.jump(jeq, auditArch, "", "enosys")
	a.stmt(ld, offNr)
	a.jump(jge, x32Bit, "enosys", "")
	for _, nr := range deniedSyscalls {
		a.jump(jeq, nr, "eperm", "")
	}
	a.jump(jeq, unix.SYS_CLONE3, "enosys", "")
	a.jump(jeq, unix.SYS_SOCKET, "family", "")
	a.jump(jeq, unix.SYS_SOCKETPAIR, "family", "")
	a.jump(jeq, unix.SYS_CLONE, "", "allow")
	// clone: flags are the first argument on amd64 and arm64.
	a.stmt(ld, offArg0)
	a.jump(jset, namespaceFlags, "eperm", "allow")
	a.label("family")
	a.stmt(ld, offArg0)
	a.jump(jeq, unix.AF_UNIX, "allow", "eperm")
	a.label("allow")
	a.stmt(ret, unix.SECCOMP_RET_ALLOW)
	a.label("eperm")
	a.stmt(ret, unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM))
	a.label("enosys")
	a.stmt(ret, unix.SECCOMP_RET_ERRNO|uint32(unix.ENOSYS))
	return a.assemble()
}

// installSeccomp applies seccompFilter to every thread of the process
// (TSYNC: the Go runtime already runs several) after setting
// no_new_privs, which an unprivileged filter requires.
func installSeccomp() error {
	if auditArch == 0 {
		return fmt.Errorf("seccomp filter not built for %s", runtime.GOARCH)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("no_new_privs: %w", err)
	}
	f := seccompFilter()
	prog := unix.SockFprog{Len: uint16(len(f)), Filter: &f[0]}
	r, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&prog)))
	runtime.KeepAlive(f)
	switch {
	case errno != 0:
		return fmt.Errorf("seccomp: %w", errno)
	case r != 0:
		return fmt.Errorf("seccomp: thread %d could not be synchronised", r)
	}
	return nil
}

// refusedFamilies are the socket families the child checks it cannot open.
var refusedFamilies = []struct {
	name        string
	family, typ int
	proto       int
}{
	{"AF_INET", unix.AF_INET, unix.SOCK_STREAM, 0},
	{"AF_INET6", unix.AF_INET6, unix.SOCK_STREAM, 0},
	{"AF_NETLINK", unix.AF_NETLINK, unix.SOCK_RAW, unix.NETLINK_ROUTE},
}

// netRefused tries to open each of refusedFamilies and reports nil when
// all are refused with EPERM or EACCES.
func netRefused() error {
	for _, f := range refusedFamilies {
		fd, err := unix.Socket(f.family, f.typ|unix.SOCK_CLOEXEC, f.proto)
		if err == nil {
			_ = unix.Close(fd)
			return fmt.Errorf("socket(%s) succeeded", f.name)
		}
		if !errors.Is(err, unix.EPERM) && !errors.Is(err, unix.EACCES) {
			return fmt.Errorf("socket(%s): %w", f.name, err)
		}
	}
	return nil
}

// confine installs the child's seccomp filter and checks it by trying to
// open inet and netlink sockets. The namespaces are the parent's part
// (spawn), the filesystem view and Landlock the first stage's.
func confine() Sandbox {
	var sb Sandbox
	var reasons []string
	if err := installSeccomp(); err != nil {
		reasons = append(reasons, "seccomp filter not installed: "+err.Error())
	} else {
		sb.Seccomp = true
	}
	if err := netRefused(); err != nil {
		reasons = append(reasons, "network sockets allowed: "+err.Error())
	} else {
		sb.InetDenied = true
	}
	sb.Reason = strings.Join(reasons, "; ")
	return sb
}
