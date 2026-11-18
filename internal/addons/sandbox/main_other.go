//go:build !linux

package sandbox

// Main does nothing here: only Linux's sandbox re-executes Sona's program.
func Main() {}
