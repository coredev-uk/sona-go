package kit

import "github.com/egoist/mygo/ui"

type clickOnceKey struct{}

// ClickRun reports a click of el, and whether it is the second click of a
// double-click. A double-click is two clicks, and the second one lands where
// the first one's action left the item (the page it opened, the station it
// started), so it is not a second choice: an item whose click is an action
// (opening a page, playing or toggling a station, the browser) runs it only
// when again is false. A click that selects still counts, as a Ctrl+click
// right after another is a choice of its own.
func ClickRun(el ui.Element) (clicked, again bool) {
	second := ui.Local(el, clickOnceKey{}, func() bool { return false })
	if el.DoubleClicked() {
		*second = true
	}
	if !el.Clicked() {
		return false, false
	}
	again, *second = *second, false
	return true, again
}

// ClickOnce reports a click of el that is not the second of a double-click:
// ClickRun for an item that only acts.
func ClickOnce(el ui.Element) bool {
	clicked, again := ClickRun(el)
	return clicked && !again
}
