package own

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons"
	"github.com/coredev-uk/sona-go/internal/cli"
)

func TestMainRefusesWhatIsNotBundled(t *testing.T) {
	for _, id := range []string{"", "demo", "sona-tools", "LASTFM", "../lastfm"} {
		var stderr bytes.Buffer
		if status := Main(id, "test", bytes.NewReader(nil), io.Discard, &stderr); status != 2 || stderr.Len() == 0 {
			t.Errorf("Main(%q) = %d, %q", id, status, stderr.String())
		}
	}
}

// Every bundled add-on says hello for the ID it runs as, with the
// permissions of its manifest, and leaves when Sona stops it.
func TestMainSpeaksProtocolV2ForEveryBundledAddon(t *testing.T) {
	for _, id := range addons.BundledIDs {
		t.Run(id, func(t *testing.T) {
			m, err := addons.BundledManifest(id)
			if err != nil {
				t.Fatal(err)
			}
			toSona, fromAddon := io.Pipe()
			toAddon, fromSona := io.Pipe()
			asked := map[string]bool{}
			sess := cli.NewSession(cli.AddonConfig{
				ID: id, Host: fakeHost{},
				Allowed: func(p string) bool { asked[p] = true; return true },
			})
			go func() { _ = sess.Serve(context.Background(), rwc{toSona, fromSona}) }()
			status := make(chan int, 1)
			go func() { status <- Main(id, "test", toAddon, fromAddon, io.Discard) }()
			settled(t, sess)
			sess.Close()
			select {
			case s := <-status:
				if s != 0 {
					t.Errorf("exit status %d", s)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("Main did not return after shutdown")
			}
			for _, p := range m.Permissions {
				if !asked[p] {
					t.Errorf("hello did not ask for %s", p)
				}
			}
		})
	}
}
