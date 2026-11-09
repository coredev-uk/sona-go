//go:build windows && cgo

package main

import (
	"errors"
	"os"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// removeSandbox is `sona --remove-sandbox`: the CDM host's AppContainer
// profile, and its entries in the access lists of the program's, GStreamer's
// and the Widevine folders, go (cdmhost.RemoveSandbox). The CDM stays in the
// user's data folder, as everything else of the user's.
func removeSandbox() error {
	var cdmRoot string
	paths, pathErr := config.ResolvePaths(os.Getenv)
	if pathErr == nil {
		cdmRoot = paths.WidevineDir()
	}
	return errors.Join(pathErr, cdmhost.RemoveSandbox(cdmRoot))
}
