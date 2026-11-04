//go:build !unix

package main

// rawKeys has nothing to set on this OS: keys need Enter.
func rawKeys() (restore func()) { return func() {} }
