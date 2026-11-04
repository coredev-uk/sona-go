package player

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/egoist/mygo/ui"
)

// sweepFPSSoftware is how often a sung word's fill is painted again where the
// window is drawn on the CPU. Each frame presents the whole window, and on a
// CPU rasteriser (llvmpipe in a VM or a headless session, MYGO_GPU=0) that is
// what a sweep costs, in proportion to the frames a second: 60 took 50% of a
// core, 30 took 29%. A fill that crosses a word in a few hundred milliseconds
// looks the same at 30.
const sweepFPSSoftware = 30

// softwareDraw reports whether MyGo draws this process's windows on the CPU.
// MyGo knows (it logs "native UI draws without the GPU") but has no public
// query for it, so this follows its own probe from outside: MYGO_GPU=0 forces
// the CPU renderer; Mesa's software drivers are asked for by LIBGL_ALWAYS_SOFTWARE
// or GALLIUM_DRIVER; and Linux without a GPU render node (a VM, a container, a
// headless session) has only llvmpipe. macOS (Metal) and Windows (Direct3D)
// always have a GPU renderer.
var softwareDraw = sync.OnceValue(func() bool {
	return softwareDrawIn(runtime.GOOS, os.Getenv, func(pattern string) []string {
		m, _ := filepath.Glob(pattern)
		return m
	})
})

func softwareDrawIn(goos string, getenv func(string) string, glob func(string) []string) bool {
	if goos != "linux" {
		return false
	}
	switch {
	case getenv("MYGO_GPU") == "0", getenv("LIBGL_ALWAYS_SOFTWARE") != "" && getenv("LIBGL_ALWAYS_SOFTWARE") != "0":
		return true
	}
	switch getenv("GALLIUM_DRIVER") {
	case "llvmpipe", "softpipe", "swrast":
		return true
	}
	return len(glob("/dev/dri/renderD*")) == 0
}

// sweepFrames asks for the next frame of a moving sweep: at the display's rate
// where a GPU draws it, at sweepFPSSoftware where the CPU does.
func sweepFrames(pt *ui.Painter) {
	if softwareDraw() {
		pt.AnimationFrameRate(sweepFPSSoftware)
		return
	}
	pt.AnimationFrame()
}
