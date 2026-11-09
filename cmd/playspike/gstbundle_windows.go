//go:build windows

package main

// The GStreamer folder beside playspike.exe, when there is one: how the
// packaged build's smoke test (build/windows/smoke.sh) runs it.
import _ "github.com/coredev-uk/sona-go/internal/playback/gstbundle"
