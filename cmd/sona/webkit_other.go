//go:build !linux

package main

// webkitVersion is only known on Linux, where the web view is WebKitGTK.
func webkitVersion() string { return "" }
