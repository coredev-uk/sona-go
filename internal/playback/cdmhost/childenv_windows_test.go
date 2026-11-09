//go:build cgo

package cdmhost

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The CDM host child needs no video plugin, and a plugin scan loads every
// plugin it finds: Direct3D 11's among them, which asks the GPUs what they
// decode. The UI process names the video plugins' folder in GST_PLUGIN_PATH_1_0
// (internal/playback/gstbundle); the child is started without it.
func TestChildEnvWithholdsPluginFolders(t *testing.T) {
	t.Setenv("GST_PLUGIN_PATH_1_0", `C:\Sona\lib\gstreamer-video`)
	t.Setenv("GST_PLUGIN_PATH", `C:\elsewhere`)
	t.Setenv("GST_PLUGIN_SYSTEM_PATH_1_0", `C:\Sona\lib\gstreamer-1.0`)
	t.Setenv("GST_REGISTRY_1_0", `C:\Sona\registry.bin`)
	env := childEnv(slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, kv := range env {
		if k, _, _ := strings.Cut(kv, "="); strings.EqualFold(k, "GST_PLUGIN_PATH_1_0") || strings.EqualFold(k, "GST_PLUGIN_PATH") {
			t.Errorf("the child's environment has %s", kv)
		}
	}
	for k, want := range map[string]string{
		"GST_PLUGIN_SYSTEM_PATH_1_0": `C:\Sona\lib\gstreamer-1.0`,
		"GST_REGISTRY_1_0":           `C:\Sona\registry.bin`,
	} {
		if got := envValue(env, k); got != want {
			t.Errorf("the child's %s = %q, want %q", k, got, want)
		}
	}
}

// copyPlugins copies the installed GStreamer's plugins (plugin names, without
// libgst and .dll) into a new folder; an installation without one fails the
// test (CI installs the official package).
func copyPlugins(t *testing.T, from string, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(from, "libgst"+n+".dll"))
		if err != nil {
			t.Fatalf("the GStreamer installation lacks the %s plugin: %v", n, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "libgst"+n+".dll"), b, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// registryOf runs the child's plugin scan with the environment set and
// returns the plugin registry it wrote.
func registryOf(t *testing.T) []byte {
	t.Helper()
	reg := privateRegistry(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := PrepareAudio(ctx, Options{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(reg)
	if err != nil {
		t.Fatalf("the child wrote no plugin registry: %v", err)
	}
	return b
}

// With the packaged layout (the audio plugins in the system folder, the video
// plugins in a folder GST_PLUGIN_PATH_1_0 names), the child's registry lists
// the audio plugins and no video one, so it never loaded d3d11 or
// videoconvertscale. The control: the same plugins in the system path are in
// the registry, so the check can see them.
func TestChildScansNoVideoPlugins(t *testing.T) {
	dirs := gstreamerDirs()
	if len(dirs) < 2 {
		t.Fatal("no GStreamer installation is loaded in this process")
	}
	from := filepath.Join(dirs[1], "gstreamer-1.0")
	audio := copyPlugins(t, from, "coreelements", "playback", "app", "audioconvert", "audioresample", "isomp4", "libav")
	video := copyPlugins(t, from, "videoparsersbad", "videoconvertscale", "d3d11")
	videoNames := []string{"libgstvideoparsersbad", "libgstvideoconvertscale", "libgstd3d11"}

	t.Setenv("GST_PLUGIN_SYSTEM_PATH_1_0", audio)
	t.Setenv("GST_PLUGIN_PATH_1_0", video)
	reg := registryOf(t)
	if !bytes.Contains(reg, []byte("libgstcoreelements")) {
		t.Fatal("the child's registry does not list the audio plugins: the check cannot tell")
	}
	for _, n := range videoNames {
		if bytes.Contains(reg, []byte(n)) {
			t.Errorf("the child's registry lists %s, which the child must not load", n)
		}
	}

	// Control: the same folder, named where the child does read plugins from.
	t.Setenv("GST_PLUGIN_SYSTEM_PATH_1_0", audio+string(os.PathListSeparator)+video)
	t.Setenv("GST_PLUGIN_PATH_1_0", "")
	reg = registryOf(t)
	for _, n := range videoNames[:2] { // d3d11 may register nothing where no GPU decodes
		if !bytes.Contains(reg, []byte(n)) {
			t.Errorf("the control's registry does not list %s: the check cannot tell", n)
		}
	}
}
