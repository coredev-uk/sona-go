//go:build unix

package own

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
)

// hubHost serves the topics of one add-on from a hub, as Sona's host does.
type hubHost struct{ hub *cli.Hub }

func (hubHost) Call(context.Context, cli.Request) (any, error) {
	return nil, &cli.Error{Code: "failed"}
}

func (h hubHost) Watch(topic string) (<-chan any, func(), error) {
	ch, cancel := h.hub.Watch(topic)
	return ch, cancel, nil
}

// fakeDiscord is Discord's IPC socket: it answers the handshake with READY
// and every SET_ACTIVITY with its reply, and hands each activity over.
type fakeDiscord struct {
	activities chan any // the "activity" of each SET_ACTIVITY: nil clears it
}

func startFakeDiscord(t *testing.T) (*fakeDiscord, string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "dsc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "discord-ipc-0")
	l, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	f := &fakeDiscord{activities: make(chan any, 16)}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f, path
}

func (f *fakeDiscord) serve(c net.Conn) {
	defer func() { _ = c.Close() }()
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			return
		}
		data := make([]byte, binary.LittleEndian.Uint32(hdr[4:]))
		if _, err := io.ReadFull(c, data); err != nil {
			return
		}
		var m struct {
			Cmd   string `json:"cmd"`
			Nonce string `json:"nonce"`
			Args  struct {
				Activity any `json:"activity"`
			} `json:"args"`
		}
		_ = json.Unmarshal(data, &m)
		reply := map[string]any{"cmd": m.Cmd, "nonce": m.Nonce}
		if binary.LittleEndian.Uint32(hdr[:4]) == 0 { // the handshake
			reply = map[string]any{"cmd": "DISPATCH", "evt": "READY", "data": map[string]any{"v": 1}}
		} else {
			f.activities <- m.Args.Activity
		}
		b, _ := json.Marshal(reply)
		out := make([]byte, 8, 8+len(b))
		binary.LittleEndian.PutUint32(out[0:], 1)
		binary.LittleEndian.PutUint32(out[4:], uint32(len(b)))
		if _, err := c.Write(append(out, b...)); err != nil {
			return
		}
	}
}

func (f *fakeDiscord) next(t *testing.T) any {
	t.Helper()
	select {
	case a := <-f.activities:
		return a
	case <-time.After(15 * time.Second):
		t.Fatal("Discord got no SET_ACTIVITY")
		return nil
	}
}

// The add-on over the real protocol and a fake Discord: the application ID
// comes in hello, a public listen is SET_ACTIVITY, a hidden one (what Sona
// says of a private listen or a preview: idle) clears it, the connection is
// reported, and leaving clears the presence.
func TestDiscordAddonSetsAndClearsTheActivity(t *testing.T) {
	disc, sock := startFakeDiscord(t)
	hub := cli.NewHub()
	var mu sync.Mutex
	var reports []bool
	toSona, fromAddon := io.Pipe()
	toAddon, fromSona := io.Pipe()
	sess := cli.NewSession(cli.AddonConfig{
		ID: "discord", Host: hubHost{hub}, Allowed: func(p string) bool { return p == cli.PermPlaybackRead },
		Credentials: map[string]string{cli.CredApplicationID: "123456789012345678"},
		OnConnection: func(c bool) {
			mu.Lock()
			reports = append(reports, c)
			mu.Unlock()
		},
	})
	go func() { _ = sess.Serve(context.Background(), rwc{toSona, fromSona}) }()
	t.Cleanup(func() { _ = fromAddon.Close(); _ = fromSona.Close() })

	c := NewClient("discord", toAddon, fromAddon, nil)
	done := make(chan error, 1)
	go func() {
		done <- c.Serve(context.Background(), []string{cli.PermPlaybackRead}, discordSetup([]string{sock}, time.Now))
	}()

	// Nothing plays yet: the presence is cleared (Discord is reached, and
	// the add-on says so).
	hub.Publish(cli.TopicStatus, cli.Status{Running: true, State: "idle"})
	if a := disc.next(t); a != nil {
		t.Fatalf("nothing plays, but the activity is %v", a)
	}

	hub.Publish(cli.TopicStatus, playing("1", "Come Together", 12))
	a, _ := disc.next(t).(map[string]any)
	if a == nil || a["details"] != "Come Together" || a["state"] != "Artist Come Together" || a["type"] != float64(2) {
		t.Fatalf("a public listen: activity = %v", a)
	}
	if assets, _ := a["assets"].(map[string]any); assets["large_image"] == "" || assets["large_text"] != "Album Come Together" {
		t.Errorf("assets = %v", a["assets"])
	}
	if ts, _ := a["timestamps"].(map[string]any); ts["start"] == nil || ts["end"] == nil {
		t.Errorf("timestamps = %v", a["timestamps"])
	}

	hub.Publish(cli.TopicStatus, cli.Status{Running: true, State: "idle"}) // a private listen
	if a := disc.next(t); a != nil {
		t.Fatalf("a hidden listen left the activity %v", a)
	}
	mu.Lock()
	if len(reports) == 0 || !reports[len(reports)-1] {
		t.Errorf("connection reports = %v, want the last true", reports)
	}
	mu.Unlock()

	sess.Close() // Sona stops the add-on
	if err := wait(t, done, "Serve after shutdown"); err != nil {
		t.Fatalf("Serve = %v", err)
	}
}

// Without a Discord to reach the add-on reports that it isn't connected,
// which is how Sona knows to look again for the socket.
func TestDiscordAddonReportsNoDiscord(t *testing.T) {
	hub := cli.NewHub()
	reported := make(chan bool, 4)
	toSona, fromAddon := io.Pipe()
	toAddon, fromSona := io.Pipe()
	sess := cli.NewSession(cli.AddonConfig{
		ID: "discord", Host: hubHost{hub}, Allowed: func(p string) bool { return p == cli.PermPlaybackRead },
		Credentials:  map[string]string{cli.CredApplicationID: "123456789012345678"},
		OnConnection: func(c bool) { reported <- c },
	})
	go func() { _ = sess.Serve(context.Background(), rwc{toSona, fromSona}) }()
	t.Cleanup(func() { _ = fromAddon.Close(); _ = fromSona.Close() })
	c := NewClient("discord", toAddon, fromAddon, nil)
	done := make(chan error, 1)
	go func() {
		done <- c.Serve(context.Background(), []string{cli.PermPlaybackRead},
			discordSetup([]string{filepath.Join(t.TempDir(), "discord-ipc-0")}, time.Now))
	}()
	hub.Publish(cli.TopicStatus, playing("1", "A", 0))
	select {
	case got := <-reported:
		if got {
			t.Error("reported connected with no Discord")
		}
	case <-time.After(15 * time.Second):
		t.Fatal("no report")
	}
	sess.Close()
	_ = wait(t, done, "Serve after shutdown")
}

// A build without Discord's application ID has nothing to show: the add-on
// stays up and idle, and doesn't touch the sockets.
func TestDiscordAddonWithoutApplicationID(t *testing.T) {
	h := newHost(t, "discord", []string{cli.PermPlaybackRead}, nil)
	out := serve(h, []string{cli.PermPlaybackRead}, discordSetup([]string{"/nonexistent/discord-ipc-0"}, time.Now))
	settled(t, h.sess)
	h.sess.Close()
	if err := wait(t, out, "Serve after shutdown"); err != nil {
		t.Fatalf("Serve = %v", err)
	}
}
