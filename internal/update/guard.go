package update

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The rollback guard. MyGo's updater swaps the app and relaunches; it keeps
// nothing to fall back on. Sona keeps the version it replaced beside the new
// one (Sona.app.previous on macOS, previous\ on Windows) and writes
// update-pending in the state folder before relaunching. The new version
// clears the file at its first paint. A start that finds the file already
// counted MaxStarts times never got that far, so it restores the previous
// version (the CDM's rule of 3 failed starts). The same kept version is what
// the user can go back to by choice (RestorePrevious, Settings → Diagnostics).

// MaxStarts is how many starts an update gets to reach its first paint.
const MaxStarts = 3

const (
	pendingFile  = "update-pending"
	rollbackFile = "update-rollback.json"
	// previousFile holds the version number of the snapshot where the
	// install has no bundle to read it from (Windows).
	previousFile = "update-previous"
)

// Pending is the update-pending file.
type Pending struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Starts int    `json:"starts"` // starts of To that haven't reached a first paint
}

// Action is what a start does about a pending update.
type Action int

const (
	// Normal: nothing pending, or a stale file (another version runs).
	Normal Action = iota
	// Counted: the update is on trial; this start was counted.
	Counted
	// Restore: the update failed MaxStarts starts; restore the previous
	// version.
	Restore
)

// Decide is the rollback rule for a start of version current.
func Decide(p *Pending, current string) Action {
	switch {
	case p == nil, p.To != current:
		return Normal
	case p.Starts >= MaxStarts:
		return Restore
	}
	return Counted
}

// Rollback reports the last restore, for Diagnostics.
type Rollback struct {
	From string    `json:"from"` // the version that was left: the one that failed to start, or the one the user went back from
	To   string    `json:"to"`   // the version restored ("" when unknown)
	At   time.Time `json:"at"`
	// Manual: the user asked for it (Settings → Diagnostics → Previous
	// version), where false is the guard undoing an update that never
	// reached its first paint.
	Manual bool `json:"manual,omitempty"`
	// Error is why the restore itself failed ("" when it worked).
	Error string `json:"error,omitempty"`
}

// Guard keeps the previous version and the pending marker of one install.
type Guard struct {
	// State is the state folder holding update-pending.
	State string
	// Root is what an update replaces: the .app bundle on macOS, the
	// directory of the executable on Windows.
	Root string
	// Previous is where the replaced version waits.
	Previous string
	// GOOS selects the layout: "darwin" swaps whole directories, anything
	// else swaps the entries of Root.
	GOOS string
}

// NewGuard locates the install an update replaces from the running
// executable. ok is false where Sona doesn't update itself: not an .app
// bundle on macOS, or an OS other than macOS and Windows.
func NewGuard(state, exe, goos string) (Guard, bool) {
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	switch goos {
	case "darwin":
		root := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
		if !strings.HasSuffix(root, ".app") {
			return Guard{}, false
		}
		return Guard{State: state, Root: root, Previous: root + ".previous", GOOS: goos}, true
	case "windows":
		root := filepath.Dir(exe)
		return Guard{State: state, Root: root, Previous: filepath.Join(root, "previous"), GOOS: goos}, true
	}
	return Guard{}, false
}

// Snapshot keeps the running version, which is version, as Previous,
// replacing an older snapshot. Files are hard-linked where the volume allows
// (the new version is unpacked elsewhere and swapped in, so the links are
// never written through) and copied otherwise. Where the install has no
// Info.plist to say which version the snapshot is (Windows), the number is
// kept in the state folder.
func (g Guard) Snapshot(version string) error {
	if err := os.RemoveAll(g.Previous); err != nil {
		return err
	}
	g.forgetVersion()
	if g.GOOS == "darwin" {
		return copyTree(g.Root, g.Previous, nil)
	}
	skip := func(rel string) bool {
		return rel == "previous" || (strings.HasPrefix(rel, ".") && strings.Contains(rel, ".old-"))
	}
	if err := copyTree(g.Root, g.Previous, skip); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(g.State, previousFile), []byte(version), 0o600)
}

// HasPrevious reports whether a previous version is kept, and which one:
// the CFBundleShortVersionString of Previous's Info.plist on macOS, the
// number Snapshot recorded elsewhere. version is "" when it can't be read
// (a snapshot made before Sona recorded it).
func (g Guard) HasPrevious() (version string, ok bool) {
	if fi, err := os.Stat(g.Previous); err != nil || !fi.IsDir() {
		return "", false
	}
	if g.GOOS == "darwin" {
		return bundleVersion(filepath.Join(g.Previous, "Contents", "Info.plist")), true
	}
	b, _ := os.ReadFile(filepath.Join(g.State, previousFile))
	return strings.TrimSpace(string(b)), true
}

func (g Guard) forgetVersion() { _ = os.Remove(filepath.Join(g.State, previousFile)) }

// bundleVersion reads CFBundleShortVersionString from an XML Info.plist;
// "" when the file is missing, binary or lacks the key.
func bundleVersion(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	d := xml.NewDecoder(f)
	var elem, key string
	for {
		tok, err := d.Token()
		if err != nil {
			return ""
		}
		switch t := tok.(type) {
		case xml.StartElement:
			elem = t.Name.Local
		case xml.EndElement:
			elem = ""
		case xml.CharData:
			switch {
			case elem == "key":
				key = string(t)
			case elem == "string" && key == "CFBundleShortVersionString":
				return strings.TrimSpace(string(t))
			}
		}
	}
}

// Mark records that the update from → to is about to start.
func (g Guard) Mark(from, to string) error {
	return g.writePending(Pending{From: from, To: to})
}

func (g Guard) writePending(p Pending) error {
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(g.State, pendingFile), b, 0o600)
}

// Unmark drops the pending marker (an install that failed before swapping,
// or a first paint).
func (g Guard) Unmark() error {
	err := os.Remove(filepath.Join(g.State, pendingFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Pending reads the pending marker; nil when none.
func (g Guard) Pending() *Pending {
	b, err := os.ReadFile(filepath.Join(g.State, pendingFile))
	if err != nil {
		return nil
	}
	var p Pending
	if json.Unmarshal(b, &p) != nil {
		return nil
	}
	return &p
}

// Begin runs at start of version current, before anything else. It counts
// the start of an update on trial. With Restore the caller calls Rollback.
func (g Guard) Begin(current string) (Action, error) {
	p := g.Pending()
	a := Decide(p, current)
	switch a {
	case Counted:
		p.Starts++
		return a, g.writePending(*p)
	case Normal:
		if p != nil { // the marker of an update that isn't running: stale
			return a, g.Unmark()
		}
	}
	return a, nil
}

// Rollback puts Previous back in place of the failed version, clears the
// marker and records the restore (LastRollback). The caller relaunches.
func (g Guard) Rollback() error {
	p := g.Pending()
	rb := Rollback{At: time.Now().UTC()}
	if p != nil {
		rb.From, rb.To = p.To, p.From
	}
	return g.restoreAndRecord(rb)
}

// RestorePrevious puts Previous back in place of the running version, which
// is current, because the user asked for it. It clears any marker and
// records the restore as a manual one (LastRollback). The running process
// goes on from its open files; the caller restarts it (the install's own
// restart, which starts whatever is installed at Root).
func (g Guard) RestorePrevious(current string) error {
	prev, _ := g.HasPrevious()
	return g.restoreAndRecord(Rollback{From: current, To: prev, Manual: true, At: time.Now().UTC()})
}

// restoreAndRecord is a restore that is written down whatever happens.
func (g Guard) restoreAndRecord(rb Rollback) error {
	err := restore(g.GOOS, g.Root, g.Previous)
	if err != nil {
		rb.Error = err.Error()
	} else {
		g.forgetVersion()
	}
	// Clear the marker whatever happened: a restore that can't work must
	// not loop on every start.
	_ = g.Unmark()
	if b, merr := json.Marshal(rb); merr == nil {
		_ = os.WriteFile(filepath.Join(g.State, rollbackFile), b, 0o600)
	}
	return err
}

// LastRollback returns the last restore; nil when none happened.
func (g Guard) LastRollback() *Rollback {
	b, err := os.ReadFile(filepath.Join(g.State, rollbackFile))
	if err != nil {
		return nil
	}
	var r Rollback
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	return &r
}

func restore(goos, root, previous string) error {
	if _, err := os.Stat(previous); err != nil {
		return fmt.Errorf("update: no previous version to restore: %w", err)
	}
	if goos == "darwin" {
		failed := root + ".failed"
		if err := os.RemoveAll(failed); err != nil {
			return err
		}
		if err := os.Rename(root, failed); err != nil {
			return err
		}
		if err := os.Rename(previous, root); err != nil {
			_ = os.Rename(failed, root)
			return err
		}
		_ = os.RemoveAll(failed)
		return nil
	}
	entries, err := os.ReadDir(previous)
	if err != nil {
		return err
	}
	// The running executable is renamed away, which Windows allows; MyGo's
	// start removes the ".<name>.old-<pid>" files it finds next time.
	suffix := ".old-" + strconv.Itoa(os.Getpid())
	for _, e := range entries {
		dst := filepath.Join(root, e.Name())
		if _, err := os.Lstat(dst); err == nil {
			if err := os.Rename(dst, filepath.Join(root, "."+e.Name()+suffix)); err != nil {
				return err
			}
		}
		if err := os.Rename(filepath.Join(previous, e.Name()), dst); err != nil {
			return err
		}
	}
	return os.RemoveAll(previous)
}

// Relaunch starts the install at root again (after a rollback): through
// Launch Services on macOS, the executable on Windows.
func Relaunch(goos, root, exe string) error {
	if goos == "darwin" {
		return exec.Command("/usr/bin/open", "-n", root).Run()
	}
	cmd := exec.Command(filepath.Join(root, filepath.Base(exe)), os.Args[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// copyTree copies src to dst (a directory), hard-linking regular files and
// copying where linking fails; skip, when set, excludes top-level names.
func copyTree(src, dst string, skip func(rel string) bool) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel != "." && skip != nil && skip(filepath.ToSlash(rel)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		to := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(to, info.Mode().Perm()|0o700)
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, to)
		case info.Mode().IsRegular():
			if os.Link(p, to) == nil {
				return nil
			}
			return copyFile(p, to, info.Mode().Perm())
		}
		return nil
	})
}

func copyFile(src, dst string, mode fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
