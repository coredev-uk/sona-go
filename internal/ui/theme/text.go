package theme

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Text is a Text in one of the type styles of the current theme ("body",
// "meta", …): its size, line height, weight and tracking.
func Text(c *ui.Context, s, style string) ui.Element {
	return StyleText(ui.Text(c, s), style)
}

// StyleText gives text, an element built by ui.Text, a type style's size,
// line height, weight and tracking. Text is it for the common case.
func StyleText(text ui.Element, style string) ui.Element {
	t := Current().Type[style]
	return text.FontSize(t.Size).FixedLineHeight(t.LineHeight).FontWeight(int(t.Weight)).LetterSpacing(t.Tracking)
}

// Fade makes e's colours (background, border, text) change over the duration
// of the motion token ("feedback", "press"), unless motion is reduced.
func Fade(e ui.Element, token string) {
	if ReducedMotion() {
		return
	}
	m := Current().Motion[token]
	e.Transition(ui.ElementTransition{Colors: true, Duration: m.Duration, Ease: m.Ease})
}

// Spin turns icon once a second while on, unless motion is reduced. The loop
// is keyed on the element, so one icon spins once.
func Spin(icon ui.Element, on bool) {
	if on && !ReducedMotion() {
		icon.Rotate(icon.Loop("spin", time.Second, ui.Linear) * 360)
	}
}

// Hairline is a 1px rule across its parent in the line colour.
func Hairline(c *ui.Context) ui.Element {
	return ui.Box(c).Height(1).FillWidth().Background(C("line"))
}
