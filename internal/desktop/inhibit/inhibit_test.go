package inhibit

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/app/dbustest"
)

// fakeLogind hands out the write end of a pipe per Inhibit call; the lock
// is released once every copy of that end is closed, which the test sees
// as EOF on the read end.
type fakeLogind struct {
	t *testing.T

	mu    sync.Mutex
	modes []string
	what  []string
	ends  []*os.File // read ends, one per lock
	srv   []*os.File // the server's copies of the write ends
}

func (f *fakeLogind) Inhibit(what, who, why, mode string) (dbus.UnixFD, *dbus.Error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modes = append(f.modes, mode)
	f.what = append(f.what, what)
	r, w, err := os.Pipe()
	if err != nil {
		f.t.Error(err)
		return -1, dbus.MakeFailedError(err)
	}
	f.ends = append(f.ends, r)
	f.srv = append(f.srv, w)
	return dbus.UnixFD(w.Fd()), nil
}

// dropServerCopies closes the server's write ends (logind keeps its own
// copy only in the lock bookkeeping, not as a holder).
func (f *fakeLogind) dropServerCopies() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, w := range f.srv {
		_ = w.Close()
	}
	f.srv = nil
}

// held reports whether lock i's write end is still open somewhere.
func (f *fakeLogind) held(t *testing.T, i int) bool {
	t.Helper()
	f.mu.Lock()
	r := f.ends[i]
	f.mu.Unlock()
	_ = r.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
	_, err := r.Read(make([]byte, 1))
	switch {
	case errors.Is(err, os.ErrDeadlineExceeded):
		return true
	case errors.Is(err, io.EOF):
		return false
	default:
		t.Fatalf("read lock end: %v", err)
		return false
	}
}

// fakePortal records Inhibit calls and Request.Close on their handles.
type fakePortal struct {
	conn *dbus.Conn

	mu     sync.Mutex
	flags  []uint32
	closed []dbus.ObjectPath
}

type fakeRequest struct {
	p    *fakePortal
	path dbus.ObjectPath
}

func (r fakeRequest) Close() *dbus.Error {
	r.p.mu.Lock()
	r.p.closed = append(r.p.closed, r.path)
	r.p.mu.Unlock()
	return nil
}

func (p *fakePortal) Inhibit(_ string, flags uint32, opts map[string]dbus.Variant) (dbus.ObjectPath, *dbus.Error) {
	tok, _ := opts["handle_token"].Value().(string)
	path := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1_1/" + tok)
	if err := p.conn.Export(fakeRequest{p, path}, path, requestIfc); err != nil {
		return "", dbus.MakeFailedError(err)
	}
	p.mu.Lock()
	p.flags = append(p.flags, flags)
	p.mu.Unlock()
	return path, nil
}

func own(t *testing.T, conn *dbus.Conn, name string) {
	t.Helper()
	if reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request %s: %v %v", name, reply, err)
	}
}

func startLogind(t *testing.T, addr string) *fakeLogind {
	t.Helper()
	conn := dbustest.Conn(t, addr)
	f := &fakeLogind{t: t}
	if err := conn.Export(f, logindPath, logindIfc); err != nil {
		t.Fatal(err)
	}
	own(t, conn, logindName)
	return f
}

func startPortal(t *testing.T, addr string) *fakePortal {
	t.Helper()
	conn := dbustest.Conn(t, addr)
	p := &fakePortal{conn: conn}
	if err := conn.Export(p, portalPath, portalIfc); err != nil {
		t.Fatal(err)
	}
	own(t, conn, portalName)
	return p
}

func dialer(t *testing.T, addr string) func() (*dbus.Conn, error) {
	return func() (*dbus.Conn, error) { return dbus.Connect(addr) }
}

func TestLogindLockHeldUntilRelease(t *testing.T) {
	addr := dbustest.Start(t)
	logind := startLogind(t, addr)
	in := New(Options{System: dialer(t, addr), Who: "Sona", Why: "Playing music"})
	t.Cleanup(func() { _ = in.Close() })

	ctx := context.Background()
	if err := in.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	if err := in.Acquire(ctx); err != nil { // already held: no second lock
		t.Fatal(err)
	}
	logind.dropServerCopies()
	if in.Backend() != BackendLogind {
		t.Fatalf("backend = %q", in.Backend())
	}
	logind.mu.Lock()
	locks, what, modes := len(logind.ends), logind.what, logind.modes
	logind.mu.Unlock()
	if locks != 1 || what[0] != "sleep" || modes[0] != "block" {
		t.Fatalf("locks %d, what %v, modes %v", locks, what, modes)
	}
	if !logind.held(t, 0) {
		t.Fatal("lock not held after Acquire")
	}
	if err := in.Release(); err != nil {
		t.Fatal(err)
	}
	if logind.held(t, 0) {
		t.Fatal("lock still held after Release")
	}
	if in.Backend() != "" {
		t.Errorf("backend after release = %q", in.Backend())
	}

	// Acquire again after a release takes a new lock.
	if err := in.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	logind.dropServerCopies()
	if !logind.held(t, 1) {
		t.Fatal("second lock not held")
	}
	if err := in.Close(); err != nil {
		t.Fatal(err)
	}
	if logind.held(t, 1) {
		t.Fatal("lock held after Close")
	}
}

func TestPortalFallback(t *testing.T) {
	addr := dbustest.Start(t)
	portal := startPortal(t, addr) // no logind on this bus
	in := New(Options{System: dialer(t, addr), Session: dbustest.Conn(t, addr), Who: "Sona", Why: "Playing music"})
	t.Cleanup(func() { _ = in.Close() })
	if err := in.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	if in.Backend() != BackendPortal {
		t.Fatalf("backend = %q, want portal", in.Backend())
	}
	portal.mu.Lock()
	flags := portal.flags
	portal.mu.Unlock()
	if len(flags) != 1 || flags[0] != portalSuspend {
		t.Fatalf("flags = %v", flags)
	}
	if err := in.Release(); err != nil {
		t.Fatal(err)
	}
	portal.mu.Lock()
	defer portal.mu.Unlock()
	if len(portal.closed) != 1 {
		t.Fatalf("request closed %d times, want 1", len(portal.closed))
	}
}

func TestNothingAvailable(t *testing.T) {
	addr := dbustest.Start(t)
	in := New(Options{System: dialer(t, addr), Session: dbustest.Conn(t, addr), Who: "Sona", Why: "x"})
	t.Cleanup(func() { _ = in.Close() })
	if err := in.Acquire(context.Background()); err == nil {
		t.Fatal("Acquire succeeded with neither logind nor the portal")
	}
	if in.Backend() != "" {
		t.Errorf("backend = %q", in.Backend())
	}
}
