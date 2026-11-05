package services

import (
	"os"
	"runtime"
	"testing"
)

func TestMemoryReportCountsTheAppAndLeavesOutAHostThatIsNotRunning(t *testing.T) {
	r := memoryReport(os.Getpid(), 0)
	if r.Host != nil {
		t.Errorf("a host report for pid 0: %+v", r.Host)
	}
	if r.Go.HeapInuseMiB <= 0 || r.Go.SysMiB < r.Go.HeapInuseMiB {
		t.Errorf("Go figures %+v: a running program has a heap, and Sys includes it", r.Go)
	}
	if runtime.GOOS == "linux" && (r.App.Error != "" || r.App.FootprintMiB <= 0) {
		t.Errorf("app %+v: Linux reads smaps_rollup", r.App)
	}
	// A pid that does not exist says why instead of reporting zeros.
	gone := memoryReport(os.Getpid(), 1<<22)
	if gone.Host == nil || gone.Host.Error == "" {
		t.Errorf("host %+v: a missing process should carry an error", gone.Host)
	}
}
