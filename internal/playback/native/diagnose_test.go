//go:build linux

package native

import (
	"context"
	"errors"
	"fmt"
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
			PID: 7, CDMPath: "/cdm/libwidevinecdm.so", CDMVersion: "4.10", CDMInterface: 10, HostInterface: 10,
			GStreamer: "1.26", Decoder: "avdec_aac", Sink: "pipewiresink",
			Sandbox: cdmhost.Sandbox{NetNS: true, MountNS: true, Landlock: 4, Seccomp: true, InetDenied: true},
		}, nil
	}
	h.e.findCDM = func(string) (string, string, error) {
		t.Error("discovery consulted after a successful probe")
		return "", "", errors.New("unused")
	}
	d, err := h.e.Diagnose(ctx(t), "/cdm")
	if err != nil {
		t.Fatal(err)
	}
	want := playback.Diagnosis{
		CDMPath: "/cdm/libwidevinecdm.so", CDMVersion: "4.10", CDMInterface: 10, HostInterface: 10,
		GStreamer: "1.26", Decoder: "avdec_aac", Sink: "pipewiresink", Sandboxed: true,
	}
	if d != want || asked != "/cdm" {
		t.Errorf("Diagnose = %+v (probe asked for %q), want %+v", d, asked, want)
	}
}

func TestDiagnoseFallsBackToDiscoveryWhenTheProbeFails(t *testing.T) {
	h := newHarness(t, time.Minute, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	h.e.probe = func(context.Context, cdmhost.Options) (cdmhost.Info, error) {
		return cdmhost.Info{Sandbox: cdmhost.Sandbox{Reason: "no seccomp"}}, fmt.Errorf("init: %w", cdmhost.ErrSandbox)
	}
	var asked string
	h.e.findCDM = func(override string) (string, string, error) {
		asked = override
		return "/found/libwidevinecdm.so", "4.11", nil
	}
	d, err := h.e.Diagnose(ctx(t), "")
	if !errors.Is(err, cdmhost.ErrSandbox) {
		t.Fatalf("err = %v, want the probe's", err)
	}
	want := playback.Diagnosis{
		HostRunning: true, HostPID: 100,
		CDMPath: "/found/libwidevinecdm.so", CDMVersion: "4.11",
		SandboxNote: "no seccomp", Blocked: true,
	}
	if d != want || asked != "" {
		t.Errorf("Diagnose = %+v (discovery asked for %q), want %+v", d, asked, want)
	}
}
