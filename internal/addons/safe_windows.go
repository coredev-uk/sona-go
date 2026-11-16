//go:build windows

package addons

import (
	"io/fs"
	"os"
)

// Windows keeps a profile's folders to their user by their ACLs, which Go's
// file modes don't show; nothing more is checked here, and docs/addons.md
// says so.
func checkFileSafe(fs.FileInfo) error { return nil }

func checkDirSafe(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return problem(ProblemUnsafe, "%s is not a folder", path)
	}
	return nil
}
