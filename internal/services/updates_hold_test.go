package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/playback/fakeengine"
	"github.com/coredev-uk/sona-go/internal/update"
)

// holdEngine is a playback engine with a host process to let go of
// (playback.HostHolder): the fake engine, pausing on a hold.
type holdEngine struct {
	*fakeengine.Engine
	mu              sync.Mutex
	holds, releases int
	holdErr         error
	onHold          func() // runs as the host is let go of
}

func (e *holdEngine) HoldHost(ctx context.Context) (func(), error) {
	if e.holdErr != nil {
		return nil, e.holdErr
	}
	if e.onHold != nil {
		e.onHold()
	}
	e.mu.Lock()
	e.holds++
	e.mu.Unlock()
	_ = e.Pause(ctx)
	return func() {
		e.mu.Lock()
		e.releases++
		e.mu.Unlock()
	}, nil
}

// held is how many holds are in force.
func (e *holdEngine) held() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.holds - e.releases
}

// windowsRig is selfRig with the install laid out as on Windows (the exe in a
// folder whose entries the update swaps) and a coordinator playing on a
// holdEngine.
func windowsRig(t *testing.T) (*selfRig, *holdEngine) {
	t.Helper()
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	folder := filepath.Join(t.TempDir(), "Sona")
	exe := filepath.Join(folder, "sona.exe")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("0.2.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	g, ok := update.NewGuard(r.g.State, exe, "windows")
	if !ok {
		t.Fatal("no guard")
	}
	r.g, r.s.guard, r.up.exe = g, &g, exe

	eng := &holdEngine{Engine: fakeengine.New()}
	coord, err := coordinator.New(coordinator.Options{Engine: eng})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	r.e.Coord = coord
	if err := coord.SetQueue(t.Context(), []coordinator.Item{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}, 1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r, eng
}

// A host that outlived the swap would load the new version's plugins from
// lib\: the playback host is let go of after the download and before the
// swap, and stays gone while Sona restarts.
func TestWindowsInstallLetsGoOfThePlaybackHostBeforeTheSwap(t *testing.T) {
	r, eng := windowsRig(t)
	heldAtSwap, before := -1, eng.held()
	r.up.beforeSwap = func() { heldAtSwap = eng.held() }
	if before != 0 {
		t.Fatalf("held before the install: %d", before)
	}
	st, err := r.s.Install(context.Background())
	if err != nil || !st.Installed {
		t.Fatalf("Install = %+v, %v", st, err)
	}
	if heldAtSwap != 1 {
		t.Fatalf("holds in force when MyGo swaps: %d, want 1", heldAtSwap)
	}
	if eng.held() != 1 {
		t.Errorf("the host was released after a good install (holds %d, releases %d): playback would start one in the files being replaced", eng.holds, eng.releases)
	}
	if s := r.e.Coord.Snapshot(); s.Current == nil || s.Current.Item.ID != "b" || len(s.Queue) != 2 {
		t.Errorf("the queue or the current item changed: %+v", s.Current)
	}
	select {
	case <-r.up.relaunch:
	case <-time.After(5 * time.Second):
		t.Fatal("never relaunched")
	}
}

// A failed install leaves playback paused where it was and able to start a
// host again.
func TestWindowsFailedInstallLeavesTheHostRestartable(t *testing.T) {
	r, eng := windowsRig(t)
	r.up.failWith = errors.New(`mygo: installing the update: rename lib: Access is denied.`)
	heldAtFailure := -1
	r.up.beforeSwap = func() { heldAtFailure = eng.held() }
	if _, err := r.s.Install(context.Background()); CodeOf(err) != CodeUpdateFailed {
		t.Fatalf("err = %v", err)
	}
	if heldAtFailure != 1 {
		t.Fatalf("the host was not let go of before the swap: %d", heldAtFailure)
	}
	if eng.held() != 0 || eng.releases != 1 {
		t.Fatalf("after the failure: holds %d, releases %d, want the host released", eng.holds, eng.releases)
	}
	coord := r.e.Coord
	if s := coord.Snapshot(); s.Current == nil || s.Current.Item.ID != "b" {
		t.Fatalf("current = %+v", s.Current)
	}
	// Playing again goes to the engine, which starts a host where it must.
	if err := coord.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls := eng.Calls(); !slices.ContainsFunc(calls, func(c string) bool { return len(c) >= 6 && c[:6] == "resume" }) {
		t.Errorf("Resume never reached the engine: %v", calls)
	}
}

// A host that won't let go does not stop the install: the swap may still go,
// and if it can't, the install fails and says so.
func TestWindowsInstallGoesOnWhenTheHostWontLetGo(t *testing.T) {
	r, eng := windowsRig(t)
	eng.holdErr = errors.New("host stuck")
	st, err := r.s.Install(context.Background())
	if err != nil || !st.Installed {
		t.Fatalf("Install = %+v, %v", st, err)
	}
	if eng.holds != 0 {
		t.Fatalf("holds = %d", eng.holds)
	}
}

// macOS replaces the bundle after Sona has quit: nothing of the app's is held
// open, and the music is not interrupted.
func TestMacInstallLeavesPlaybackAlone(t *testing.T) {
	r := newSelfRig(t, selfOpts{version: "0.2.0", latest: "0.3.0", key: true})
	eng := &holdEngine{Engine: fakeengine.New()}
	coord, err := coordinator.New(coordinator.Options{Engine: eng})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = coord.Close() })
	r.e.Coord = coord
	if _, err := r.s.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st, err := r.s.Install(context.Background()); err != nil || !st.Installed {
		t.Fatalf("Install = %+v, %v", st, err)
	}
	if eng.holds != 0 {
		t.Fatalf("the host was held for an update that replaces the bundle after quit: %d", eng.holds)
	}
}

// keepWindowsPrevious leaves the windowsRig as an update from 0.1.0 does:
// 0.1.0 is kept in previous\ (its number in the state folder) and the running
// sona.exe is the rig's 0.2.0.
func keepWindowsPrevious(t *testing.T, r *selfRig) {
	t.Helper()
	swap := func(v string) {
		_ = os.Remove(r.up.exe)
		if err := os.WriteFile(r.up.exe, []byte(v), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	swap("0.1.0")
	if err := r.g.Stage("0.1.0"); err != nil {
		t.Fatal(err)
	}
	if err := r.g.Commit(); err != nil {
		t.Fatal(err)
	}
	swap("0.2.0")
	if p := r.s.Previous(); !p.Available || p.Version != "0.1.0" {
		t.Fatalf("Previous = %+v", p)
	}
}

// A manual restore renames lib\ as an update does: the playback host is let
// go of before the files move, and stays gone while Sona restarts.
func TestWindowsRestoreLetsGoOfThePlaybackHostFirst(t *testing.T) {
	r, eng := windowsRig(t)
	keepWindowsPrevious(t, r)
	runningAtHold := ""
	eng.onHold = func() {
		b, _ := os.ReadFile(r.up.exe)
		runningAtHold = string(b)
	}
	if err := r.s.RestorePrevious(); err != nil {
		t.Fatal(err)
	}
	if runningAtHold != "0.2.0" {
		t.Fatalf("the host was let go of with sona.exe at %q, want before the restore moved it", runningAtHold)
	}
	if eng.held() != 1 {
		t.Errorf("holds in force after a good restore: %d, want 1 (a play would start a host in the files being replaced)", eng.held())
	}
	if b, _ := os.ReadFile(r.up.exe); string(b) != "0.1.0" {
		t.Fatalf("the app is %q, want the previous version", b)
	}
	if s := r.e.Coord.Snapshot(); s.Current == nil || s.Current.Item.ID != "b" || len(s.Queue) != 2 {
		t.Errorf("the queue or the current item changed: %+v", s.Current)
	}
	if !r.relaunched(t) {
		t.Fatal("never asked the app to restart")
	}
}

// A failed restore lets playback start a host again, and can be tried again.
func TestWindowsFailedRestoreReleasesTheHost(t *testing.T) {
	r, eng := windowsRig(t)
	keepWindowsPrevious(t, r)
	// sona.exe can't be set aside: the name it would take is a folder in the way.
	blocker := filepath.Join(r.g.Root, ".sona.exe.old-"+strconv.Itoa(os.Getpid()))
	if err := os.MkdirAll(filepath.Join(blocker, "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.s.RestorePrevious(); CodeOf(err) != CodeRestoreFailed {
		t.Fatalf("err = %v", err)
	}
	if eng.holds != 1 || eng.held() != 0 {
		t.Fatalf("after the failure: holds %d, releases %d, want one hold released", eng.holds, eng.releases)
	}
	if b, _ := os.ReadFile(r.up.exe); string(b) != "0.2.0" {
		t.Fatalf("the app is %q after a failed restore", b)
	}
	if err := r.e.Coord.Resume(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls := eng.Calls(); !slices.ContainsFunc(calls, func(c string) bool { return len(c) >= 6 && c[:6] == "resume" }) {
		t.Errorf("Resume never reached the engine: %v", calls)
	}
	if err := os.RemoveAll(blocker); err != nil {
		t.Fatal(err)
	}
	if err := r.s.RestorePrevious(); err != nil {
		t.Fatal(err)
	}
	if !r.relaunched(t) {
		t.Fatal("never asked the app to restart")
	}
}

// A host that won't let go does not stop the restore.
func TestWindowsRestoreGoesOnWhenTheHostWontLetGo(t *testing.T) {
	r, eng := windowsRig(t)
	keepWindowsPrevious(t, r)
	eng.holdErr = errors.New("host stuck")
	if err := r.s.RestorePrevious(); err != nil {
		t.Fatal(err)
	}
	if !r.relaunched(t) {
		t.Fatal("never asked the app to restart")
	}
}
