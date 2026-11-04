//go:build darwin

package main

import (
	"os"
	"path/filepath"
)

// init points GStreamer at the framework inside Sona.app, as the GStreamer
// deploy guide describes (build/darwin/bundle.sh lays it out: lib/plugins and
// libexec/gst-plugin-scanner). It runs before main, so before anything starts GStreamer, and the CDM host child (this
// same executable) inherits it. A plain development binary has no
// Contents/Frameworks and keeps whatever its environment says.
func init() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	root := filepath.Join(filepath.Dir(exe), "..", "Frameworks", "GStreamer")
	if _, err := os.Stat(filepath.Join(root, "lib", "plugins")); err != nil {
		return
	}
	root = filepath.Clean(root)
	cache, err := os.UserCacheDir()
	if err != nil {
		return
	}
	registry := filepath.Join(cache, "Sona", "gstreamer-1.0")
	if os.MkdirAll(registry, 0o700) != nil {
		return
	}
	for k, v := range map[string]string{
		"GST_PLUGIN_SYSTEM_PATH_1_0": filepath.Join(root, "lib", "plugins"),
		"GST_PLUGIN_PATH_1_0":        "", // only the bundle's plugins, never the system's
		"GST_PLUGIN_SCANNER":         filepath.Join(root, "libexec", "gst-plugin-scanner"),
		"GST_REGISTRY_1_0":           filepath.Join(registry, "registry.bin"),
	} {
		_ = os.Setenv(k, v)
	}
}
