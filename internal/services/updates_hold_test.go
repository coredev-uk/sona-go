package services

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
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
}

func (e *holdEngine) HoldHost(ctx context.Context) (func(), error) {
	if e.holdErr != nil {
		return nil, e.holdErr
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

// The host has GStreamer's plugins in lib\ open, and a folder holding a loaded
// DLL can't be renamed: the playback host is let go of after the download and
// before the swap, and stays gone while Sona restarts.
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
