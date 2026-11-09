//go:build cgo

package cdmhost

// osTestChildMode runs the Windows test child modes: none yet (the sandbox's
// probe child comes with the sandbox).
func osTestChildMode(string) {}
