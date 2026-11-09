//go:build !(windows && cgo)

package main

import "errors"

// removeSandbox: only the Windows sandbox (an AppContainer profile and the
// access list entries granted to it) has anything to remove; the other
// platforms' sandboxes leave nothing behind.
func removeSandbox() error {
	return errors.New("this build has no Windows sandbox to remove")
}
