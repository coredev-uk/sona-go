package kit

import "github.com/egoist/mygo/ui"

// framesSoftware is how often a drawing that moves is painted again where the
// window is drawn on the CPU. Each frame presents the whole window, and on a
// CPU rasteriser (llvmpipe in a VM or a headless session, MYGO_GPU=0) that is
// what a moving drawing costs, in proportion to the frames a second: 60 took
// 50% of a core, 30 took 29%. A fill that crosses a word in a few hundred
// milliseconds, or a bar that follows the clock, looks the same at 30.
const framesSoftware = 30

// AnimationFrames asks for the next frame of a drawing that moves with the
// clock: at the display's rate where a GPU draws it, at framesSoftware where
// the CPU does (MyGo's own answer, Painter.Accelerated: false when drawing in
// memory or on a software renderer, and it repaints if the answer changes).
// Call it in the Draw callback, in every frame the drawing moves.
func AnimationFrames(pt *ui.Painter) {
	if !pt.Accelerated() {
		pt.AnimationFrameRate(framesSoftware)
		return
	}
	pt.AnimationFrame()
}
