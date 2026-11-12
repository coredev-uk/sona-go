//go:build linux || (cgo && (darwin || windows))

package cdmhost

// joinReason puts the parent's reason for an unconfined or partly confined
// child ahead of the child's own, for Sandbox.withParent (sandbox_<os>.go).
func joinReason(parent, own string) string {
	switch {
	case parent == "":
		return own
	case own == "":
		return parent
	}
	return parent + "; " + own
}
