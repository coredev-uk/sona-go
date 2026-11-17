package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// ent is one entry of a test archive.
type ent struct {
	name string
	typ  byte
	link string
}

func reg(name string) ent     { return ent{name: name, typ: tar.TypeReg} }
func dir(name string) ent     { return ent{name: name, typ: tar.TypeDir} }
func sym(name, to string) ent { return ent{name: name, typ: tar.TypeSymlink, link: to} }

func tarGz(t *testing.T, ents ...ent) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range ents {
		h := &tar.Header{Name: e.name, Typeflag: e.typ, Mode: 0o755, Linkname: e.link}
		switch e.typ {
		case tar.TypeReg:
			h.Size = 1
		case tar.TypeXGlobalHeader:
			h = &tar.Header{Name: "pax_global_header", Typeflag: e.typ, PAXRecords: map[string]string{"comment": "x"}}
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := tw.Write([]byte("x")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestCheckArchiveFollowsMyGosExtractor pins CheckArchive to what MyGo's
// ExtractArchive (mygo/internal/update, identical in v0.8.3, which the
// released apps run, to v0.9.2) does with the same entries: each verdict of
// ok below was observed by running that function on the archive. The first
// case is the archive every Windows release shipped until the update-archive
// script: its "./" entry fails the whole update.
func TestCheckArchiveFollowsMyGosExtractor(t *testing.T) {
	for name, c := range map[string]struct {
		ents []ent
		ok   bool
	}{
		"the ./ entry tar . writes":     {[]ent{dir("./"), reg("./sona.exe")}, false},
		"the . entry":                   {[]ent{dir("."), reg("sona.exe")}, false},
		"./ names without the root one": {[]ent{reg("./sona.exe"), dir("./lib/"), reg("./lib/a.dll")}, true},
		"plain names":                   {[]ent{reg("sona.exe"), dir("lib/"), reg("lib/a.dll")}, true},
		"files without their folders":   {[]ent{reg("lib/gst/a.dll"), reg("sona.exe")}, true},
		"a dotfile":                     {[]ent{reg(".mygo-files"), reg("sona.exe")}, true},
		"a hidden folder":               {[]ent{dir(".hidden/"), reg(".hidden/a"), reg("sona.exe")}, true},
		"a . inside a name":             {[]ent{reg("a/./b"), reg("sona.exe")}, true},
		"a doubled slash":               {[]ent{reg("a//b"), reg("sona.exe")}, true},
		"parent folder":                 {[]ent{reg("../evil"), reg("sona.exe")}, false},
		"parent folder inside":          {[]ent{reg("a/../../evil"), reg("sona.exe")}, false},
		"a folder named ..":             {[]ent{dir("a/.."), reg("sona.exe")}, false},
		"absolute path":                 {[]ent{reg("/etc/evil"), reg("sona.exe")}, false},
		"a link within the archive":     {[]ent{reg("sona.exe"), sym("l", "sona.exe"), sym("d/l", "../sona.exe")}, true},
		"an absolute link":              {[]ent{reg("sona.exe"), sym("l", "/etc/passwd")}, false},
		"a link out":                    {[]ent{reg("sona.exe"), sym("l", "../x")}, false},
		"a nested link out":             {[]ent{reg("sona.exe"), sym("d/l", "../../x")}, false},
		"a hard link":                   {[]ent{reg("sona.exe"), {name: "h", typ: tar.TypeLink, link: "sona.exe"}}, false},
		"a pax global header":           {[]ent{{typ: tar.TypeXGlobalHeader}, reg("sona.exe")}, false},
		"a device":                      {[]ent{{name: "c", typ: tar.TypeChar}, reg("sona.exe")}, false},
		"a fifo":                        {[]ent{{name: "f", typ: tar.TypeFifo}, reg("sona.exe")}, false},
		"bad entry after good ones":     {[]ent{reg("sona.exe"), dir("lib/"), dir("./")}, false},
	} {
		t.Run(name, func(t *testing.T) {
			err := CheckArchive(bytes.NewReader(tarGz(t, c.ents...)), "")
			if c.ok != (err == nil) {
				t.Fatalf("CheckArchive = %v, want ok %v", err, c.ok)
			}
			if err != nil && !errors.Is(err, ErrArchive) {
				t.Errorf("err = %v, not ErrArchive", err)
			}
		})
	}
}

// What MyGo's extractor takes on a Unix machine and Windows would read
// differently, or can't do, is refused here (the one place CheckArchive is
// stricter than MyGo).
func TestCheckArchiveRefusesWhatWindowsReadsDifferently(t *testing.T) {
	for name, ents := range map[string][]ent{
		"a backslash":        {reg("a\\b"), reg("sona.exe")},
		"a backslash parent": {reg("..\\evil"), reg("sona.exe")},
		"a drive":            {reg("C:evil"), reg("sona.exe")},
		"a symbolic link":    {reg("sona.exe"), sym("l", "sona.exe")},
	} {
		t.Run(name, func(t *testing.T) {
			if err := CheckArchive(bytes.NewReader(tarGz(t, ents...)), "windows-amd64"); !errors.Is(err, ErrArchive) {
				t.Fatalf("CheckArchive = %v", err)
			}
		})
	}
}

func TestCheckArchiveLayout(t *testing.T) {
	for _, c := range []struct {
		name, target string
		ents         []ent
		ok           bool
	}{
		{"windows with sona.exe", "windows-amd64", []ent{reg("sona.exe"), reg("lib/a.dll")}, true},
		{"windows without sona.exe", "windows-amd64", []ent{reg("lib/a.dll")}, false},
		{"windows wrapped in a folder", "windows-amd64", []ent{dir("Sona/"), reg("Sona/sona.exe")}, false},
		{"mac with Sona.app alone", "darwin-arm64", []ent{dir("Sona.app/"), reg("Sona.app/Contents/Info.plist")}, true},
		{"mac with something beside", "darwin-arm64", []ent{dir("Sona.app/"), reg("README")}, false},
		{"mac without Sona.app", "darwin-arm64", []ent{reg("sona")}, false},
		{"an empty archive", "windows-amd64", nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := CheckArchive(bytes.NewReader(tarGz(t, c.ents...)), c.target)
			if c.ok != (err == nil) {
				t.Fatalf("CheckArchive = %v, want ok %v", err, c.ok)
			}
		})
	}
	if err := CheckArchive(bytes.NewReader([]byte("not a gzip")), "windows-amd64"); !errors.Is(err, ErrArchive) {
		t.Errorf("garbage: %v", err)
	}
}

// TestWindowsUpdateArchiveScript runs build/windows/update-archive.sh, which
// package.sh makes the Windows archive with, on a folder laid out like the
// package, and has CheckArchive judge the result: the archive passes, and
// the `tar -czf OUT .` the script replaced does not.
func TestWindowsUpdateArchiveScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the script runs in Git Bash on the Windows build machine")
	}
	for _, tool := range []string{"bash", "tar", "ls"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("no %s", tool)
		}
	}
	root := t.TempDir()
	for _, f := range []string{
		"sona.exe", "sona.ico", "sona_cdm_shim.dll", "libgstreamer-1.0-0.dll", ".hidden",
		"lib/gstreamer-1.0/libgstcoreelements.dll", "libexec/gstreamer-1.0/gst-plugin-scanner.exe", "licenses/LICENSE.txt",
	} {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	check := func(t *testing.T, archive string) error {
		t.Helper()
		f, err := os.Open(archive)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		return CheckArchive(f, "windows-amd64")
	}

	out := filepath.Join(t.TempDir(), "update.tar.gz")
	script := filepath.Join("..", "..", "build", "windows", "update-archive.sh")
	if b, err := exec.Command("bash", script, root, out).CombinedOutput(); err != nil {
		t.Fatalf("update-archive.sh: %v\n%s", err, b)
	}
	if err := check(t, out); err != nil {
		t.Fatalf("the script's archive: %v", err)
	}
	if b, err := exec.Command("tar", "-tzf", out).Output(); err != nil || !bytes.Contains(b, []byte(".hidden\n")) {
		t.Errorf("the archive lacks the dotfile: %v\n%s", err, b)
	}

	old := filepath.Join(t.TempDir(), "old.tar.gz")
	if b, err := exec.Command("tar", "-C", root, "-czf", old, ".").CombinedOutput(); err != nil {
		t.Fatalf("tar: %v\n%s", err, b)
	}
	if err := check(t, old); !errors.Is(err, ErrArchive) {
		t.Fatalf("an archive of `tar -czf OUT .` passed: %v", err)
	}
}
