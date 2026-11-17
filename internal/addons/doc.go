// Package addons is what Sona knows of add-ons before it talks to one: the
// manifest (addon.json), finding add-ons in a folder and refusing the unsafe,
// the environment and process group a native add-on runs in, the host that
// runs a WebAssembly add-on inside Sona (wasm.go) and its log. The
// protocol is internal/cli's (docs/addons.md); the supervisor that starts add-ons
// is internal/services' AddonsService.
//
// Permissions are advisory until the OS sandbox (#304): a native add-on runs
// with its user's rights. Everything here keeps the rest honest: only the user's
// own folder, no look-up through PATH or the working directory, no
// environment but an allowlist, and no child that outlives Sona. A
// WebAssembly module is the other kind: it has no ambient authority, so it
// needs none of that, and the host's job is to keep it that way (wasmfs.go)
// and to cap what it costs Sona (wasm.go).
package addons

import "runtime"

// Supported reports whether this operating system can run add-ons: the
// platforms that have process groups or job objects to end an add-on's
// children with. A platform without them reports it through
// PlatformService.Capabilities, and no add-on starts there.
func Supported() bool {
	switch runtime.GOOS {
	case "linux", "darwin", "windows":
		return true
	}
	return false
}
