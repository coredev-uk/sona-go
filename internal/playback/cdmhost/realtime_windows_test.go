//go:build cgo

package cdmhost

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/gst"
)

func TestMMCSSOutcome(t *testing.T) {
	for _, tc := range []struct {
		name  string
		ts    []threadPriority
		probe string
		want  Realtime
		ok    bool
	}{{
		name: "no thread of the child",
	}, {
		name: "a render thread in MMCSS's range",
		ts:   []threadPriority{{tid: 10, priority: 24}},
		want: Realtime{State: playback.RealtimeGranted, Via: "MMCSS", Priority: 24, Threads: 1},
		ok:   true,
	}, {
		name: "the highest of the threads that got it",
		ts:   []threadPriority{{tid: 10, priority: 23}, {tid: 11, priority: 8}, {tid: 12, priority: 26}},
		want: Realtime{State: playback.RealtimeGranted, Via: "MMCSS", Priority: 26, Threads: 2},
		ok:   true,
	}, {
		name: "the lowest priority MMCSS gives",
		ts:   []threadPriority{{tid: 10, priority: 16}},
		want: Realtime{State: playback.RealtimeGranted, Via: "MMCSS", Priority: 16, Threads: 1},
		ok:   true,
	}, {
		name:  "THREAD_PRIORITY_TIME_CRITICAL without MMCSS, refused",
		ts:    []threadPriority{{tid: 10, priority: 15}},
		probe: `AvSetMmThreadCharacteristics("Pro Audio"): Access is denied. (error 5)`,
		want: Realtime{
			State: playback.RealtimeDenied, Via: "MMCSS", Refusal: playback.RealtimeRefused,
			Reason: `MMCSS: AvSetMmThreadCharacteristics("Pro Audio"): Access is denied. (error 5)`,
		},
		ok: true,
	}, {
		name: "not registered, though the child could register",
		ts:   []threadPriority{{tid: 10, priority: 8}},
		want: Realtime{
			State: playback.RealtimeDenied, Via: "MMCSS",
			Reason: "the audio thread runs at priority 8, below MMCSS's 16, though the host could register a thread of its own",
		},
		ok: true,
	}} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := mmcssOutcome(tc.ts, tc.probe)
			if got != tc.want || ok != tc.ok {
				t.Errorf("mmcssOutcome = %+v, %v; want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

// The threads a report names are read only where they are the process's
// own.
func TestObserveMMCSSSkipsForeignThreads(t *testing.T) {
	if rt, ok := observeMMCSS(os.Getpid(), threadReport{TIDs: []int{-1, 0}}); ok {
		t.Fatalf("observe = %+v, true", rt)
	}
	ts, err := processThreads(os.Getpid())
	if err != nil || len(ts) == 0 {
		t.Fatalf("processThreads = %v, %v", ts, err)
	}
	// A thread of this process at normal priority.
	rt, ok := observeMMCSS(os.Getpid(), threadReport{TIDs: []int{ts[0].tid, -1}})
	if !ok || rt.Via != "MMCSS" {
		t.Fatalf("observe = %+v, %v", rt, ok)
	}
}

// This process can register a thread with MMCSS's "Pro Audio" task and
// revert it.
func TestRegisterMMCSS(t *testing.T) {
	if err := registerMMCSS(); err != nil {
		t.Fatal(err)
	}
}

// threadName reads a name the thread gave itself.
func TestThreadName(t *testing.T) {
	done := make(chan string)
	go func() {
		// The goroutine's thread keeps the name: it ends with it.
		runtime.LockOSThread()
		name, _ := windows.UTF16PtrFromString("sona-test-thread")
		set := modkernel32.NewProc("SetThreadDescription")
		if hr, _, _ := set.Call(uintptr(windows.CurrentThread()), uintptr(unsafe.Pointer(name))); int32(hr) < 0 {
			done <- "SetThreadDescription failed"
			return
		}
		done <- threadName(int(windows.GetCurrentThreadId()))
	}()
	if got := <-done; got != "sona-test-thread" {
		t.Fatalf("threadName = %q", got)
	}
}

// A sandboxed host playing through wasapi2sink, muted, reports the
// priority its render thread got from MMCSS. It needs a render endpoint.
func TestSandboxedHostReportsMMCSS(t *testing.T) {
	if !gst.Have("wasapi2sink") || !gst.Have("decodebin") {
		t.Skip("wasapi2sink or decodebin not installed")
	}
	if devs, err := gst.AudioSinks(); err != nil || len(devs) == 0 {
		t.Skipf("no audio output: %v", err)
	}
	data, err := os.ReadFile(filepath.Join("..", "gst", "testdata", "sine2s.m4a"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h, err := Start(ctx, Options{Clear: true, Sink: "wasapi2sink"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	if sb := h.Info().Sandbox; os.Getenv(envAllowUnsandboxed) != "1" && !sb.Active() {
		t.Errorf("the host is not sandboxed: %+v", sb)
	}
	for _, err := range []error{
		h.SetVolume(ctx, 0),
		h.LoadFile(ctx, 1, int64(len(data)), 0, 0),
		h.FeedFile(ctx, 1, 0, data),
		h.EndOfFile(ctx, 1),
		h.Play(ctx),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	waitHostEvent(t, h, EventStarted, 1, 10*time.Second)
	rt := waitRealtime(t, h, func(rt Realtime) bool { return rt.State != "" })
	t.Logf("realtime %+v (sandbox %+v)", rt, h.Info().Sandbox)
	if rt.State != playback.RealtimeGranted || rt.Via != "MMCSS" || rt.Priority < mmcssMinPriority || rt.Threads < 1 {
		t.Errorf("realtime %+v, want granted by MMCSS", rt)
	}
}
