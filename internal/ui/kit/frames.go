package kit

import "github.com/egoist/mygo/ui"

// framesSoftware is how often a drawing that moves is painted again where the
// window is drawn on the CPU. Each frame presents the whole window, and on a
// CPU rasteriser (llvmpipe in a VM or a headless session, MYGO_GPU=0) that is
// what a moving drawing costs, in proportion to the frames a second: 60 took
// 50% of a core, 30 took 29%. A fill that crosses a word in a few hundred
// milliseconds, or a bar that follows the clock, looks the same at 30.
const framesSoftware = 30

// framesAmbient is the most a drawing that is slow by design is painted
// again where a GPU draws it. A wave of 2.4 s, a level meter of a second or a
// bar that moves a few DIPs a second gains nothing visible above 60, while a
// 160 Hz display would paint it 160 times a second, each paint a present.
const framesAmbient = 60

// AnimationFrames asks for the next frame of a drawing that moves with the
// clock and that the eye follows closely, as a word's fill or letters
// rising: at the display's rate where a GPU draws it, at framesSoftware where
// the CPU does (MyGo's own answer, Painter.Accelerated: false when drawing in
// memory or on a software renderer, and it repaints if the answer changes).
// Call it in the Draw callback, in every frame the drawing moves. Motion that
// is slow by design asks with AmbientFrames instead.
func AnimationFrames(pt *ui.Painter) {
	if !accelerated(pt) {
		request(pt, framesSoftware)
		return
	}
	request(pt, 0)
}

// AmbientFrames is AnimationFrames for motion that is slow by design (the
// instrumental gap's dots, the equaliser, a spinner, the seek bar's
// playhead): at most framesAmbient frames a second on a GPU, framesSoftware
// on the CPU. MyGo draws the window at the fastest anything in it asks for,
// so an ambient request never slows a drawing that asks for the display's
// rate (AnimationFrames, Painter.AnimationFrame), and one that does ask for
// it does not speed the ambient one up: they are painted together, at the
// display's rate. With only ambient drawings moving, the window is painted
// at 60 or at the nearest rate below the display's that it shows evenly (a
// whole number of refreshes a frame: 80 on a 160 Hz display, 72 on 144 Hz).
func AmbientFrames(pt *ui.Painter) {
	if !accelerated(pt) {
		request(pt, framesSoftware)
		return
	}
	request(pt, framesAmbient)
}

// FrameProbe records what AnimationFrames and AmbientFrames ask for, for
// tests: a Tester draws in memory (Painter.Accelerated is false) at a
// refresh rate it does not set, so it cannot show what a 160 Hz GPU window
// would be asked for.
type FrameProbe struct {
	// Accelerated is the answer given in place of Painter.Accelerated.
	Accelerated bool
	// Asked is each request, in frames a second: 0 for the display's rate.
	Asked []float32
}

var probe *FrameProbe

// ProbeFrames has the frame requests drawn from now on follow accelerated
// instead of the painter's answer and be recorded; stop ends it. Tests that
// use it must not run in parallel.
func ProbeFrames(accelerated bool) (p *FrameProbe, stop func()) {
	probe = &FrameProbe{Accelerated: accelerated}
	return probe, func() { probe = nil }
}

func accelerated(pt *ui.Painter) bool {
	if probe != nil {
		return probe.Accelerated
	}
	return pt.Accelerated()
}

// request asks for the next frame at fps frames a second, at the display's
// rate for 0.
func request(pt *ui.Painter, fps float32) {
	if probe != nil {
		probe.Asked = append(probe.Asked, fps)
	}
	if fps == 0 {
		pt.AnimationFrame()
		return
	}
	pt.AnimationFrameRate(fps)
}
