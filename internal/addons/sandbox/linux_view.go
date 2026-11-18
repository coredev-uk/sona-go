//go:build linux

package sandbox

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"golang.org/x/sys/unix"
)

// The add-on's filesystem is not the user's with parts hidden: it is a new
// one (a tmpfs root) that holds only what the policy names, each as a bind
// mount of the real thing:
//
//   - read-only: the system's programs and libraries (/usr, /bin, /lib*, the
//     Nix and Guix stores), the few files of /etc a program needs to run, to
//     resolve names and to check certificates, the add-on's own folder and
//     what the policy lets it read;
//   - read and write, never run: its data folder;
//   - the devices null, zero, full, random and urandom on a /dev of their own,
//     and a /proc of its own (it sees only the processes of its PID
//     namespace).
//
// Nothing else exists: not the home directory, /tmp, /run (so no D-Bus, no
// agent socket, no X11 or Wayland socket, no audio server), /sys, nor any
// terminal (/dev/tty, /dev/pts). A path the policy doesn't name can't be
// opened on any kernel; Landlock (linux_landlock.go) then asks the same of the
// kernel a second time where it is there.

// systemPaths are the system's files the add-on may read, when they exist.
// A symlink is followed and what it leads to is mounted at the link's place.
var systemPaths = []string{
	"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/libx32",
	"/nix/store", "/gnu/store", "/run/current-system",
	"/etc/static", "/etc/alternatives",
	"/etc/ssl", "/etc/ca-certificates", "/etc/ca-certificates.conf", "/etc/pki", "/etc/crypto-policies",
	"/etc/ld.so.cache", "/etc/ld.so.conf", "/etc/ld.so.conf.d",
	"/etc/passwd", "/etc/group", "/etc/nsswitch.conf", "/etc/hosts", "/etc/host.conf", "/etc/gai.conf",
	"/etc/resolv.conf", "/etc/services", "/etc/protocols",
	"/etc/localtime", "/etc/timezone", "/etc/os-release", "/etc/locale.conf", "/etc/locale.alias", "/etc/mime.types",
}

// devices are the device files of the add-on's /dev.
var devices = []string{"/dev/null", "/dev/zero", "/dev/full", "/dev/random", "/dev/urandom"}

// bindKind is how a mount is made.
type bindKind int

const (
	bindReadOnly bindKind = iota // readable and runnable, not writable
	bindData                     // readable and writable, not runnable
	bindSocket                   // a Unix socket the add-on may connect to
	bindDevice                   // a device file in /dev
)

// A bind is one mount of the new root: src (on the old root) appears at dst.
type bind struct {
	src, dst string
	dir      bool
	kind     bindKind
	fd       int // src, opened before anything is mounted
}

// view is what buildView made: the places Landlock allows.
type view struct {
	binds []bind
}

// buildView builds the add-on's filesystem in this process's mount namespace
// and enters it. It needs CAP_SYS_ADMIN in the user namespace.
func buildView(req request) (*view, error) {
	if err := unix.Mount("", "/", "", unix.MS_REC|unix.MS_PRIVATE, ""); err != nil {
		return nil, fmt.Errorf("make mounts private: %w", err)
	}
	v := &view{}
	defer func() {
		for _, b := range v.binds {
			if b.fd > 0 {
				_ = unix.Close(b.fd)
			}
		}
	}()
	// add mounts path read-only, or as kind says, at its own place and, for
	// what the policy names, also where it really is: the program's path was
	// resolved, and the add-on's own folder may be a link.
	add := func(path string, kind bindKind, must, alsoReal bool) error {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			if must {
				return fmt.Errorf("%s: %w", path, err)
			}
			return nil
		}
		var st unix.Stat_t
		if err := unix.Stat(real, &st); err != nil {
			if must {
				return fmt.Errorf("%s: %w", path, err)
			}
			return nil
		}
		isDir := st.Mode&unix.S_IFMT == unix.S_IFDIR
		places := []string{path}
		if alsoReal {
			places = slices.Compact([]string{path, real})
		}
		for _, dst := range places {
			if slices.ContainsFunc(v.binds, func(b bind) bool { return b.dst == dst }) {
				continue
			}
			v.binds = append(v.binds, bind{src: real, dst: dst, dir: isDir, kind: kind})
		}
		return nil
	}
	for _, p := range systemPaths {
		_ = add(p, bindReadOnly, false, false)
	}
	if err := add(req.Dir, bindReadOnly, true, true); err != nil {
		return nil, err
	}
	for _, p := range req.Read {
		if err := add(p, bindReadOnly, false, true); err != nil {
			return nil, err
		}
	}
	if err := add(req.Data, bindData, true, true); err != nil {
		return nil, err
	}
	for _, p := range req.Sockets {
		_ = add(p, bindSocket, false, true)
	}
	for _, p := range devices {
		_ = add(p, bindDevice, false, false)
	}

	// Everything is opened before the first mount: a source may lie under a
	// directory this view covers (the program in /tmp, in an AppImage).
	for i := range v.binds {
		fd, err := unix.Open(v.binds[i].src, unix.O_PATH|unix.O_CLOEXEC, 0)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", v.binds[i].src, err)
		}
		v.binds[i].fd = fd
	}

	// The new root is a tmpfs inside a scratch tmpfs on a directory that
	// exists; both end up under the old root, which goes.
	scratch := ""
	for _, d := range []string{"/tmp", "/var/tmp", "/mnt", "/media", "/srv", "/opt"} {
		if fi, err := os.Lstat(d); err == nil && fi.IsDir() {
			scratch = d
			break
		}
	}
	if scratch == "" {
		return nil, errors.New("no directory to build the new root in")
	}
	const mountFlags = unix.MS_NOSUID | unix.MS_NODEV
	if err := unix.Mount("tmpfs", scratch, "tmpfs", mountFlags, "mode=0700,size=1m"); err != nil {
		return nil, fmt.Errorf("scratch tmpfs on %s: %w", scratch, err)
	}
	root := filepath.Join(scratch, "r")
	for _, step := range []struct {
		name string
		do   func() error
	}{
		{"make the new root", func() error { return os.Mkdir(root, 0o755) }},
		{"mount the new root", func() error { return unix.Mount("tmpfs", root, "tmpfs", mountFlags, "mode=0755,size=8m") }},
		{"make the old root's place", func() error { return os.Mkdir(filepath.Join(root, ".old"), 0o700) }},
	} {
		if err := step.do(); err != nil {
			return nil, fmt.Errorf("%s: %w", step.name, err)
		}
	}

	// Mount points shortest first, so that a nested one lands on top.
	slices.SortStableFunc(v.binds, func(a, b bind) int { return cmp.Compare(len(a.dst), len(b.dst)) })
	for _, b := range v.binds {
		if b.kind == bindDevice {
			continue // after /dev exists
		}
		if err := mountBind(root, b); err != nil {
			return nil, err
		}
	}

	dev := filepath.Join(root, "dev")
	if err := os.Mkdir(dev, 0o755); err != nil {
		return nil, fmt.Errorf("make /dev: %w", err)
	}
	if err := unix.Mount("tmpfs", dev, "tmpfs", mountFlags|unix.MS_NOEXEC, "mode=0755,size=64k"); err != nil {
		return nil, fmt.Errorf("mount /dev: %w", err)
	}
	for _, b := range v.binds {
		if b.kind == bindDevice {
			if err := mountBind(root, b); err != nil {
				return nil, err
			}
		}
	}
	for name, target := range map[string]string{"fd": "/proc/self/fd", "stdin": "/proc/self/fd/0", "stdout": "/proc/self/fd/1", "stderr": "/proc/self/fd/2"} {
		if err := os.Symlink(target, filepath.Join(dev, name)); err != nil {
			return nil, fmt.Errorf("make /dev/%s: %w", name, err)
		}
	}
	proc := filepath.Join(root, "proc")
	if err := os.Mkdir(proc, 0o555); err != nil {
		return nil, fmt.Errorf("make /proc: %w", err)
	}
	if err := unix.Mount("proc", proc, "proc", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		return nil, fmt.Errorf("mount /proc: %w", err)
	}

	if err := unix.PivotRoot(root, filepath.Join(root, ".old")); err != nil {
		return nil, fmt.Errorf("pivot_root: %w", err)
	}
	if err := unix.Chdir("/"); err != nil {
		return nil, fmt.Errorf("chdir: %w", err)
	}
	if err := unix.Unmount("/.old", unix.MNT_DETACH); err != nil {
		return nil, fmt.Errorf("drop the old root: %w", err)
	}
	_ = unix.Rmdir("/.old")
	// What is left of the new root and /dev is its own tmpfs: no writing.
	for _, d := range []string{"/", "/dev"} {
		if err := unix.Mount("", d, "", unix.MS_REMOUNT|unix.MS_RDONLY|unix.MS_NOSUID|unix.MS_NODEV, ""); err != nil {
			return nil, fmt.Errorf("make %s read-only: %w", d, err)
		}
	}
	return v, nil
}

// mountBind makes b's mount point in root and mounts b's source there.
func mountBind(root string, b bind) error {
	dst := filepath.Join(root, b.dst)
	if b.dir {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return fmt.Errorf("mount point %s: %w", b.dst, err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("mount point %s: %w", b.dst, err)
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("mount point %s: %w", b.dst, err)
		}
		_ = f.Close()
	}
	src := "/proc/self/fd/" + strconv.Itoa(b.fd)
	if err := unix.Mount(src, dst, "", unix.MS_BIND|unix.MS_REC, ""); err != nil {
		return fmt.Errorf("bind %s: %w", b.src, err)
	}
	var set attrs
	switch b.kind {
	case bindReadOnly:
		set = attrs{readOnly: true, noSuid: true, noDev: true}
	case bindData:
		set = attrs{noSuid: true, noDev: true, noExec: true}
	case bindSocket:
		set = attrs{noSuid: true, noDev: true, noExec: true}
	case bindDevice:
		set = attrs{noSuid: true, noExec: true}
	}
	if err := setAttrs(dst, set, b.dir); err != nil {
		return fmt.Errorf("restrict %s: %w", b.dst, err)
	}
	return nil
}

// attrs are the mount flags a bind mount is given on top of its source's.
type attrs struct {
	readOnly, noSuid, noDev, noExec bool
}

// setAttrs adds attrs to the mount at path and, for a directory, the mounts
// beneath it that came with it.
func setAttrs(path string, a attrs, recursive bool) error {
	var set uint64
	for _, f := range []struct {
		on  bool
		bit uint64
	}{{a.readOnly, unix.MOUNT_ATTR_RDONLY}, {a.noSuid, unix.MOUNT_ATTR_NOSUID}, {a.noDev, unix.MOUNT_ATTR_NODEV}, {a.noExec, unix.MOUNT_ATTR_NOEXEC}} {
		if f.on {
			set |= f.bit
		}
	}
	var flags uint
	if recursive {
		flags = unix.AT_RECURSIVE
	}
	err := unix.MountSetattr(unix.AT_FDCWD, path, flags, &unix.MountAttr{Attr_set: set})
	if !errors.Is(err, unix.ENOSYS) {
		return err
	}
	// Before Linux 5.12: the top mount only. A remount must repeat the
	// flags the mount is locked with.
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return err
	}
	remount := uintptr(unix.MS_BIND | unix.MS_REMOUNT)
	for _, f := range [][2]uintptr{
		{unix.ST_NOSUID, unix.MS_NOSUID},
		{unix.ST_NODEV, unix.MS_NODEV},
		{unix.ST_NOEXEC, unix.MS_NOEXEC},
		{unix.ST_NOATIME, unix.MS_NOATIME},
		{unix.ST_NODIRATIME, unix.MS_NODIRATIME},
		{unix.ST_RELATIME, unix.MS_RELATIME},
		{unix.ST_RDONLY, unix.MS_RDONLY},
	} {
		if uintptr(st.Flags)&f[0] != 0 {
			remount |= f[1]
		}
	}
	if a.readOnly {
		remount |= unix.MS_RDONLY
	}
	if a.noSuid {
		remount |= unix.MS_NOSUID
	}
	if a.noDev {
		remount |= unix.MS_NODEV
	}
	if a.noExec {
		remount |= unix.MS_NOEXEC
	}
	return unix.Mount("", path, "", remount, "")
}
