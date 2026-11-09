//go:build windows

package gstbundle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironment(t *testing.T) {
	dir := t.TempDir()
	if _, ok := environment(dir, filepath.Join(dir, "reg"), `C:\Windows`); ok {
		t.Fatal("a folder with no plugins got a bundle environment")
	}
	if err := os.MkdirAll(filepath.Join(dir, pluginDir), 0o755); err != nil {
		t.Fatal(err)
	}
	env, ok := environment(dir, filepath.Join(dir, "reg"), `C:\Windows`)
	if !ok {
		t.Fatal("no environment for a folder with plugins")
	}
	want := map[string]string{
		"GST_PLUGIN_SYSTEM_PATH_1_0": filepath.Join(dir, "lib", "gstreamer-1.0"),
		"GST_PLUGIN_PATH_1_0":        "",
		"GST_PLUGIN_SCANNER":         filepath.Join(dir, "libexec", "gstreamer-1.0", "gst-plugin-scanner.exe"),
		"GST_REGISTRY_1_0":           filepath.Join(dir, "reg", "registry.bin"),
		"PATH":                       dir + `;C:\Windows`,
	}
	for k, v := range want {
		if got, present := env[k]; !present || got != v {
			t.Errorf("%s = %q (set: %v), want %q", k, got, present, v)
		}
	}
	if len(env) != len(want) {
		t.Errorf("%d variables, want %d: %v", len(env), len(want), env)
	}
}
