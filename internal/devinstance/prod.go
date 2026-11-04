//go:build !dev

package devinstance

// Name is always empty in a production build.
func Name() string { return "" }
