//go:build unix

package addons

import (
	"io/fs"
	"os"
	"syscall"
)

// checkFileSafe refuses a file that another user owns or that group or
// others may write: either could replace the code Sona is about to run.
func checkFileSafe(fi fs.FileInfo) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return problem(ProblemUnsafe, "no owner information")
	}
	if int(st.Uid) != os.Geteuid() {
		return problem(ProblemUnsafe, "owned by user %d", st.Uid)
	}
	if fi.Mode().Perm()&0o022 != 0 {
		return problem(ProblemUnsafe, "writable by group or others (%v)", fi.Mode().Perm())
	}
	return nil
}

// checkDirSafe is checkFileSafe for a folder that must also be a real
// folder: a link to one is refused (Lstat), so the folder can't be swapped
// for another place.
func checkDirSafe(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return problem(ProblemUnsafe, "%s is not a folder", path)
	}
	return checkFileSafe(fi)
}
