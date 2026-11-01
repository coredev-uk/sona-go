package cdmhost

import (
	"encoding/gob"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"

	"github.com/coredev-uk/sona-go/internal/app/dbustest"
	"github.com/coredev-uk/sona-go/internal/playback"
)

// fakeRTKit serves RTKit's methods (and the portal's, which match) on a
// private bus. It refuses thread 0.
type fakeRTKit struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeRTKit) record(call string, tid uint64) *dbus.Error {
	if tid == 0 {
		return dbus.NewError("org.freedesktop.DBus.Error.AccessDenied", []any{"Operation not permitted"})
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	return nil
}

func (f *fakeRTKit) MakeThreadRealtimeWithPID(pid, tid uint64, priority uint32) *dbus.Error {
	return f.record(fmt.Sprintf("realtime %d/%d@%d", pid, tid, priority), tid)
}

func (f *fakeRTKit) MakeThreadHighPriorityWithPID(pid, tid uint64, nice int32) *dbus.Error {
	return f.record(fmt.Sprintf("nice %d/%d@%d", pid, tid, nice), tid)
}

// fakeProps serves org.freedesktop.DBus.Properties.Get for one interface.
type fakeProps struct {
	iface string
	props map[string]any
}

func (p fakeProps) Get(iface, name string) (dbus.Variant, *dbus.Error) {
	v, ok := p.props[name]
	if iface != p.iface || !ok {
		return dbus.Variant{}, dbus.NewError("org.freedesktop.DBus.Error.UnknownProperty", []any{name})
	}
	return dbus.MakeVariant(v), nil
}

// busBroker speaks RTKit's and the portal's D-Bus interface: it reads the
// limits from the properties and calls the methods with their signatures.
func TestBusBroker(t *testing.T) {
	addr := dbustest.Start(t)
	server := dbustest.Conn(t, addr)
	connect := func(o ...dbus.ConnOption) (*dbus.Conn, error) { return dbus.Connect(addr, o...) }
	for _, s := range []struct{ label, name, path, iface string }{
		{"RTKit", rtkitName, rtkitPath, rtkitName},
		{"portal", portalName, portalPath, portalRT},
	} {
		t.Run(s.label, func(t *testing.T) {
			f := &fakeRTKit{}
			props := fakeProps{s.iface, map[string]any{"MaxRealtimePriority": int32(20), "MinNiceLevel": int32(-15), "RTTimeUSecMax": int64(200000)}}
			if err := server.Export(f, dbus.ObjectPath(s.path), s.iface); err != nil {
				t.Fatal(err)
			}
			if err := server.Export(props, dbus.ObjectPath(s.path), "org.freedesktop.DBus.Properties"); err != nil {
				t.Fatal(err)
			}
			if _, err := server.RequestName(s.name, dbus.NameFlagDoNotQueue); err != nil {
				t.Fatal(err)
			}
			b, err := dialBus(connect, s.label, s.name, s.path, s.iface)
			if err != nil {
				t.Fatal(err)
			}
			defer b.close()
			if lim := b.limits(); lim != rtkitDefaults || b.name() != s.label {
				t.Fatalf("%s limits %+v", b.name(), lim)
			}
			if err := b.makeRealtime(7, 10, 20); err != nil {
				t.Fatal(err)
			}
			if err := b.makeHighPriority(7, 11, -15); err != nil {
				t.Fatal(err)
			}
			if err := b.makeRealtime(7, 0, 20); err == nil || !strings.Contains(err.Error(), "Operation not permitted") {
				t.Fatalf("refused call: %v", err)
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if want := []string{"realtime 7/10@20", "nice 7/11@-15"}; !slices.Equal(f.calls, want) {
				t.Fatalf("calls %q, want %q", f.calls, want)
			}
		})
	}
	// Nobody serves the name: the dial fails and says who was missing.
	if _, err := dialBus(connect, "RTKit", "org.example.NoRTKit", rtkitPath, rtkitName); err == nil || !strings.HasPrefix(err.Error(), "RTKit: ") {
		t.Fatalf("dial without a broker: %v", err)
	}
}

// threadsChild marks its own thread SCHED_RESET_ON_FORK, as libpipewire
// marks the threads it creates, reports what audioThreadIDs finds plus
// PID 1's thread, which is not its own, and waits to be closed.
func threadsChild() {
	runtime.LockOSThread()
	param := struct{ priority int32 }{}
	const schedOther = 0 // SCHED_OTHER, which x/sys/unix does not name
	if _, _, errno := unix.RawSyscall(unix.SYS_SCHED_SETSCHEDULER, 0, schedOther|unix.SCHED_RESET_ON_FORK, uintptr(unsafe.Pointer(&param))); errno != 0 {
		fmt.Fprintln(os.Stderr, "sched_setscheduler:", errno)
		os.Exit(1)
	}
	tids := audioThreadIDs()
	if !slices.Contains(tids, unix.Gettid()) {
		fmt.Fprintln(os.Stderr, "audioThreadIDs missed the marked thread:", tids)
		os.Exit(1)
	}
	_ = gob.NewEncoder(os.Stdout).Encode(frame{Threads: &threadReport{TIDs: append(tids, 1)}})
	_, _ = os.Stdin.Read(make([]byte, 1))
	os.Exit(0)
}

// The child's thread report reaches the parent, which asks the broker for
// the reporting child's own threads only, after capping its
// RLIMIT_RTTIME, and shows the outcome in Info.
func TestThreadReportRaisesChildThreads(t *testing.T) {
	t.Setenv(envTestChild, "threads")
	b := &fakeBroker{lim: rtkitDefaults}
	f := &fakeRTEnv{b: b}
	env := f.env()
	env.owns = ownsThread
	env.limitRTTime = limitRTTime
	h, err := spawn(Options{realtime: &env})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	pid := h.cmd.Process.Pid
	if rt := h.Info().Realtime; rt.State != "" {
		t.Fatalf("Realtime before any report: %+v", rt)
	}
	rt := waitRealtime(t, h, func(rt Realtime) bool { return rt.State != "" })
	want := Realtime{State: playback.RealtimeGranted, Via: "RTKit", Priority: 20, Threads: 1}
	if rt != want {
		t.Fatalf("Realtime = %+v, want %+v", rt, want)
	}
	asked, _, _ := b.calls()
	if len(asked) != 1 || !strings.HasPrefix(asked[0], fmt.Sprintf("%d/", pid)) || strings.HasSuffix(asked[0], "/1@20") {
		t.Fatalf("asked for %q", asked)
	}
	var lim unix.Rlimit
	if err := unix.Prlimit(pid, unix.RLIMIT_RTTIME, nil, &lim); err != nil {
		t.Fatal(err)
	}
	if lim.Cur != 200000 || lim.Max != 200000 {
		t.Fatalf("child's RLIMIT_RTTIME = %+v", lim)
	}
	select {
	case <-h.Done():
		t.Fatalf("the report ended the child: %v", h.Err())
	case <-time.After(100 * time.Millisecond):
	}
}

// limitRTTime only ever lowers a limit.
func TestLimitRTTimeKeepsLowerLimit(t *testing.T) {
	t.Setenv(envTestChild, "threads")
	h, err := spawn(Options{realtime: &rtEnv{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	pid := h.cmd.Process.Pid
	if err := unix.Prlimit(pid, unix.RLIMIT_RTTIME, &unix.Rlimit{Cur: 1000, Max: 300000}, nil); err != nil {
		t.Fatal(err)
	}
	if err := limitRTTime(pid, 200000); err != nil {
		t.Fatal(err)
	}
	var lim unix.Rlimit
	if err := unix.Prlimit(pid, unix.RLIMIT_RTTIME, nil, &lim); err != nil {
		t.Fatal(err)
	}
	if lim.Cur != 1000 || lim.Max != 200000 {
		t.Fatalf("RLIMIT_RTTIME = %+v", lim)
	}
}

func TestOwnsThread(t *testing.T) {
	if !ownsThread(os.Getpid(), unix.Gettid()) {
		t.Error("the test's own thread")
	}
	if ownsThread(os.Getpid(), 1) {
		t.Error("PID 1's thread")
	}
}
