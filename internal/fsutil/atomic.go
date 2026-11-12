// Package fsutil holds small filesystem helpers shared by the packages that
// persist state.
package fsutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// tempSuffix ends every temporary file WriteFileAtomic creates.
const tempSuffix = ".tmp"

// WriteFileAtomic replaces path with data: it writes a perm-mode temporary
// file in the same directory, syncs it, renames it over path and syncs the
// directory, so a crash leaves either the old or the new file, never a
// partial one. The directory must exist. A failed write removes its
// temporary file.
func WriteFileAtomic(path string, data []byte, perm fs.FileMode) (err error) {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*"+tempSuffix)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()
	// CreateTemp already uses 0600; Chmod applies perm whatever the umask.
	if err = f.Chmod(perm); err == nil {
		if _, err = f.Write(data); err == nil {
			err = f.Sync()
		}
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err = os.Rename(tmp, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	SyncDir(dir)
	return nil
}

// IsTemp reports whether name is a temporary file left by an interrupted
// WriteFileAtomic, for sweeps of a directory it writes into.
func IsTemp(name string) bool {
	return strings.HasPrefix(name, ".") && strings.HasSuffix(name, tempSuffix)
}

// SyncDir flushes a directory's entries (a rename) to disk. Best effort: not
// every OS or filesystem supports it.
func SyncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}
