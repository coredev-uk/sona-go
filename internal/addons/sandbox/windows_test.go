//go:build windows

package sandbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/coredev-uk/sona-go/internal/appcontainer"
)

// envRequire, set (CI), turns a test that skips because the machine refuses
// AppContainers into a failure.
const envRequire = "SONA_APPCONTAINER_REQUIRE"

const appContainerMappings = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppContainer\Mappings\`

func profileRegistered(sid *windows.SID) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, appContainerMappings+sid.String(), registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = k.Close()
	return true
}

// hasEntry reports whether path's access list has an entry for sid.
func hasEntry(t *testing.T, path string, sid *windows.SID) bool {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil {
			continue
		}
		if (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(sid) {
			return true
		}
	}
	return false
}

// TestRemoveProfiles creates the profiles of two add-ons of the test's own
// (never a real add-on's ID), grants them folders as Start does, and removes
// them: the profiles, their package folders and every entry of their SIDs are
// gone, the add-ons' files stay, a second run succeeds, and an ID that never
// ran is no error.
func TestRemoveProfiles(t *testing.T) {
	ids := []string{fmt.Sprintf("rmtest-a-%d", os.Getpid()), fmt.Sprintf("rmtest-b-%d", os.Getpid())}
	t.Cleanup(func() { _ = RemoveProfiles(ids, nil, func(string) []string { return nil }) })

	root := t.TempDir()
	program := filepath.Join(root, "program")
	if err := os.MkdirAll(program, 0o700); err != nil {
		t.Fatal(err)
	}
	type made struct {
		prof      *appcontainer.Profile
		dir, data string
		file      string
	}
	byID := map[string]*made{}
	for _, id := range ids {
		prof, err := profile(id)
		if err != nil {
			if os.Getenv(envRequire) != "" {
				t.Fatal(err)
			}
			t.Skip(err)
		}
		m := &made{prof: prof, dir: filepath.Join(root, "addons", id), data: filepath.Join(root, "state", id, "data")}
		for _, d := range []string{m.dir, m.data} {
			if err := os.MkdirAll(d, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		m.file = filepath.Join(m.data, "kept.txt")
		if err := os.WriteFile(m.file, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		exe := filepath.Join(m.dir, "addon.exe")
		if err := os.WriteFile(exe, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := grant(prof, Spec{Policy: Policy{Dir: m.dir, Data: m.data, Read: []string{program}}, ID: id, Exe: exe}); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{m.dir, exe, m.data, m.file, program} {
			if !hasEntry(t, p, prof.SID) {
				t.Fatalf("add-on %s: no entry for its AppContainer on %s after the grant", id, p)
			}
		}
		if !profileRegistered(prof.SID) {
			t.Fatalf("the profile of %s has no registry mapping", id)
		}
		byID[id] = m
	}

	all := append([]string{"rmtest-never-ran"}, ids...)
	for range 2 {
		err := RemoveProfiles(all, []string{program}, func(id string) []string {
			if m := byID[id]; m != nil {
				return []string{m.dir, m.data}
			}
			return []string{filepath.Join(root, "addons", id)}
		})
		if err != nil {
			t.Fatal(err)
		}
		for id, m := range byID {
			for _, p := range []string{m.dir, m.data, m.file, program} {
				if hasEntry(t, p, m.prof.SID) {
					t.Errorf("add-on %s: an entry for its AppContainer is left on %s", id, p)
				}
			}
			if profileRegistered(m.prof.SID) {
				t.Errorf("the profile of %s is still registered", id)
			}
			if _, err := os.Stat(m.prof.Folder); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the package folder of %s is left (%v)", id, err)
			}
			if b, err := os.ReadFile(m.file); err != nil || string(b) != "data" {
				t.Errorf("the data of %s did not stay: %q, %v", id, b, err)
			}
		}
	}
}
