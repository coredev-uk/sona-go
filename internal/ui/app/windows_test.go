package app

import (
	"fmt"
	"slices"
	"testing"
)

// OnFocusChange observers hear the main window's focus and blur, in the
// order they registered.
func TestWindowsFocusObservers(t *testing.T) {
	w := NewWindows(false)
	var got []string
	w.OnFocusChange(func(on bool) { got = append(got, fmt.Sprint("a", on)) })
	w.OnFocusChange(func(on bool) { got = append(got, fmt.Sprint("b", on)) })
	w.focusChanged(true)
	w.focusChanged(false)
	if want := []string{"atrue", "btrue", "afalse", "bfalse"}; !slices.Equal(got, want) {
		t.Fatalf("observers heard %v, want %v", got, want)
	}
}
