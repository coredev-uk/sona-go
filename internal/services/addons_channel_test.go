//go:build unix

package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/cli"
)

// The add-on channel is the one the supervisor gives the add-on (its
// standard streams); a program that finds Sona's command transport (the
// D-Bus bus or the socket) and says hello there, naming an installed add-on,
// gets nothing: the v1 transport refuses every v2 method, so it neither
// reaches the add-on's permissions nor takes the place of the real one.
func TestAddonHelloOverTheCommandTransportIsRefused(t *testing.T) {
	a := newAddonEnv(t)
	script := hello("demo", "playback.read") + "exec 3<&0\ncat <&3 > in.log &\n" +
		"until [ -e go ]; do sleep 0.05; done\n" +
		`printf '%s\n' '{"id":2,"method":"call","params":{"command":"status"}}'` + "\nwait\n"
	dir := a.install(t, "demo", manifestJSON("demo", "playback.read", "playback.control"), script)
	if err := a.svc.Enable("demo", []string{"playback.read", "playback.control"}); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	a.waitState(t, "demo", AddonRunning)

	for _, raw := range []string{
		`{"id":1,"method":"hello","params":{"id":"demo","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":["playback.read","playback.control"]}}`,
		`{"command":"hello","args":["demo"]}`,
		`{"command":"call","args":["play"]}`,
		`{"command":"subscribe","args":["status"]}`,
	} {
		r := cli.Handle(t.Context(), a.srv, []byte(raw))
		if r.Error == nil || r.Error.Code != cli.CodeUnknownCommand || r.Result != nil {
			t.Errorf("%s over the command transport → %+v", raw, r)
		}
	}
	// Nothing was played, and the add-on's own session is the one it was.
	if st := a.status(t, "status"); st.State == cli.StatePlaying {
		t.Errorf("a command ran: %+v", st)
	}
	// Whatever the refused commands wrote to the channel was written before
	// they returned, so the add-on's second request, which it sends only now,
	// is answered after it: once that answer is in the log, so is anything
	// else the channel was given, and only the add-on's own hello was answered.
	if err := os.WriteFile(filepath.Join(dir, "go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	in := filepath.Join(dir, "in.log")
	waitUntil(t, "the add-on's second answer", fileHas(in, `"id":2,"result"`))
	if b, _ := os.ReadFile(in); strings.Count(string(b), `"id":1,"result"`) != 1 {
		t.Errorf("the add-on's channel saw other hellos:\n%s", b)
	}
	// The channel is the process's own standard streams, nothing nameable:
	// no socket or bus name was created for it.
	entries, _ := os.ReadDir(filepath.Join(a.root, "demo"))
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sock") {
			t.Errorf("a socket %q appeared in the add-on's folder", e.Name())
		}
	}
}
