package update

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// ErrArchive means an update archive has an entry MyGo's updater refuses, or
// isn't laid out the way Sona's install expects.
var ErrArchive = errors.New("update: the update archive can't be installed")

// CheckArchive reads the gzip tar update archive of target (Target's
// "darwin-arm64" or "windows-amd64") and reports the first thing wrong with
// it, without extracting anything. It applies the rules of MyGo's
// ExtractArchive (mygo/internal/update, the same in v0.8.3 and v0.9.0, the
// code the installed apps run), which refuses an archive whole at the first
// entry it dislikes:
//
//   - a name that, cleaned, is "." or isn't a relative path inside the
//     archive: the "./" entry `tar -czf x.tar.gz .` writes first made every
//     Windows update fail with `invalid path in update archive: "./"`;
//   - a symbolic link that is absolute or points out of the archive;
//   - anything but files, directories and symbolic links (hard links and
//     pax global headers, which `git archive` writes, are refused).
//
// and adds what MyGo would only find out on the machine: a name with a
// backslash, a colon or a NUL byte (a path separator or a drive on Windows,
// where MyGo does not look for them), and a symbolic link in the Windows
// archive (creating one needs a privilege a user doesn't have). It also
// checks the layout: the Windows archive holds sona.exe at its top level, and
// the macOS archive holds Sona.app and nothing else.
//
// Every problem is ErrArchive.
func CheckArchive(r io.Reader, target string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrArchive, err)
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	top := map[string]bool{} // top-level names
	var entries int
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("%w: %v", ErrArchive, err)
		}
		entries++
		name := path.Clean(hdr.Name)
		if !fs.ValidPath(name) || name == "." {
			return fmt.Errorf("%w: invalid path in update archive: %q", ErrArchive, hdr.Name)
		}
		if strings.ContainsAny(name, "\\:\x00") {
			return fmt.Errorf("%w: %q has a character that is a path separator or a drive on Windows", ErrArchive, hdr.Name)
		}
		first, _, _ := strings.Cut(name, "/")
		top[first] = true
		switch hdr.Typeflag {
		case tar.TypeDir, tar.TypeReg:
		case tar.TypeSymlink:
			resolved := path.Join(path.Dir(name), hdr.Linkname)
			if path.IsAbs(hdr.Linkname) || !fs.ValidPath(resolved) {
				return fmt.Errorf("%w: link %q points outside the app", ErrArchive, hdr.Name)
			}
			if target == "windows-amd64" {
				return fmt.Errorf("%w: %q is a symbolic link, which Windows can't create without a privilege", ErrArchive, hdr.Name)
			}
		default:
			return fmt.Errorf("%w: unsupported entry in update archive: %q", ErrArchive, hdr.Name)
		}
	}
	if entries == 0 {
		return fmt.Errorf("%w: the archive is empty", ErrArchive)
	}
	switch target {
	case "windows-amd64":
		if !top["sona.exe"] {
			return fmt.Errorf("%w: no sona.exe at the top level (a folder wraps the files?)", ErrArchive)
		}
	case "darwin-arm64":
		if len(top) != 1 || !top["Sona.app"] {
			return fmt.Errorf("%w: the archive must hold Sona.app alone", ErrArchive)
		}
	}
	return nil
}
