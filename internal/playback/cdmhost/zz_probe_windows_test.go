//go:build cgo

package cdmhost

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Temporary: which way of re-setting a folder's list reaches files renamed in
// from elsewhere? Logs the outcome of each; asserts nothing.
var procTreeSetNamedSecurityInfo = modAdvapi32.NewProc("TreeSetNamedSecurityInfoW")

func probeScene(t *testing.T) (install, exe string) {
	t.Helper()
	prof, err := appContainer()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	install = filepath.Join(root, "Sona")
	exe = filepath.Join(install, "sona.exe")
	if err := os.MkdirAll(install, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := grantAccess(prof.sid, install, accessRead, true, exe); err != nil {
		t.Fatal(err)
	}
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
	return install, exe
}

func probeReport(t *testing.T, name, install string, err error) {
	t.Helper()
	prof, _ := appContainer()
	t.Logf("PROBE %-34s sona.exe=%v lib=%v plugin.dll=%v err=%v", name,
		hasEntry(t, filepath.Join(install, "sona.exe"), prof.sid),
		hasEntry(t, filepath.Join(install, "lib"), prof.sid),
		hasEntry(t, filepath.Join(install, "lib", "plugin.dll"), prof.sid), err)
}

func dirDACL(t *testing.T, path string) *windows.ACL {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	return acl
}

func treeSet(path string, acl *windows.ACL, flags uint32, action uint32) error {
	p, _ := windows.UTF16PtrFromString(path)
	r, _, _ := procTreeSetNamedSecurityInfo.Call(
		uintptr(unsafe.Pointer(p)), uintptr(windows.SE_FILE_OBJECT), uintptr(flags),
		0, 0, uintptr(unsafe.Pointer(acl)), 0, uintptr(action),
		0, 1 /* ProgressInvokeNever */, 0)
	if r != 0 {
		return windows.Errno(r)
	}
	return nil
}

func TestGrantAccessProbeStrategies(t *testing.T) {
	prof, err := appContainer()
	if err != nil {
		t.Skip(err)
	}
	t.Run("same dacl again", func(t *testing.T) {
		install, exe := probeScene(t)
		_, err := grantAccess(prof.sid, install, accessRead, true, exe)
		probeReport(t, "same dacl again", install, err)
	})
	t.Run("revoke then grant", func(t *testing.T) {
		install, exe := probeScene(t)
		err := revokeAccess(prof.sid, install)
		if err == nil {
			_, err = grantAccess(prof.sid, install, accessRead, true, exe)
		}
		probeReport(t, "revoke then grant", install, err)
	})
	t.Run("tree reset keep explicit", func(t *testing.T) {
		install, _ := probeScene(t)
		err := treeSet(install, dirDACL(t, install), windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, 3)
		probeReport(t, "tree reset keep explicit", install, err)
	})
	t.Run("tree reset", func(t *testing.T) {
		install, _ := probeScene(t)
		err := treeSet(install, dirDACL(t, install), windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, 2)
		probeReport(t, "tree reset", install, err)
	})
	t.Run("tree set", func(t *testing.T) {
		install, _ := probeScene(t)
		err := treeSet(install, dirDACL(t, install), windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, 1)
		probeReport(t, "tree set", install, err)
	})
	t.Run("set unprotected", func(t *testing.T) {
		install, _ := probeScene(t)
		err := windows.SetNamedSecurityInfo(install, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dirDACL(t, install), nil)
		probeReport(t, "set unprotected", install, err)
	})
}
