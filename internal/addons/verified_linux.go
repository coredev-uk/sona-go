//go:build linux

package addons

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// CanRunVerified reports whether this system can run an add-on from the very
// file that was checked: Linux, through /proc/self/fd.
const CanRunVerified = true

// runFD is the path of the child's descriptor 3 (Spec.ExeFile).
const runFD = "/proc/self/fd/3"

// OpenVerified opens exe once and checks the open file, not the path: it is
// a regular file the user owns that group and others can't write, it has an
// execute bit, and the place the descriptor stands for is inside dir (which
// is already link-free). A path swapped for another file, or for a link,
// after this returns changes nothing: the descriptor is the file that was
// checked, and Spec.ExeFile runs that. The caller closes it once the add-on
// has started.
func OpenVerified(dir, exe string) (*os.File, error) {
	// O_NOFOLLOW: a link in the last place is refused, not followed.
	fd, err := syscall.Open(exe, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, problem(ProblemNoExecutable, "open %s: %v", filepath.Base(exe), err)
	}
	f := os.NewFile(uintptr(fd), exe)
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, problem(ProblemUnreadable, "%v", err)
	}
	if !fi.Mode().IsRegular() {
		_ = f.Close()
		return nil, problem(ProblemNoExecutable, "%s is not a regular file", filepath.Base(exe))
	}
	if fi.Mode().Perm()&0o111 == 0 {
		_ = f.Close()
		return nil, problem(ProblemNoExecutable, "%s is not executable", filepath.Base(exe))
	}
	if err := checkFileSafe(fi); err != nil {
		_ = f.Close()
		return nil, err
	}
	// Where the descriptor really is: inside the folder.
	at, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", f.Fd()))
	if err != nil {
		_ = f.Close()
		return nil, problem(ProblemUnreadable, "%v", err)
	}
	base, err := filepath.EvalSymlinks(dir)
	if err != nil {
		_ = f.Close()
		return nil, problem(ProblemUnreadable, "%v", err)
	}
	if rel, err := filepath.Rel(base, at); err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		_ = f.Close()
		return nil, problem(ProblemEscapes, "the program is not inside the folder")
	}
	return f, nil
}
