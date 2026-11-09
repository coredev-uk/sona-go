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
	want := map[string]string{
		"GST_PLUGIN_SYSTEM_PATH_1_0": filepath.Join(dir, "lib", "gstreamer-1.0"),
		"GST_PLUGIN_PATH_1_0":        "", // no video folder: only the bundle's plugins
		"GST_PLUGIN_SCANNER":         filepath.Join(dir, "libexec", "gstreamer-1.0", "gst-plugin-scanner.exe"),
		"GST_REGISTRY_1_0":           filepath.Join(dir, "reg", "registry.bin"),
		"PATH":                       dir + `;C:\Windows`,
	}
	check := func() {
		t.Helper()
		env, ok := environment(dir, filepath.Join(dir, "reg"), `C:\Windows`)
		if !ok {
			t.Fatal("no environment for a folder with plugins")
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
	check()

	// The video plugins' folder is an addition, beside the audio one.
	if err := os.MkdirAll(filepath.Join(dir, videoDir), 0o755); err != nil {
		t.Fatal(err)
	}
	want["GST_PLUGIN_PATH_1_0"] = filepath.Join(dir, "lib", "gstreamer-video")
	check()
	if rel, err := filepath.Rel(filepath.Join(dir, pluginDir), filepath.Join(dir, videoDir)); err != nil || rel[0] != '.' {
		t.Errorf("the video folder is inside the plugin folder (%q, %v): the child's scan would find it", rel, err)
	}
}
