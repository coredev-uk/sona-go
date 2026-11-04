package kit

import "github.com/egoist/mygo/ui"

// sliderPending holds the sliders whose value changed and whose pointer is
// still held: by element ID, main thread only.
var sliderPending = map[uint64]bool{}

// SliderCommitted reports, once, that the user finished changing the slider
// sl: the frame after the pointer let go of it, wherever the pointer is (a
// drag that drifts off the thin slider is still held: MyGo's Pressed says
// otherwise, Dragged's held flag does not), or the frame of a key's change
// (no pointer holds it then). A seek bar commits with it, so a drag seeks
// once instead of on every frame the pointer is outside. Call it every frame
// for the slider, after its constructor.
func SliderCommitted(sl ui.Element) bool {
	id := sl.ID()
	_, _, held := sl.Dragged()
	if sl.Changed() {
		sliderPending[id] = true
	}
	if sliderPending[id] && !held {
		delete(sliderPending, id)
		return true
	}
	return false
}
