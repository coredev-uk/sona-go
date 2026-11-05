//go:build probe && !windows

// Package memprobe is the memory hook of probe builds (never a release
// build): with SONA_MEMPROFILE_DIR set, SIGUSR1 makes the process write the
// Go heap's profile and the runtime's own figures, so cmd/memprobe can read
// them next to the operating system's numbers. The playback host child is
// this same binary and gets the signal too; its sandbox keeps it from
// writing files, so it logs the figures instead and the parent relays the
// line into sona.log.
//
// Import it for its side effect (cmd/sona/memprobe_probe.go,
// cmd/playspike/memprobe_probe.go).
package memprobe

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"

	"github.com/coredev-uk/sona-go/internal/playback/cdmhost"
)

// EnvDir names the directory the profiles go into: <pid>.heap.pb.gz (the Go
// heap after a collection; `go tool pprof -sample_index=inuse_space`) and
// <pid>.memstats.json (runtime.MemStats).
const EnvDir = "SONA_MEMPROFILE_DIR"

func init() {
	dir := os.Getenv(EnvDir)
	if dir == "" {
		return
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGUSR1)
	child := os.Getenv(cdmhost.EnvChild) == "1"
	go func() {
		for range ch {
			if child {
				logChild()
			} else {
				write(dir)
			}
		}
	}()
}

func write(dir string) {
	pid := os.Getpid()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	if b, err := json.MarshalIndent(ms, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, fmt.Sprintf("%d.memstats.json", pid)), b, 0o600)
	}
	f, err := os.Create(filepath.Join(dir, fmt.Sprintf("%d.heap.pb.gz", pid)))
	if err != nil {
		return
	}
	defer f.Close()
	_ = pprof.Lookup("heap").WriteTo(f, 0)
}

// logChild writes the host's Go figures as one JSON log record on stderr.
func logChild() {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	rec := map[string]any{
		"time": time.Now().Format(time.RFC3339Nano), "level": "INFO", "msg": "memprobe: host memstats", "pid": os.Getpid(),
		"HeapAlloc": ms.HeapAlloc, "HeapInuse": ms.HeapInuse, "HeapIdle": ms.HeapIdle, "HeapReleased": ms.HeapReleased,
		"HeapSys": ms.HeapSys, "StackInuse": ms.StackInuse, "Sys": ms.Sys, "NumGC": ms.NumGC,
	}
	b, _ := json.Marshal(rec)
	fmt.Fprintln(os.Stderr, string(b))
}
