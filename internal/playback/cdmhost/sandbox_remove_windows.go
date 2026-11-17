//go:build cgo

package cdmhost

import (
	"fmt"
	"os"

	"github.com/coredev-uk/sona-go/internal/appcontainer"
)

// RemoveSandbox takes the Windows sandbox off the machine, for the
// uninstaller (`sona --remove-sandbox`; the portable zip's user runs it
// before deleting the folder): the entries the parent's grants
// (appcontainer.Grant) left in the access lists of the program's folder, of
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
	var trees []string
	if cdmRoot != "" {
		// The grant is on the folder holding the CDM's library (a version's
		// platform folder), and a version a later download replaced is gone
		// already, but a rolled-back one may remain: every folder of the
		// tree has its entry looked at.
		trees = []string{cdmRoot}
	}
	return appcontainer.Remove(profileName, folders, trees)
}
