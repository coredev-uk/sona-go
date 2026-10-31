package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/godbus/dbus/v5"
)

// Commands are the `sona <command> …` subcommands. A launch whose first
// argument names one never starts Sona: it sends its arguments to the
// running instance's Command method, prints the reply and exits. Adding one
// is a name here and a handler in the Commands map main gives
// Instance.HandleCommands.
var commandNames = []string{"private"}

// Commands maps a command name to its handler in the running instance. A
// handler gets the arguments after the name and returns the text to print.
type Commands map[string]func(args []string) (string, error)

// ErrNotRunning is returned by SendCommand when no Sona owns BusName.
var ErrNotRunning = errors.New("no running Sona")

// commandTimeout bounds a command's round trip.
const commandTimeout = 10 * time.Second

// errCommand is the D-Bus error a failed command replies with; its body is
// the message to print.
const errCommand = Interface + ".Error.Failed"

// IsCommand reports whether args (os.Args[1:]) are a subcommand.
func IsCommand(args []string) bool {
	return len(args) > 0 && slices.Contains(commandNames, args[0])
}

// HandleCommands installs the instance's command handlers. Until it is
// called, commands fail with "Sona is starting".
func (in *Instance) HandleCommands(c Commands) {
	in.mu.Lock()
	in.commands = c
	in.mu.Unlock()
}

// command is the exported Command(as args) → (s reply) method.
func (in *Instance) command(args []string) (string, *dbus.Error) {
	in.mu.Lock()
	c, closed := in.commands, in.closed
	in.mu.Unlock()
	switch {
	case closed:
		return "", dbus.NewError(errCommand, []any{"Sona is shutting down"})
	case len(args) == 0:
		return "", dbus.NewError(errCommand, []any{"no command"})
	case c == nil:
		return "", dbus.NewError(errCommand, []any{"Sona is starting; try again"})
	}
	run, ok := c[args[0]]
	if !ok {
		return "", dbus.NewError(errCommand, []any{fmt.Sprintf("unknown command %q", args[0])})
	}
	out, err := run(args[1:])
	if err != nil {
		return "", dbus.NewError(errCommand, []any{err.Error()})
	}
	return out, nil
}

// SendCommand runs args in the running instance and returns its reply. It
// fails with ErrNotRunning when none is running, and with the instance's
// message when the command fails.
func SendCommand(ctx context.Context, conn *dbus.Conn, args []string) (string, error) {
	var out string
	err := conn.Object(BusName, ObjectPath).
		CallWithContext(ctx, Interface+".Command", dbus.FlagNoAutoStart, args).Store(&out)
	var dErr dbus.Error
	switch {
	case err == nil:
		return out, nil
	case errors.As(err, &dErr) && (dErr.Name == "org.freedesktop.DBus.Error.ServiceUnknown" || dErr.Name == "org.freedesktop.DBus.Error.NameHasNoOwner"):
		return "", ErrNotRunning
	case errors.As(err, &dErr) && dErr.Name == errCommand && len(dErr.Body) > 0:
		return "", fmt.Errorf("%v", dErr.Body[0])
	}
	return "", err
}

// RunCommand is a subcommand launch (see IsCommand): it sends args to the
// running instance over the session bus, prints the reply to stdout or the
// failure to stderr, and returns the exit status (0, or 1 on failure). With
// no instance running it changes nothing: commands act on the running app
// only.
func RunCommand(args []string, stdout, stderr io.Writer) int {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "sona: no D-Bus session bus:", err)
		return 1
	}
	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	out, err := SendCommand(ctx, conn, args)
	if errors.Is(err, ErrNotRunning) {
		_, _ = fmt.Fprintln(stderr, "sona: Sona isn't running; start it first")
		return 1
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "sona:", err)
		return 1
	}
	if _, err := fmt.Fprintln(stdout, out); err != nil {
		return 1
	}
	return 0
}
