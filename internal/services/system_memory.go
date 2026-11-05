package services

import (
	"runtime"

	"github.com/coredev-uk/sona-go/internal/procmem"
)

// MemoryReport is the bundle's "Memory" section: what the operating system
// counts for the app's own process and for the playback host child (two
// separate rows in Activity Monitor), and the Go runtime's own account of the
// app's process, so a report of high memory says which process holds it and
// whether it is the Go heap (which Sona can shrink) or native memory (the
// GPU's buffers, GStreamer, the Widevine CDM). Sizes are MiB.
type MemoryReport struct {
	// App is the app's process.
	App MemoryProcess `json:"app"`
	// Host is the playback host child; nil while none runs.
	Host *MemoryProcess `json:"host,omitempty"`
	// Go is the app process's Go runtime.
	Go MemoryGo `json:"go"`
}

// MemoryProcess is a process as the operating system counts it
// (procmem.Usage); Error says why a figure is missing.
type MemoryProcess struct {
	PID int `json:"pid"`
	// ResidentMiB is the pages in RAM, shared library pages included.
	ResidentMiB int `json:"residentMiB"`
	// FootprintMiB is what the platform's tools show: macOS's physical
	// footprint (Activity Monitor), Linux's proportional set size.
	FootprintMiB int    `json:"footprintMiB"`
	Error        string `json:"error,omitempty"`
}

// MemoryGo is runtime.MemStats of the app's process, in MiB: HeapInuseMiB
// is the live heap and what the collector has not swept yet, HeapIdleMiB the
// free spans the heap still holds, of which HeapReleasedMiB were given back
// to the operating system; SysMiB is everything the runtime asked for.
type MemoryGo struct {
	HeapAllocMiB    int `json:"heapAllocMiB"`
	HeapInuseMiB    int `json:"heapInuseMiB"`
	HeapIdleMiB     int `json:"heapIdleMiB"`
	HeapReleasedMiB int `json:"heapReleasedMiB"`
	StackMiB        int `json:"stackMiB"`
	SysMiB          int `json:"sysMiB"`
	NumGC           int `json:"numGC"`
}

func toMiB(b uint64) int { return int((b + 1<<19) >> 20) }

func memoryProcess(pid int) MemoryProcess {
	p := MemoryProcess{PID: pid}
	u, err := procmem.Of(pid)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	p.ResidentMiB, p.FootprintMiB = toMiB(u.Resident), toMiB(u.Footprint)
	return p
}

// memoryReport reads the figures of the app (os.Getpid) and, with hostPID
// non-zero, the playback host.
func memoryReport(appPID, hostPID int) MemoryReport {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	r := MemoryReport{
		App: memoryProcess(appPID),
		Go: MemoryGo{
			HeapAllocMiB: toMiB(ms.HeapAlloc), HeapInuseMiB: toMiB(ms.HeapInuse), HeapIdleMiB: toMiB(ms.HeapIdle),
			HeapReleasedMiB: toMiB(ms.HeapReleased), StackMiB: toMiB(ms.StackInuse), SysMiB: toMiB(ms.Sys), NumGC: int(ms.NumGC),
		},
	}
	if hostPID != 0 {
		h := memoryProcess(hostPID)
		r.Host = &h
	}
	return r
}
