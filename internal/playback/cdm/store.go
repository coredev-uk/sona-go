package cdm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// The install directory (config.Paths.WidevineDir):
//
//	current.json                         the pointer (below)
//	<version>/<platform dir>/<library>   one directory per kept version,
//	<version>/<platform dir>/manifest.json   with the library's manifest
//	.lock                                held while installing
//	.staging-*, .download-*.crx3         an install in progress
//
// The pointer is replaced atomically (written beside, then renamed), so a
// version is current only once it is complete and verified; no symlinks,
// so the layout works on Windows.
const (
	pointerFile   = "current.json"
	lockFile      = ".lock"
	stagingPrefix = ".staging-"
	partPrefix    = ".download-"
)

// pointer is current.json.
type pointer struct {
	// Platform is the platform directory the versions were installed
	// for; a pointer for another platform (a data directory copied from
	// another computer) counts as nothing installed.
	Platform string `json:"platform"`
	// Current is the version new playback hosts load; Previous the one it
	// replaced, kept for rollback.
	Current  string `json:"current,omitempty"`
	Previous string `json:"previous,omitempty"`
	// CheckedAt is the last update check that read the manifest.
	CheckedAt time.Time `json:"checkedAt,omitzero"`
	// Failures counts playback hosts in a row that failed to load
	// Current (see Manager.ReportInit).
	Failures int `json:"failures,omitempty"`
	// Rejected are versions that failed to load, with the failure code;
	// they are not installed again.
	Rejected map[string]string `json:"rejected,omitempty"`
	// SHA256 is each kept version's library hash (hex), recorded when it
	// was installed: a library that no longer matches it was damaged on
	// disk (repair).
	SHA256 map[string]string `json:"sha256,omitempty"`
}

// store is one platform's view of the install directory.
type store struct {
	root string
	p    Platform
}

func (s store) dir(version string) string { return filepath.Join(s.root, version, s.p.Dir) }
func (s store) lib(version string) string { return filepath.Join(s.dir(version), s.p.Lib) }

// installed reports whether version's library is in place.
func (s store) installed(version string) bool {
	if version == "" {
		return false
	}
	st, err := os.Stat(s.lib(version))
	return err == nil && st.Mode().IsRegular()
}

// hash returns the SHA-256 (hex) of version's library.
func (s store) hash(version string) (string, error) {
	f, err := os.Open(s.lib(version))
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// intact checks that version's library is still the one installed: in
// place and with the recorded hash.
func (s store) intact(ptr pointer, version string) error {
	want := ptr.SHA256[version]
	if want == "" {
		return errors.New("no hash recorded at install")
	}
	got, err := s.hash(version)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("library SHA-256 is %s, installed as %s", got, want)
	}
	return nil
}

// repair drops the kept versions whose library is missing or damaged
// (intact fails: truncated, overwritten, half deleted), removing their
// directories. A damaged Current gives way to an intact Previous, else
// to nothing, so the next check downloads it again; it is not rejected,
// since the package itself was fine. It reports whether ptr changed.
// Callers hold the lock.
func (s store) repair(ptr *pointer, log *slog.Logger) bool {
	changed := false
	for _, v := range []*string{&ptr.Current, &ptr.Previous} {
		if *v == "" {
			continue
		}
		if err := s.intact(*ptr, *v); err != nil {
			log.Warn("widevine: installed CDM is missing or damaged; removing it", "version", *v, "err", err)
			_ = os.RemoveAll(filepath.Join(s.root, *v))
			delete(ptr.SHA256, *v)
			*v = ""
			changed = true
		}
	}
	if ptr.Current == "" && ptr.Previous != "" {
		ptr.Current, ptr.Previous, ptr.Failures = ptr.Previous, "", 0
		log.Warn("widevine: back to the previous version", "version", ptr.Current)
	}
	if ptr.Current == "" {
		ptr.Failures = 0
	}
	return changed
}

// lock takes the install directory's lock (.lock), creating the
// directory on first use and waiting for another process installing
// there; unlock releases it.
func (s store) lock() (unlock func(), err error) {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return nil, &Error{Code: CodeDisk, Err: err}
	}
	return lockPath(filepath.Join(s.root, lockFile))
}

// read returns the pointer; a missing one, or one for another platform,
// is the zero pointer.
func (s store) read() (pointer, error) {
	b, err := os.ReadFile(filepath.Join(s.root, pointerFile))
	if errors.Is(err, fs.ErrNotExist) {
		return pointer{Platform: s.p.Dir}, nil
	}
	if err != nil {
		return pointer{Platform: s.p.Dir}, &Error{Code: CodeDisk, Err: err}
	}
	var ptr pointer
	if err := json.Unmarshal(b, &ptr); err != nil || ptr.Platform != s.p.Dir {
		// Unreadable or foreign: start over; prune clears the rest.
		return pointer{Platform: s.p.Dir}, nil
	}
	for _, v := range []*string{&ptr.Current, &ptr.Previous} {
		if !versionPattern.MatchString(*v) {
			*v = ""
		}
	}
	return ptr, nil
}

// write replaces the pointer atomically.
func (s store) write(ptr pointer) error {
	ptr.Platform = s.p.Dir
	b, err := json.MarshalIndent(ptr, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, pointerFile+".*")
	if err != nil {
		return &Error{Code: CodeDisk, Err: err}
	}
	_, werr := tmp.Write(append(b, '\n'))
	if werr == nil {
		werr = tmp.Sync()
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp.Name(), filepath.Join(s.root, pointerFile))
	}
	if werr != nil {
		_ = os.Remove(tmp.Name())
		return &Error{Code: CodeDisk, Err: werr}
	}
	syncDir(s.root)
	return nil
}

// syncDir flushes a directory's entries (a rename) where the OS allows
// opening a directory for it.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// prune removes every version directory but keep's, and leftovers of
// interrupted installs. A version a running playback host still has
// loaded is never in danger: the host loaded Current or Previous, both
// kept, and removing a loaded library is harmless on Linux and macOS and
// refused (then retried at the next prune) on Windows. Callers hold the
// lock, so no pointer write is in flight.
func (s store) prune(keep ...string) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case name == pointerFile, name == lockFile:
			continue
		case e.IsDir() && slices.Contains(keep, name) && name != "":
			continue
		}
		_ = os.RemoveAll(filepath.Join(s.root, name))
	}
}

// place moves a verified staging directory into version's place,
// replacing a leftover there.
func (s store) place(staging, version string) error {
	if err := os.MkdirAll(filepath.Join(s.root, version), 0o700); err != nil {
		return &Error{Code: CodeDisk, Err: err}
	}
	dst := s.dir(version)
	if err := os.RemoveAll(dst); err != nil {
		return &Error{Code: CodeDisk, Err: err}
	}
	if err := os.Rename(staging, dst); err != nil {
		return &Error{Code: CodeDisk, Err: err}
	}
	syncDir(filepath.Join(s.root, version))
	return nil
}
