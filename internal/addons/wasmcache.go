package addons

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/tetratelabs/wazero"
)

// MaxWasmCacheBytes bounds the folder of compiled modules. wazero never
// prunes it, and every version of every module adds an entry of several
// times the module's size; past the bound the whole folder is dropped and
// rebuilt as modules start (compiling is a second or two, once).
const MaxWasmCacheBytes = 256 << 20

// openCache opens the on-disk compilation cache in dir, which holds machine
// code wazero loads and Sona then runs. wazero keys each entry by the hash
// of the module and of its own version, and only checks a CRC on load, so
// whoever can write the folder can run code in Sona. So:
//
//   - the folder is Sona's private cache folder (0700), outside every
//     add-on's reach, the sandbox's data folder included;
//   - an entry or folder that isn't the user's own, or that others can write,
//     is not used: the whole cache is dropped and started again;
//   - it is kept to MaxWasmCacheBytes.
//
// An error means the cache can't be used: the host keeps compiled code in
// memory instead and says why.
func openCache(dir string) (wazero.CompilationCache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("cache folder: %w", err)
	}
	if err := vetCache(dir); err != nil {
		// Not trusted: start again rather than load it.
		if rerr := os.RemoveAll(dir); rerr != nil {
			return nil, fmt.Errorf("cache folder is unsafe (%v) and can't be cleared: %w", err, rerr)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("cache folder: %w", err)
		}
		if err := vetCache(dir); err != nil {
			return nil, fmt.Errorf("cache folder: %w", err)
		}
	}
	c, err := wazero.NewCompilationCacheWithDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cache folder: %w", err)
	}
	return c, nil
}

// vetCache checks every folder and file in the cache and its size. The
// folder is walked without following links.
func vetCache(dir string) error {
	var total int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			if err := checkDirSafe(path); err != nil {
				return err
			}
		case fi.Mode().IsRegular():
			if err := checkFileSafe(fi); err != nil {
				return err
			}
			total += fi.Size()
		default:
			return fmt.Errorf("%s is neither a file nor a folder", path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if total > MaxWasmCacheBytes {
		return errors.New("the cache is over its size")
	}
	return nil
}
