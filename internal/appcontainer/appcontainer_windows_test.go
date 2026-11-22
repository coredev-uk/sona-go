//go:build windows

package appcontainer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// envRequire, set (CI), turns a test that skips because the machine refuses
// AppContainers into a failure.
const envRequire = "SONA_APPCONTAINER_REQUIRE"

// testProfile is a profile of the test's own, never Sona's (the other tests
// use those), removed when the test ends.
func testProfile(t *testing.T) *Profile {
	t.Helper()
	name := fmt.Sprintf("sona.appcontainer.test%d", os.Getpid())
	prof, err := Open(name, "Sona test", "A profile of Sona's tests")
	if err != nil {
		if os.Getenv(envRequire) != "" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	t.Cleanup(func() { _ = Remove(name, nil, nil) })
	return prof
}

// appContainerMappings is the registry key (under HKCU) that maps each
// AppContainer profile's SID to its name and folder.
const appContainerMappings = `Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppContainer\Mappings\`

// profileRegistered reports whether the profile with SID sid exists.
func profileRegistered(sid *windows.SID) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, appContainerMappings+sid.String(), registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	_ = k.Close()
	return true
}

// hasEntry reports whether path's access list has an entry for sid
// (inherited ones too).
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
	return aclHasEntry(acl, sid, false)
}

// TestGrantAccess grants the AppContainer's SID read access to a folder and
// finds the entry; a second grant changes nothing, and a drive root is
// refused.
func TestGrantAccess(t *testing.T) {
	prof := testProfile(t)
	dir := t.TempDir()
	dacl := func() *windows.ACL {
		t.Helper()
		sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
		if err != nil {
			t.Fatal(err)
		}
		acl, _, err := sd.DACL()
		if err != nil {
			t.Fatal(err)
		}
		return acl
	}
	who := []*windows.SID{prof.SID}
	if aclAllows(dacl(), who, AccessRead, true) {
		t.Fatal("the new folder already grants the AppContainer read access")
	}
	for range 2 {
		if _, err := Grant(prof.SID, dir, AccessRead, true); err != nil {
			t.Fatal(err)
		}
	}
	if !aclAllows(dacl(), who, AccessRead, true) {
		t.Error("no read entry for the AppContainer after the grant")
	}
	if aclAllows(dacl(), who, AccessModify, true) {
		t.Error("the AppContainer was given more than read access")
	}
	if _, err := Grant(prof.SID, `C:\`, AccessRead, false); !errors.Is(err, ErrDriveRoot) || !strings.Contains(err.Error(), "drive root") {
		t.Errorf("grant on a drive root: %v", err)
	}
}

// TestGrantAccessCoversFilesAnUpdateRenamedIn is an update of the install:
// MyGo unpacks the new version in a folder beside it and renames the entries
// in (swapFiles). The install folder keeps its entry for the AppContainer; the
// files renamed in carry what their own folder gave them (windows-2025: they
// lack the entry, and setting the folder's list again, also after taking the
// entry off and adding it afresh, does not hand it down). Once the host
// starts (Grant with the program as sentinel) the new files have it, the
// folders among them as well, and a start after that changes nothing.
func TestGrantAccessCoversFilesAnUpdateRenamedIn(t *testing.T) {
	prof := testProfile(t)
	root := t.TempDir()
	install := filepath.Join(root, "Sona")
	exe := filepath.Join(install, "sona.exe")
	if err := os.MkdirAll(install, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if refreshed, err := Grant(prof.SID, install, AccessRead, true, exe); err != nil || !refreshed {
		t.Fatalf("first grant: refreshed %v, %v", refreshed, err)
	}
	if refreshed, err := Grant(prof.SID, install, AccessRead, true, exe); err != nil || refreshed {
		t.Fatalf("a second start changed the list: refreshed %v, %v", refreshed, err)
	}

	// The update.
	unpacked := filepath.Join(root, ".Sona.update-1", "app")
	if err := os.MkdirAll(filepath.Join(unpacked, "lib"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"sona.exe", filepath.Join("lib", "plugin.dll")} {
		if err := os.WriteFile(filepath.Join(unpacked, f), []byte("new"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Rename(exe, filepath.Join(install, ".sona.exe.old-1")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sona.exe", "lib"} {
		if err := os.Rename(filepath.Join(unpacked, name), filepath.Join(install, name)); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := Grant(prof.SID, install, AccessRead, true, exe); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{exe, filepath.Join(install, "lib"), filepath.Join(install, "lib", "plugin.dll")} {
		if !hasEntry(t, p, prof.SID) {
			t.Errorf("no entry for the AppContainer on %s after the update and the next start", p)
		}
	}
	if refreshed, err := Grant(prof.SID, install, AccessRead, true, exe); err != nil || refreshed {
		t.Errorf("the start after that changed the list: refreshed %v, %v", refreshed, err)
	}
}

// TestRemoveSandbox is the uninstaller's cleanup: it creates a profile of
// the test's own (never Sona's, which the other tests use) and grants it
// folders, a program's and a CDM's, then removes the profile. The profile,
// its package folder and every entry of its SID, the ones the CDM's files
// inherited too, are gone; the CDM's files and folders stay; and a second
// run finds nothing to do.
func TestRemoveSandbox(t *testing.T) {
	name := fmt.Sprintf("sona.appcontainer.test%d", os.Getpid())
	prof, err := Open(name, "Sona test", "A profile of Sona's tests")
	if err != nil {
		if os.Getenv(envRequire) != "" {
			t.Fatal(err)
		}
		t.Skip(err)
	}
	t.Cleanup(func() { _ = Remove(name, nil, nil) }) // a failed test leaves no profile
	if !profileRegistered(prof.SID) {
		t.Fatalf("the new profile %s has no registry mapping", name)
	}
	pkg := prof.Folder // %LOCALAPPDATA%\Packages\<name>
	if _, err := os.Stat(pkg); err != nil {
		t.Fatalf("the profile's package folder: %v", err)
	}

	root := t.TempDir()
	program := filepath.Join(root, "program")
	cdmRoot := filepath.Join(root, "widevine")
	cdmDir := filepath.Join(cdmRoot, "4.10.2710.0", "win_x64")
	lib := filepath.Join(cdmDir, "widevinecdm.dll")
	for _, d := range []string{program, cdmDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(lib, []byte("cdm"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{program, cdmDir} {
		if _, err := Grant(prof.SID, d, AccessRead, true); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{program, cdmDir, lib} { // the file inherits its folder's entry
		if !hasEntry(t, p, prof.SID) {
			t.Fatalf("no entry for the AppContainer on %s after the grant", p)
		}
	}

	for range 2 {
		if err := Remove(name, []string{program}, []string{cdmRoot}); err != nil {
			t.Fatal(err)
		}
		for _, p := range []string{program, cdmDir, lib} {
			if hasEntry(t, p, prof.SID) {
				t.Errorf("an entry for the AppContainer is left on %s", p)
			}
		}
		if profileRegistered(prof.SID) {
			t.Error("the profile is still registered")
		}
		if _, err := os.Stat(pkg); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("the profile's package folder is left (%v)", err)
		}
		if b, err := os.ReadFile(lib); err != nil || string(b) != "cdm" {
			t.Errorf("the CDM did not stay: %q, %v", b, err)
		}
	}
}

func TestEnvHelpers(t *testing.T) {
	env := WithEnv([]string{"Path=a", "TEMP=old", "x=1"}, "TEMP=new", "tmp=new")
	if EnvValue(env, "temp") != "new" || EnvValue(env, "PATH") != "a" || EnvValue(env, "X") != "1" || len(env) != 4 {
		t.Errorf("withEnv: %q", env)
	}
	block, err := envBlock([]string{"b=2", "A=1"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := windows.UTF16FromString("A=1")
	b, _ := windows.UTF16FromString("b=2")
	want := append(append(a, b...), 0)
	if !slices.Equal(block, want) {
		t.Errorf("envBlock = %v, want %v", block, want)
	}
	if _, err := envBlock([]string{"a\x00b=1"}); err == nil {
		t.Error("envBlock accepted a NUL")
	}
}
