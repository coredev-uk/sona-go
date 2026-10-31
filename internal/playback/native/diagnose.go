//go:build linux || (darwin && cgo)

package native

import (
	"context"
	"errors"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// probeTimeout bounds the CDM host probe behind Diagnose.
const probeTimeout = 15 * time.Second

// Diagnose checks the playback stack: whether a host is running, and a
// throwaway probe host that loads the CDM a new host would load and
// reports the CDM, GStreamer, decoder, sink and sandbox. It returns the
// probe's error alongside what the probe got as far as reporting; with no
// CDM ready (the CDM source's error) there is no probe.
func (e *Engine) Diagnose(ctx context.Context) (playback.Diagnosis, error) {
	var d playback.Diagnosis
	if info, running := e.Host(); running {
		d.HostRunning, d.HostPID = true, info.PID
	}
	lib, err := e.cdm.Library(ctx)
	if err != nil {
		return d, err
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	info, err := e.probe(ctx, cdmhost.Options{CDMPath: lib, Logger: e.log})
	d.CDMPath, d.CDMVersion = info.CDMPath, info.CDMVersion
	if d.CDMPath == "" {
		// The child never got far enough to say.
		d.CDMPath = lib
	}
	d.CDMInterface, d.HostInterface = info.CDMInterface, info.HostInterface
	d.GStreamer, d.Decoder, d.Sink = info.GStreamer, info.Decoder, info.Sink
	if info.PID != 0 { // a probe child started
		d.Sandbox = info.Sandbox.Kind()
	}
	d.Sandboxed, d.SandboxNote = info.Sandbox.Active(), info.Sandbox.Reason
	d.Blocked = errors.Is(err, cdmhost.ErrSandbox)
	return d, err
}
