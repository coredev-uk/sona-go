package addons

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/experimental/sysfs"
	wsys "github.com/tetratelabs/wazero/sys"
)

// confinedFS is the one folder a WebAssembly add-on may use: its data folder.
//
// wazero's own directory mount follows a link inside the folder wherever it
// points, and lets the module create one (`path_symlink`): a module could
// make a link to `~/.ssh` and read through it, with no permission at all.
// This wrapper closes that. No path may pass through a link, and the module
// can't make one. The `..` and absolute paths the module might try are
// already refused by wazero's WASI layer (fs.ValidPath), and checked again
// here.
//
// The check and the operation are two steps, which is safe because nothing
// races them: the module is one thread, and every file call it makes is
// served one at a time. A link the user puts in the folder is refused too:
// the data folder holds files and folders.
type confinedFS struct {
	experimentalsys.FS
	root string
}

// newConfinedFS mounts dir, which must be a folder, not a link to one.
func newConfinedFS(dir string) (experimentalsys.FS, error) {
	fi, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, errors.New("the data folder is not a folder: " + dir)
	}
	return confinedFS{FS: sysfs.DirFS(dir), root: dir}, nil
}

// check refuses a path that leaves the folder or passes through a link. When
// followLast is false the last element may itself be a link: the operation
// acts on the link, not on what it points to.
func (c confinedFS) check(path string, followLast bool) experimentalsys.Errno {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	cur := c.root
	for i, p := range parts {
		switch p {
		case "", ".":
			continue
		case "..":
			return experimentalsys.EPERM
		}
		cur = filepath.Join(cur, p)
		if i == len(parts)-1 && !followLast {
			return 0
		}
		fi, err := os.Lstat(cur)
		if errors.Is(err, fs.ErrNotExist) {
			// Nothing here: what follows is the operation's own ENOENT or
			// a creation inside a folder that is not a link.
			return 0
		}
		if err != nil {
			return experimentalsys.UnwrapOSError(err)
		}
		if fi.Mode()&(fs.ModeSymlink|fs.ModeIrregular) != 0 {
			return experimentalsys.EPERM
		}
	}
	return 0
}

func (c confinedFS) OpenFile(path string, flag experimentalsys.Oflag, perm fs.FileMode) (experimentalsys.File, experimentalsys.Errno) {
	if errno := c.check(path, true); errno != 0 {
		return nil, errno
	}
	return c.FS.OpenFile(path, flag, perm)
}

func (c confinedFS) Lstat(path string) (st wsys.Stat_t, errno experimentalsys.Errno) {
	if errno = c.check(path, false); errno != 0 {
		return st, errno
	}
	return c.FS.Lstat(path)
}

func (c confinedFS) Stat(path string) (st wsys.Stat_t, errno experimentalsys.Errno) {
	if errno = c.check(path, true); errno != 0 {
		return st, errno
	}
	return c.FS.Stat(path)
}

func (c confinedFS) Mkdir(path string, perm fs.FileMode) experimentalsys.Errno {
	if errno := c.check(path, false); errno != 0 {
		return errno
	}
	return c.FS.Mkdir(path, perm)
}

func (c confinedFS) Chmod(path string, perm fs.FileMode) experimentalsys.Errno {
	if errno := c.check(path, true); errno != 0 {
		return errno
	}
	return c.FS.Chmod(path, perm)
}

func (c confinedFS) Rename(from, to string) experimentalsys.Errno {
	if errno := c.check(from, false); errno != 0 {
		return errno
	}
	if errno := c.check(to, false); errno != 0 {
		return errno
	}
	return c.FS.Rename(from, to)
}

func (c confinedFS) Rmdir(path string) experimentalsys.Errno {
	if errno := c.check(path, false); errno != 0 {
		return errno
	}
	return c.FS.Rmdir(path)
}

func (c confinedFS) Unlink(path string) experimentalsys.Errno {
	if errno := c.check(path, false); errno != 0 {
		return errno
	}
	return c.FS.Unlink(path)
}

func (c confinedFS) Link(oldPath, newPath string) experimentalsys.Errno {
	if errno := c.check(oldPath, false); errno != 0 {
		return errno
	}
	if errno := c.check(newPath, false); errno != 0 {
		return errno
	}
	return c.FS.Link(oldPath, newPath)
}

// Symlink is refused: a module has no use for a link that the folder's check
// would then refuse to follow.
func (c confinedFS) Symlink(string, string) experimentalsys.Errno { return experimentalsys.EPERM }

func (c confinedFS) Readlink(path string) (string, experimentalsys.Errno) {
	if errno := c.check(path, false); errno != 0 {
		return "", errno
	}
	return c.FS.Readlink(path)
}

func (c confinedFS) Utimens(path string, atim, mtim int64) experimentalsys.Errno {
	if errno := c.check(path, true); errno != 0 {
		return errno
	}
	return c.FS.Utimens(path, atim, mtim)
}
