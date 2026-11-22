//go:build cgo

package gst

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// listOnly is set in the child the test below starts: it lists the outputs
// and nothing else, so that what it has loaded is the listing's alone.
const listOnly = "SONA_GST_TEST_LIST_ONLY"

// The UI process lists the outputs (Settings → Playback); a GStreamer plugin
// it loaded from the install's lib\gstreamer-1.0 would keep an update or a
// restore from renaming lib\. The listing loads none, and gives endpoint IDs
// (wasapi2's device.id) with at most one default.
func TestAudioSinksLoadNoPlugin(t *testing.T) {
	if os.Getenv(listOnly) == "1" {
		listAndReportModules(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestAudioSinksLoadNoPlugin$", "-test.v")
	cmd.Env = append(os.Environ(), listOnly+"=1")
	out, err := cmd.CombinedOutput()
	t.Logf("%s", out)
	if err != nil {
		t.Fatal(err)
	}
}

func listAndReportModules(t *testing.T) {
	devs, err := AudioSinks()
	if err != nil {
		t.Fatal(err)
	}
	defaults := 0
	for _, d := range devs {
		t.Logf("%q %q default=%v", d.Name, d.Description, d.Default)
		if !strings.HasPrefix(d.Name, "{0.0.0.") || d.Description == "" {
			t.Errorf("not a render endpoint: %+v", d)
		}
		if d.Default {
			defaults++
		}
	}
	if defaults > 1 {
		t.Errorf("%d defaults", defaults)
	}
	for _, m := range modules(t) {
		if strings.Contains(strings.ToLower(m), `\gstreamer-1.0\`) {
			t.Errorf("listing the outputs loaded a GStreamer plugin: %s", m)
		}
	}
}

// modules are the paths of the DLLs this process has loaded.
func modules(t *testing.T) []string {
	t.Helper()
	self := windows.CurrentProcess()
	hs := make([]windows.Handle, 1024)
	var need uint32
	size := uint32(unsafe.Sizeof(hs[0]))
	if err := windows.EnumProcessModules(self, &hs[0], uint32(len(hs))*size, &need); err != nil {
		t.Fatal(err)
	}
	n := int(need / size)
	if n > len(hs) {
		t.Fatalf("%d modules", n)
	}
	out := make([]string, 0, n)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	for _, h := range hs[:n] {
		if err := windows.GetModuleFileNameEx(self, h, &buf[0], uint32(len(buf))); err == nil {
			out = append(out, windows.UTF16ToString(buf))
		}
	}
	return out
}
