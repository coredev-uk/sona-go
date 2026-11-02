package native

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

func TestDiagnoseReportsTheProbe(t *testing.T) {
	h := newHarness(t, time.Minute, false)
	var asked string
	h.e.probe = func(_ context.Context, o cdmhost.Options) (cdmhost.Info, error) {
		asked = o.CDMPath
		return cdmhost.Info{
			PID: 7, CDMPath: o.CDMPath, CDMVersion: "4.10", CDMInterface: 10, HostInterface: 10,
			GStreamer: "1.26", Decoder: "avdec_aac", Sink: "pipewiresink",
			Sandbox: cdmhost.Sandbox{NetNS: true, MountNS: true, Landlock: 4, Seccomp: true, InetDenied: true},
		}, nil
	}
	d, err := h.e.Diagnose(ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	want := playback.Diagnosis{
		CDMPath: testCDM, CDMVersion: "4.10", CDMInterface: 10, HostInterface: 10,
		GStreamer: "1.26", Decoder: "avdec_aac", Sink: "pipewiresink", Sandbox: playback.SandboxLinux, Sandboxed: true,
	}
	if !reflect.DeepEqual(d, want) || asked != testCDM {
		t.Errorf("Diagnose = %+v (probe asked for %q), want %+v", d, asked, want)
	}
}

func TestDiagnoseReportsTheLibraryWhenTheProbeFails(t *testing.T) {
	h := newHarness(t, time.Minute, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	h.e.probe = func(context.Context, cdmhost.Options) (cdmhost.Info, error) {
		return cdmhost.Info{PID: 7, Sandbox: cdmhost.Sandbox{NetNS: true, MountNS: true, Landlock: 3, InetDenied: true, Reason: "no seccomp"}}, fmt.Errorf("init: %w", cdmhost.ErrSandbox)
	}
	d, err := h.e.Diagnose(ctx(t))
	if !errors.Is(err, cdmhost.ErrSandbox) {
		t.Fatalf("err = %v, want the probe's", err)
	}
	want := playback.Diagnosis{
		HostRunning: true, HostPID: 100, CDMPath: testCDM,
		Sandbox: playback.SandboxLinux, SandboxGaps: []playback.SandboxGap{playback.SandboxGapSeccomp}, Blocked: true,
	}
	if !reflect.DeepEqual(d, want) {
		t.Errorf("Diagnose = %+v, want %+v", d, want)
	}
}

// Without a CDM ready there is nothing to probe: Diagnose returns the
// source's error (playback.ErrNoCDM) and spawns no child.
func TestDiagnoseWithoutACDM(t *testing.T) {
	h := newHarness(t, time.Minute, false)
	src := &fakeCDM{err: fmt.Errorf("downloading: %w", playback.ErrNoCDM)}
	h.e.cdm = src
	h.e.probe = func(context.Context, cdmhost.Options) (cdmhost.Info, error) {
		t.Error("probe ran without a CDM")
		return cdmhost.Info{}, nil
	}
	if _, err := h.e.Diagnose(ctx(t)); !errors.Is(err, playback.ErrNoCDM) {
		t.Fatalf("err = %v, want ErrNoCDM", err)
	}
}
