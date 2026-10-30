package gst

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// TestAudioSinksListsNewSink creates a PipeWire null sink and checks it
// shows up by node.name. It needs a running PipeWire and pw-cli.
func TestAudioSinksListsNewSink(t *testing.T) {
	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "pipewire-0")); err != nil {
		t.Skip("no PipeWire daemon")
	}
	if !Have("pipewiresink") {
		t.Skip("no PipeWire GStreamer plugin")
	}
	pwcli, err := exec.LookPath("pw-cli")
	if err != nil {
		t.Skip("no pw-cli")
	}
	name := fmt.Sprintf("sona-test-%d", os.Getpid())
	out, err := exec.Command(pwcli, "create-node", "adapter",
		`{ factory.name=support.null-audio-sink node.name=`+name+` node.description="Sona test sink" media.class=Audio/Sink object.linger=true audio.position=[FL FR] }`).CombinedOutput()
	if err != nil {
		t.Fatalf("pw-cli create-node: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command(pwcli, "destroy", name).Run() })

	var devs []Device
	for deadline := time.Now().Add(3 * time.Second); ; {
		if devs, err = AudioSinks(); err != nil {
			t.Fatal(err)
		}
		i := slices.IndexFunc(devs, func(d Device) bool { return d.Name == name })
		if i >= 0 {
			if devs[i].Description != "Sona test sink" {
				t.Errorf("description %q", devs[i].Description)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s not listed in %+v", name, devs)
		}
		time.Sleep(100 * time.Millisecond)
	}
	defaults := 0
	for _, d := range devs {
		if d.Default {
			defaults++
		}
	}
	if defaults > 1 {
		t.Errorf("%d default sinks", defaults)
	}
}
