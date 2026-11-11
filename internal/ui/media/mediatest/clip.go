// Package mediatest makes the media fixtures the UI's tests need (a generated
// H.264 clip) and runs the tests of packages that play video as an app does
// (Main).
package mediatest

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

// encoders are the H.264 encoders a clip may be made with, by GStreamer
// element name; the first one the GStreamer in use has is taken.
var encoders = []string{"x264enc", "openh264enc", "vah264enc", "vtenc_h264"}

// tools are the paths of gst-launch-1.0 and gst-inspect-1.0, which the tests
// that make a fixture skip without.
func tools(t testing.TB) (launch, inspect string) {
	t.Helper()
	find := func(name string) string {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Skipf("%s is not installed", name)
		}
		return path
	}
	return find("gst-launch-1.0"), find("gst-inspect-1.0")
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
	return ClipSize(t, frames, "ball", 96, 64)
}

// ClipPattern is Clip with another videotestsrc pattern: "smpte" is colour
// bars that never move, so any frame of the clip is the same picture.
func ClipPattern(t testing.TB, frames int, pattern string) string {
	t.Helper()
	return ClipSize(t, frames, pattern, 96, 64)
}

// ClipSize is Clip with a pattern and a picture size: Apple's smallest
// motion rendition is 486×486.
func ClipSize(t testing.TB, frames int, pattern string, w, h int) string {
	t.Helper()
	launch, inspect := tools(t)
	enc := ""
	for _, e := range encoders {
		if exec.Command(inspect, e).Run() == nil {
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
		"video/x-raw,width="+strconv.Itoa(w)+",height="+strconv.Itoa(h)+",framerate=30/1", "!", "videoconvert", "!", "video/x-raw,format=I420", "!", enc, "!", "h264parse", "!",
		"mp4mux", "!", "filesink", "location="+path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("GStreamer could not make the fixture with %s: %v\n%s", enc, err, out)
	}
	return filepath.FromSlash(path)
}
