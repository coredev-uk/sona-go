//go:build !linux

package shell

// pinFontDPI works around a WebKitGTK scaling bug; other web runtimes
// don't need it.
func pinFontDPI() {}
