//go:build linux

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
// throwaway probe host that loads the CDM at cdmPath ("" discovers one)
// and reports the CDM, GStreamer, decoder, sink and sandbox. It returns the
// probe's error alongside what the probe got as far as reporting; when the
// child could not say which CDM it would load, CDMPath and CDMVersion come
// from discovery instead.
func (e *Engine) Diagnose(ctx context.Context, cdmPath string) (playback.Diagnosis, error) {
	var d playback.Diagnosis
	if info, running := e.Host(); running {
		d.HostRunning, d.HostPID = true, info.PID
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	info, err := e.probe(ctx, cdmhost.Options{CDMPath: cdmPath, Logger: e.log})
	d.CDMPath, d.CDMVersion = info.CDMPath, info.CDMVersion
	d.CDMInterface, d.HostInterface = info.CDMInterface, info.HostInterface
	d.GStreamer, d.Decoder, d.Sink = info.GStreamer, info.Decoder, info.Sink
	d.Sandboxed, d.SandboxNote = info.Sandbox.Active(), info.Sandbox.Reason
	d.Blocked = errors.Is(err, cdmhost.ErrSandbox)
	if err != nil && d.CDMPath == "" {
		// The child never got far enough to say; report what discovery finds.
		if p, v, ferr := e.findCDM(cdmPath); ferr == nil {
			d.CDMPath, d.CDMVersion = p, v
		}
	}
	return d, err
}

// FindCDM returns the Widevine CDM the host would load: override (a
// library or the directory holding it) when set, else an installed
// browser's copy, with its version.
func (e *Engine) FindCDM(override string) (path, version string, err error) {
	return e.findCDM(override)
}
