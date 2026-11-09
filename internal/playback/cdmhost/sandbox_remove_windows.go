//go:build cgo

package cdmhost

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// hrNotFound is HRESULT_FROM_WIN32(ERROR_NOT_FOUND): DeleteAppContainerProfile
// on a profile that does not exist.
const hrNotFound = 0x80070490

// RemoveSandbox takes the Windows sandbox off the machine, for the
// uninstaller (`sona --remove-sandbox`; the portable zip's user runs it
// before deleting the folder): the entries the parent's grants
// (grantAccess) left in the access lists of the program's folder, of
// GStreamer's and of the plugin folders the environment names, and of the
// Widevine folder cdmRoot (the CDM itself, which lives in the user's data,
// stays: only the SID's entries go), and then the AppContainer profile with
// its package folder (%LOCALAPPDATA%\Packages\sona.cdmhost). It finds
// nothing wrong with what is not there, so it can run again. A step that
// fails does not stop the others; the error says each.
func RemoveSandbox(cdmRoot string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("find the program: %w", err)
	}
	return removeSandbox(appContainerName, readFolders(exe, os.Environ()), cdmRoot)
}

// removeSandbox removes the profile profileName, the entries of its SID from
// the access lists of folders (and of everything under each one, which
// inherited them) and from those of the folders inside cdmRoot ("" for none).
func removeSandbox(profileName string, folders []string, cdmRoot string) error {
	if err := procDeleteAppContainerProfile.Find(); err != nil {
		return nil // a Windows without AppContainers: Sona made none
	}
	name, err := windows.UTF16PtrFromString(profileName)
	if err != nil {
		return err
	}
	var psid *windows.SID
	if hr, _, _ := procDeriveAppContainerSid.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&psid))); hr != 0 {
		return hresult("DeriveAppContainerSidFromAppContainerName", hr)
	}
	sid, err := psid.Copy()
	_, _, _ = procFreeSid.Call(uintptr(unsafe.Pointer(psid)))
	if err != nil {
		return fmt.Errorf("copy the AppContainer SID: %w", err)
	}

	var errs []error
	for _, d := range folders {
		if err := revokeAccess(sid, d); err != nil {
			errs = append(errs, err)
		}
	}
	if cdmRoot != "" {
		// The grant is on the folder holding the CDM's library (a version's
		// platform folder), and a version a later download replaced is gone
		// already, but a rolled-back one may remain: every folder of the
		// tree has its entry looked at.
		walkErr := filepath.WalkDir(cdmRoot, func(path string, d fs.DirEntry, err error) error {
			switch {
			case errors.Is(err, fs.ErrNotExist):
			case err != nil:
				errs = append(errs, err)
			case d.IsDir():
				if err := revokeAccess(sid, path); err != nil {
					errs = append(errs, err)
				}
			}
			return nil
		})
		if walkErr != nil {
			errs = append(errs, walkErr)
		}
	}

	// The package folder's path is known only while the profile exists.
	folder, folderErr := containerFolder(sid)
	if hr, _, _ := procDeleteAppContainerProfile.Call(uintptr(unsafe.Pointer(name))); hr != 0 && uint32(hr) != hrNotFound {
		errs = append(errs, hresult("DeleteAppContainerProfile", hr))
	} else if folderErr == nil && strings.EqualFold(filepath.Base(folder), profileName) {
		// Windows deletes the folder with the profile; make sure of it
		// (and of nothing else: only a folder named for the profile).
		if err := os.RemoveAll(folder); err != nil {
			errs = append(errs, fmt.Errorf("remove the AppContainer's folder: %w", err))
		}
	}
	return errors.Join(errs...)
}

// revokeAccess removes the explicit entries for sid from path's access list
// (grantAccess adds them), which takes away the entries a directory's
// objects inherited from them. A path that is gone, has no list or has no
// entry for sid is left alone.
func revokeAccess(sid *windows.SID, path string) error {
	if filepath.Dir(path) == path {
		return fmt.Errorf("refusing to change the access list of %s, a drive root", path)
	}
	grantMu.Lock()
	defer grantMu.Unlock()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read the access list of %s: %w", path, err)
	}
	if sd == nil {
		return nil
	}
	old, _, err := sd.DACL()
	if errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) || (err == nil && old == nil) {
		return nil // no list: everyone has full access
	}
	if err != nil {
		return fmt.Errorf("read the access list of %s: %w", path, err)
	}
	if !aclHasEntry(old, sid, true) {
		runtime.KeepAlive(sd)
		return nil
	}
	var pin runtime.Pinner
	pin.Pin(sid)
	defer pin.Unpin()
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessMode:  windows.REVOKE_ACCESS,
		Inheritance: windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}}, old)
	runtime.KeepAlive(sd)
	if err != nil {
		return fmt.Errorf("build the access list of %s: %w", path, err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		return fmt.Errorf("remove the sandbox's access to %s: %w", path, err)
	}
	return nil
}

// aclHasEntry reports whether acl has an allow or deny entry for sid;
// explicit: only one not inherited from a parent.
func aclHasEntry(acl *windows.ACL, sid *windows.SID, explicit bool) bool {
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if windows.GetAce(acl, i, &ace) != nil {
			continue
		}
		if t := ace.Header.AceType; t != windows.ACCESS_ALLOWED_ACE_TYPE && t != windows.ACCESS_DENIED_ACE_TYPE {
			continue
		}
		if explicit && ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
			continue
		}
		if (*windows.SID)(unsafe.Pointer(&ace.SidStart)).Equals(sid) {
			return true
		}
	}
	return false
}
