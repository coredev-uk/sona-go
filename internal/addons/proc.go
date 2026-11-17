package addons

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/addons/sandbox"
)

// Spec is how to run one native add-on. The process is always confined by the OS
// (package sandbox): what it may reach is the fields below, nothing else of
// the user's system. Start fails, and runs nothing, where the system can't
// confine it.
type Spec struct {
	// ID is the add-on's ID.
	ID string
	// Exe is the absolute path Entry.Exe gives; it is run as it is.
	Exe string
	// Args are the program's arguments after its name. Only Sona's own
	// bundled add-ons have any (BundledArgs); an installed add-on is run
	// with none.
	Args []string
	// ExeFile, when set, is the one open file that is run instead of Exe's
	// path (OpenVerified): the file that was checked is the file that runs,
	// whatever happens to the path meanwhile. Exe then only names it.
	ExeFile *os.File
	// Dir is the add-on's folder: the working directory, readable, never
	// writable.
	Dir string
	// Data is the add-on's private folder (SONA_ADDON_DATA), the one place
	// it can write; EnsureData made it.
	Data string
	// Network is whether the add-on has the network: its manifest asks for
	// the `network` permission and the user approved it.
	Network bool
	// Read are more folders it may read, and Sockets Unix sockets of the
	// user's it may connect to. Only code sets them, for a bundled add-on:
	// a manifest cannot.
	Read, Sockets []string
	Env           []string
	// Stderr receives the add-on's standard error (its log).
	Stderr io.Writer
}

// Running is a started add-on of either runtime: a process (Proc) or a
// WebAssembly module (WasmProc). The supervisor serves the session over its
// channel and stops it the same way.
type Running interface {
	// Conn is the protocol channel.
	Conn() io.ReadWriteCloser
	// Done is closed when the add-on has ended; Err is how, once it is.
	Done() <-chan struct{}
	Err() error
	// Kill ends it now.
	Kill()
	// Stop ends it with a grace, then kills it, and returns once it is gone.
	Stop(grace time.Duration)
	// Where says what runs, for the add-on's log ("pid 4242").
	Where() string
	// Ready is closed when the add-on is running, which is when the time it
	// has to say hello begins. A process is running when it starts; a
	// module is compiled first, which takes seconds the first time and is
	// not the module's delay.
	Ready() <-chan struct{}
}

// Proc is a running add-on. Its standard input and output are the protocol
// channel (Conn).
type Proc struct {
	child *sandbox.Child

	closeOnce sync.Once
}

// Start runs the add-on confined, in a process group of its own (a job object
// on Windows) that the OS ends with Sona where it can: children die when Sona
// quits or crashes. Callers run Start in a goroutine locked to its OS thread
// (runtime.LockOSThread) for as long as the add-on runs, as Linux's
// parent-death signal is tied to the thread that started the process.
//
// A *sandbox.Error (SandboxReason) says the sandbox could not be set up; the
// add-on was not started.
func Start(spec Spec) (*Proc, error) {
	c, err := sandbox.Start(sandbox.Spec{
		Policy: sandbox.Policy{
			Network: spec.Network, Dir: spec.Dir, Data: spec.Data, Read: spec.Read, Sockets: spec.Sockets,
		},
		ID: spec.ID, Exe: spec.Exe, ExeFile: spec.ExeFile, Args: spec.Args, Env: spec.Env, Stderr: spec.Stderr,
	})
	if err != nil {
		return nil, err
	}
	return &Proc{child: c}, nil
}

// Notes are what the sandbox could not do and runs without, for the add-on's
// log; empty when it is complete.
func (p *Proc) Notes() []string { return p.child.Notes }

// Conn is the channel as one io.ReadWriteCloser for cli.Session.Serve:
// reads the add-on's standard output, writes its standard input, and Close
// closes both (the add-on sees end of input).
func (p *Proc) Conn() io.ReadWriteCloser { return procConn{p} }

type procConn struct{ p *Proc }

func (c procConn) Read(b []byte) (int, error)  { return c.p.child.Out.Read(b) }
func (c procConn) Write(b []byte) (int, error) { return c.p.child.In.Write(b) }
func (c procConn) Close() error {
	c.p.closeOnce.Do(func() {
		_ = c.p.child.In.Close()
		_ = c.p.child.Out.Close()
	})
	return nil
}

// Done is closed when the process has exited and been reaped.
func (p *Proc) Done() <-chan struct{} { return p.child.Done() }

// Err is the exit status once Done is closed: nil for a clean exit.
func (p *Proc) Err() error { return p.child.Err() }

// Pid is the process ID.
func (p *Proc) Pid() int { return p.child.Pid() }

// Where is the process ID, for the log.
func (p *Proc) Where() string { return fmt.Sprintf("pid %d", p.Pid()) }

// ready is closed: a process is running as soon as it starts.
var ready = func() chan struct{} {
	c := make(chan struct{})
	close(c)
	return c
}()

// Ready is closed from the start.
func (p *Proc) Ready() <-chan struct{} { return ready }

// Terminate asks the add-on and its children to stop: SIGTERM to the group
// on Unix; nothing on Windows, where closing the channel is the request.
func (p *Proc) Terminate() { p.child.Terminate() }

// Kill ends the add-on and every process in its group or job now.
func (p *Proc) Kill() { p.child.Kill() }

// Stop ends the add-on: its channel closes (end of input) and it is given
// grace to leave; then it and its children are asked to stop, given grace
// again, and killed. It returns once the process is reaped.
func (p *Proc) Stop(grace time.Duration) {
	_ = procConn{p}.Close()
	if !p.wait(grace) {
		p.Terminate()
		p.wait(grace)
	}
	// The leader may be gone while a child lives on: kill the group either way.
	p.Kill()
	<-p.child.Done()
}

func (p *Proc) wait(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-p.child.Done():
		return true
	case <-t.C:
		return false
	}
}
