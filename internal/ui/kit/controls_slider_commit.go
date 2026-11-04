package kit

import "github.com/egoist/mygo/ui"

// SliderCommitted reports, once, that the user has let go of a slider whose
// value they were setting: the moment to act on the value (a seek) instead
// of acting on every change of a drag. The caller sets *active when the
// slider reports Changed and keeps the value it reported; SliderCommitted
// then returns true in the first frame in which the slider is no longer
// held, and clears *active. A change made with the keyboard is committed in
// the same frame.
//
// "Held" is the press that began on the slider, wherever the pointer has
// moved since (Element.Dragged), not Element.Pressed, which is false the
// moment the pointer leaves the slider's 16 DIPs while the button is still
// down: a drag that drifted off the track would commit on every movement.
func SliderCommitted(sl ui.Element, active *bool) bool {
	if !*active {
		return false
	}
	if _, _, held := sl.Dragged(); held {
		return false
	}
	*active = false
	return true
}
