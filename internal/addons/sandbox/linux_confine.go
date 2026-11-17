//go:build linux

package sandbox

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Landlock is the second layer under the filesystem view: the kernel refuses
// what the view doesn't hold even if the process found a way to it. Where the
// kernel has no Landlock the view alone confines the add-on (report.Landlock
// is 0, and Sona says so in the add-on's log).

// llHandled are the rights of the ABI, all handled: whatever a rule doesn't
// grant is refused.
func llHandled(abi int) (fs, net, scoped uint64) {
	fs = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_READ_FILE |
		unix.LANDLOCK_ACCESS_FS_READ_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
		unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
		unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
		unix.LANDLOCK_ACCESS_FS_MAKE_SYM
	if abi >= 2 {
		fs |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		fs |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 4 {
		net = unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP
	}
	if abi >= 5 {
		fs |= unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
	}
	if abi >= 6 {
		scoped = unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | unix.LANDLOCK_SCOPE_SIGNAL
	}
	if abi >= 9 {
		fs |= unix.LANDLOCK_ACCESS_FS_RESOLVE_UNIX
	}
	if abi >= 10 {
		net |= unix.LANDLOCK_ACCESS_NET_BIND_UDP | unix.LANDLOCK_ACCESS_NET_CONNECT_SEND_UDP
	}
	return fs, net, scoped
}

// restrictLandlock confines this thread, and what it starts, to the view: read
// and run in the read-only places, read and write (never run) in the data
// folder, the devices, and connecting to the policy's sockets; no abstract
// Unix sockets or signals outside the sandbox (ABI 6) and, without network,
// no TCP or UDP at all. It returns the ABI enforced; 0, and no error, when
// the kernel has no Landlock.
func restrictLandlock(req request, v *view) (int, error) {
	r, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	switch errno {
	case 0:
	case unix.ENOSYS, unix.EOPNOTSUPP:
		return 0, nil // no Landlock in this kernel, or switched off
	default:
		return 0, fmt.Errorf("version: %w", errno)
	}
	abi := int(r)
	fsAll, netAll, scoped := llHandled(abi)
	attr := unix.LandlockRulesetAttr{Handled_access_fs: fsAll, Scoped: scoped}
	if !req.Network {
		attr.Handled_access_net = netAll
	}
	// A kernel that knows fewer fields accepts the larger struct as long as
	// the fields it lacks are zero.
	r, _, errno = unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return 0, fmt.Errorf("create ruleset: %w", errno)
	}
	ruleset := int(r)
	defer func() { _ = unix.Close(ruleset) }()

	const (
		read     = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_READ_DIR
		fileOnly = unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_READ_FILE |
			unix.LANDLOCK_ACCESS_FS_TRUNCATE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV | unix.LANDLOCK_ACCESS_FS_RESOLVE_UNIX
	)
	allow := func(path string, access uint64, dir bool) error {
		access &= fsAll
		if !dir {
			access &= fileOnly // the kernel refuses directory rights on a file
		}
		if access == 0 {
			return nil
		}
		fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		defer func() { _ = unix.Close(fd) }()
		rule := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
		if _, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(ruleset), unix.LANDLOCK_RULE_PATH_BENEATH, uintptr(unsafe.Pointer(&rule)), 0, 0, 0); errno != 0 {
			return fmt.Errorf("rule for %s: %w", path, errno)
		}
		return nil
	}
	write := fsAll &^ (unix.LANDLOCK_ACCESS_FS_EXECUTE | unix.LANDLOCK_ACCESS_FS_RESOLVE_UNIX)
	for _, b := range v.binds {
		var access uint64
		switch b.kind {
		case bindReadOnly:
			access = read | unix.LANDLOCK_ACCESS_FS_EXECUTE
		case bindData:
			access = write
		case bindDevice:
			access = unix.LANDLOCK_ACCESS_FS_READ_FILE | unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_TRUNCATE | unix.LANDLOCK_ACCESS_FS_IOCTL_DEV
		case bindSocket:
			access = unix.LANDLOCK_ACCESS_FS_RESOLVE_UNIX
		}
		if err := allow(b.dst, access, b.dir); err != nil {
			return 0, err
		}
	}
	// Its own /proc: the processes of its namespace, to read.
	if err := allow("/proc", read, true); err != nil {
		return 0, err
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(ruleset), 0, 0); errno != 0 {
		return 0, fmt.Errorf("restrict: %w", errno)
	}
	return abi, nil
}

// The seccomp filter is the one layer the add-on's own code can't get around
// with a path: it works on system calls. It fails the calls an add-on has no
// use for and that widen the kernel's attack surface, and it allows sockets
// only for what the policy grants.

// auditArch is the seccomp architecture of this build; 0 when the filter does
// not support it (Start then refuses the add-on).
var auditArch = map[string]uint32{
	"amd64": unix.AUDIT_ARCH_X86_64,
	"arm64": unix.AUDIT_ARCH_AARCH64,
}[runtime.GOARCH]

// x32Bit marks x32 ABI syscall numbers on amd64; nothing runs in that ABI, so
// it is refused rather than filtered a second time.
const x32Bit = 0x40000000

// deniedSyscalls fail with EPERM: debugging or reading other processes,
// io_uring (its IORING_OP_SOCKET would bypass the socket check), kernel
// attack surface an add-on has no use for, and namespace and mount changes.
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

// socketFamilies are the families socket() may open: the network's only with
// the permission, Unix sockets only where the policy names one. Every add-on
// may make an anonymous pair of Unix sockets (socketpair), which reaches
// nothing: Python's event loop and Node's child processes use them.
func socketFamilies(p Policy) []uint32 {
	var f []uint32
	if p.Network {
		f = append(f, unix.AF_INET, unix.AF_INET6)
	}
	if len(p.Sockets) > 0 {
		f = append(f, unix.AF_UNIX)
	}
	return f
}

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
			panic("sandbox: seccomp jump out of range: " + to)
		}
		return uint8(d)
	}
	for _, j := range a.jumps {
		a.ins[j.at].Jt, a.ins[j.at].Jf = off(j.at, j.jt), off(j.at, j.jf)
	}
	return a.ins
}

// seccompFilter refuses syscalls of a foreign ABI (i386 int 0x80, x32) and
// clone3, whose flags it cannot inspect, with ENOSYS (glibc then uses clone),
// fails deniedSyscalls, clone with namespace flags, and socket() of any family
// the policy doesn't grant, and socketpair() of any but AF_UNIX, with EPERM,
// and allows everything else.
func seccompFilter(p Policy) []unix.SockFilter {
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
	a.jump(jeq, unix.SYS_SOCKET, "socket", "")
	a.jump(jeq, unix.SYS_SOCKETPAIR, "pair", "")
	a.jump(jeq, unix.SYS_CLONE, "", "allow")
	// clone: flags are the first argument on amd64 and arm64.
	a.stmt(ld, offArg0)
	a.jump(jset, namespaceFlags, "eperm", "allow")
	a.label("socket")
	a.stmt(ld, offArg0)
	for _, fam := range socketFamilies(p) {
		a.jump(jeq, fam, "allow", "")
	}
	a.stmt(ret, unix.SECCOMP_RET_ERRNO|uint32(unix.EPERM))
	a.label("pair")
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

// installSeccomp applies seccompFilter to this thread (the one the add-on is
// started from; no_new_privs is already set, which an unprivileged filter
// requires).
func installSeccomp(req request) error {
	if auditArch == 0 {
		return fmt.Errorf("the syscall filter is not built for %s", runtime.GOARCH)
	}
	f := seccompFilter(req.Policy)
	prog := unix.SockFprog{Len: uint16(len(f)), Filter: &f[0]}
	_, _, errno := unix.Syscall(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, 0, uintptr(unsafe.Pointer(&prog)))
	runtime.KeepAlive(f)
	if errno != 0 {
		return fmt.Errorf("seccomp: %w", errno)
	}
	return nil
}
