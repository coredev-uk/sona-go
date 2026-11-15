package discord

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// pipeServer is one instance of a named pipe this test serves, as Discord
// serves \\.\pipe\discord-ipc-N.
func pipeServer(t *testing.T) (name string, server *os.File) {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	name = `\\.\pipe\sona-discord-test-` + hex.EncodeToString(b[:])
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateNamedPipe(p, windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT, 1, 4096, 4096, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	server = os.NewFile(uintptr(h), name)
	t.Cleanup(func() { _ = server.Close() })
	return name, server
}

// accept waits for the client. A client that came first, or came and went,
// is no error.
func accept(server *os.File) error {
	err := windows.ConnectNamedPipe(windows.Handle(server.Fd()), nil)
	if err != nil && !errors.Is(err, windows.ERROR_PIPE_CONNECTED) && !errors.Is(err, windows.ERROR_NO_DATA) {
		return fmt.Errorf("ConnectNamedPipe: %w", err)
	}
	return nil
}

// readFrame reads one frame; m is its payload when it is a JSON frame.
func readFrame(r io.Reader) (op uint32, m message, err error) {
	var hdr [8]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, m, err
	}
	data := make([]byte, binary.LittleEndian.Uint32(hdr[4:]))
	if _, err := io.ReadFull(r, data); err != nil {
		return 0, m, err
	}
	op = binary.LittleEndian.Uint32(hdr[:4])
	if op == opFrame {
		err = json.Unmarshal(data, &m)
	}
	return op, m, err
}

func writeFrame(w io.Writer, m message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	buf := binary.LittleEndian.AppendUint32(nil, opFrame)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(data)))
	_, err = w.Write(append(buf, data...))
	return err
}

// serveDiscord answers a handshake with READY and one command with its
// nonce, and reports the command (or what went wrong).
func serveDiscord(server *os.File) (string, error) {
	if err := accept(server); err != nil {
		return "", err
	}
	if op, _, err := readFrame(server); err != nil || op != opHandshake {
		return "", fmt.Errorf("handshake: op %d, %v", op, err)
	}
	if err := writeFrame(server, message{Cmd: "DISPATCH", Evt: "READY"}); err != nil {
		return "", err
	}
	_, m, err := readFrame(server)
	if err != nil {
		return "", err
	}
	return m.Cmd, writeFrame(server, message{Cmd: m.Cmd, Nonce: m.Nonce})
}

// A pipe of the user's own carries the handshake and the activity, within
// the deadline of the caller's context.
func TestSetActivityOverOwnPipe(t *testing.T) {
	name, server := pipeServer(t)
	type served struct {
		cmd string
		err error
	}
	done := make(chan served, 1)
	go func() {
		cmd, err := serveDiscord(server)
		done <- served{cmd, err}
	}()
	c := &Client{clientID: "123", paths: func() []string { return []string{name} }}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := c.SetActivity(ctx, &Activity{Title: "Song", Artist: "Artist"}); err != nil {
		t.Fatalf("SetActivity: %v", err)
	}
	if got := <-done; got.err != nil || got.cmd != "SET_ACTIVITY" {
		t.Fatalf("Discord got %q (%v)", got.cmd, got.err)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

// A Discord that stops answering doesn't hold the client past its context.
func TestPipeReadHonoursTheDeadline(t *testing.T) {
	name, server := pipeServer(t)
	go func() { _ = accept(server) }() // and never answers the handshake
	c := &Client{clientID: "123", paths: func() []string { return []string{name} }}
	ctx, cancel := context.WithTimeout(t.Context(), 500*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.SetActivity(ctx, nil) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("SetActivity succeeded with no answer")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("SetActivity ignored its deadline on a pipe")
	}
}

// A pipe served by another user's process gets nothing: not the handshake,
// not the track.
func TestRefusesAnotherUsersPipe(t *testing.T) {
	name, server := pipeServer(t)
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		t.Fatal(err)
	}
	other := func(uint32) (*windows.SID, error) { return system, nil }
	if _, err := dialPipe(t.Context(), name, other); !errors.Is(err, errNotOwnPipe) {
		t.Fatalf("dialPipe = %v, want errNotOwnPipe", err)
	}
	// The refused client has closed its end: the server reads the end of
	// the pipe, with not a byte before it.
	if err := accept(server); err != nil {
		t.Fatal(err)
	}
	if got, err := io.ReadAll(server); len(got) != 0 {
		t.Fatalf("the other user's server read %q (%v)", got, err)
	}
}

// The lookup the client uses finds this process's own user.
func TestProcessUserIsTheCurrentUser(t *testing.T) {
	got, err := processUser(windows.GetCurrentProcessId())
	if err != nil {
		t.Fatal(err)
	}
	self, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if !windows.EqualSid(got, self.User.Sid) {
		t.Fatalf("processUser = %v, want %v", got, self.User.Sid)
	}
}

func TestNoDiscordRunning(t *testing.T) {
	c := &Client{clientID: "123", paths: func() []string { return []string{`\\.\pipe\sona-discord-test-absent`} }}
	if err := c.SetActivity(t.Context(), nil); err == nil || c.Connected() {
		t.Fatalf("SetActivity = %v, connected %v: want not running", err, c.Connected())
	}
}
