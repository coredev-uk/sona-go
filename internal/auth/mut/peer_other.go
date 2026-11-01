//go:build !linux

package mut

import "net"

// ownerOnly guards Linux, where every user can read the sign-in URL from
// another process's command line. macOS refuses KERN_PROCARGS2 across
// users, so there the start cookie is the binding. Windows has no socket
// owner check yet (docs/security-review.md F7 lists it as a residual risk).
func ownerOnly(ln net.Listener) net.Listener { return ln }
