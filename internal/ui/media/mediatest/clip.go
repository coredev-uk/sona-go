// Package mediatest makes the media fixtures the UI's tests need.
package mediatest

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// encoders are the H.264 encoders a clip may be made with, by GStreamer
// element name; the first one the GStreamer in use has is taken.
var encoders = []string{"x264enc", "openh264enc", "vah264enc", "vtenc_h264"}

// ToolsEnv names a folder holding gst-launch-1.0 and gst-inspect-1.0 that
// make the fixture instead of the ones on PATH. They then run in an
// environment with no GST_* variable, so a test binary that points GStreamer
// at a package's trimmed plugins (build/windows/smoke.sh runs the decoder's
// tests that way) still gets its fixture from a whole installation, which
// has an encoder.
const ToolsEnv = "SONA_FIXTURE_GST_BIN"

// tools are the paths of gst-launch-1.0 and gst-inspect-1.0 and the
// environment to run them in (nil: this process's).
func tools(t testing.TB) (launch, inspect string, env []string) {
	t.Helper()
	find := func(name string) string {
		var path string
		var err error
		if dir := os.Getenv(ToolsEnv); dir != "" {
			path, err = exec.LookPath(filepath.Join(dir, name))
		} else {
			path, err = exec.LookPath(name)
		}
		if err != nil {
			t.Skipf("%s is not installed", name)
		}
		return path
	}
	launch, inspect = find("gst-launch-1.0"), find("gst-inspect-1.0")
	if os.Getenv(ToolsEnv) != "" {
		for _, kv := range os.Environ() {
			if !strings.HasPrefix(strings.ToUpper(kv), "GST_") {
				env = append(env, kv)
			}
		}
		if env == nil {
			env = []string{} // an empty environment, not the inherited one
		}
	}
	return launch, inspect, env
}

// Clip writes a small H.264 MP4 (a moving ball, 96×64 at 30 fps, frames
// pictures) into the test's directory with GStreamer's own encoder and
// returns its path, 4:2:0 (I420) so that every decoder takes it: without the
// constraint x264enc may encode 4:4:4, which Media Foundation does not decode.
// The fixture is generated, never committed. It skips the
// test where gst-launch-1.0 or an H.264 encoder is missing; whether anything
// decodes the clip is the caller's to check (media.Codec).
func Clip(t testing.TB, frames int) string {
	t.Helper()
	return ClipPattern(t, frames, "ball")
}

// ClipPattern is Clip with another videotestsrc pattern: "smpte" is colour
// bars that never move, so any frame of the clip is the same picture.
func ClipPattern(t testing.TB, frames int, pattern string) string {
	t.Helper()
	launch, inspect, env := tools(t)
	enc := ""
	for _, e := range encoders {
		cmd := exec.Command(inspect, e)
		cmd.Env = env
		if cmd.Run() == nil {
			enc = e
			break
		}
	}
	if enc == "" {
		t.Skip("no H.264 encoder in this GStreamer")
	}
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if runtime.GOOS == "windows" {
		// gst-launch reads a backslash as an escape and drops it.
		path = filepath.ToSlash(path)
	}
	cmd := exec.Command(launch, "-q", "videotestsrc", "num-buffers="+strconv.Itoa(frames), "pattern="+pattern, "!",
		"video/x-raw,width=96,height=64,framerate=30/1", "!", "videoconvert", "!", "video/x-raw,format=I420", "!", enc, "!", "h264parse", "!",
		"mp4mux", "!", "filesink", "location="+path)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("GStreamer could not make the fixture with %s: %v\n%s", enc, err, out)
	}
	return filepath.FromSlash(path)
}
