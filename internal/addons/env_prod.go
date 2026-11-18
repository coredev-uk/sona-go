//go:build !dev

package addons

// devEnv is false in a release build: no development variable reaches an add-on.
func devEnv(string) bool { return false }
