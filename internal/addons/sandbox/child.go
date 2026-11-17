package sandbox

import "os"

// Child is a started add-on: the channel to it and the process, or the group
// or job of processes, it leads. The OS ends the group with Sona where it
// can (Linux's parent-death signal, Windows' job object).
type Child struct {
	// In is the add-on's standard input, our end; Out its standard output.
	// They are the protocol channel.
	In, Out *os.File
	// Notes are what the sandbox could not do and runs without, in plain
	// words for the add-on's log: empty when it is complete. A layer that
	// is missing never replaces the others, which are always there.
	Notes []string

	pid       int
	done      chan struct{}
	err       error
	terminate func()
	kill      func()
}

// Pid is the process ID (the add-on's, or on Linux its init's).
func (c *Child) Pid() int { return c.pid }

// Done is closed once the add-on has exited and been reaped.
func (c *Child) Done() <-chan struct{} { return c.done }

// Err is the exit status once Done is closed: nil for a clean exit.
func (c *Child) Err() error {
	<-c.done
	return c.err
}

// Terminate asks the add-on and its children to stop: SIGTERM to the group
// on Unix; nothing on Windows, where closing the channel is the request.
func (c *Child) Terminate() {
	if c.terminate != nil {
		c.terminate()
	}
}

// Kill ends the add-on and every process in its group, init's namespace or
// job now.
func (c *Child) Kill() { c.kill() }
