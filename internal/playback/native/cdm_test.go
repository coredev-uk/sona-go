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

// A CDM installed while a host runs never reaches that host: it keeps the
// library it started with, and the next host loads the new one.
func TestNewCDMAppliesToTheNextHost(t *testing.T) {
	h := newHarness(t, 30*time.Millisecond, false)
	if err := h.e.Play(ctx(t), "A", 0); err != nil {
		t.Fatal(err)
	}
	const v2 = "/cdm/4.11/libwidevinecdm.so"
	h.cdm.set(v2, nil)
	if err := h.e.Play(ctx(t), "B", 0); err != nil {
		t.Fatal(err)
	}
	if h.nhosts() != 1 {
		t.Fatalf("%d hosts: the update restarted the running host", h.nhosts())
	}
	if err := h.e.Pause(ctx(t)); err != nil {
		t.Fatal(err)
	}
	eventually(t, "idle shutdown", func() bool { _, ok := h.e.Host(); return !ok })
	if err := h.e.Resume(ctx(t)); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	libs := h.libs
	h.mu.Unlock()
	if len(libs) != 2 || libs[0] != testCDM || libs[1] != v2 {
		t.Fatalf("hosts loaded %q, want [%s %s]", libs, testCDM, v2)
	}
}

// Without a CDM ready the play fails with the source's error
// (playback.ErrNoCDM) and no host starts.
func TestPlayWithoutACDM(t *testing.T) {
	h := newHarness(t, -1, false)
	h.cdm.set("", fmt.Errorf("cdm: still downloading: %w", playback.ErrNoCDM))
	err := h.e.Play(ctx(t), "A", 0)
	if !errors.Is(err, playback.ErrNoCDM) {
		t.Fatalf("Play = %v, want ErrNoCDM", err)
	}
	if h.nhosts() != 0 {
		t.Fatalf("%d hosts started without a CDM", h.nhosts())
	}
}

// The source hears every host start that loaded its CDM and every one the
// CDM made fail, and nothing about failures that aren't the CDM's.
func TestHostStartsAreReported(t *testing.T) {
	cdmLoad := fmt.Errorf("%w: dlopen failed", cdmhost.ErrCDMLoad)
	for _, tc := range []struct {
		name   string
		err    error
		report bool
	}{
		{"loaded", nil, true},
		{"CDM failed to load", cdmLoad, true},
		{"host died while loading", fmt.Errorf("%w: signal: segmentation fault", cdmhost.ErrExited), true},
		{"sandbox", fmt.Errorf("%w: no seccomp", cdmhost.ErrSandbox), false},
		{"audio", errors.New("gst: no sink"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, -1, false)
			if tc.err != nil {
				h.e.spawn = func(context.Context, string) (host, error) { return nil, tc.err }
			}
			err := h.e.Play(ctx(t), "A", 0)
			if !errors.Is(err, tc.err) {
				t.Fatalf("Play = %v, want %v", err, tc.err)
			}
			h.cdm.mu.Lock()
			reports := h.cdm.reports
			h.cdm.mu.Unlock()
			switch {
			case !tc.report && len(reports) != 0:
				t.Fatalf("reported %v", reports)
			case tc.report && (len(reports) != 1 || !errors.Is(reports[0], tc.err) || (tc.err == nil) != (reports[0] == nil)):
				t.Fatalf("reports %v, want [%v]", reports, tc.err)
			}
		})
	}
}
