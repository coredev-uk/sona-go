package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/cli"
)

// `sona <command>` over D-Bus (Linux): the running instance's Run method
// answers commands (internal/cli's protocol, a JSON request and reply in
// a string each) and its StatusChanged signal carries each status change
// for `sona status --follow`. BusClient is the command line's side.

const (
	// methodRun is Run(s request) → (s reply).
	methodRun = Interface + ".Run"
	// signalStatus is StatusChanged(s status), a cli.Status in JSON.
	signalStatus = "StatusChanged"

	// serveTimeout bounds a command in the running instance.
	serveTimeout = time.Minute

	errServiceUnknown   = "org.freedesktop.DBus.Error.ServiceUnknown"
	errNameHasNoOwner   = "org.freedesktop.DBus.Error.NameHasNoOwner"
	errUnknownMethod    = "org.freedesktop.DBus.Error.UnknownMethod"
	errUnknownInterface = "org.freedesktop.DBus.Error.UnknownInterface"
	errUnknownObject    = "org.freedesktop.DBus.Error.UnknownObject"
)

// HandleCommands serves srv's commands and publishes its status changes
// until Close. Until it is called, commands fail with cli.CodeStarting.
func (in *Instance) HandleCommands(srv cli.Server) {
	updates, cancel := srv.Watch()
	in.mu.Lock()
	if in.closed || in.server != nil {
		in.mu.Unlock()
		cancel()
		return
	}
	in.server, in.stopWatch = srv, cancel
	in.watching = make(chan struct{})
	in.mu.Unlock()
	go func() {
		defer close(in.watching)
		for st := range updates {
			b, err := json.Marshal(st)
			if err == nil {
				err = in.conn.Emit(ObjectPath, Interface+"."+signalStatus, string(b))
			}
			if err != nil {
				in.log.Warn("app: publish status", "err", err)
			}
		}
	}()
}

// run is the exported Run method.
func (in *Instance) run(req string) (string, *dbus.Error) {
	in.mu.Lock()
	srv, closed := in.server, in.closed
	in.mu.Unlock()
	var reply cli.Reply
	if closed {
		reply.Error = &cli.Error{Code: cli.CodeShuttingDown}
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), serveTimeout)
		defer cancel()
		reply = cli.Handle(ctx, srv, []byte(req))
	}
	b, err := json.Marshal(reply)
	if err != nil {
		return "", dbus.MakeFailedError(err)
	}
	return string(b), nil
}

// BusClient sends commands to the running Sona over the session bus
// (cli.Transport).
type BusClient struct {
	conn *dbus.Conn
	own  bool
}

// DialBus connects to the session bus. Without one there is no Sona to
// reach: it fails with cli.ErrNotRunning.
func DialBus() (*BusClient, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w (no D-Bus session bus: %v)", cli.ErrNotRunning, err)
	}
	return &BusClient{conn: conn, own: true}, nil
}

// NewBusClient sends commands over conn, which stays the caller's.
func NewBusClient(conn *dbus.Conn) *BusClient { return &BusClient{conn: conn} }

// Close closes the connection DialBus opened.
func (c *BusClient) Close() error {
	if c.own {
		return c.conn.Close()
	}
	return nil
}

// Call implements cli.Transport.
func (c *BusClient) Call(ctx context.Context, req cli.Request) (json.RawMessage, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var out string
	err = c.conn.Object(BusName, ObjectPath).CallWithContext(ctx, methodRun, dbus.FlagNoAutoStart, string(b)).Store(&out)
	if err != nil {
		return nil, busError(err)
	}
	return cli.DecodeReply([]byte(out))
}

// busError maps a failed call: no owner is cli.ErrNotRunning, an
// instance without the Run method is another version.
func busError(err error) error {
	var dErr dbus.Error
	if !errors.As(err, &dErr) {
		return err
	}
	switch dErr.Name {
	case errServiceUnknown, errNameHasNoOwner:
		return cli.ErrNotRunning
	case errUnknownMethod, errUnknownInterface, errUnknownObject:
		return &cli.Error{Code: cli.CodeIncompatible, Message: dErr.Error()}
	}
	return err
}

// Follow implements cli.Transport: the current status (Run "status"),
// then each StatusChanged from the instance that answered, until it
// releases BusName (Sona quit) or ctx ends.
func (c *BusClient) Follow(ctx context.Context, each func(cli.Status) error) error {
	sigs := make(chan *dbus.Signal, 16)
	c.conn.Signal(sigs)
	defer c.conn.RemoveSignal(sigs)
	status := []dbus.MatchOption{
		dbus.WithMatchSender(BusName), dbus.WithMatchObjectPath(ObjectPath),
		dbus.WithMatchInterface(Interface), dbus.WithMatchMember(signalStatus),
	}
	owner := []dbus.MatchOption{
		dbus.WithMatchSender("org.freedesktop.DBus"), dbus.WithMatchInterface("org.freedesktop.DBus"),
		dbus.WithMatchMember("NameOwnerChanged"), dbus.WithMatchArg(0, BusName),
	}
	for _, m := range [][]dbus.MatchOption{status, owner} {
		if err := c.conn.AddMatchSignalContext(ctx, m...); err != nil {
			return err
		}
		defer func() { _ = c.conn.RemoveMatchSignal(m...) }()
	}
	// The instance answering now; signals from any later one don't count.
	var unique string
	if err := c.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, BusName).Store(&unique); err != nil {
		return busError(err)
	}
	raw, err := c.Call(ctx, cli.Request{Command: "status"})
	if err != nil {
		return err
	}
	var st cli.Status
	if err := json.Unmarshal(raw, &st); err != nil {
		return &cli.Error{Code: cli.CodeIncompatible, Message: err.Error()}
	}
	if err := each(st); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sig, ok := <-sigs:
			if !ok {
				return errors.New("app: the session bus connection closed")
			}
			switch {
			case sig.Name == "org.freedesktop.DBus.NameOwnerChanged" && len(sig.Body) == 3:
				if name, _ := sig.Body[0].(string); name == BusName {
					if old, _ := sig.Body[1].(string); old == unique {
						return nil
					}
				}
			case sig.Name == Interface+"."+signalStatus && sig.Sender == unique && len(sig.Body) == 1:
				body, _ := sig.Body[0].(string)
				var st cli.Status
				if err := json.Unmarshal([]byte(body), &st); err != nil {
					return &cli.Error{Code: cli.CodeIncompatible, Message: err.Error()}
				}
				if err := each(st); err != nil {
					return err
				}
			}
		}
	}
}
