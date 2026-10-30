// Package inhibit keeps the system from suspending while it is held:
// through a systemd-logind inhibitor lock ("sleep"), or, where logind is
// unreachable (sandboxes, non-systemd systems), the XDG desktop portal's
// Inhibit interface.
package inhibit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
)

// Backend names reported by [Inhibitor.Backend].
const (
	BackendLogind = "logind"
	BackendPortal = "portal"
)

const (
	logindName = "org.freedesktop.login1"
	logindPath = dbus.ObjectPath("/org/freedesktop/login1")
	logindIfc  = "org.freedesktop.login1.Manager"

	portalName = "org.freedesktop.portal.Desktop"
	portalPath = dbus.ObjectPath("/org/freedesktop/portal/desktop")
	portalIfc  = "org.freedesktop.portal.Inhibit"
	requestIfc = "org.freedesktop.portal.Request"

	// portalSuspend is the portal's "Suspend" inhibit flag.
	portalSuspend uint32 = 4
)

// Options configure an Inhibitor.
type Options struct {
	// System dials the system bus for logind (dbus.ConnectSystemBus). It is
	// called on the first Acquire; the connection is closed by Close. Nil
	// skips logind.
	System func() (*dbus.Conn, error)
	// Session is the session bus for the portal fallback (owned by the
	// caller). Nil skips the portal.
	Session *dbus.Conn
	Who     string // the application's name
	Why     string // the reason shown in inhibitor lists
}

// Inhibitor holds at most one suspend inhibitor. Its methods are safe for
// concurrent use.
type Inhibitor struct {
	opts Options

	mu      sync.Mutex
	system  *dbus.Conn
	held    lock // nil when released
	backend string
}

type lock interface{ release() error }

// New returns a released Inhibitor.
func New(opts Options) *Inhibitor { return &Inhibitor{opts: opts} }

// Acquire takes the inhibitor unless it is already held: a logind
// "sleep" lock, else the portal's suspend inhibition.
func (in *Inhibitor) Acquire(ctx context.Context) error {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.held != nil {
		return nil
	}
	var errs []error
	if in.opts.System != nil {
		l, err := in.logind(ctx)
		if err == nil {
			in.held, in.backend = l, BackendLogind
			return nil
		}
		errs = append(errs, err)
	}
	if in.opts.Session != nil {
		l, err := portalInhibit(ctx, in.opts.Session, in.opts.Why)
		if err == nil {
			in.held, in.backend = l, BackendPortal
			return nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		return errors.New("inhibit: no bus to inhibit suspend on")
	}
	return errors.Join(errs...)
}

// Release drops the inhibitor if it is held.
func (in *Inhibitor) Release() error {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.held == nil {
		return nil
	}
	err := in.held.release()
	in.held, in.backend = nil, ""
	return err
}

// Backend reports the backend of the inhibitor held, "" when released.
func (in *Inhibitor) Backend() string {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.backend
}

// Close releases the inhibitor and closes the system bus connection.
func (in *Inhibitor) Close() error {
	err := in.Release()
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.system != nil {
		err = errors.Join(err, in.system.Close())
		in.system = nil
	}
	return err
}

// logind takes a logind "sleep" lock in "block" mode. ("block-weak" is not
// enforced against the lock owner's own user, and idle daemons such as
// hypridle suspend as that user, so it would not keep music playing.)
func (in *Inhibitor) logind(ctx context.Context) (lock, error) {
	if in.system == nil {
		conn, err := in.opts.System()
		if err != nil {
			return nil, fmt.Errorf("inhibit: system bus: %w", err)
		}
		in.system = conn
	}
	var fd dbus.UnixFD
	err := in.system.Object(logindName, logindPath).
		CallWithContext(ctx, logindIfc+".Inhibit", 0, "sleep", in.opts.Who, in.opts.Why, "block").
		Store(&fd)
	if err != nil {
		// A broken connection (e.g. the system bus restarted) is redialled
		// next time.
		_ = in.system.Close()
		in.system = nil
		return nil, fmt.Errorf("inhibit: logind: %w", err)
	}
	if fd < 0 {
		return nil, errors.New("inhibit: logind returned no file descriptor")
	}
	return fdLock{os.NewFile(uintptr(fd), "logind-inhibit")}, nil
}

// fdLock is a logind lock: it lasts until its file descriptor is closed.
type fdLock struct{ f *os.File }

func (l fdLock) release() error { return l.f.Close() }

var tokens atomic.Uint64

// portalInhibit asks the desktop portal to inhibit suspend.
func portalInhibit(ctx context.Context, conn *dbus.Conn, why string) (lock, error) {
	opts := map[string]dbus.Variant{
		"reason":       dbus.MakeVariant(why),
		"handle_token": dbus.MakeVariant("sona_inhibit_" + strconv.FormatUint(tokens.Add(1), 10)),
	}
	var handle dbus.ObjectPath
	err := conn.Object(portalName, portalPath).CallWithContext(ctx, portalIfc+".Inhibit", 0, "", portalSuspend, opts).Store(&handle)
	if err != nil {
		return nil, fmt.Errorf("inhibit: portal: %w", err)
	}
	return portalLock{conn, handle}, nil
}

// portalLock lasts until its request object is closed.
type portalLock struct {
	conn   *dbus.Conn
	handle dbus.ObjectPath
}

func (l portalLock) release() error {
	if err := l.conn.Object(portalName, l.handle).Call(requestIfc+".Close", 0).Err; err != nil {
		return fmt.Errorf("inhibit: portal release: %w", err)
	}
	return nil
}
