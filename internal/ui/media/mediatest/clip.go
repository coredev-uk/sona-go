// Package mediatest makes the media fixtures the UI's tests need.
package mediatest

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
)

// encoders are the H.264 encoders a clip may be made with, by GStreamer
// element name; the first one the GStreamer in use has is taken.
var encoders = []string{"x264enc", "openh264enc", "vah264enc", "vtenc_h264"}

// Clip writes a small H.264 MP4 (a moving ball, 96×64 at 30 fps, frames
// pictures) into the test's directory with GStreamer's own encoder and
// returns its path. The fixture is generated, never committed. It skips the
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
	launch, err := exec.LookPath("gst-launch-1.0")
	if err != nil {
		t.Skip("gst-launch-1.0 is not installed")
	}
	inspect, err := exec.LookPath("gst-inspect-1.0")
	if err != nil {
		t.Skip("gst-inspect-1.0 is not installed")
	}
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
	out, err := exec.Command(launch, "-q", "videotestsrc", "num-buffers="+strconv.Itoa(frames), "pattern="+pattern, "!",
		"video/x-raw,width=96,height=64,framerate=30/1", "!", "videoconvert", "!", enc, "!", "h264parse", "!",
		"mp4mux", "!", "filesink", "location="+path).CombinedOutput()
	if err != nil {
		t.Skipf("GStreamer could not make the fixture with %s: %v\n%s", enc, err, out)
	}
	return path
}
