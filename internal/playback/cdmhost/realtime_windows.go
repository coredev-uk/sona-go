//go:build cgo

package cdmhost

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/coredev-uk/sona-go/internal/playback"
)

// RealtimeSupported: on Windows the output's render thread registers with
// the Multimedia Class Scheduler (MMCSS) itself, and the parent reads the
// priority it got.
const RealtimeSupported = true

// realtimeInitial is a new host's Realtime: nothing read yet.
var realtimeInitial = Realtime{}

// defaultRTEnv reads the priority of the threads the child reports.
var defaultRTEnv = rtEnv{observe: observeMMCSS}

const (
	// renderThread is the name of wasapi2sink's render loop thread
	// (gst_wasapi2_rbuf_loop_thread), which registers itself with MMCSS's
	// "Pro Audio" task before it opens the device, and so before the
	// output reaches PLAYING. GLib names its threads on Windows.
	renderThread = "GstWasapi2Rbuf"
	// mmcssTask is the MMCSS task wasapi2sink registers with.
	mmcssTask = "Pro Audio"
	// mmcssMinPriority is the lowest base priority of a thread MMCSS runs
	// in the realtime range. A thread of a process in the normal priority
	// class, as the child is, tops out at 15 (THREAD_PRIORITY_TIME_CRITICAL)
	// without it; "Pro Audio" (scheduling category High) gets 23 to 26.
	mmcssMinPriority = 16
)

var (
	modkernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procGetThreadDescription = modkernel32.NewProc("GetThreadDescription")
	modavrt                  = windows.NewLazySystemDLL("avrt.dll")
	procAvSetMmThreadChars   = modavrt.NewProc("AvSetMmThreadCharacteristicsW")
	procAvRevertMmThreadChar = modavrt.NewProc("AvRevertMmThreadCharacteristics")
)

// threadPriority is a thread's base priority, as Windows reports it.
type threadPriority struct {
	tid, priority int
}

// processThreads lists process pid's threads with their base priority.
func processThreads(pid int) ([]threadPriority, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var ts []threadPriority
	e := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snap, &e); err == nil; err = windows.Thread32Next(snap, &e) {
		if int(e.OwnerProcessID) == pid {
			ts = append(ts, threadPriority{tid: int(e.ThreadID), priority: int(e.BasePri)})
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("list threads: %w", err)
	}
	return ts, nil
}

// threadName is thread tid's description (SetThreadDescription), "" when
// it has none or cannot be read.
func threadName(tid int) string {
	if procGetThreadDescription.Find() != nil {
		return ""
	}
	h, err := windows.OpenThread(windows.THREAD_QUERY_LIMITED_INFORMATION, false, uint32(tid))
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var p *uint16
	if hr, _, _ := procGetThreadDescription.Call(uintptr(h), uintptr(unsafe.Pointer(&p))); int32(hr) < 0 || p == nil {
		return ""
	}
	defer func() { _, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(p))) }()
	return windows.UTF16PtrToString(p)
}

// audioThreadIDs lists the child's audio threads: wasapi2sink's render
// threads, by name. Another sink's threads are not looked for.
func audioThreadIDs() []int {
	ts, err := processThreads(int(windows.GetCurrentProcessId()))
	if err != nil {
		return nil
	}
	var tids []int
	for _, t := range ts {
		if threadName(t.tid) == renderThread {
			tids = append(tids, t.tid)
		}
	}
	return tids
}

// schedProbe is why the child could not register a thread of its own with
// MMCSS's "Pro Audio" task, "" when it could: the reason a render thread
// was left out, as wasapi2sink ignores its own call's failure. Tried once
// per child.
func schedProbe() string { return mmcssProbe() }

var mmcssProbe = sync.OnceValue(func() string {
	if err := registerMMCSS(); err != nil {
		return err.Error()
	}
	return ""
})

// registerMMCSS registers a thread with MMCSS's "Pro Audio" task, as
// wasapi2sink's render thread does, and reverts it. The thread is the
// goroutine's own: one whose registration could not be reverted is not
// handed back to the Go scheduler but ends with the goroutine.
func registerMMCSS() error {
	if err := procAvSetMmThreadChars.Find(); err != nil {
		return err
	}
	task, err := windows.UTF16PtrFromString(mmcssTask)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		var index uint32
		h, _, err := procAvSetMmThreadChars.Call(uintptr(unsafe.Pointer(task)), uintptr(unsafe.Pointer(&index)))
		if h == 0 {
			runtime.UnlockOSThread()
			done <- fmt.Errorf("AvSetMmThreadCharacteristics(%q): %w", mmcssTask, win32Error(err))
			return
		}
		if ok, _, err := procAvRevertMmThreadChar.Call(h); ok == 0 {
			done <- fmt.Errorf("AvRevertMmThreadCharacteristics: %w", win32Error(err))
			return
		}
		runtime.UnlockOSThread()
		done <- nil
	}()
	return <-done
}

// win32Error names a Win32 error by its message and code.
func win32Error(err error) error {
	var errno windows.Errno
	if errors.As(err, &errno) {
		return fmt.Errorf("%w (error %d)", errno, uint32(errno))
	}
	return err
}

// observeMMCSS reads the base priority of the threads r reports that are
// process pid's own.
func observeMMCSS(pid int, r threadReport) (Realtime, bool) {
	ts, err := processThreads(pid)
	if err != nil {
		return Realtime{}, false
	}
	ts = slices.DeleteFunc(ts, func(t threadPriority) bool { return !slices.Contains(r.TIDs, t.tid) })
	return mmcssOutcome(ts, r.Probe)
}

// mmcssOutcome is the scheduling threads ts got: granted, at the highest
// base priority among them, when any runs in MMCSS's realtime range; else
// denied, as refused when the child's own registration (probe) failed.
// ok is false when ts is empty.
func mmcssOutcome(ts []threadPriority, probe string) (rt Realtime, ok bool) {
	if len(ts) == 0 {
		return Realtime{}, false
	}
	rt.Via = "MMCSS"
	highest := 0
	for _, t := range ts {
		highest = max(highest, t.priority)
		if t.priority >= mmcssMinPriority {
			rt.Threads++
		}
	}
	switch {
	case rt.Threads > 0:
		rt.State, rt.Priority = playback.RealtimeGranted, highest
	case probe != "":
		rt.State, rt.Refusal, rt.Reason = playback.RealtimeDenied, playback.RealtimeRefused, "MMCSS: "+probe
	default:
		rt.State = playback.RealtimeDenied
		rt.Reason = fmt.Sprintf("the audio thread runs at priority %d, below MMCSS's %d, though the host could register a thread of its own", highest, mmcssMinPriority)
	}
	return rt, true
}
