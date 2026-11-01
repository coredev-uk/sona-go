package app

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
)

func acquireSocket(t *testing.T, dir string, act Activation) (*SocketInstance, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return AcquireSocket(ctx, dir, act, nil)
}

// socketDir is a folder for a test's socket. Not t.TempDir: its long
// names can exceed the 104 bytes macOS allows a socket path.
func socketDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "sona")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestSocketSingleInstance(t *testing.T) {
	dir := socketDir(t)
	primary, err := acquireSocket(t, dir, Activation{Args: []string{"--first"}})
	if err != nil {
		t.Fatalf("primary: %v", err)
	}
	t.Cleanup(func() { _ = primary.Close() })
	if fi, err := os.Stat(filepath.Join(dir, SocketName)); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("socket file = %v, %v; want mode 0600", fi, err)
	}

	second := Activation{
		Args:            []string{"music://music.apple.com/us/album/abbey-road/1441164426?i=1441164430"},
		ActivationToken: "tok-123",
		WorkingDir:      "/tmp",
	}
	if inst, err := acquireSocket(t, dir, second); !errors.Is(err, ErrForwarded) || inst != nil {
		t.Fatalf("second = %v, %v; want ErrForwarded", inst, err)
	}
	select {
	case got := <-primary.Activations():
		if !reflect.DeepEqual(got, second) {
			t.Errorf("activation = %+v, want %+v", got, second)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("primary received no activation")
	}
	if _, err := acquireSocket(t, dir, Activation{}); !errors.Is(err, ErrForwarded) {
		t.Fatalf("empty forward: %v", err)
	}
	if got := <-primary.Activations(); len(got.Args) != 0 || got.ActivationToken != "" || got.WorkingDir != "" {
		t.Errorf("empty activation = %+v", got)
	}

	// After the primary quits, its socket is gone and the next launch is
	// primary.
	if err := primary.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := <-primary.Activations(); ok {
		t.Error("Activations not closed")
	}
	if _, err := os.Stat(filepath.Join(dir, SocketName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("socket left behind: %v", err)
	}
	next, err := acquireSocket(t, dir, Activation{})
	if err != nil {
		t.Fatalf("after Close: %v", err)
	}
	_ = next.Close()
}

// A socket file left by a crashed Sona (nobody listens, nobody holds the
// lock) reads as not running, and the next launch replaces it.
func TestSocketStale(t *testing.T) {
	dir := socketDir(t)
	path := filepath.Join(dir, SocketName)
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	_ = ln.Close()

	client, err := DialSocket(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(context.Background(), cli.Request{Command: "status"}); !errors.Is(err, cli.ErrNotRunning) {
		t.Errorf("stale socket: err = %v, want ErrNotRunning", err)
	}
	primary, err := acquireSocket(t, dir, Activation{})
	if err != nil {
		t.Fatalf("acquire over a stale socket: %v", err)
	}
	_ = primary.Close()
}

func TestSocketCommands(t *testing.T) {
	dir := socketDir(t)
	if _, err := DialSocket(dir); !errors.Is(err, cli.ErrNotRunning) {
		t.Fatalf("no instance: err = %v, want ErrNotRunning", err)
	}
	primary, err := acquireSocket(t, dir, Activation{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = primary.Close() })
	client, err := DialSocket(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := commandContract(t, client, primary.HandleCommands)
	select {
	case a := <-primary.Activations():
		t.Errorf("command delivered as an activation: %+v", a)
	default:
	}

	// A client that never sends its line doesn't hold up the others.
	idle, err := net.Dial("unix", filepath.Join(dir, SocketName))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = idle.Close() }()
	if _, err := client.Call(context.Background(), cli.Request{Command: "status"}); err != nil {
		t.Errorf("with an idle connection open: %v", err)
	}

	followUntilQuit(t, client, srv, func() {
		if err := primary.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := client.Call(context.Background(), cli.Request{Command: "status"}); !errors.Is(err, cli.ErrNotRunning) {
		t.Errorf("after Close: err = %v, want ErrNotRunning", err)
	}
}

// Cancelling the context ends a call that waits on a slow command.
func TestSocketCallCancel(t *testing.T) {
	dir := socketDir(t)
	primary, err := acquireSocket(t, dir, Activation{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = primary.Close() })
	block := make(chan struct{})
	t.Cleanup(func() { close(block) })
	primary.HandleCommands(slowServer{block})
	client, _ := DialSocket(dir)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := client.Call(ctx, cli.Request{Command: "search", Args: []string{"x"}}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the deadline", err)
	}
}

type slowServer struct{ block chan struct{} }

func (s slowServer) Call(ctx context.Context, _ cli.Request) (any, error) {
	select {
	case <-s.block:
	case <-ctx.Done():
	}
	return nil, errors.New("too late")
}

func (slowServer) Watch() (<-chan cli.Status, func()) {
	ch := make(chan cli.Status)
	return ch, func() {}
}

// The peer check accepts this process's own user.
func TestPeerIsUser(t *testing.T) {
	path := filepath.Join(socketDir(t), "s")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		c, err := net.Dial("unix", path)
		if err == nil {
			time.Sleep(100 * time.Millisecond)
			_ = c.Close()
		}
	}()
	c, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := peerIsUser(c); err != nil {
		t.Errorf("peerIsUser(own process) = %v", err)
	}
	if err := peerIsUser(&net.TCPConn{}); err == nil {
		t.Error("peerIsUser accepted a connection that isn't a Unix socket")
	}
}
