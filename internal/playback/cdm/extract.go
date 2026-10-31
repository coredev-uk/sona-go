package cdm

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Bounds on what is taken from the archive.
const (
	maxEntries       = 64       // Google's has 8
	maxLibraryBytes  = 64 << 20 // the library is ~22 MiB
	maxManifestJSON  = 64 << 10
	maxSignatureFile = 64 << 10
)

// manifestFile is the component manifest at the archive's root; it is
// installed next to the library, where cdmhost and LibraryVersion read it.
const manifestFile = "manifest.json"

// componentName is the component manifest's name for the Widevine CDM.
const componentName = "WidevineCdm"

// extract writes the platform's library, its signature file when the
// package has one (macOS and Windows: Widevine's host verification reads
// it) and the component manifest from the archive at a in f into dir, and
// returns the manifest's version. Every entry name is checked: one that is
// absolute, climbs out with "..", uses backslashes or repeats a wanted
// file fails the whole archive, though only the wanted files are written,
// each under its own fixed name.
func extract(f io.ReaderAt, a archive, p Platform, dir string) (string, error) {
	zr, err := zip.NewReader(io.NewSectionReader(f, a.offset, a.size), a.size)
	if err != nil {
		return "", fail(CodeArchive, "archive: %w", err)
	}
	if len(zr.File) > maxEntries {
		return "", fail(CodeArchive, "archive: %d entries", len(zr.File))
	}
	sub := "_platform_specific/" + p.Dir + "/"
	want := map[string]struct {
		limit    int64
		required bool
	}{
		sub + p.Lib:          {maxLibraryBytes, true},
		sub + p.Lib + ".sig": {maxSignatureFile, false},
		manifestFile:         {maxManifestJSON, true},
	}
	found := map[string]bool{}
	for _, zf := range zr.File {
		if err := safeName(zf.Name); err != nil {
			return "", fail(CodeArchive, "archive: %w", err)
		}
		w, ok := want[zf.Name]
		if !ok {
			continue
		}
		if found[zf.Name] {
			return "", fail(CodeArchive, "archive: %s twice", zf.Name)
		}
		found[zf.Name] = true
		if !zf.Mode().IsRegular() {
			return "", fail(CodeArchive, "archive: %s is not a regular file", zf.Name)
		}
		if zf.UncompressedSize64 > uint64(w.limit) {
			return "", fail(CodeTooLarge, "archive: %s is %d bytes, over %d", zf.Name, zf.UncompressedSize64, w.limit)
		}
		mode := os.FileMode(0o644)
		if zf.Name == sub+p.Lib {
			mode = 0o755
		}
		if err := writeEntry(zf, filepath.Join(dir, path.Base(zf.Name)), w.limit, mode); err != nil {
			return "", err
		}
	}
	for name, w := range want {
		if w.required && !found[name] {
			return "", fail(CodeArchive, "archive: no %s", name)
		}
	}
	return readComponentManifest(filepath.Join(dir, manifestFile))
}

// safeName refuses entry names that could leave the extraction directory
// in a naive extractor (zip-slip).
func safeName(name string) error {
	switch {
	case name == "", strings.HasPrefix(name, "/"), strings.Contains(name, "\\"), strings.ContainsRune(name, 0),
		len(name) >= 2 && name[1] == ':':
		return fmt.Errorf("unsafe entry name %q", name)
	}
	for el := range strings.SplitSeq(name, "/") {
		if el == ".." {
			return fmt.Errorf("unsafe entry name %q", name)
		}
	}
	return nil
}

// writeEntry copies one entry, at most limit bytes (whatever its header
// claims), checking its CRC.
func writeEntry(zf *zip.File, dst string, limit int64, mode os.FileMode) (err error) {
	rc, err := zf.Open()
	if err != nil {
		return fail(CodeArchive, "archive: %s: %w", zf.Name, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return &Error{Code: CodeDisk, Err: err}
	}
	defer func() {
		if cerr := out.Close(); err == nil && cerr != nil {
			err = &Error{Code: CodeDisk, Err: cerr}
		}
	}()
	n, err := io.Copy(out, io.LimitReader(rc, limit+1))
	var pe *os.PathError
	switch {
	case errors.As(err, &pe):
		return &Error{Code: CodeDisk, Err: err}
	case err != nil:
		return fail(CodeArchive, "archive: %s: %w", zf.Name, err)
	case n > limit:
		return fail(CodeTooLarge, "archive: %s is over %d bytes", zf.Name, limit)
	}
	return out.Sync()
}

// readComponentManifest returns the version of the Widevine component
// manifest at p.
func readComponentManifest(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", &Error{Code: CodeDisk, Err: err}
	}
	var m struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return "", fail(CodeArchive, "component manifest: %w", err)
	}
	if m.Name != componentName || !versionPattern.MatchString(m.Version) {
		return "", fail(CodeArchive, "component manifest names %q %q", m.Name, m.Version)
	}
	return m.Version, nil
}

// LibraryVersion is the version in the component manifest next to lib
// (Sona's and Firefox's layout) or two levels up (Chrome's
// WidevineCdm/_platform_specific/<platform>/ layout), "" when there is
// none.
func LibraryVersion(lib string) string {
	dir := filepath.Dir(lib)
	for _, m := range []string{filepath.Join(dir, manifestFile), filepath.Join(dir, "..", "..", manifestFile)} {
		if v, err := readComponentManifest(m); err == nil {
			return v
		}
	}
	return ""
}
