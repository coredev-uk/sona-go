//go:build windows && cgo

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/addons/sandbox"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// removeSandbox is `sona --remove-sandbox`: the CDM host's AppContainer
// profile, and its entries in the access lists of the program's, GStreamer's
// and the Widevine folders, go (cdmhost.RemoveSandbox), and so do the
// add-ons' profiles (one each, named by ID) with their entries on the
// add-on's folder, its data folder and, for Sona's own add-ons, the program's
// folders. The add-ons are the bundled ones, the installed ones and any left
// over: a folder in the add-ons' state folder (their data, which stays) names
// an add-on that ran. The CDM and the add-ons' data stay in the user's data
// folders, as everything else of the user's.
func removeSandbox() error {
	var cdmRoot string
	paths, pathErr := config.ResolvePaths(os.Getenv)
	if pathErr == nil {
		cdmRoot = paths.WidevineDir()
	}
	errs := []error{pathErr, cdmhost.RemoveSandbox(cdmRoot)}

	var reads []string
	if exe, err := os.Executable(); err != nil {
		errs = append(errs, fmt.Errorf("find the program: %w", err))
	} else {
		reads = sandbox.ProgramReads(exe, os.Environ())
	}
	var ids []string
	trees := func(string) []string { return nil }
	if pathErr == nil {
		ids = sandbox.ProfileIDs(addons.BundledIDs, paths.AddonsDir(), paths.AddonLogDir())
		trees = func(id string) []string {
			return []string{filepath.Join(paths.AddonsDir(), id), paths.AddonDataDir(id)}
		}
	} else {
		ids = sandbox.ProfileIDs(addons.BundledIDs)
	}
	errs = append(errs, sandbox.RemoveProfiles(ids, reads, trees))
	return errors.Join(errs...)
}
