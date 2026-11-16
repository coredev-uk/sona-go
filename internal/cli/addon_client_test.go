package cli

import (
	"bytes"
	"io"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// pipes joins a child's standard output and input into the add-on channel.
type pipes struct {
	io.Reader
	io.Writer
	closeIn io.Closer
}

func (p pipes) Close() error { return p.closeIn.Close() }

// The protocol is language-neutral: add-ons written in shell and in Python
// (testdata) complete hello, subscribe, receive a now-playing event and are
// refused a command they have no permission for, over the standard streams
// the supervisor gives an add-on.
func TestAddonClientsInOtherLanguages(t *testing.T) {
	for _, tc := range []struct{ name, bin, script string }{
		{"shell", "sh", "testdata/addon-client.sh"},
		{"python", "python3", "testdata/addon-client.py"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if runtime.GOOS == "windows" && tc.bin == "sh" {
				t.Skip("no sh")
			}
			bin, err := exec.LookPath(tc.bin)
			if err != nil {
				t.Skipf("%s not available", tc.bin)
			}
			host := newFakeHost()
			host.hub.Publish(TopicNowPlaying, NowPlaying{
				Track: &Track{ID: "100", Title: "Come Together", Artist: "The Beatles"}, StartedAt: 1,
			})
			s := NewSession(AddonConfig{
				ID: "demo", Host: host,
				Allowed: func(p string) bool { return p == PermPlaybackRead },
			})
			cmd := exec.CommandContext(t.Context(), bin, tc.script)
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			served := make(chan error, 1)
			go func() { served <- s.Serve(t.Context(), pipes{stdout, stdin, stdin}) }()
			select {
			case err := <-served:
				if err != nil {
					t.Errorf("session: %v", err)
				}
			case <-time.After(30 * time.Second):
				t.Fatal("the add-on never finished")
			}
			if err := cmd.Wait(); err != nil {
				t.Fatalf("%s add-on: %v\n%s", tc.name, err, stderr.String())
			}
		})
	}
}
