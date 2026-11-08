package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/update"
)

// keepPrevious leaves the rig's app as an update from version v does: v is
// kept beside the install as the previous version (its Info.plist says
// which), and the running app is the rig's own version.
func (r *selfRig) keepPrevious(t *testing.T, v, running string) {
	t.Helper()
	plist := func(version string) {
		p := filepath.Join(r.g.Root, "Contents", "Info.plist")
		_ = os.Remove(p)
		body := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<plist version=\"1.0\"><dict>\n" +
			"<key>CFBundleShortVersionString</key><string>" + version + "</string>\n</dict></plist>\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// What the update leaves is files replaced by new ones, never written
	// through (the snapshot hard-links them).
	swap := func(path, content string) {
		_ = os.Remove(path)
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	swap(r.up.exe, v)
	plist(v)
	if err := r.g.Snapshot(v); err != nil {
		t.Fatal(err)
	}
	swap(r.up.exe, running)
	plist(running)
}

func (r *selfRig) relaunched(t *testing.T) bool {
	t.Helper()
	select {
	case <-r.up.relaunch:
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}

func TestNoPreviousVersionIsReportedAsSuch(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.3.0", latest: "0.3.0", key: true})
	if p := r.s.Previous(); p.Available || p.Version != "" {
		t.Fatalf("Previous = %+v with nothing kept", p)
	}
	err := r.s.RestorePrevious()
	if !errors.Is(err, update.ErrRestore) || CodeOf(err) != CodeRestoreFailed {
		t.Fatalf("RestorePrevious err = %v", err)
	}
	select {
	case <-r.up.relaunch:
		t.Fatal("restarted with nothing to restore")
	default:
	}
}

func TestThePreviousVersionIsReportedWithItsNumber(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.3.0-rc.4", latest: "0.3.0-rc.4", key: true})
	r.keepPrevious(t, "0.3.0-rc.3", "0.3.0-rc.4")
	if p := r.s.Previous(); !p.Available || p.Version != "0.3.0-rc.3" {
		t.Fatalf("Previous = %+v", p)
	}
}

func TestThePreviousVersionIsUnavailableWhereSonaDoesNotUpdateItself(t *testing.T) {
	for name, tc := range map[string]struct {
		opts   selfOpts
		mutate func(*selfRig)
	}{
		"no signing key (Linux packages and unsigned builds)": {opts: selfOpts{version: "0.3.0", latest: "0.3.0"}},
		"a development build": {opts: selfOpts{version: "dev", latest: "0.3.0", key: true}},
		"no guard (not in an app bundle)": {
			opts:   selfOpts{version: "0.3.0", latest: "0.3.0", key: true},
			mutate: func(r *selfRig) { r.s.guard = nil },
		},
		"not writable": {
			opts:   selfOpts{version: "0.3.0", latest: "0.3.0", key: true},
			mutate: func(r *selfRig) { r.up.enabled = false; r.s.status.SelfUpdate, r.s.status.Disabled = r.s.selfState() },
		},
		"an update installing": {
			opts:   selfOpts{version: "0.3.0", latest: "0.3.0", key: true},
			mutate: func(r *selfRig) { r.s.status.Installing = true },
		},
		"an update installed, restarting": {
			opts:   selfOpts{version: "0.3.0", latest: "0.3.0", key: true},
			mutate: func(r *selfRig) { r.s.status.Installed = true },
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := newSelfRig(t, tc.opts)
			r.keepPrevious(t, "0.2.0", tc.opts.version)
			if tc.mutate != nil {
				tc.mutate(r)
			}
			if p := r.s.Previous(); p.Available {
				t.Fatalf("Previous = %+v", p)
			}
			if err := r.s.RestorePrevious(); err == nil {
				t.Fatal("restored where it must not")
			}
			select {
			case <-r.up.relaunch:
				t.Fatal("restarted")
			default:
			}
			if b, _ := os.ReadFile(r.up.exe); string(b) != tc.opts.version {
				t.Fatalf("the app changed: %q", b)
			}
		})
	}
}

func TestRestoringWhereSonaDoesNotUpdateItselfIsUnsupported(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.3.0", latest: "0.3.0"}) // unsigned
	if err := r.s.RestorePrevious(); CodeOf(err) != CodeUnsupported {
		t.Fatalf("err = %v", err)
	}
}

func TestRestorePreviousPutsItBackAndRestarts(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.3.0", latest: "0.3.0", key: true})
	r.keepPrevious(t, "0.2.0", "0.3.0")
	if err := r.g.Mark("0.2.0", "0.3.0"); err != nil { // still on trial
		t.Fatal(err)
	}
	if err := r.s.RestorePrevious(); err != nil {
		t.Fatal(err)
	}
	if !r.relaunched(t) {
		t.Fatal("never asked the app to restart")
	}
	if b, _ := os.ReadFile(r.up.exe); string(b) != "0.2.0" {
		t.Fatalf("the app is %q, want the previous version", b)
	}
	if r.g.Pending() != nil {
		t.Fatal("the restored version starts on trial")
	}
	rb := r.g.LastRollback()
	if rb == nil || !rb.Manual || rb.From != "0.3.0" || rb.To != "0.2.0" || rb.Error != "" {
		t.Fatalf("last rollback = %+v", rb)
	}
	// Until Sona has restarted, nothing is left to restore or restore twice.
	if p := r.s.Previous(); p.Available {
		t.Fatalf("Previous = %+v while restarting", p)
	}
	if err := r.s.RestorePrevious(); CodeOf(err) != CodeRestoreFailed {
		t.Fatalf("second RestorePrevious err = %v", err)
	}
	// The Diagnostics bundle tells it was by choice.
	b, ok := r.s.bundle().(map[string]any)
	if !ok || b["lastRollback"] == nil {
		t.Fatalf("bundle = %+v", b)
	}
	if got, _ := b["lastRollback"].(*update.Rollback); got == nil || !got.Manual {
		t.Fatalf("bundle lastRollback = %+v", b["lastRollback"])
	}
}

func TestAFailedRestoreLeavesSonaAsItWasAndCanBeTriedAgain(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.3.0", latest: "0.3.0", key: true})
	r.keepPrevious(t, "0.2.0", "0.3.0")
	// The bundle's own folder is gone: it can't be renamed aside.
	app := r.g.Root + ".moved"
	if err := os.Rename(r.g.Root, app); err != nil {
		t.Fatal(err)
	}
	err := r.s.RestorePrevious()
	if !errors.Is(err, update.ErrRestore) || CodeOf(err) != CodeRestoreFailed {
		t.Fatalf("err = %v", err)
	}
	select {
	case <-r.up.relaunch:
		t.Fatal("restarted after a failed restore")
	default:
	}
	if rb := r.g.LastRollback(); rb == nil || !rb.Manual || rb.Error == "" {
		t.Fatalf("last rollback = %+v", rb)
	}
	if p := r.s.Previous(); !p.Available || p.Version != "0.2.0" {
		t.Fatalf("Previous = %+v after a failure", p)
	}
	// Put things right and try again.
	if err := os.Rename(app, r.g.Root); err != nil {
		t.Fatal(err)
	}
	if err := r.s.RestorePrevious(); err != nil {
		t.Fatal(err)
	}
	if !r.relaunched(t) {
		t.Fatal("never asked the app to restart")
	}
}

func TestTheVersionTheUserWentBackFromIsNotOfferedByTheNotice(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.3.0", latest: "0.3.0", key: true})
	r.keepPrevious(t, "0.2.0", "0.3.0")
	if err := r.s.RestorePrevious(); err != nil {
		t.Fatal(err)
	}
	if !r.relaunched(t) {
		t.Fatal("never asked the app to restart")
	}

	// The next run is 0.2.0, which the release feed says is behind 0.3.0.
	next := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	next.s.guard = &r.g
	next.s.status.Declined = next.s.declined()
	if next.s.Status().Declined != "0.3.0" {
		t.Fatalf("Declined = %q after restoring from 0.3.0", next.s.Status().Declined)
	}
	next.e.Gate.Open() // the start-up check, once the window has painted
	deadline := time.Now().Add(5 * time.Second)
	for next.hit(manifestPath) == 0 || next.s.Status().Checked == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("no start-up check (requests %v)", next.hits)
		}
		time.Sleep(5 * time.Millisecond)
	}
	st := next.s.Status()
	if !st.Available || st.Latest != "0.3.0" {
		t.Fatalf("status = %+v: Settings still says what is out", st)
	}
	if st.Notify() {
		t.Fatal("the sidebar notice offers the version the user just left")
	}

	// A check by hand offers it.
	st, err := next.s.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Declined != "" || !st.Notify() {
		t.Fatalf("after a check by hand: %+v", st)
	}
}

func TestOnlyThatVersionIsHeldBack(t *testing.T) {
	st := UpdateStatus{Available: true, Latest: "0.3.0", Declined: "0.3.0"}
	if st.Notify() {
		t.Error("the declined version is offered")
	}
	st.Latest = "0.3.1"
	if !st.Notify() {
		t.Error("a newer release than the declined one is held back")
	}
	if (UpdateStatus{Latest: "0.3.0", Declined: "0.3.0"}).Notify() {
		t.Error("nothing available is offered")
	}
	if !(UpdateStatus{Available: true, Latest: "0.3.0"}).Notify() {
		t.Error("an update isn't offered without a decline")
	}
}

func TestOnlyAFinishedManualRestoreDeclines(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	declined := func() string { return r.s.declined() }
	if declined() != "" {
		t.Fatal("declined with no restore on record")
	}
	write := func(rb update.Rollback) {
		t.Helper()
		writeRollback(t, r.g, rb)
	}
	write(update.Rollback{From: "0.3.0", To: "0.2.0"}) // the guard's own rollback
	if declined() != "" {
		t.Error("an automatic rollback declines the version")
	}
	write(update.Rollback{From: "0.3.0", To: "0.2.0", Manual: true, Error: "boom"})
	if declined() != "" {
		t.Error("a failed restore declines the version")
	}
	write(update.Rollback{From: "0.4.0", To: "0.1.0", Manual: true})
	if declined() != "" {
		t.Error("a restore to another version declines the version")
	}
	write(update.Rollback{From: "0.3.0", To: "0.2.0", Manual: true})
	if declined() != "0.3.0" {
		t.Errorf("declined = %q after restoring from 0.3.0", declined())
	}
	write(update.Rollback{From: "0.3.0", Manual: true}) // a snapshot without a number
	if declined() != "0.3.0" {
		t.Errorf("declined = %q with the restored version unknown", declined())
	}
}

// writeRollback writes rb as the last rollback in g's state folder.
func writeRollback(t *testing.T, g update.Guard, rb update.Rollback) {
	t.Helper()
	b, err := json.Marshal(rb)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g.State, "update-rollback.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Installing the version the user went back from is choosing it: the
// sidebar follows the install.
func TestInstallingTheDeclinedVersionEndsTheDecline(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	r.s.status.Declined = "0.3.0"
	if _, err := r.s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.s.status.Declined = "0.3.0" // the start-up check left it, Check by hand didn't run
	st, err := r.s.Install(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Declined != "" || !st.Notify() {
		t.Fatalf("status = %+v", st)
	}
}
