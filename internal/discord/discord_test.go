//go:build unix

package discord

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// fakeDiscord is a Discord client's IPC socket.
type fakeDiscord struct {
	t     *testing.T
	ln    net.Listener
	conns chan net.Conn
}

// listen serves discord-ipc-0 in a fresh runtime directory, which becomes
// the only place the client looks.
func listen(t *testing.T) *fakeDiscord {
	t.Helper()
	dir := socketDir(t)
	for _, env := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		t.Setenv(env, dir)
	}
	return listenAt(t, filepath.Join(dir, "discord-ipc-0"))
}

func listenAt(t *testing.T, path string) *fakeDiscord {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeDiscord{t: t, ln: ln, conns: make(chan net.Conn, 4)}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			f.conns <- c
		}
	}()
	return f
}

func (f *fakeDiscord) accept() net.Conn {
	f.t.Helper()
	select {
	case c := <-f.conns:
		f.t.Cleanup(func() { _ = c.Close() })
		return c
	case <-time.After(5 * time.Second):
		f.t.Fatal("client never connected")
		return nil
	}
}

func readFrame(t *testing.T, c net.Conn) (uint32, map[string]any) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var hdr [8]byte
	if _, err := io.ReadFull(c, hdr[:]); err != nil {
		t.Fatalf("read header: %v", err)
	}
	data := make([]byte, binary.LittleEndian.Uint32(hdr[4:]))
	if _, err := io.ReadFull(c, data); err != nil {
		t.Fatalf("read payload: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("payload %q: %v", data, err)
	}
	return binary.LittleEndian.Uint32(hdr[:4]), m
}

func writeFrame(t *testing.T, c net.Conn, op uint32, v any) {
	t.Helper()
	data, _ := json.Marshal(v)
	buf := make([]byte, 8, 8+len(data))
	binary.LittleEndian.PutUint32(buf[0:], op)
	binary.LittleEndian.PutUint32(buf[4:], uint32(len(data)))
	if _, err := c.Write(append(buf, data...)); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// handshake answers the client's HANDSHAKE with READY and returns it.
func handshake(t *testing.T, c net.Conn) map[string]any {
	t.Helper()
	op, m := readFrame(t, c)
	if op != opHandshake {
		t.Fatalf("first frame op = %d, want handshake", op)
	}
	writeFrame(t, c, opFrame, map[string]any{"cmd": "DISPATCH", "evt": "READY", "data": map[string]any{"v": 1}})
	return m
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestSetActivityHandshakeAndPayload(t *testing.T) {
	f := listen(t)
	c := New("1234567890")
	start := time.UnixMilli(1_700_000_000_000)
	act := &Activity{
		Title: "Come Together", Artist: "The Beatles", Album: "Abbey Road",
		ArtworkURL: "https://is1-ssl.mzstatic.com/image/thumb/x/512x512bb.jpg",
		Start:      start, End: start.Add(259 * time.Second),
	}
	errc := make(chan error, 1)
	go func() { errc <- c.SetActivity(ctx(t), act) }()

	conn := f.accept()
	hello := handshake(t, conn)
	if hello["v"] != float64(1) || hello["client_id"] != "1234567890" {
		t.Fatalf("handshake = %v", hello)
	}
	op, m := readFrame(t, conn)
	if op != opFrame || m["cmd"] != "SET_ACTIVITY" {
		t.Fatalf("frame op %d = %v", op, m)
	}
	nonce, _ := m["nonce"].(string)
	if nonce == "" {
		t.Fatal("SET_ACTIVITY without nonce")
	}
	args := m["args"].(map[string]any)
	if args["pid"] != float64(os.Getpid()) {
		t.Errorf("pid = %v, want %d", args["pid"], os.Getpid())
	}
	a := args["activity"].(map[string]any)
	want := map[string]any{"type": float64(2), "details": "Come Together", "state": "The Beatles"}
	for k, v := range want {
		if a[k] != v {
			t.Errorf("activity.%s = %v, want %v", k, a[k], v)
		}
	}
	ts := a["timestamps"].(map[string]any)
	if ts["start"] != float64(start.UnixMilli()) || ts["end"] != float64(start.Add(259*time.Second).UnixMilli()) {
		t.Errorf("timestamps = %v", ts)
	}
	assets := a["assets"].(map[string]any)
	if assets["large_image"] != act.ArtworkURL || assets["large_text"] != "Abbey Road" {
		t.Errorf("assets = %v", assets)
	}

	// A ping arriving before the reply is answered with the same payload.
	writeFrame(t, conn, opPing, map[string]any{"n": 7})
	if op, m := readFrame(t, conn); op != opPong || m["n"] != float64(7) {
		t.Fatalf("pong op %d = %v", op, m)
	}
	// Replies to other nonces are not this command's.
	writeFrame(t, conn, opFrame, map[string]any{"cmd": "SET_ACTIVITY", "nonce": "other", "evt": "ERROR", "data": map[string]any{"message": "x"}})
	writeFrame(t, conn, opFrame, map[string]any{"cmd": "SET_ACTIVITY", "nonce": nonce, "data": map[string]any{}})
	if err := <-errc; err != nil {
		t.Fatalf("SetActivity: %v", err)
	}
	if !c.Connected() {
		t.Error("not connected after a successful SetActivity")
	}

	// Clearing sends no activity on the same connection.
	go func() { errc <- c.SetActivity(ctx(t), nil) }()
	_, m = readFrame(t, conn)
	if _, ok := m["args"].(map[string]any)["activity"]; ok {
		t.Errorf("clear sent an activity: %v", m)
	}
	writeFrame(t, conn, opFrame, map[string]any{"cmd": "SET_ACTIVITY", "nonce": m["nonce"]})
	if err := <-errc; err != nil {
		t.Fatalf("clear: %v", err)
	}
}

func TestSetActivityReportsDiscordError(t *testing.T) {
	f := listen(t)
	c := New("1")
	errc := make(chan error, 1)
	go func() { errc <- c.SetActivity(ctx(t), &Activity{Title: "Song", Artist: "Artist"}) }()
	conn := f.accept()
	handshake(t, conn)
	_, m := readFrame(t, conn)
	writeFrame(t, conn, opFrame, map[string]any{
		"cmd": "SET_ACTIVITY", "nonce": m["nonce"], "evt": "ERROR",
		"data": map[string]any{"code": 4000, "message": "child \"activity\" fails"},
	})
	err := <-errc
	if err == nil || !strings.Contains(err.Error(), "child \"activity\" fails") {
		t.Fatalf("err = %v, want Discord's message", err)
	}
	if !c.Connected() {
		t.Error("a rejected payload dropped a healthy connection")
	}
}

func TestHandshakeRefused(t *testing.T) {
	f := listen(t)
	c := New("bad")
	errc := make(chan error, 1)
	go func() { errc <- c.SetActivity(ctx(t), nil) }()
	conn := f.accept()
	readFrame(t, conn)
	writeFrame(t, conn, opClose, map[string]any{"code": 4000, "message": "Invalid Client ID"})
	if err := <-errc; err == nil || !strings.Contains(err.Error(), "Invalid Client ID") {
		t.Fatalf("err = %v", err)
	}
	if c.Connected() {
		t.Error("connected after a refused handshake")
	}
}

func TestRedialsAfterDiscordRestart(t *testing.T) {
	f := listen(t)
	c := New("1")
	errc := make(chan error, 1)
	go func() { errc <- c.SetActivity(ctx(t), nil) }()
	conn := f.accept()
	handshake(t, conn)
	_, m := readFrame(t, conn)
	writeFrame(t, conn, opFrame, map[string]any{"cmd": "SET_ACTIVITY", "nonce": m["nonce"]})
	if err := <-errc; err != nil {
		t.Fatal(err)
	}

	_ = conn.Close() // Discord quits; a new instance listens
	go func() { errc <- c.SetActivity(ctx(t), &Activity{Title: "Song", Artist: "Artist"}) }()
	conn = f.accept()
	handshake(t, conn)
	_, m = readFrame(t, conn)
	if m["cmd"] != "SET_ACTIVITY" {
		t.Fatalf("after redial: %v", m)
	}
	writeFrame(t, conn, opFrame, map[string]any{"cmd": "SET_ACTIVITY", "nonce": m["nonce"]})
	if err := <-errc; err != nil {
		t.Fatalf("SetActivity after restart: %v", err)
	}
}

func TestFindsFlatpakSocket(t *testing.T) {
	dir := socketDir(t)
	for _, env := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		t.Setenv(env, dir)
	}
	sub := filepath.Join(dir, "app", "com.discordapp.Discord")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	f := listenAt(t, filepath.Join(sub, "discord-ipc-3"))
	c := New("1")
	errc := make(chan error, 1)
	go func() { errc <- c.SetActivity(ctx(t), nil) }()
	conn := f.accept()
	handshake(t, conn)
	_, m := readFrame(t, conn)
	writeFrame(t, conn, opFrame, map[string]any{"cmd": "SET_ACTIVITY", "nonce": m["nonce"]})
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
}

func TestNotRunningAndNotConfigured(t *testing.T) {
	if err := New("").SetActivity(ctx(t), nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("empty client ID: err = %v", err)
	}
	c := New("1")
	c.paths = func() []string { return []string{filepath.Join(t.TempDir(), "discord-ipc-0")} }
	if err := c.SetActivity(ctx(t), nil); err == nil || c.Connected() {
		t.Fatalf("no Discord: err = %v, connected %v", err, c.Connected())
	}
}

// Only a socket of this user's, in a directory only it controls, is
// dialled: not a regular file, a symlink, another user's socket, or one
// in the shared temp directory while a runtime directory exists.
func TestIgnoresUntrustedSockets(t *testing.T) {
	run, tmp := socketDir(t), socketDir(t)
	t.Setenv("XDG_RUNTIME_DIR", run)
	for _, env := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(env, tmp)
	}
	listenAt(t, filepath.Join(tmp, "discord-ipc-0"))
	if err := os.WriteFile(filepath.Join(run, "discord-ipc-1"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(tmp, "discord-ipc-0"), filepath.Join(run, "discord-ipc-2")); err != nil {
		t.Fatal(err)
	}
	if got := socketPaths(); len(got) != 0 {
		t.Fatalf("socketPaths = %v, want none", got)
	}

	own := filepath.Join(run, "discord-ipc-3")
	listenAt(t, own)
	if got := socketPaths(); len(got) != 1 || got[0] != own {
		t.Fatalf("socketPaths = %v, want [%s]", got, own)
	}
	// The same socket, seen as another user's.
	if got := findSockets([]string{run}, os.Getuid()+1); len(got) != 0 {
		t.Fatalf("another user's socket accepted: %v", got)
	}

	// No runtime directory: the temp directories are the fallback (TMPDIR
	// first; a real Discord may add /tmp's own socket after it).
	t.Setenv("XDG_RUNTIME_DIR", "")
	if got := socketPaths(); len(got) == 0 || got[0] != filepath.Join(tmp, "discord-ipc-0") {
		t.Fatalf("socketPaths without XDG_RUNTIME_DIR = %v", got)
	}
}

func TestFieldLimits(t *testing.T) {
	for _, s := range []string{"", "X", "Ü", strings.Repeat("a", 200), strings.Repeat("é", 129)} {
		got := field(s)
		n := utf8.RuneCountInString(got)
		if n < 2 || n > 128 {
			t.Errorf("field(%d runes) = %d runes", utf8.RuneCountInString(s), n)
		}
	}
	if got := field("Come Together"); got != "Come Together" {
		t.Errorf("field changed a valid value: %q", got)
	}
}

// socketDir is a folder for a test's socket. Not t.TempDir: its long
// names can exceed the 104 bytes macOS allows a socket path. Not the default
// temporary directory either on macOS: its /var/folders/… (49 bytes on a CI
// runner) leaves too little for the nested Flatpak path of
// TestFindsFlatpakSocket, so the folder is under /tmp where that exists.
func socketDir(t *testing.T) string {
	t.Helper()
	base := ""
	if st, err := os.Stat("/tmp"); err == nil && st.IsDir() {
		base = "/tmp"
	}
	dir, err := os.MkdirTemp(base, "sona")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
