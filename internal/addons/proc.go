package addons

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Spec is how to run one add-on.
type Spec struct {
	// Exe is the absolute path Entry.Exe gives; it is run as it is.
	Exe string
	// ExeFile, when set, is the one open file that is run instead of Exe's
	// path (OpenVerified): the file that was checked is the file that runs,
	// whatever happens to the path meanwhile. Exe then only names it.
	ExeFile *os.File
	// Dir is the working directory: the add-on's folder.
	Dir string
	Env []string
	// Stderr receives the add-on's standard error (its log).
	Stderr io.Writer
}

// waitDelay bounds the wait for the add-on's standard error to close after
// it exits: a grandchild that kept the pipe open must not hold Wait.
const waitDelay = 2 * time.Second

// Proc is a running add-on. Its standard input and output are the protocol
// channel (Conn).
type Proc struct {
	cmd  *exec.Cmd
	in   *os.File // the add-on's standard input, our end
	out  *os.File // the add-on's standard output, our end
	grp  *group
	done chan struct{}
	err  error

	closeOnce sync.Once
}

// Start runs the add-on in a process group of its own (a job object on
// Windows) that the OS ends with Sona where it can: children die when Sona
// quits or crashes. Callers run Start and Wait in a goroutine locked to its
// OS thread (runtime.LockOSThread), as Linux's parent-death signal is tied
// to the thread that started the process.
func Start(spec Spec) (*Proc, error) {
	// The channel: two pipes; the add-on's ends are closed here once it runs.
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("addons: pipe: %w", err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		_ = inR.Close()
		_ = inW.Close()
		return nil, fmt.Errorf("addons: pipe: %w", err)
	}
	// Run exactly Exe: exec.Command would look a bare name up in PATH or the
	// working directory. Exe is absolute and already resolved.
	cmd := &exec.Cmd{Path: spec.Exe, Args: []string{spec.Exe}, Dir: spec.Dir, Env: spec.Env}
	if spec.ExeFile != nil {
		// The child's first extra file is its descriptor 3: run that open
		// file, not the path.
		cmd.Path = runFD
		cmd.ExtraFiles = []*os.File{spec.ExeFile}
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, spec.Stderr
	cmd.WaitDelay = waitDelay
	cmd.SysProcAttr = sysProcAttr()
	if err := cmd.Start(); err != nil {
		for _, f := range []*os.File{inR, inW, outR, outW} {
			_ = f.Close()
		}
		return nil, fmt.Errorf("addons: start: %w", err)
	}
	_ = inR.Close()
	_ = outW.Close()
	p := &Proc{cmd: cmd, in: inW, out: outR, done: make(chan struct{})}
	p.grp, err = joinGroup(cmd)
	if err != nil {
		// Not in its group, it could outlive Sona: stop it rather than run it.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = inW.Close()
		_ = outR.Close()
		return nil, err
	}
	go func() {
		p.err = cmd.Wait()
		p.grp.release()
		close(p.done)
	}()
	return p, nil
}

// Conn is the channel as one io.ReadWriteCloser for cli.Session.Serve:
// reads the add-on's standard output, writes its standard input, and Close
// closes both (the add-on sees end of input).
func (p *Proc) Conn() io.ReadWriteCloser { return procConn{p} }

type procConn struct{ p *Proc }

func (c procConn) Read(b []byte) (int, error)  { return c.p.out.Read(b) }
func (c procConn) Write(b []byte) (int, error) { return c.p.in.Write(b) }
func (c procConn) Close() error {
	c.p.closeOnce.Do(func() {
		_ = c.p.in.Close()
		_ = c.p.out.Close()
	})
	return nil
}

// Done is closed when the process has exited and been reaped.
func (p *Proc) Done() <-chan struct{} { return p.done }

// Err is the exit status once Done is closed: nil for a clean exit.
func (p *Proc) Err() error {
	<-p.done
	return p.err
}

// Pid is the process ID.
func (p *Proc) Pid() int { return p.cmd.Process.Pid }

// Terminate asks the add-on and its children to stop: SIGTERM to the group
// on Unix; nothing on Windows, where closing the channel is the request.
func (p *Proc) Terminate() { p.grp.terminate(p.cmd) }

// Kill ends the add-on and every process in its group or job now.
func (p *Proc) Kill() { p.grp.kill(p.cmd) }

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
	<-p.done
}

func (p *Proc) wait(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-p.done:
		return true
	case <-t.C:
		return false
	}
}
