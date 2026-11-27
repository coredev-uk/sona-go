package kit

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// Markdown shows finished Markdown text (a release's notes, GitHub's flavour:
// headings, lists, links, bold and italic, inline code, and the blocks a
// changelog may carry besides) in Sona's type and colours. It is MyGo's
// ui/markdown view with the house look; the view parses the text once, keeps
// the shaped paragraphs while the text, the theme and the width stay, and
// builds nothing per frame that did not change.
//
// Keep one per place that shows Markdown, in its state, and give it the text
// on each build. It must not be copied.
//
//	md := kit.NewMarkdown(openURL)
//	md.View(c, notes)
type Markdown struct {
	opts markdown.Options
	// tokens are the theme tokens the look was made from (nil before the
	// first View): another set is a theme or a mode chosen.
	tokens *theme.Tokens
}

// NewMarkdown is a Markdown whose links open through open: a link to an
// http, https or mailto address calls it with the address, and a link of any
// other scheme (a file, a program, javascript:), a path or a "#heading" does
// nothing, as the text may come from anyone who can write a release. open is
// the app's one way to open a web page (module.Env.OpenURL); nil makes
// links inert.
func NewMarkdown(open func(url string)) *Markdown {
	m := &Markdown{}
	m.opts.OnLink = func(url string) {
		if open != nil && ui.SafeToOpen(url) {
			open(url)
		}
	}
	return m
}

// View builds src and returns the element, a column as wide as its parent
// lets it be; wrap it in a scroll area when src is long. Its text is
// selectable and copies as text, HTML and Markdown. Links are in --ink, set
// apart from the text by weight rather than by colour (amber means "playing"
// and nothing else); a link takes the keyboard focus, shows its address in a
// tooltip and opens with Enter.
func (m *Markdown) View(c *ui.Context, src string) ui.Element {
	if tk := theme.Current(); m.tokens != tk {
		m.opts.Theme, m.tokens = markdownTheme(tk), tk
	}
	return markdown.View(c, src, &m.opts)
}

// markdownTheme is what the house look sets of MyGo's Markdown theme; the
// rest (the window theme's font, size and colours, code in the system's
// monospaced font, tinted code spans) is MyGo's default for the window's
// theme, which Sona's tokens already fill (theme.Apply).
//
// Headings are the type scale's, not a document's: a dialog's text is read
// at body size, so "heading" (16/600) marks the top two levels and the rest
// are body size at 600, with room above them that groups what follows and no
// rule under them. Links are ink at 600.
func markdownTheme(tk *theme.Tokens) *markdown.Theme {
	body, head := tk.Type["body"], tk.Type["heading"]
	th := &markdown.Theme{
		LineHeight: body.LineHeight / body.Size,
		Link:       tk.Color["ink"],
		LinkWeight: 600,
	}
	for i := range th.Headings {
		size := float32(1)
		if i < 2 {
			size = head.Size / body.Size
		}
		th.Headings[i] = markdown.HeadingStyle{Size: size, Weight: int(head.Weight), Top: 1.25, Bottom: 0.4}
	}
	return th
}
