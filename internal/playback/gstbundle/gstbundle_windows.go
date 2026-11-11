//go:build windows

package gstbundle

import (
	"os"
	"path/filepath"
)

// The layout beside the executable (build/windows/package.sh):
//
//	sona.exe, libgstreamer-1.0-0.dll, … the DLLs sona.exe and the plugins import
//	lib\gstreamer-1.0\libgst*.dll                 the plugins Sona uses
//	libexec\gstreamer-1.0\gst-plugin-scanner.exe  loads plugins for the registry
const (
	pluginDir  = `lib\gstreamer-1.0`
	scannerExe = `libexec\gstreamer-1.0\gst-plugin-scanner.exe`
)

func init() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return
	}
	env, ok := environment(filepath.Dir(exe), filepath.Join(cache, "Sona", "gstreamer-1.0"), os.Getenv("PATH"))
	if !ok {
		return
	}
	if os.MkdirAll(filepath.Dir(env["GST_REGISTRY_1_0"]), 0o700) != nil {
		return
	}
	for k, v := range env {
		_ = os.Setenv(k, v)
	}
}

// environment is what GStreamer needs to run from the folder dir, or false
// when dir holds no plugins. registryDir is where the plugin registry is
// kept (the install folder may not be writable); path is the current PATH.
func environment(dir, registryDir, path string) (map[string]string, bool) {
	if st, err := os.Stat(filepath.Join(dir, pluginDir)); err != nil || !st.IsDir() {
		return nil, false
	}
	return map[string]string{
		"GST_PLUGIN_SYSTEM_PATH_1_0": filepath.Join(dir, pluginDir),
		"GST_PLUGIN_PATH_1_0":        "", // only the bundle's plugins, never another installation's
		"GST_PLUGIN_SCANNER":         filepath.Join(dir, scannerExe),
		"GST_REGISTRY_1_0":           filepath.Join(registryDir, "registry.bin"),
		// The scanner sits two folders down, so Windows looks for the DLLs
		// it and the plugins import beside it, then on PATH: put the
		// executable's folder, where they are, first.
		"PATH": dir + string(os.PathListSeparator) + path,
	}, true
}
