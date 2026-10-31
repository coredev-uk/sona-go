package cdm

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// fakeClock is the Manager's clock: Now is set by the test, and every
// After is handed to the test to fire.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	waits chan *fakeTimer
}

type fakeTimer struct {
	d time.Duration
	c chan time.Time
}

func newClock(now time.Time) *fakeClock {
	return &fakeClock{now: now, waits: make(chan *fakeTimer, 64)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	tm := &fakeTimer{d: d, c: make(chan time.Time, 1)}
	c.waits <- tm
	return tm.c
}

// next returns the Manager's next wait: it has finished what came before.
func (c *fakeClock) next(t *testing.T) *fakeTimer {
	t.Helper()
	select {
	case tm := <-c.waits:
		return tm
	case <-time.After(20 * time.Second):
		t.Fatal("the manager never waited")
		return nil
	}
}

func (tm *fakeTimer) fire() { tm.c <- time.Time{} }

// statuses records what Subscribe delivers.
type statuses struct {
	mu  sync.Mutex
	got []Status
}

func (s *statuses) add(st Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.got = append(s.got, st)
}

func (s *statuses) all() []Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.got)
}

// managerFixture is a Manager on the fake source and clock.
type managerFixture struct {
	f   *fakeServer
	clk *fakeClock
	o   Options
	dir string
}

func newManagerFixture(t *testing.T) *managerFixture {
	t.Helper()
	f := newFakeServer(t)
	dir := t.TempDir()
	clk := newClock(t0)
	o := testOptions(t, f, dir)
	o.Now, o.After = clk.Now, clk.After
	return &managerFixture{f: f, clk: clk, o: o, dir: dir}
}

func (mf *managerFixture) manager(t *testing.T) *Manager {
	t.Helper()
	m, err := New(mf.o)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}

// preinstall installs version as checked at checkedAt.
func (mf *managerFixture) preinstall(t *testing.T, version string, checkedAt time.Time) Installed {
	t.Helper()
	mf.f.publish(mf.o.Platform, version, makeCRX(t, mf.o.Platform, version))
	o := mf.o
	o.Now = at(checkedAt)
	inst, err := Ensure(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	return inst
}

func (mf *managerFixture) manifestRequests() int {
	return mf.f.count(manifestHost + "/widevinecdm.json")
}

func wantNoCDM(t *testing.T, err error, sentinel error, code string) {
	t.Helper()
	if !errors.Is(err, sentinel) || !errors.Is(err, playback.ErrNoCDM) {
		t.Fatalf("Library error = %v, want %v", err, sentinel)
	}
	if got := CodeOf(err); code != "" && got != code {
		t.Fatalf("Library error code = %q, want %q", got, code)
	}
}

func TestManagerStatusBeforeStartReadsNoNetwork(t *testing.T) {
	t.Run("nothing installed", func(t *testing.T) {
		mf := newManagerFixture(t)
		m := mf.manager(t)
		if st := m.Status(); st != (Status{State: StateIdle, Source: SourceDownload}) {
			t.Errorf("Status = %+v", st)
		}
		if m.Source() != SourceDownload {
			t.Errorf("Source = %s", m.Source())
		}
		_, err := m.Library()
		wantNoCDM(t, err, ErrUnavailable, "")
		if mf.f.total() != 0 {
			t.Error("requests made")
		}
	})
	t.Run("installed", func(t *testing.T) {
		mf := newManagerFixture(t)
		inst := mf.preinstall(t, "1.0.0.0", t0.Add(-time.Hour))
		requests := mf.f.total()
		m := mf.manager(t)
		st := m.Status()
		if st.State != StateReady || st.Version != "1.0.0.0" || st.Path != inst.Path || !st.CheckedAt.Equal(t0.Add(-time.Hour)) || st.Code != "" {
			t.Errorf("Status = %+v", st)
		}
		if lib, err := m.Library(); err != nil || lib != inst.Path {
			t.Errorf("Library = %s, %v", lib, err)
		}
		if mf.f.total() != requests {
			t.Error("requests made")
		}
	})
}

func TestManagerStartInstalls(t *testing.T) {
	mf := newManagerFixture(t)
	crx := makeCRX(t, mf.o.Platform, "1.0.0.0")
	mf.f.publish(mf.o.Platform, "1.0.0.0", crx)
	m := mf.manager(t)
	var rec statuses
	cancel := m.Subscribe(rec.add)
	defer cancel()
	m.Start()
	if tm := mf.clk.next(t); tm.d != DefaultUpdateInterval {
		t.Errorf("next check in %v, want %v", tm.d, DefaultUpdateInterval)
	}
	got := rec.all()
	if len(got) < 3 {
		t.Fatalf("statuses = %+v", got)
	}
	if got[0].State != StateDownloading || got[0].Received != 0 {
		t.Errorf("first status = %+v", got[0])
	}
	for _, st := range got[1 : len(got)-1] {
		if st.State != StateDownloading || st.Total != int64(len(crx)) || st.Received <= 0 || st.Received > st.Total {
			t.Errorf("progress status = %+v", st)
		}
	}
	last := got[len(got)-1]
	lib := filepath.Join(mf.dir, "1.0.0.0", "linux_x64", "libwidevinecdm.so")
	if last.State != StateReady || last.Version != "1.0.0.0" || last.Path != lib || last.Received != 0 || last.Updating || !last.CheckedAt.Equal(t0) {
		t.Errorf("last status = %+v", last)
	}
	if st := m.Status(); st != last {
		t.Errorf("Status = %+v, want %+v", st, last)
	}
	if got, err := m.Library(); err != nil || got != lib {
		t.Errorf("Library = %s, %v", got, err)
	}
}

// A fresh data directory has no widevine directory yet: the first check
// creates it.
func TestManagerStartInstallsIntoAMissingDirectory(t *testing.T) {
	mf := newManagerFixture(t)
	mf.dir = filepath.Join(mf.dir, "data", "sona", "widevine")
	mf.o.Dir = mf.dir
	mf.f.publish(mf.o.Platform, "1.0.0.0", makeCRX(t, mf.o.Platform, "1.0.0.0"))
	m := mf.manager(t)
	m.Start()
	mf.clk.next(t)
	if st := m.Status(); st.State != StateReady || st.Version != "1.0.0.0" || st.Code != "" {
		t.Errorf("Status = %+v", st)
	}
	wantInstalled(t, mf.dir, "1.0.0.0")
}

func TestManagerLibraryWhileDownloading(t *testing.T) {
	mf := newManagerFixture(t)
	crx := makeCRX(t, mf.o.Platform, "1.0.0.0")
	pub := mf.f.publish(mf.o.Platform, "1.0.0.0", crx)
	started, release := make(chan struct{}, 1), make(chan struct{})
	mf.f.handle(pub.pkg, func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(crx)))
		_, _ = w.Write(crx)
	})
	m := mf.manager(t)
	m.Start()
	<-started
	if st := m.Status(); st.State != StateDownloading {
		t.Errorf("Status = %+v", st)
	}
	_, err := m.Library()
	wantNoCDM(t, err, ErrDownloading, "")
	close(release)
	mf.clk.next(t)
	if _, err := m.Library(); err != nil {
		t.Fatal(err)
	}
}

func TestManagerFailureThenRetry(t *testing.T) {
	mf := newManagerFixture(t)
	mf.o.RetryInterval = 7 * time.Minute
	p := mf.o.Platform
	crx := makeCRX(t, p, "1.0.0.0")
	pub := mf.f.publish(p, "1.0.0.0", crx)
	sum := sha512.Sum512([]byte("not it"))
	pub.entry["hashValue"] = hex.EncodeToString(sum[:])
	mf.f.serveManifest("1.0.0.0", map[string]any{p.Mozilla: pub.entry})

	m := mf.manager(t)
	m.Start()
	if tm := mf.clk.next(t); tm.d != 7*time.Minute {
		t.Errorf("retry in %v, want the retry interval", tm.d)
	}
	if st := m.Status(); st.State != StateFailed || st.Code != CodeHashMismatch {
		t.Errorf("Status = %+v", st)
	}
	_, err := m.Library()
	wantNoCDM(t, err, ErrUnavailable, CodeHashMismatch)
	wantInstalled(t, mf.dir)

	mf.f.publish(p, "1.0.0.0", crx)
	m.Retry()
	if tm := mf.clk.next(t); tm.d != DefaultUpdateInterval {
		t.Errorf("next check in %v", tm.d)
	}
	if st := m.Status(); st.State != StateReady || st.Code != "" || st.Version != "1.0.0.0" {
		t.Errorf("Status after Retry = %+v", st)
	}
}

func TestManagerRetryStarts(t *testing.T) {
	mf := newManagerFixture(t)
	mf.f.publish(mf.o.Platform, "1.0.0.0", makeCRX(t, mf.o.Platform, "1.0.0.0"))
	m := mf.manager(t)
	m.Retry()
	mf.clk.next(t)
	if st := m.Status(); st.State != StateReady {
		t.Errorf("Status = %+v", st)
	}
}

func TestManagerChecksForUpdatesOncePerInterval(t *testing.T) {
	t.Run("checked recently", func(t *testing.T) {
		mf := newManagerFixture(t)
		mf.preinstall(t, "1.0.0.0", t0.Add(-2*time.Hour))
		manifests := mf.manifestRequests()
		m := mf.manager(t)
		m.Start()
		tm := mf.clk.next(t)
		if tm.d != 22*time.Hour {
			t.Errorf("first check in %v, want 22h", tm.d)
		}
		if mf.manifestRequests() != manifests {
			t.Error("checked at Start")
		}
		// When it is due, the check runs and the next is a day later.
		mf.f.publish(mf.o.Platform, "2.0.0.0", makeCRX(t, mf.o.Platform, "2.0.0.0"))
		mf.clk.set(t0.Add(22 * time.Hour))
		tm.fire()
		if tm := mf.clk.next(t); tm.d != DefaultUpdateInterval {
			t.Errorf("next check in %v", tm.d)
		}
		if st := m.Status(); st.Version != "2.0.0.0" || !st.CheckedAt.Equal(t0.Add(22*time.Hour)) {
			t.Errorf("Status = %+v", st)
		}
	})
	t.Run("checked over a day ago", func(t *testing.T) {
		mf := newManagerFixture(t)
		mf.preinstall(t, "1.0.0.0", t0.Add(-25*time.Hour))
		manifests, downloads := mf.manifestRequests(), mf.f.packageRequests()
		m := mf.manager(t)
		m.Start()
		if tm := mf.clk.next(t); tm.d != DefaultUpdateInterval {
			t.Errorf("next check in %v", tm.d)
		}
		if mf.manifestRequests() != manifests+1 || mf.f.packageRequests() != downloads {
			t.Errorf("requests: %v", mf.f.hits)
		}
		if ptr := readPointer(t, mf.dir); !ptr.CheckedAt.Equal(t0) {
			t.Errorf("CheckedAt = %v", ptr.CheckedAt)
		}
		if st := m.Status(); st.State != StateReady || !st.CheckedAt.Equal(t0) || st.Updating {
			t.Errorf("Status = %+v", st)
		}
	})
}

func TestManagerBacksOffAfterFailedChecks(t *testing.T) {
	for _, installed := range []bool{false, true} {
		t.Run("installed="+strconv.FormatBool(installed), func(t *testing.T) {
			mf := newManagerFixture(t)
			mf.o.RetryInterval, mf.o.UpdateInterval = time.Hour, 5*time.Hour
			if installed {
				mf.preinstall(t, "1.0.0.0", t0.Add(-48*time.Hour))
			}
			mf.f.status(manifestHost+"/widevinecdm.json", http.StatusServiceUnavailable)
			m := mf.manager(t)
			m.Start()
			for i, want := range []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 5 * time.Hour, 5 * time.Hour} {
				tm := mf.clk.next(t)
				if tm.d != want {
					t.Errorf("wait %d = %v, want %v", i, tm.d, want)
				}
				st := m.Status()
				if st.Code != CodeNetwork || (st.State == StateReady) != installed {
					t.Errorf("Status = %+v", st)
				}
				if i == 4 {
					// The check passes again: back to the update interval.
					mf.f.publish(mf.o.Platform, "1.0.0.0", makeCRX(t, mf.o.Platform, "1.0.0.0"))
				}
				tm.fire()
			}
			if tm := mf.clk.next(t); tm.d != 5*time.Hour {
				t.Errorf("wait after a passing check = %v", tm.d)
			}
			if st := m.Status(); st.State != StateReady || st.Code != "" {
				t.Errorf("Status = %+v", st)
			}
		})
	}
}

func TestManagerRetriesAFailedUpdateDownloadSoon(t *testing.T) {
	mf := newManagerFixture(t)
	mf.o.RetryInterval = 10 * time.Minute
	mf.preinstall(t, "1.0.0.0", t0.Add(-48*time.Hour))
	p := mf.o.Platform
	pub := mf.f.publish(p, "2.0.0.0", makeCRX(t, p, "2.0.0.0"))
	mf.f.status(pub.pkg, http.StatusBadGateway)
	mf.f.status(pub.mirror, http.StatusBadGateway)
	m := mf.manager(t)
	m.Start()
	if tm := mf.clk.next(t); tm.d != 10*time.Minute {
		t.Errorf("retry in %v, want the retry interval", tm.d)
	}
	if st := m.Status(); st.State != StateReady || st.Version != "1.0.0.0" || st.Code != CodeNetwork || !st.CheckedAt.Equal(t0.Add(-48*time.Hour)) {
		t.Errorf("Status = %+v", st)
	}
}

func TestManagerUpdateAppliesToTheNextHostOnly(t *testing.T) {
	mf := newManagerFixture(t)
	mf.preinstall(t, "1.0.0.0", t0.Add(-48*time.Hour))
	m := mf.manager(t)
	lib1, err := m.Library()
	if err != nil {
		t.Fatal(err)
	}
	var rec statuses
	m.Subscribe(rec.add)
	mf.f.publish(mf.o.Platform, "2.0.0.0", makeCRX(t, mf.o.Platform, "2.0.0.0"))
	m.Start()
	mf.clk.next(t)
	lib2, err := m.Library()
	if err != nil || lib2 == lib1 || !strings.Contains(lib2, "2.0.0.0") {
		t.Fatalf("Library = %s, %v", lib2, err)
	}
	if _, err := os.Stat(lib1); err != nil {
		t.Errorf("the running host's library is gone: %v", err)
	}
	if ptr := readPointer(t, mf.dir); ptr.Current != "2.0.0.0" || ptr.Previous != "1.0.0.0" {
		t.Errorf("pointer = %+v", ptr)
	}
	got := rec.all()
	if len(got) == 0 || !got[0].Updating || got[0].State != StateReady || got[0].Version != "1.0.0.0" {
		t.Errorf("first status = %+v", got)
	}
	for _, st := range got {
		if st.State != StateReady {
			t.Errorf("status during the update = %+v", st)
		}
	}
}

// rollbackFixture has 1.0.0.0 as Previous and 2.0.0.0 as Current, and a
// Manager that is not started.
func rollbackFixture(t *testing.T, versions ...string) (*managerFixture, *Manager, []string) {
	t.Helper()
	mf := newManagerFixture(t)
	libs := make([]string, 0, len(versions))
	for _, v := range versions {
		ptr := installVersion(t, mf.f, mf.o, v)
		libs = append(libs, store{root: mf.dir, p: mf.o.Platform}.lib(ptr.Current))
	}
	return mf, mf.manager(t), libs
}

// report runs ReportInit and waits for it to be recorded.
func report(m *Manager, lib string, err error) {
	m.ReportInit(lib, err)
	m.wg.Wait()
}

var errInit = errors.New("CDM failed to initialise")

func TestManagerRollsBackAfterRepeatedLoadFailures(t *testing.T) {
	mf, m, libs := rollbackFixture(t, "1.0.0.0", "2.0.0.0")
	var rec statuses
	m.Subscribe(rec.add)
	for i := 1; i < rollbackAfter; i++ {
		report(m, libs[1], errInit)
		if ptr := readPointer(t, mf.dir); ptr.Failures != i || ptr.Current != "2.0.0.0" {
			t.Fatalf("after %d failures: pointer %+v", i, ptr)
		}
	}
	report(m, libs[1], errInit)
	if lib, err := m.Library(); err != nil || lib != libs[0] {
		t.Errorf("Library = %s, %v; want %s", lib, err, libs[0])
	}
	ptr := readPointer(t, mf.dir)
	if ptr.Current != "1.0.0.0" || ptr.Previous != "" || ptr.Failures != 0 || ptr.Rejected["2.0.0.0"] != CodeLoad {
		t.Errorf("pointer = %+v", ptr)
	}
	wantInstalled(t, mf.dir, "1.0.0.0")
	if st := m.Status(); st.State != StateReady || st.Version != "1.0.0.0" {
		t.Errorf("Status = %+v", st)
	}
	if got := rec.all(); len(got) == 0 || got[len(got)-1].Version != "1.0.0.0" {
		t.Errorf("statuses = %+v", got)
	}

	// The rolled-back version is not installed again.
	downloads := mf.f.packageRequests()
	if _, changed, err := runCheck(t, mf.o); err != nil || changed {
		t.Fatalf("check: changed %v, %v", changed, err)
	}
	if mf.f.packageRequests() != downloads {
		t.Error("downloaded the rolled-back version again")
	}
}

func TestManagerSuccessResetsTheFailureCount(t *testing.T) {
	mf, m, libs := rollbackFixture(t, "1.0.0.0", "2.0.0.0")
	for _, err := range []error{errInit, errInit, nil, errInit, errInit} {
		report(m, libs[1], err)
	}
	if lib, err := m.Library(); err != nil || lib != libs[1] {
		t.Errorf("Library = %s, %v", lib, err)
	}
	if ptr := readPointer(t, mf.dir); ptr.Current != "2.0.0.0" || ptr.Failures != 2 {
		t.Errorf("pointer = %+v", ptr)
	}
	wantInstalled(t, mf.dir, "1.0.0.0", "2.0.0.0")
}

// Reports arriving faster than they are recorded (hosts restarting while
// an update check holds the lock) are recorded in call order, and a
// success behind queued failures still resets the count.
func TestManagerRecordsQueuedReportsInOrder(t *testing.T) {
	mf, m, libs := rollbackFixture(t, "1.0.0.0", "2.0.0.0")
	m.op.Lock() // an update check is running: nothing is recorded yet
	for _, err := range []error{errInit, errInit, nil, errInit, errInit} {
		m.ReportInit(libs[1], err)
	}
	m.op.Unlock()
	m.wg.Wait()
	if ptr := readPointer(t, mf.dir); ptr.Current != "2.0.0.0" || ptr.Failures != 2 {
		t.Errorf("pointer = %+v; want 2.0.0.0 with 2 failures", ptr)
	}
	if lib, err := m.Library(); err != nil || lib != libs[1] {
		t.Errorf("Library = %s, %v", lib, err)
	}
	wantInstalled(t, mf.dir, "1.0.0.0", "2.0.0.0")
}

// damage truncates a library, as a full disk or an interrupted copy
// would.
func damage(t *testing.T, lib string) {
	t.Helper()
	if err := os.Truncate(lib, 4); err != nil {
		t.Fatal(err)
	}
}

func fileSHA256(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestManagerReplacesADamagedInstallAtStart(t *testing.T) {
	mf := newManagerFixture(t)
	inst := mf.preinstall(t, "1.0.0.0", t0.Add(-2*time.Hour))
	want := fileSHA256(t, inst.Path)
	if got := readPointer(t, mf.dir).SHA256["1.0.0.0"]; got != want {
		t.Fatalf("recorded SHA-256 %q, want %q", got, want)
	}
	damage(t, inst.Path)
	downloads := mf.f.packageRequests()
	m := mf.manager(t)
	if st := m.Status(); st.State != StateReady {
		t.Fatalf("Status before Start = %+v (it reads no library)", st)
	}
	m.Start()
	if tm := mf.clk.next(t); tm.d != DefaultUpdateInterval {
		t.Errorf("next check in %v", tm.d)
	}
	if mf.f.packageRequests() != downloads+1 {
		t.Errorf("downloaded %d packages, want 1", mf.f.packageRequests()-downloads)
	}
	if lib, err := m.Library(); err != nil || lib != inst.Path || fileSHA256(t, lib) != want {
		t.Errorf("Library = %s, %v", lib, err)
	}
	if ptr := readPointer(t, mf.dir); ptr.Current != "1.0.0.0" || ptr.SHA256["1.0.0.0"] != want || len(ptr.Rejected) != 0 {
		t.Errorf("pointer = %+v", ptr)
	}
}

func TestManagerHostFailureWithADamagedLibrary(t *testing.T) {
	t.Run("previous version kept", func(t *testing.T) {
		mf, m, libs := rollbackFixture(t, "1.0.0.0", "2.0.0.0")
		damage(t, libs[1])
		report(m, libs[1], errInit) // one failure is enough: no rollback count
		ptr := readPointer(t, mf.dir)
		if ptr.Current != "1.0.0.0" || ptr.Previous != "" || ptr.Failures != 0 || len(ptr.Rejected) != 0 || ptr.SHA256["2.0.0.0"] != "" {
			t.Errorf("pointer = %+v; want back on 1.0.0.0, 2.0.0.0 not rejected", ptr)
		}
		if lib, err := m.Library(); err != nil || lib != libs[0] {
			t.Errorf("Library = %s, %v", lib, err)
		}
		wantInstalled(t, mf.dir, "1.0.0.0")
	})
	t.Run("only version", func(t *testing.T) {
		mf := newManagerFixture(t)
		inst := mf.preinstall(t, "1.0.0.0", t0.Add(-2*time.Hour))
		want := fileSHA256(t, inst.Path)
		m := mf.manager(t)
		m.Start()
		mf.clk.next(t) // the next update check, 22 h away
		downloads := mf.f.packageRequests()
		damage(t, inst.Path)
		m.ReportInit(inst.Path, errInit)
		// The download starts at once, not when the update check is due.
		if tm := mf.clk.next(t); tm.d != DefaultUpdateInterval {
			t.Errorf("next check in %v", tm.d)
		}
		if mf.f.packageRequests() != downloads+1 {
			t.Errorf("downloaded %d packages, want 1", mf.f.packageRequests()-downloads)
		}
		if lib, err := m.Library(); err != nil || lib != inst.Path || fileSHA256(t, lib) != want {
			t.Errorf("Library = %s, %v", lib, err)
		}
	})
}

func TestEnsureReplacesADamagedInstall(t *testing.T) {
	f := newFakeServer(t)
	o := testOptions(t, f, t.TempDir())
	f.publish(o.Platform, "1.0.0.0", makeCRX(t, o.Platform, "1.0.0.0"))
	inst, err := Ensure(t.Context(), o)
	if err != nil {
		t.Fatal(err)
	}
	want := fileSHA256(t, inst.Path)
	downloads := f.packageRequests()
	if again, err := Ensure(t.Context(), o); err != nil || again != inst || f.packageRequests() != downloads {
		t.Fatalf("intact: Ensure = %+v, %v; %d downloads", again, err, f.packageRequests()-downloads)
	}
	damage(t, inst.Path)
	if again, err := Ensure(t.Context(), o); err != nil || again != inst || f.packageRequests() != downloads+1 {
		t.Fatalf("damaged: Ensure = %+v, %v; %d downloads", again, err, f.packageRequests()-downloads)
	}
	if got := fileSHA256(t, inst.Path); got != want {
		t.Errorf("library SHA-256 %s, want %s", got, want)
	}
}

func TestManagerKeepsTheOnlyVersionThatFails(t *testing.T) {
	mf, m, libs := rollbackFixture(t, "1.0.0.0")
	for range rollbackAfter + 1 {
		report(m, libs[0], errInit)
	}
	if lib, err := m.Library(); err != nil || lib != libs[0] {
		t.Errorf("Library = %s, %v", lib, err)
	}
	if ptr := readPointer(t, mf.dir); ptr.Current != "1.0.0.0" || len(ptr.Rejected) != 0 {
		t.Errorf("pointer = %+v", ptr)
	}
	wantInstalled(t, mf.dir, "1.0.0.0")
}

func TestManagerIgnoresReportsForOtherLibraries(t *testing.T) {
	mf, m, libs := rollbackFixture(t, "1.0.0.0", "2.0.0.0")
	before := readPointer(t, mf.dir)
	for range rollbackAfter {
		report(m, libs[0], errInit)
		report(m, filepath.Join(t.TempDir(), "libwidevinecdm.so"), errInit)
		report(m, "", errInit)
	}
	after := readPointer(t, mf.dir)
	if after.Current != "2.0.0.0" || after.Failures != 0 || !after.CheckedAt.Equal(before.CheckedAt) {
		t.Errorf("pointer = %+v", after)
	}
	if lib, err := m.Library(); err != nil || lib != libs[1] {
		t.Errorf("Library = %s, %v", lib, err)
	}
}

func TestManagerOverride(t *testing.T) {
	writeLib := func(t *testing.T, dir string) string {
		t.Helper()
		lib := filepath.Join(dir, "libwidevinecdm.so")
		if err := os.WriteFile(lib, fakeLibrary(linuxAMD64(t)), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, manifestFile), componentManifestJSON(componentName, "4.10.2830.0"), 0o600); err != nil {
			t.Fatal(err)
		}
		return lib
	}
	ready := func(t *testing.T, override, want string) {
		t.Helper()
		mf := newManagerFixture(t)
		mf.o.Override = override
		m := mf.manager(t)
		if m.Source() != SourceOverride {
			t.Errorf("Source = %s", m.Source())
		}
		if st := m.Status(); st != (Status{State: StateReady, Source: SourceOverride, Path: want, Version: "4.10.2830.0"}) {
			t.Errorf("Status = %+v", st)
		}
		if lib, err := m.Library(); err != nil || lib != want {
			t.Errorf("Library = %s, %v", lib, err)
		}
		m.Start()
		m.Retry()
		for range rollbackAfter {
			report(m, want, errInit)
		}
		m.Close()
		if mf.f.total() != 0 || len(mf.clk.waits) != 0 {
			t.Errorf("override made %d requests, %d waits", mf.f.total(), len(mf.clk.waits))
		}
		if names := dirNames(t, mf.dir); len(names) != 0 {
			t.Errorf("install directory holds %q", names)
		}
	}
	t.Run("file", func(t *testing.T) {
		dir := t.TempDir()
		lib := writeLib(t, dir)
		ready(t, lib, lib)
	})
	t.Run("directory", func(t *testing.T) {
		dir := t.TempDir()
		lib := writeLib(t, dir)
		ready(t, dir, lib)
	})
	t.Run("wins over an installed CDM", func(t *testing.T) {
		mf := newManagerFixture(t)
		mf.preinstall(t, "1.0.0.0", t0)
		dir := t.TempDir()
		mf.o.Override = writeLib(t, dir)
		m := mf.manager(t)
		if lib, err := m.Library(); err != nil || lib != mf.o.Override {
			t.Errorf("Library = %s, %v", lib, err)
		}
	})

	missing := map[string]func(t *testing.T) string{
		"missing file":          func(t *testing.T) string { return filepath.Join(t.TempDir(), "libwidevinecdm.so") },
		"directory without one": func(t *testing.T) string { return t.TempDir() },
		"relative":              func(*testing.T) string { return "libwidevinecdm.so" },
		"directory named as the library": func(t *testing.T) string {
			dir := filepath.Join(t.TempDir(), "libwidevinecdm.so")
			if err := os.MkdirAll(filepath.Join(dir, "libwidevinecdm.so"), 0o700); err != nil {
				t.Fatal(err)
			}
			return dir
		},
	}
	for name, path := range missing {
		t.Run(name, func(t *testing.T) {
			mf := newManagerFixture(t)
			mf.o.Override = path(t)
			m := mf.manager(t)
			if st := m.Status(); st.State != StateFailed || st.Code != CodeOverrideMissing || st.Source != SourceOverride {
				t.Errorf("Status = %+v", st)
			}
			_, err := m.Library()
			wantNoCDM(t, err, ErrUnavailable, CodeOverrideMissing)
			m.Start()
			m.Close()
			if mf.f.total() != 0 {
				t.Error("requests made")
			}
		})
	}
}

func TestManagerCloseCancelsADownload(t *testing.T) {
	mf := newManagerFixture(t)
	crx := makeCRX(t, mf.o.Platform, "1.0.0.0")
	pub := mf.f.publish(mf.o.Platform, "1.0.0.0", crx)
	started := make(chan struct{}, 1)
	mf.f.handle(pub.pkg, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(crx)))
		_, _ = w.Write(crx[:len(crx)/2])
		w.(http.Flusher).Flush()
		started <- struct{}{}
		<-r.Context().Done()
	})
	m := mf.manager(t)
	m.Start()
	<-started
	done := make(chan struct{})
	begin := time.Now()
	go func() {
		m.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not return")
	}
	if d := time.Since(begin); d > 5*time.Second {
		t.Errorf("Close took %v", d)
	}
	if _, err := m.Library(); err == nil {
		t.Error("Library succeeded")
	}
	wantInstalled(t, mf.dir)
	if mf.f.count(pub.mirror) != 0 {
		t.Error("tried the mirror after Close")
	}
	// Closed: nothing starts again.
	m.Start()
	m.Retry()
	m.ReportInit("x", errInit)
}
