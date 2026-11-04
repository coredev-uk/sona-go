// Package app holds process-level integration: single instance and the
// `sona <command>` transport (the D-Bus session bus on Linux, a local
// socket on macOS and Windows), parsing of the Apple Music links Sona is
// launched with, the XDG autostart entry and the xdg-desktop-portal file
// chooser.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/devinstance"
)

const (
	// appID is the application ID, the base of BusName.
	appID = "dev.coredev.Sona"
	// ObjectPath exports Interface on the primary instance.
	ObjectPath dbus.ObjectPath = "/dev/coredev/Sona"
	// Interface carries Activate(as args, a{sv} platform_data),
	// Run(s request) → (s reply) and the StatusChanged(s status) signal
	// (command.go).
	Interface = "dev.coredev.Sona.Instance"

	// Platform data keys sent with Activate.
	keyActivationToken = "activation-token"
	keyWorkingDir      = "cwd"

	// activationBuffer is how many activations queue before new ones drop.
	activationBuffer = 16
)

// BusName is owned by the running (primary) instance. It matches the
// application ID; a development build run with SONA_DEV_INSTANCE=<name>
// appends ".instance_<name>" (internal/devinstance), so two worktrees each
// have their own primary instance and `sona status` reaches the one the
// same variable names. A production build never has a suffix.
var BusName = appID + devinstance.Suffix()

// ErrForwarded is returned by Acquire when another instance owns BusName and
// accepted the activation. The caller should exit successfully.
var ErrForwarded = errors.New("app: forwarded to the running instance")

// Activation is one launch of Sona: its arguments (URLs included) and the
// launch context the primary instance needs to focus its window.
type Activation struct {
	Args []string
	// ActivationToken is the launcher's XDG_ACTIVATION_TOKEN (Wayland) or
	// DESKTOP_STARTUP_ID (X11); empty when launched without one.
	ActivationToken string
	WorkingDir      string
}

// CurrentActivation describes this process's launch: args (usually
// os.Args[1:]), the activation token from the environment and the working
// directory.
func CurrentActivation(args []string) Activation {
	tok := os.Getenv("XDG_ACTIVATION_TOKEN")
	if tok == "" {
		tok = os.Getenv("DESKTOP_STARTUP_ID")
	}
	wd, _ := os.Getwd()
	return Activation{Args: args, ActivationToken: tok, WorkingDir: wd}
}

// Instance is the primary instance's claim on BusName.
type Instance struct {
	conn *dbus.Conn
	log  *slog.Logger

	mu     sync.Mutex
	ch     chan Activation
	closed bool
	// server answers commands (nil until HandleCommands); stopWatch ends
	// its status updates and watching closes once the last is published.
	server    cli.Server
	stopWatch func()
	watching  chan struct{}

	closeOnce sync.Once
	closeErr  error
}

const introspectXML = `<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN"
 "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
  <interface name="org.freedesktop.DBus.Introspectable">
    <method name="Introspect">
      <arg name="xml_data" type="s" direction="out"/>
    </method>
  </interface>
  <interface name="` + Interface + `">
    <method name="Activate">
      <arg name="args" type="as" direction="in"/>
      <arg name="platform_data" type="a{sv}" direction="in"/>
    </method>
    <method name="Run">
      <arg name="request" type="s" direction="in"/>
      <arg name="reply" type="s" direction="out"/>
    </method>
    <signal name="StatusChanged">
      <arg name="status" type="s"/>
    </signal>
  </interface>
</node>`

// Acquire makes this process the primary instance, or forwards act to the
// one already running.
//
// If BusName is free it is claimed and the returned Instance delivers later
// launches on Activations. If another process owns it, act is sent to that
// process's Activate method and Acquire returns (nil, ErrForwarded). An owner
// that vanishes mid-handshake is retried. ctx bounds the forwarding call.
// conn stays owned by the caller and must outlive the Instance. A nil log
// discards output.
func Acquire(ctx context.Context, conn *dbus.Conn, act Activation, log *slog.Logger) (*Instance, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	in := &Instance{conn: conn, log: log, ch: make(chan Activation, activationBuffer)}
	// Export first so Activate works the moment the name is ours.
	if err := conn.ExportMethodTable(map[string]any{"Activate": in.activate, "Run": in.run}, ObjectPath, Interface); err != nil {
		return nil, fmt.Errorf("app: export: %w", err)
	}
	if err := conn.Export(introspect.Introspectable(introspectXML), ObjectPath, "org.freedesktop.DBus.Introspectable"); err != nil {
		in.unexport()
		return nil, fmt.Errorf("app: export: %w", err)
	}

	var lastErr error
	for range 3 {
		reply, err := conn.RequestName(BusName, dbus.NameFlagDoNotQueue)
		if err != nil {
			in.unexport()
			return nil, fmt.Errorf("app: request %s: %w", BusName, err)
		}
		if reply == dbus.RequestNameReplyPrimaryOwner || reply == dbus.RequestNameReplyAlreadyOwner {
			return in, nil
		}
		err = forward(ctx, conn, act)
		if err == nil {
			in.unexport()
			return nil, ErrForwarded
		}
		lastErr = err
		var dErr dbus.Error
		if !errors.As(err, &dErr) ||
			(dErr.Name != "org.freedesktop.DBus.Error.ServiceUnknown" && dErr.Name != "org.freedesktop.DBus.Error.NameHasNoOwner") {
			break
		}
	}
	in.unexport()
	return nil, fmt.Errorf("app: forward to running instance: %w", lastErr)
}

func forward(ctx context.Context, conn *dbus.Conn, act Activation) error {
	args := act.Args
	if args == nil {
		args = []string{}
	}
	platform := map[string]dbus.Variant{}
	if act.ActivationToken != "" {
		platform[keyActivationToken] = dbus.MakeVariant(act.ActivationToken)
	}
	if act.WorkingDir != "" {
		platform[keyWorkingDir] = dbus.MakeVariant(act.WorkingDir)
	}
	return conn.Object(BusName, ObjectPath).
		CallWithContext(ctx, Interface+".Activate", dbus.FlagNoAutoStart, args, platform).Err
}

// activate handles a forwarded launch. It never blocks the caller: when the
// buffer is full the activation is dropped and logged.
func (in *Instance) activate(args []string, platform map[string]dbus.Variant) *dbus.Error {
	act := Activation{Args: args}
	act.ActivationToken, _ = platform[keyActivationToken].Value().(string)
	act.WorkingDir, _ = platform[keyWorkingDir].Value().(string)

	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closed {
		return dbus.NewError("org.freedesktop.DBus.Error.Failed", []any{"Sona is shutting down"})
	}
	select {
	case in.ch <- act:
	default:
		in.log.Warn("app: activation dropped, queue full", "args", len(args))
	}
	return nil
}

// Activations delivers launches forwarded by later instances. It is closed
// by Close.
func (in *Instance) Activations() <-chan Activation { return in.ch }

// Close releases BusName, stops accepting activations and commands, ends
// the status signals and closes the Activations channel. Safe to call
// more than once.
func (in *Instance) Close() error {
	in.closeOnce.Do(func() {
		in.mu.Lock()
		in.closed = true
		close(in.ch)
		stop, watching := in.stopWatch, in.watching
		in.mu.Unlock()
		if stop != nil {
			stop()
			<-watching
		}
		if _, err := in.conn.ReleaseName(BusName); err != nil {
			in.closeErr = fmt.Errorf("app: release %s: %w", BusName, err)
		}
		in.unexport()
	})
	return in.closeErr
}

func (in *Instance) unexport() {
	_ = in.conn.Export(nil, ObjectPath, Interface)
	_ = in.conn.Export(nil, ObjectPath, "org.freedesktop.DBus.Introspectable")
}
