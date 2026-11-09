//go:build windows

package main

// GStreamer from the folder beside sona.exe, when the package has one
// (build/windows/package.sh); its init sets the environment before anything
// starts GStreamer, and the CDM host child inherits it.
import _ "github.com/coredev-uk/sona-go/internal/playback/gstbundle"
