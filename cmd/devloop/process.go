package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

// process is the running app.
type process struct {
	cmd  *exec.Cmd
	done chan struct{} // closed once it exited
	err  error         // how it exited, valid after done
}

// startProcess runs the built binary with the loop's arguments and the
// environment of the loop (see appEnv). onReady is called with the time the
// app first printed cfg.ready.
func startProcess(cfg config, onReady func(time.Time)) (*process, error) {
	cmd := exec.Command(cfg.out, cfg.args...)
	cmd.Dir = cfg.root
	cmd.Env = appEnv(cfg, os.Environ())
	ownGroup(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = &tap{w: os.Stderr, needle: []byte(cfg.ready), hit: onReady}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &process{cmd: cmd, done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// appEnv is the environment the app starts in: env (the loop's), plus
// SONA_SOURCE, where the dev build finds the token and catalogue files to
// reload, and with cfg.background MYGO_DEV_BACKGROUND=1, which MyGo's dev
// builds read to start without coming to the front and to show their windows
// without the focus (what `mygo dev -background` sets), so that a restart on
// a save leaves the focus where the editor has it.
func appEnv(cfg config, env []string) []string {
	out := slices.Clone(env)
	if !slices.ContainsFunc(out, func(kv string) bool { return strings.HasPrefix(kv, "SONA_SOURCE=") && kv != "SONA_SOURCE=" }) {
		out = append(out, "SONA_SOURCE="+cfg.root)
	}
	if cfg.background {
		out = append(out, "MYGO_DEV_BACKGROUND=1") // the last of a repeated key is the one a process sees
	}
	return out
}

// terminate asks the app to quit (SIGTERM: MyGo runs the quit sequence, so
// the dev build saves its state) and kills it when it takes longer than
// grace.
func (p *process) terminate(grace time.Duration) {
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		_ = p.cmd.Process.Kill() // no SIGTERM here (Windows)
	}
	select {
	case <-p.done:
	case <-time.After(grace):
		logf("the app did not quit in %s: killing it", grace)
		_ = p.cmd.Process.Kill()
		<-p.done
	}
}

// tap copies the app's stderr through and calls hit the first time a line
// contains needle.
type tap struct {
	w      io.Writer
	needle []byte
	hit    func(time.Time)

	mu   sync.Mutex
	line []byte
	done bool
}

func (t *tap) Write(b []byte) (int, error) {
	t.mu.Lock()
	if !t.done {
		t.line = append(t.line, b...)
		for {
			i := bytes.IndexByte(t.line, '\n')
			if i < 0 {
				break
			}
			if bytes.Contains(t.line[:i], t.needle) {
				t.done = true
				t.line = nil
				go t.hit(time.Now())
				break
			}
			t.line = t.line[i+1:]
		}
		if len(t.line) > 1<<16 {
			t.line = nil
		}
	}
	t.mu.Unlock()
	return t.w.Write(b)
}
