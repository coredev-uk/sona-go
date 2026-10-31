package cdmhost

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

// sandboxStage1 is the child's first stage (see enterSandbox). It refreshes
// GStreamer's plugin registry while the home directory is still writable,
// builds the filesystem view when the parent started it in namespaces,
// restricts this thread with Landlock and re-executes the binary from it:
// the new image keeps the mount namespace, Landlock domain and
// no_new_privs, and has no capabilities. It exits if the re-exec fails.
func sandboxStage1() {
	runtime.LockOSThread()
	// The registry lives in the home directory, read-only from here on;
	// GStreamer in the second stage then reads an up-to-date one. fd 1 is
	// the protocol: point it at stderr meanwhile. A child that only loads
	// the CDM (the load check) never starts GStreamer.
	if os.Getenv(envCDMOnly) != "1" {
		if out, err := unix.Dup(1); err == nil {
			if unix.Dup3(2, 1, 0) == nil {
				_ = gst.Init()
				_ = unix.Dup3(out, 1, 0)
			}
			_ = unix.Close(out)
		}
	}
	var rep stageReport
	if os.Getenv(envSandboxNS) == "1" {
		if err := isolateFS(); err != nil {
			rep.reasons = append(rep.reasons, "no private filesystem view: "+err.Error())
		} else {
			rep.mount = true
		}
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		rep.reasons = append(rep.reasons, "no_new_privs: "+err.Error())
	}
	abi, err := restrictLandlock(rep.mount)
	if err != nil {
		rep.reasons = append(rep.reasons, "no Landlock: "+err.Error())
	}
	rep.landlock = abi
	// Ambient capabilities would survive the exec; nothing else does.
	_ = unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0)
	env := slices.DeleteFunc(os.Environ(), func(kv string) bool { return strings.HasPrefix(kv, envSandboxNS+"=") })
	env = append(env, envSandboxStage+"="+rep.String())
	err = syscall.Exec("/proc/self/exe", os.Args, env)
	fmt.Fprintln(os.Stderr, "cdmhost: re-execute into the sandbox:", err)
	os.Exit(1)
}

// fsLayout is the filesystem view the child gets in its mount namespace.
type fsLayout struct {
	home       string
	runtimeDir string
	// fresh are the directories replaced by an empty tmpfs (the runtime
	// directory last, after /run); hidden the home directories replaced by
	// an empty read-only one.
	fresh  []string
	hidden []string
	// keep are the files and directories under fresh or hidden ones that
	// are bound back: the audio server's socket and the program's own
	// files.
	keep []string
	// sockets are the Unix sockets the child may connect to (Landlock).
	sockets []string
	// cdmDir holds the CDM library the parent will load (envCDMDir); it
	// is kept and made read-only.
	cdmDir string
}

// layout derives the child's filesystem view from its environment.
func layout() fsLayout {
	l := fsLayout{home: os.Getenv("HOME"), runtimeDir: os.Getenv("XDG_RUNTIME_DIR")}
	l.fresh = []string{"/tmp", "/var/tmp", "/run"}
	if l.runtimeDir != "" {
		l.fresh = append(l.fresh, l.runtimeDir)
	}
	if l.home != "" && l.home != "/" {
		xdg := func(env, def string) string {
			if d := os.Getenv(env); filepath.IsAbs(d) {
				return d
			}
			return filepath.Join(l.home, def)
		}
		l.hidden = []string{
			filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "sona"), // settings, credentials.json
			filepath.Join(xdg("XDG_STATE_HOME", ".local/state"), "sona"),
			filepath.Join(xdg("XDG_DATA_HOME", ".local/share"), "keyrings"),
			filepath.Join(l.home, ".ssh"),
			filepath.Join(l.home, ".gnupg"),
			filepath.Join(l.home, ".password-store"),
		}
	}
	l.sockets = audioSockets(l.runtimeDir)
	l.keep = append(l.keep, l.sockets...)
	if exe, err := os.Executable(); err == nil {
		l.keep = append(l.keep, filepath.Dir(exe))
	}
	if d := os.Getenv("APPDIR"); d != "" { // AppImage, mounted under /tmp
		l.keep = append(l.keep, d)
	}
	for _, env := range []string{"LD_LIBRARY_PATH", "GST_PLUGIN_PATH", "GST_PLUGIN_PATH_1_0", "GST_PLUGIN_SYSTEM_PATH", "GST_PLUGIN_SYSTEM_PATH_1_0"} {
		l.keep = append(l.keep, filepath.SplitList(os.Getenv(env))...)
	}
	for _, env := range []string{"GST_PLUGIN_SCANNER", "GST_PLUGIN_SCANNER_1_0", "GST_REGISTRY", "GST_REGISTRY_1_0"} {
		if p := os.Getenv(env); p != "" {
			l.keep = append(l.keep, p)
		}
	}
	if d := os.Getenv(envCDMDir); filepath.IsAbs(d) {
		l.cdmDir = filepath.Clean(d)
		l.keep = append(l.keep, l.cdmDir)
	}
	return l
}

// audioSockets are the audio server sockets the output can use: PipeWire's,
// else PulseAudio's.
func audioSockets(runtimeDir string) []string {
	pw := cmp.Or(os.Getenv("PIPEWIRE_REMOTE"), "pipewire-0")
	if !filepath.IsAbs(pw) {
		pw = filepath.Join(cmp.Or(os.Getenv("PIPEWIRE_RUNTIME_DIR"), runtimeDir), pw)
	}
	pulse := strings.TrimPrefix(os.Getenv("PULSE_SERVER"), "unix:")
	if !filepath.IsAbs(pulse) && runtimeDir != "" {
		pulse = filepath.Join(runtimeDir, "pulse", "native")
	}
	for _, p := range []string{pw, pulse} {
		if fi, err := os.Stat(p); err == nil && fi.Mode().Type() == os.ModeSocket {
			return []string{p}
		}
	}
	return nil
}

// under reports whether p is dir or beneath it.
func under(p, dir string) bool {
	return p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}

// isolateFS builds the filesystem view described on Sandbox in the child's
// mount namespace. It needs CAP_SYS_ADMIN in the child's user namespace.
func isolateFS() error {
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return fmt.Errorf("make mounts private: %w", err)
	}
	l := layout()
	// Resolve what is bound back before anything is covered, holding it
	// open: once its directory is covered its path no longer reaches it.
	type held struct {
		path string
		fd   int
		dir  bool
	}
	var keep []held
	defer func() {
		for _, k := range keep {
			_ = unix.Close(k.fd)
		}
	}()
	covered := func(p string) bool {
		for _, d := range slices.Concat(l.fresh, l.hidden) {
			if under(p, d) {
				return true
			}
		}
		return false
	}
	for _, p := range l.keep {
		if !filepath.IsAbs(p) {
			continue
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		if !covered(p) || slices.ContainsFunc(keep, func(k held) bool { return k.path == p }) {
			continue
		}
		fd, err := unix.Open(p, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			continue
		}
		var st unix.Stat_t
		if unix.Fstat(fd, &st) != nil {
			_ = unix.Close(fd)
			continue
		}
		keep = append(keep, held{p, fd, st.Mode&unix.S_IFMT == unix.S_IFDIR})
	}
	// /run's top-level symlinks (NixOS's current-system, opengl-driver;
	// /run/shm) point at libraries and configuration, never at sockets.
	runLinks := map[string]string{}
	if entries, err := os.ReadDir("/run"); err == nil {
		for _, e := range entries {
			if e.Type() == os.ModeSymlink {
				if t, err := os.Readlink(filepath.Join("/run", e.Name())); err == nil {
					runLinks[e.Name()] = t
				}
			}
		}
	}

	for _, d := range l.fresh {
		fi, err := os.Lstat(d)
		if err != nil || !fi.IsDir() {
			if d == l.runtimeDir {
				// It lived under /run, now empty: recreate it.
				if err := os.MkdirAll(d, 0o700); err != nil {
					return fmt.Errorf("runtime directory: %w", err)
				}
			} else {
				continue
			}
		}
		mode := "1777"
		switch d {
		case "/run":
			mode = "0755"
		case l.runtimeDir:
			mode = "0700"
		}
		if err := unix.Mount("tmpfs", d, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV, "mode="+mode+",size=64m"); err != nil {
			return fmt.Errorf("tmpfs on %s: %w", d, err)
		}
		if d == "/run" {
			for name, target := range runLinks {
				_ = os.Symlink(target, filepath.Join("/run", name))
			}
		}
	}

	if l.home != "" && l.home != "/" {
		if err := readOnly(l.home); err != nil {
			return fmt.Errorf("read-only home: %w", err)
		}
		for _, d := range l.hidden {
			if fi, err := os.Lstat(d); err != nil || !fi.IsDir() {
				continue
			}
			if err := unix.Mount("tmpfs", d, "tmpfs", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC|unix.MS_RDONLY, "mode=0700,size=4k"); err != nil {
				return fmt.Errorf("hide %s: %w", d, err)
			}
		}
	}

	// Bind back what is kept, shortest paths first so nested ones land on
	// top. A mount point under a hidden (read-only) directory cannot be
	// created; such a path stays hidden.
	slices.SortFunc(keep, func(a, b held) int { return cmp.Compare(len(a.path), len(b.path)) })
	for _, k := range keep {
		if k.dir {
			if err := os.MkdirAll(k.path, 0o755); err != nil {
				continue
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(k.path), 0o755); err != nil {
				continue
			}
			if f, err := os.OpenFile(k.path, os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				_ = f.Close()
			}
		}
		src := "/proc/self/fd/" + strconv.Itoa(k.fd)
		if err := unix.Mount(src, k.path, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
			return fmt.Errorf("bind %s: %w", k.path, err)
		}
	}
	// The CDM's directory is read-only wherever it lies: under the home
	// directory it already is; under the private /tmp, Landlock would
	// otherwise let the child write there.
	if l.cdmDir != "" {
		if fi, err := os.Stat(l.cdmDir); err == nil && fi.IsDir() {
			if err := readOnly(l.cdmDir); err != nil {
				return fmt.Errorf("read-only CDM directory: %w", err)
			}
		}
	}
	return nil
}

// readOnly makes dir, and every mount beneath it, a read-only bind mount.
func readOnly(dir string) error {
	if err := unix.Mount(dir, dir, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return err
	}
	err := unix.MountSetattr(unix.AT_FDCWD, dir, unix.AT_RECURSIVE, &unix.MountAttr{Attr_set: unix.MOUNT_ATTR_RDONLY})
	if !errors.Is(err, unix.ENOSYS) {
		return err
	}
	// Before Linux 5.12: the top mount only. A remount must repeat the
	// flags the mount is locked with.
	var st unix.Statfs_t
	if err := unix.Statfs(dir, &st); err != nil {
		return err
	}
	flags := uintptr(unix.MS_BIND | unix.MS_REMOUNT | unix.MS_RDONLY)
	for _, f := range [][2]uintptr{{unix.ST_NOSUID, unix.MS_NOSUID}, {unix.ST_NODEV, unix.MS_NODEV}, {unix.ST_NOEXEC, unix.MS_NOEXEC}, {unix.ST_NOATIME, unix.MS_NOATIME}, {unix.ST_NODIRATIME, unix.MS_NODIRATIME}, {unix.ST_RELATIME, unix.MS_RELATIME}} {
		if uintptr(st.Flags)&f[0] != 0 {
			flags |= f[1]
		}
	}
	return unix.Mount("", dir, "", flags, "")
}

// llWrite are the Landlock filesystem rights that write, from ABI 1; each
// handled right is denied unless a rule allows it.
const llWrite = unix.LANDLOCK_ACCESS_FS_WRITE_FILE | unix.LANDLOCK_ACCESS_FS_REMOVE_DIR | unix.LANDLOCK_ACCESS_FS_REMOVE_FILE |
	unix.LANDLOCK_ACCESS_FS_MAKE_CHAR | unix.LANDLOCK_ACCESS_FS_MAKE_DIR | unix.LANDLOCK_ACCESS_FS_MAKE_REG |
	unix.LANDLOCK_ACCESS_FS_MAKE_SOCK | unix.LANDLOCK_ACCESS_FS_MAKE_FIFO | unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK |
	unix.LANDLOCK_ACCESS_FS_MAKE_SYM

// restrictLandlock confines this thread (and what it executes) with
// Landlock: writes only under /dev (and the private /tmp when the
// filesystem view is in place), no TCP or UDP, connect() only to the audio
// server's socket, abstract Unix sockets and signals scoped to the
// sandbox. It returns the ABI version enforced, 0 when the kernel has no
// Landlock.
func restrictLandlock(privateTmp bool) (int, error) {
	r, _, errno := unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, unix.LANDLOCK_CREATE_RULESET_VERSION)
	if errno != 0 {
		return 0, errno
	}
	abi := int(r)
	attr := unix.LandlockRulesetAttr{Handled_access_fs: llWrite}
	if abi >= 2 {
		attr.Handled_access_fs |= unix.LANDLOCK_ACCESS_FS_REFER
	}
	if abi >= 3 {
		attr.Handled_access_fs |= unix.LANDLOCK_ACCESS_FS_TRUNCATE
	}
	if abi >= 4 {
		attr.Handled_access_net = unix.LANDLOCK_ACCESS_NET_BIND_TCP | unix.LANDLOCK_ACCESS_NET_CONNECT_TCP
	}
	if abi >= 6 {
		attr.Scoped = unix.LANDLOCK_SCOPE_ABSTRACT_UNIX_SOCKET | unix.LANDLOCK_SCOPE_SIGNAL
	}
	if abi >= 9 {
		attr.Handled_access_fs |= unix.LANDLOCK_ACCESS_FS_RESOLVE_UNIX
	}
	if abi >= 10 {
		attr.Handled_access_net |= unix.LANDLOCK_ACCESS_NET_BIND_UDP | unix.LANDLOCK_ACCESS_NET_CONNECT_SEND_UDP
	}
	// A kernel that knows fewer fields accepts the larger struct as long
	// as the fields it lacks are zero.
	r, _, errno = unix.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		return 0, fmt.Errorf("create ruleset: %w", errno)
	}
	ruleset := int(r)
	defer func() { _ = unix.Close(ruleset) }()

	writable := attr.Handled_access_fs &^ unix.LANDLOCK_ACCESS_FS_RESOLVE_UNIX
	dirs := []string{"/dev"}
	if privateTmp {
		dirs = append(dirs, "/tmp", "/var/tmp")
	}
	allow := func(path string, access uint64) error {
		fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		defer func() { _ = unix.Close(fd) }()
		rule := unix.LandlockPathBeneathAttr{Allowed_access: access, Parent_fd: int32(fd)}
		_, _, errno := unix.Syscall6(unix.SYS_LANDLOCK_ADD_RULE, uintptr(ruleset), unix.LANDLOCK_RULE_PATH_BENEATH, uintptr(unsafe.Pointer(&rule)), 0, 0, 0)
		if errno != 0 {
			return fmt.Errorf("rule for %s: %w", path, errno)
		}
		return nil
	}
	for _, d := range dirs {
		if err := allow(d, writable); err != nil && !errors.Is(err, unix.ENOENT) {
			return 0, err
		}
	}
	if abi >= 9 {
		for _, s := range layout().sockets {
			if err := allow(s, unix.LANDLOCK_ACCESS_FS_RESOLVE_UNIX); err != nil {
				return 0, err
			}
		}
	}
	if _, _, errno := unix.Syscall(unix.SYS_LANDLOCK_RESTRICT_SELF, uintptr(ruleset), 0, 0); errno != 0 {
		return 0, fmt.Errorf("restrict: %w", errno)
	}
	return abi, nil
}
