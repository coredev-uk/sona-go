package player

import "github.com/egoist/mygo/ui"

// sweepFPSSoftware is how often a sung word's fill is painted again where the
// window is drawn on the CPU. Each frame presents the whole window, and on a
// CPU rasteriser (llvmpipe in a VM or a headless session, MYGO_GPU=0) that is
// what a sweep costs, in proportion to the frames a second: 60 took 50% of a
// core, 30 took 29%. A fill that crosses a word in a few hundred milliseconds
// looks the same at 30.
const sweepFPSSoftware = 30

// sweepFrames asks for the next frame of a moving sweep: at the display's rate
// where a GPU draws it, at sweepFPSSoftware where the CPU does (MyGo's own
// answer, Painter.Accelerated: false when drawing in memory or on a software
// renderer, and it repaints if the answer changes).
func sweepFrames(pt *ui.Painter) {
	if !pt.Accelerated() {
		pt.AnimationFrameRate(sweepFPSSoftware)
		return
	}
	pt.AnimationFrame()
}
