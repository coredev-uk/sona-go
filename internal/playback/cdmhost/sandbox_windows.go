//go:build cgo

package cdmhost

import (
	"log/slog"
	"strconv"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// Sandbox describes how the CDM host child is confined on Windows. Nothing
// confines it yet: the child is an ordinary process of the user's.
type Sandbox struct {
	Reason string // why the child is unconfined, for the log
}

// Gaps are the missing layers (Diagnostics).
func (Sandbox) Gaps() []playback.SandboxGap { return nil }

// Active reports whether the child is confined: never yet.
func (Sandbox) Active() bool { return false }

// Kind is the sandbox the child runs in (Diagnostics).
func (Sandbox) Kind() string { return playback.SandboxNone }

// Attrs are the layers as key-value pairs for a log line (playspike).
func (s Sandbox) Attrs() []any { return []any{"reason", strconv.Quote(s.Reason)} }

// loadsCDM is true: an unconfined child still loads the CDM.
func (Sandbox) loadsCDM() bool { return true }

// sandboxEnv are the sandbox's own variables, never inherited by a child.
var sandboxEnv []string

// parentSandbox is the parent's part of the sandbox.
type parentSandbox struct{}

// startSandboxed starts the child as an ordinary process.
func startSandboxed(exe string, env []string, log *slog.Logger) (proc, parentSandbox, error) {
	log.Warn("cdmhost: starting the playback host without a sandbox: the Windows sandbox is not implemented yet")
	p, err := startChild([]string{exe}, env, nil)
	return p, parentSandbox{}, err
}

// withParent adds the parent's part to the child's report.
func (s Sandbox) withParent(parentSandbox) Sandbox { return s }

// enterSandbox runs in the child before the CDM or GStreamer load.
func enterSandbox() Sandbox {
	return Sandbox{Reason: "the Windows sandbox is not implemented yet"}
}
