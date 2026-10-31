package cdm

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"testing"
)

// extractInto extracts a ZIP of entries for p into a fresh directory
// inside its own parent, returning both.
func extractInto(t *testing.T, p Platform, entries []zipEntry) (version, dir, parent string, err error) {
	t.Helper()
	zip := buildZip(t, entries)
	parent = t.TempDir()
	dir = filepath.Join(parent, "out")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	version, err = extract(bytes.NewReader(zip), archive{size: int64(len(zip))}, p, dir)
	return version, dir, parent, err
}

func TestExtractWritesOnlyTheWantedFiles(t *testing.T) {
	for _, p := range platforms {
		t.Run(p.String(), func(t *testing.T) {
			other := platform(t, "linux", "arm64")
			if p.Dir == other.Dir {
				other = linuxAMD64(t)
			}
			entries := append(packageEntries(p, "4.10.3112.0"),
				zipEntry{name: libEntry(other), body: fakeLibrary(other)},
				zipEntry{name: "_platform_specific/" + p.Dir + "/README", body: []byte("x")},
				zipEntry{name: "_platform_specific/" + p.Dir + "/sub/" + p.Lib, body: []byte("x")},
			)
			version, dir, parent, err := extractInto(t, p, entries)
			if err != nil {
				t.Fatal(err)
			}
			if version != "4.10.3112.0" {
				t.Errorf("version = %s", version)
			}
			if got, want := dirNames(t, dir), []string{manifestFile, p.Lib, p.Lib + ".sig"}; !slices.Equal(got, sortedCopy(want)) {
				t.Errorf("extracted %q, want %q", got, sortedCopy(want))
			}
			if got := dirNames(t, parent); !slices.Equal(got, []string{"out"}) {
				t.Errorf("parent holds %q", got)
			}
			lib, err := os.ReadFile(filepath.Join(dir, p.Lib))
			if err != nil || !bytes.Equal(lib, fakeLibrary(p)) {
				t.Fatalf("library = %d bytes, %v", len(lib), err)
			}
			if err := checkLibrary(filepath.Join(dir, p.Lib), p); err != nil {
				t.Error(err)
			}
			if runtime.GOOS != "windows" {
				st, err := os.Stat(filepath.Join(dir, p.Lib))
				if err != nil || st.Mode().Perm() != 0o755 {
					t.Errorf("library mode = %v, %v", st.Mode(), err)
				}
			}
		})
	}
}

func sortedCopy(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

func TestExtractWithoutSignatureFile(t *testing.T) {
	p := linuxAMD64(t)
	_, dir, _, err := extractInto(t, p, without(packageEntries(p, "1.2.3.4"), libEntry(p)+".sig"))
	if err != nil {
		t.Fatal(err)
	}
	if got := dirNames(t, dir); !slices.Equal(got, []string{p.Lib, manifestFile}) {
		t.Errorf("extracted %q", got)
	}
}

func TestExtractRefusesUnsafeNames(t *testing.T) {
	p := linuxAMD64(t)
	for _, name := range []string{
		"../evil",
		"../out/evil",
		"/abs",
		"/" + libEntry(p),
		`a\b`,
		`..\evil`,
		"_platform_specific/../../evil",
		"_platform_specific/" + p.Dir + "/../../../evil",
		"C:/evil",
		"c:evil",
		"evil\x00.so",
		"",
	} {
		for _, first := range []bool{true, false} {
			t.Run(strconv.Quote(name)+"/first="+strconv.FormatBool(first), func(t *testing.T) {
				bad := zipEntry{name: name, body: []byte("evil")}
				entries := packageEntries(p, "1.2.3.4")
				if first {
					entries = append([]zipEntry{bad}, entries...)
				} else {
					entries = append(entries, bad)
				}
				_, _, parent, err := extractInto(t, p, entries)
				wantCode(t, err, CodeArchive)
				if got := dirNames(t, parent); !slices.Equal(got, []string{"out"}) {
					t.Errorf("parent holds %q", got)
				}
			})
		}
	}
}

func TestExtractRefusesBadArchives(t *testing.T) {
	p := linuxAMD64(t)
	good := packageEntries(p, "1.2.3.4")
	lib := libEntry(p)
	many := slices.Clone(good)
	for i := range maxEntries {
		many = append(many, zipEntry{name: "_metadata/" + strconv.Itoa(i), body: []byte("x")})
	}
	tests := []struct {
		name    string
		entries []zipEntry
		code    string
	}{
		{"duplicate library", append(slices.Clone(good), zipEntry{name: lib, body: fakeLibrary(p)}), CodeArchive},
		{"duplicate manifest", append(slices.Clone(good), zipEntry{name: manifestFile, body: componentManifestJSON(componentName, "9.9.9.9")}), CodeArchive},
		{"duplicate signature", append(slices.Clone(good), zipEntry{name: lib + ".sig", body: []byte("x")}), CodeArchive},
		{"no library", without(good, lib), CodeArchive},
		{"only another platform's library", append(without(good, lib), zipEntry{name: libEntry(platform(t, "linux", "arm64")), body: fakeLibrary(platform(t, "linux", "arm64"))}), CodeArchive},
		{"no manifest", without(good, manifestFile), CodeArchive},
		{"too many entries", many, CodeArchive},
		{"library a symlink", replaced(good, zipEntry{name: lib, body: []byte("/usr/lib/libc.so.6"), mode: fs.ModeSymlink | 0o777}), CodeArchive},
		{"manifest a symlink", replaced(good, zipEntry{name: manifestFile, body: []byte("/etc/passwd"), mode: fs.ModeSymlink | 0o777}), CodeArchive},
		{"library a directory", replaced(good, zipEntry{name: lib, mode: fs.ModeDir | 0o755}), CodeArchive},
		{"library declared over the limit", replaced(good, zipEntry{name: lib, body: fakeLibrary(p), declared: maxLibraryBytes + 1}), CodeTooLarge},
		{"manifest declared over the limit", replaced(good, zipEntry{name: manifestFile, body: componentManifestJSON(componentName, "1.2.3.4"), declared: maxManifestJSON + 1}), CodeTooLarge},
		{"signature declared over the limit", replaced(good, zipEntry{name: lib + ".sig", body: []byte("x"), declared: maxSignatureFile + 1}), CodeTooLarge},
		{"manifest of another component", replaced(good, zipEntry{name: manifestFile, body: componentManifestJSON("ClearKeyCdm", "1.2.3.4")}), CodeArchive},
		{"manifest version climbs", replaced(good, zipEntry{name: manifestFile, body: componentManifestJSON(componentName, "../1")}), CodeArchive},
		{"manifest not JSON", replaced(good, zipEntry{name: manifestFile, body: []byte("WidevineCdm 1.2.3.4")}), CodeArchive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, parent, err := extractInto(t, p, tt.entries)
			wantCode(t, err, tt.code)
			if got := dirNames(t, parent); !slices.Equal(got, []string{"out"}) {
				t.Errorf("parent holds %q", got)
			}
		})
	}
}

func TestExtractStopsAtTheDeclaredSize(t *testing.T) {
	// A header claiming fewer bytes than the entry holds: the archive
	// reader refuses the extra bytes, so no more than claimed is written.
	p := linuxAMD64(t)
	body := bytes.Repeat([]byte{0x7f}, 1<<20)
	entries := replaced(packageEntries(p, "1.2.3.4"), zipEntry{name: libEntry(p), body: body, declared: 4096})
	_, dir, _, err := extractInto(t, p, entries)
	wantCode(t, err, CodeArchive)
	if st, err := os.Stat(filepath.Join(dir, p.Lib)); err == nil && st.Size() > 4096 {
		t.Errorf("wrote %d bytes", st.Size())
	}
}

func TestExtractRefusesNonArchives(t *testing.T) {
	dir := t.TempDir()
	junk := []byte("not a zip at all")
	_, err := extract(bytes.NewReader(junk), archive{size: int64(len(junk))}, linuxAMD64(t), dir)
	wantCode(t, err, CodeArchive)
}

func TestSafeName(t *testing.T) {
	for _, ok := range []string{"manifest.json", "_platform_specific/linux_x64/libwidevinecdm.so", "a/..b/c", "..a", "a/b..", "./a"} {
		if err := safeName(ok); err != nil {
			t.Errorf("safeName(%q) = %v", ok, err)
		}
	}
}

func TestLibraryVersion(t *testing.T) {
	writeFile := func(t *testing.T, path string, b []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Run("beside the library", func(t *testing.T) {
		dir := t.TempDir()
		lib := filepath.Join(dir, "libwidevinecdm.so")
		writeFile(t, lib, []byte("lib"))
		writeFile(t, filepath.Join(dir, manifestFile), componentManifestJSON(componentName, "4.10.3112.0"))
		if v := LibraryVersion(lib); v != "4.10.3112.0" {
			t.Errorf("LibraryVersion = %q", v)
		}
	})
	t.Run("Chrome layout", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "WidevineCdm")
		lib := filepath.Join(root, "_platform_specific", "linux_x64", "libwidevinecdm.so")
		writeFile(t, lib, []byte("lib"))
		writeFile(t, filepath.Join(root, manifestFile), componentManifestJSON(componentName, "4.10.2891.0"))
		if v := LibraryVersion(lib); v != "4.10.2891.0" {
			t.Errorf("LibraryVersion = %q", v)
		}
	})
	t.Run("beside wins", func(t *testing.T) {
		root := t.TempDir()
		lib := filepath.Join(root, "a", "b", "libwidevinecdm.so")
		writeFile(t, lib, []byte("lib"))
		writeFile(t, filepath.Join(root, manifestFile), componentManifestJSON(componentName, "1.0.0.0"))
		writeFile(t, filepath.Join(root, "a", "b", manifestFile), componentManifestJSON(componentName, "2.0.0.0"))
		if v := LibraryVersion(lib); v != "2.0.0.0" {
			t.Errorf("LibraryVersion = %q", v)
		}
	})
	t.Run("none", func(t *testing.T) {
		dir := t.TempDir()
		lib := filepath.Join(dir, "libwidevinecdm.so")
		writeFile(t, lib, []byte("lib"))
		if v := LibraryVersion(lib); v != "" {
			t.Errorf("LibraryVersion = %q", v)
		}
	})
	t.Run("another component's manifest", func(t *testing.T) {
		dir := t.TempDir()
		lib := filepath.Join(dir, "libwidevinecdm.so")
		writeFile(t, lib, []byte("lib"))
		writeFile(t, filepath.Join(dir, manifestFile), componentManifestJSON("Other", "1.0.0.0"))
		if v := LibraryVersion(lib); v != "" {
			t.Errorf("LibraryVersion = %q", v)
		}
	})
}
