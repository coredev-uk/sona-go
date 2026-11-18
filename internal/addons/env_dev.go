//go:build dev

package addons

import "strings"

// devEnv reports whether a development build passes name to Sona's own
// add-ons: the fake service endpoints a live run points them at
// (internal/addons/own, endpoints_dev.go).
func devEnv(name string) bool { return strings.HasPrefix(name, "SONA_DEV_ENDPOINT_") }
