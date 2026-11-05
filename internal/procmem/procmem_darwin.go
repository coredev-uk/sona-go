package procmem

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	loadOnce  sync.Once
	loadErr   error
	pidRusage func(pid, flavor int32, buf unsafe.Pointer) int32
)

// rusageInfoV0 is proc_pid_rusage's flavor RUSAGE_INFO_V0.
const rusageInfoV0 = 0

func load() {
	// libproc's functions live in libSystem, which every process has loaded;
	// purego needs no cgo to reach them.
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		loadErr = err
		return
	}
	defer func() {
		if r := recover(); r != nil {
			loadErr = fmt.Errorf("procmem: proc_pid_rusage: %v", r)
		}
	}()
	purego.RegisterLibFunc(&pidRusage, lib, "proc_pid_rusage")
}

// Of asks proc_pid_rusage: the resident size and the physical footprint,
// which is what Activity Monitor's Memory column shows.
func Of(pid int) (Usage, error) {
	loadOnce.Do(load)
	if loadErr != nil {
		return Usage{}, loadErr
	}
	buf := make([]byte, 512) // room for any later flavor's layout
	if r := pidRusage(int32(pid), rusageInfoV0, unsafe.Pointer(&buf[0])); r != 0 {
		return Usage{}, fmt.Errorf("procmem: proc_pid_rusage(%d) failed: %d", pid, r)
	}
	u, ok := decodeRusage(buf)
	if !ok {
		return Usage{}, fmt.Errorf("procmem: short rusage buffer")
	}
	return u, nil
}
