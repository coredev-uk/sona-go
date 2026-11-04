//go:build !probe

package main

// probeExit is `sona -probe`'s exit status, in probe builds (probe_on.go).
func probeExit() {}
