package addons

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// Entry is one folder of the add-ons directory: the add-on it holds, or why
// it is refused.
type Entry struct {
	// Dir is the folder's name, and ID the manifest's ID when it could be
	// read ("" otherwise); Manifest is valid only when Problem is nil.
	Dir      string
	ID       string
	Manifest Manifest
	// Exe is the add-on's executable: an absolute path with every link in
	// it resolved, inside the folder. Empty when Problem is set.
	Exe     string
	Problem *Problem
	// Bundled is one of Sona's own add-ons (bundled.go): registered in code,
	// never read from a folder. Exe is then Sona's executable and Args are
	// what make it run the add-on.
	Bundled bool
	Args    []string
}

// Name is how the entry is called in a list: the manifest's ID, else the
// folder's name.
func (e Entry) Name() string {
	if e.ID != "" {
		return e.ID
	}
	return e.Dir
}

// ScanOptions tune Scan. There are none: no option lets a folder hold one of
// the IDs reserved for Sona's own add-ons, which are registered in code
// (bundled.go) and never read from a folder.
type ScanOptions struct{}

// Scan lists root's add-ons, the valid and the refused, sorted by folder
// name. A root that doesn't exist holds none. At most config.MaxAddons
// folders are read; the rest are one refused entry. Files and folders
// whose name starts with a dot are ignored.
func Scan(root string, opts ScanOptions) ([]Entry, error) {
	// The root itself may be a link (a dotfiles setup); what it points to
	// must still be the user's own.
	fi, err := os.Stat(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("addons: %w", err)
	}
	if !fi.IsDir() {
		return nil, problem(ProblemUnsafe, "%s is not a folder", root)
	}
	if err := checkFileSafe(fi); err != nil {
		return nil, err
	}
	des, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("addons: read %s: %w", root, err)
	}
	var out []Entry
	for _, de := range des {
		name := de.Name()
		if strings.HasPrefix(name, ".") || de.Type().IsRegular() {
			continue
		}
		if len(out) >= MaxScanned {
			out = append(out, Entry{Dir: "", Problem: problem(ProblemTooMany, "more than %d folders", MaxScanned)})
			break
		}
		out = append(out, ScanOne(filepath.Join(root, name), opts))
	}
	return out, nil
}

// MaxScanned bounds the folders Scan reads.
const MaxScanned = 100

// ScanOne checks the add-on in one folder.
func ScanOne(dir string, opts ScanOptions) Entry {
	e := Entry{Dir: filepath.Base(dir)}
	fail := func(p *Problem) Entry { e.Problem = p; return e }
	if err := checkDirSafe(dir); err != nil {
		return fail(asProblem(err))
	}
	data, err := readManifest(filepath.Join(dir, ManifestName))
	if err != nil {
		return fail(asProblem(err))
	}
	m, err := ParseManifest(data)
	if err != nil {
		// Keep the ID when the text names a plausible one, so a list can say
		// which add-on is broken.
		var raw struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(data, &raw) == nil && idExpr.MatchString(raw.ID) {
			e.ID = raw.ID
		}
		return fail(asProblem(err))
	}
	e.ID = m.ID
	if !strings.EqualFold(m.ID, e.Dir) {
		return fail(problem(ProblemIDMismatch, "id %q in folder %q", m.ID, e.Dir))
	}
	if Reserved(m.ID) {
		return fail(problem(ProblemReservedID, "%q", m.ID))
	}
	resolve := ResolveExecutable
	if m.Wasm() {
		resolve = ResolveModule
	}
	exe, err := resolve(dir, m.Command)
	if err != nil {
		return fail(asProblem(err))
	}
	e.Manifest, e.Exe = m, exe
	return e
}

func asProblem(err error) *Problem {
	var p *Problem
	if errors.As(err, &p) {
		return p
	}
	return problem(ProblemUnreadable, "%v", err)
}

// readManifest reads the manifest file: a regular file of the user's that no
// one else may write, at most MaxManifestBytes (one more is read, so a
// larger file is refused, not cut).
func readManifest(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, problem(ProblemUnreadable, "%v", err)
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		return nil, problem(ProblemUnreadable, "%v", err)
	}
	if !fi.Mode().IsRegular() {
		return nil, problem(ProblemUnreadable, "%s is not a regular file", ManifestName)
	}
	if err := checkFileSafe(fi); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxManifestBytes+1))
	if err != nil {
		return nil, problem(ProblemUnreadable, "%v", err)
	}
	return data, nil
}

// ResolveExecutable returns the absolute, link-free path of command inside
// dir, once every rule holds: the path stays inside the folder after links
// are resolved, it is a regular file, on Unix an executable one, and neither
// it nor any folder on the way is another user's or writable by group or
// others. The caller runs exactly this path: no PATH or working-directory
// look-up.
func ResolveExecutable(dir, command string) (string, error) {
	return resolveFile(dir, command, true)
}

// ResolveModule is ResolveExecutable for a WebAssembly module: the same
// rules, except that the file is data Sona reads, so it needs no execute bit.
func ResolveModule(dir, command string) (string, error) {
	return resolveFile(dir, command, false)
}

func resolveFile(dir, command string, executable bool) (string, error) {
	if err := checkCommand(command); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", problem(ProblemUnreadable, "%v", err)
	}
	base, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", problem(ProblemUnreadable, "%v", err)
	}
	exe, err := filepath.EvalSymlinks(filepath.Join(base, filepath.FromSlash(command)))
	if err != nil {
		return "", problem(ProblemNoExecutable, "%v", err)
	}
	rel, err := filepath.Rel(base, exe)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", problem(ProblemEscapes, "%s resolves outside the folder", command)
	}
	fi, err := os.Lstat(exe)
	if err != nil {
		return "", problem(ProblemNoExecutable, "%v", err)
	}
	if !fi.Mode().IsRegular() {
		return "", problem(ProblemNoExecutable, "%s is not a regular file", command)
	}
	if executable && runtime.GOOS != "windows" && fi.Mode().Perm()&0o111 == 0 {
		return "", problem(ProblemNoExecutable, "%s is not executable", command)
	}
	if err := checkFileSafe(fi); err != nil {
		return "", err
	}
	// Every folder between the add-on's and the executable.
	parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
	cur := base
	for _, p := range slices.Concat([]string{"."}, parts) {
		cur = filepath.Join(cur, p)
		if err := checkDirSafe(cur); err != nil {
			return "", err
		}
	}
	return exe, nil
}
