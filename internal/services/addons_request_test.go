//go:build unix

package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/config"
)

// A lease runs one of Sona's own add-ons that has nothing to do, and lets it
// go when released (twice is safe); another add-on is left alone, and an ID
// that is not Sona's own holds nothing.
func TestAddonsLeaseRunsAnIdleBundledAddon(t *testing.T) {
	a := newAddonEnv(t)
	a.bundledProgram(t)
	a.svc.start()
	if st := a.info(t, "listenbrainz").State; st != AddonIdle {
		t.Fatalf("state = %s, want idle", st)
	}
	release := a.svc.Lease("listenbrainz")
	a.waitState(t, "listenbrainz", AddonRunning)
	if st := a.info(t, "discord").State; st != AddonIdle {
		t.Errorf("discord = %s: a lease on another add-on started it", st)
	}
	release()
	release()
	a.waitState(t, "listenbrainz", AddonIdle)
	a.svc.Lease("demo")()
}

// A stuck `"enabled": false`, as 0.7.0-rc.3's crash rule saved it for one of
// Sona's own add-ons, keeps nothing from it: a lease runs the add-on, and a
// request reaches it.
func TestAddonsLeaseRunsABundledAddonWhateverAStuckSavedSwitchSays(t *testing.T) {
	a := newAddonEnv(t)
	a.answeringProgram(t)
	if _, err := a.settings.Update(func(s *config.Settings) {
		s.Addons = config.Addons{"listenbrainz": {Enabled: false}}
	}); err != nil {
		t.Fatal(err)
	}
	a.svc.start()
	defer a.svc.Lease("listenbrainz")()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var res cli.ScrobbleResult
	if err := a.svc.Request(ctx, "listenbrainz", cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{Credential: "tok"}, &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Account != "acct" {
		t.Errorf("result = %+v", res)
	}
}

// answeringProgram makes a script stand in for Sona's executable that says
// hello, reads one request and answers it as ListenBrainz's account "acct",
// writing the request it got to the returned file.
func (a *addonEnv) answeringProgram(t *testing.T) (got string) {
	t.Helper()
	dir := t.TempDir()
	exe, got := filepath.Join(dir, "sona"), filepath.Join(dir, "request.log")
	script := `#!/bin/sh
printf '{"id":1,"method":"hello","params":{"id":"%s","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":["network"]}}\n' "$2"
read hello_answer
read request
printf '%s\n' "$request" > ` + got + `
printf '{"id":1,"result":{"ok":true,"account":"acct"}}\n'
cat > /dev/null
`
	if err := os.WriteFile(exe, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	a.svc.opts.exe = exe
	return got
}

// Request waits for the add-on to come up, sends the request over its channel
// and returns the answer; it is only for Sona's own add-ons.
func TestAddonsRequestReachesABundledAddon(t *testing.T) {
	a := newAddonEnv(t)
	got := a.answeringProgram(t)

	// Before the network opens nothing runs, and a caller with a short
	// deadline is told the add-on is unavailable.
	short, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	err := a.svc.Request(short, "listenbrainz", cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{}, nil)
	cancel()
	if !errors.Is(err, ErrAddonUnavailable) {
		t.Fatalf("before the network opens: %v", err)
	}

	a.svc.start()
	defer a.svc.Lease("listenbrainz")()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	var res cli.ScrobbleResult
	if err := a.svc.Request(ctx, "listenbrainz", cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{Credential: "tok"}, &res); err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.Account != "acct" {
		t.Errorf("result = %+v", res)
	}
	b, _ := os.ReadFile(got)
	if !strings.Contains(string(b), `"method":"scrobble.status"`) || !strings.Contains(string(b), `"credential":"tok"`) {
		t.Errorf("the add-on got %q", b)
	}

	// One that is not Sona's own is never sent a request.
	if err := a.svc.Request(ctx, "demo", cli.AddonMethodScrobbleStatus, cli.ScrobbleStatusParams{}, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("a third-party ID: %v", err)
	}
}
