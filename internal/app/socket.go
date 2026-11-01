package app

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
)

// Single instance and `sona <command>` over a local socket, for macOS and
// Windows (it builds and is tested everywhere; Linux uses D-Bus). The
// primary instance holds an exclusive lock on sona.lock and listens on
// sona.sock beside it, in Sona's per-user state folder (0700). It answers
// only connections from its own user (peer credentials: SO_PEERCRED on
// Linux, LOCAL_PEERCRED on macOS, the peer process's token user on
// Windows) and closes any other.
//
// A connection carries one line from the client, a socketFrame (JSON),
// and the answer: one cli.Reply line for a command or an activation, or
// for a status stream a cli.Reply line per status until Sona quits and
// closes it.

const (
	// SocketName and LockName are the files in the state folder.
	SocketName = "sona.sock"
	LockName   = "sona.lock"

	// maxFrame bounds a client's line.
	maxFrame = 1 << 20
	// ioTimeout bounds reading a client's line and writing an answer.
	ioTimeout = 10 * time.Second
	// acquireAttempts is how often a launch tries to become primary or
	// reach the primary before giving up.
	acquireAttempts = 3
	acquireRetry    = 200 * time.Millisecond
)

// errLocked is tryLock's answer when another process holds the lock.
var errLocked = errors.New("app: locked by another process")

// socketFrame is a client's request: exactly one field is set.
type socketFrame struct {
	Run      json.RawMessage `json:"run,omitempty"` // a cli.Request
	Follow   bool            `json:"follow,omitempty"`
	Activate *wireActivation `json:"activate,omitempty"`
}

type wireActivation struct {
	Args            []string `json:"args"`
	ActivationToken string   `json:"activationToken,omitempty"`
	WorkingDir      string   `json:"cwd,omitempty"`
}

// SocketInstance is the primary instance's claim on the socket.
type SocketInstance struct {
	ln     net.Listener
	unlock func()
	log    *slog.Logger
	ctx    context.Context // cancelled by Close: ends commands and streams
	cancel context.CancelFunc

	mu     sync.Mutex
	ch     chan Activation
	closed bool
	server cli.Server // nil until HandleCommands
	conns  map[net.Conn]struct{}

	wg        sync.WaitGroup
	closeOnce sync.Once
}

// AcquireSocket makes this process the primary instance, or forwards act
// to the one already running, through the lock and the socket in dir.
// Like Acquire it returns (nil, ErrForwarded) when another instance took
// act; ctx bounds the forwarding. A nil log discards output.
func AcquireSocket(ctx context.Context, dir string, act Activation, log *slog.Logger) (*SocketInstance, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	sock := filepath.Join(dir, SocketName)
	var lastErr error
	for attempt := range acquireAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("app: forward to running instance: %w", ctx.Err())
			case <-time.After(acquireRetry):
			}
		}
		unlock, err := tryLock(filepath.Join(dir, LockName))
		if err == nil {
			return listen(sock, unlock, log)
		}
		if !errors.Is(err, errLocked) {
			return nil, err
		}
		// Another instance holds the lock: hand it act. It may still be
		// starting to listen, or quitting; try again.
		c := &SocketClient{path: sock}
		_, lastErr = c.roundTrip(ctx, socketFrame{Activate: &wireActivation{
			Args: nonNil(act.Args), ActivationToken: act.ActivationToken, WorkingDir: act.WorkingDir,
		}})
		if lastErr == nil {
			return nil, ErrForwarded
		}
	}
	return nil, fmt.Errorf("app: forward to running instance: %w", lastErr)
}

// listen serves the socket once the lock is ours. A socket file left by
// an instance that crashed is replaced.
func listen(path string, unlock func(), log *slog.Logger) (*SocketInstance, error) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		unlock()
		return nil, fmt.Errorf("app: remove stale socket: %w", err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		unlock()
		return nil, fmt.Errorf("app: listen: %w", err)
	}
	// The folder is the user's alone already; the file says so too.
	if err := restrictSocket(path); err != nil {
		_ = ln.Close()
		unlock()
		return nil, fmt.Errorf("app: restrict socket: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	in := &SocketInstance{
		ln: ln, unlock: unlock, log: log, ctx: ctx, cancel: cancel,
		ch: make(chan Activation, activationBuffer), conns: map[net.Conn]struct{}{},
	}
	in.wg.Add(1)
	go in.accept()
	return in, nil
}

func nonNil(args []string) []string {
	if args == nil {
		return []string{}
	}
	return args
}

// Activations delivers launches forwarded by later instances. It is
// closed by Close.
func (in *SocketInstance) Activations() <-chan Activation { return in.ch }

// HandleCommands serves srv's commands until Close. Until it is called,
// commands fail with cli.CodeStarting.
func (in *SocketInstance) HandleCommands(srv cli.Server) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if !in.closed && in.server == nil {
		in.server = srv
	}
}

// Close stops listening, ends every connection (status streams end, so
// followers see Sona quit), closes Activations and releases the lock.
// Safe to call more than once.
func (in *SocketInstance) Close() error {
	var err error
	in.closeOnce.Do(func() {
		in.mu.Lock()
		in.closed = true
		close(in.ch)
		conns := make([]net.Conn, 0, len(in.conns))
		for c := range in.conns {
			conns = append(conns, c)
		}
		in.mu.Unlock()
		in.cancel()
		err = in.ln.Close() // removes the socket file
		for _, c := range conns {
			_ = c.Close()
		}
		in.wg.Wait()
		in.unlock()
	})
	return err
}

func (in *SocketInstance) accept() {
	defer in.wg.Done()
	for {
		c, err := in.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			in.log.Warn("app: accept", "err", err)
			select {
			case <-in.ctx.Done():
				return
			case <-time.After(50 * time.Millisecond):
			}
			continue
		}
		in.mu.Lock()
		if in.closed {
			in.mu.Unlock()
			_ = c.Close()
			return
		}
		in.conns[c] = struct{}{}
		in.wg.Add(1)
		in.mu.Unlock()
		go in.handle(c)
	}
}

func (in *SocketInstance) handle(c net.Conn) {
	defer in.wg.Done()
	defer func() {
		in.mu.Lock()
		delete(in.conns, c)
		in.mu.Unlock()
		_ = c.Close()
	}()
	if err := peerIsUser(c); err != nil {
		in.log.Warn("app: refused a connection to the command socket", "err", err)
		return
	}
	_ = c.SetReadDeadline(time.Now().Add(ioTimeout))
	line, err := bufio.NewReader(io.LimitReader(c, maxFrame)).ReadBytes('\n')
	if err != nil {
		return
	}
	_ = c.SetReadDeadline(time.Time{})
	var f socketFrame
	if err := json.Unmarshal(line, &f); err != nil {
		in.reply(c, cli.Reply{Error: &cli.Error{Code: cli.CodeIncompatible, Message: err.Error()}})
		return
	}
	in.mu.Lock()
	srv, closed := in.server, in.closed
	in.mu.Unlock()
	switch {
	case closed:
		in.reply(c, cli.Reply{Error: &cli.Error{Code: cli.CodeShuttingDown}})
	case f.Activate != nil:
		in.reply(c, in.activate(f.Activate))
	case f.Run != nil:
		in.reply(c, cli.Handle(in.ctx, srv, f.Run))
	case f.Follow && srv == nil:
		in.reply(c, cli.Reply{Error: &cli.Error{Code: cli.CodeStarting}})
	case f.Follow:
		in.follow(c, srv)
	default:
		in.reply(c, cli.Reply{Error: &cli.Error{Code: cli.CodeIncompatible}})
	}
}

// activate queues a forwarded launch; like the D-Bus Activate it never
// blocks, dropping the launch when the queue is full.
func (in *SocketInstance) activate(a *wireActivation) cli.Reply {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closed {
		return cli.Reply{Error: &cli.Error{Code: cli.CodeShuttingDown}}
	}
	select {
	case in.ch <- Activation{Args: a.Args, ActivationToken: a.ActivationToken, WorkingDir: a.WorkingDir}:
	default:
		in.log.Warn("app: activation dropped, queue full", "args", len(a.Args))
	}
	return cli.Reply{Result: json.RawMessage("true")}
}

// follow streams srv's status to c until Sona quits, the client hangs up
// or a write fails.
func (in *SocketInstance) follow(c net.Conn, srv cli.Server) {
	updates, cancel := srv.Watch()
	defer cancel()
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		_, _ = io.Copy(io.Discard, c) // returns when the client hangs up or c closes
	}()
	for {
		select {
		case <-in.ctx.Done():
			return
		case <-gone:
			return
		case st, ok := <-updates:
			if !ok {
				return
			}
			if !in.write(c, cli.StatusReply(st)) {
				return
			}
		}
	}
}

func (in *SocketInstance) reply(c net.Conn, r cli.Reply) {
	b, err := json.Marshal(r)
	if err != nil {
		in.log.Warn("app: encode reply", "err", err)
		return
	}
	in.write(c, b)
}

// write sends one line; false when the client is gone.
func (in *SocketInstance) write(c net.Conn, line []byte) bool {
	_ = c.SetWriteDeadline(time.Now().Add(ioTimeout))
	_, err := c.Write(append(line, '\n'))
	return err == nil
}

// SocketClient sends commands to the running Sona over its socket
// (cli.Transport). Each command is a connection of its own.
type SocketClient struct {
	path string
}

// DialSocket returns a client for the socket in dir (Sona's state
// folder). It fails with cli.ErrNotRunning when there is no socket.
func DialSocket(dir string) (*SocketClient, error) {
	path := filepath.Join(dir, SocketName)
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, cli.ErrNotRunning
	}
	return &SocketClient{path: path}, nil
}

// Close does nothing: connections close after each command.
func (*SocketClient) Close() error { return nil }

// dial connects; a missing socket or one nobody listens on (left by a
// crash) is cli.ErrNotRunning.
func (c *SocketClient) dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || refused(err) {
			return nil, cli.ErrNotRunning
		}
		return nil, err
	}
	return conn, nil
}

// send dials and writes f; ctx's end closes the connection.
func (c *SocketClient) send(ctx context.Context, f socketFrame) (net.Conn, func() bool, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	b, err := json.Marshal(f)
	if err == nil {
		_ = conn.SetWriteDeadline(time.Now().Add(ioTimeout))
		_, err = conn.Write(append(b, '\n'))
		_ = conn.SetWriteDeadline(time.Time{})
	}
	if err != nil {
		stop()
		_ = conn.Close()
		return nil, nil, c.failed(ctx, err)
	}
	return conn, stop, nil
}

// failed is err, or ctx's error when ctx's end caused it.
func (c *SocketClient) failed(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (c *SocketClient) roundTrip(ctx context.Context, f socketFrame) (json.RawMessage, error) {
	conn, stop, err := c.send(ctx, f)
	if err != nil {
		return nil, err
	}
	defer func() {
		stop()
		_ = conn.Close()
	}()
	line, err := bufio.NewReader(io.LimitReader(conn, maxFrame)).ReadBytes('\n')
	if err != nil {
		return nil, c.failed(ctx, err)
	}
	return cli.DecodeReply(line)
}

// Call implements cli.Transport.
func (c *SocketClient) Call(ctx context.Context, req cli.Request) (json.RawMessage, error) {
	b, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return c.roundTrip(ctx, socketFrame{Run: b})
}

// Follow implements cli.Transport: it reads statuses until Sona closes
// the stream (it quit: nil) or ctx ends.
func (c *SocketClient) Follow(ctx context.Context, each func(cli.Status) error) error {
	conn, stop, err := c.send(ctx, socketFrame{Follow: true})
	if err != nil {
		return err
	}
	defer func() {
		stop()
		_ = conn.Close()
	}()
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadBytes('\n')
		if errors.Is(err, io.EOF) && len(line) == 0 {
			return c.failed(ctx, nil)
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return c.failed(ctx, err)
		}
		raw, derr := cli.DecodeReply(line)
		if derr != nil {
			return derr
		}
		var st cli.Status
		if err := json.Unmarshal(raw, &st); err != nil {
			return &cli.Error{Code: cli.CodeIncompatible, Message: err.Error()}
		}
		if err := each(st); err != nil {
			return err
		}
	}
}
