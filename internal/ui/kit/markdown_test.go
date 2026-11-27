package kit

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

type markdownApp struct {
	md     *Markdown
	src    string
	opened []string
}

func newMarkdownApp(t *testing.T, src string) (*markdownApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	a := &markdownApp{src: src}
	a.md = NewMarkdown(func(url string) { a.opened = append(a.opened, url) })
	return a, ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Padding(20).Width(480).Children(func() { a.md.View(c, a.src) })
	}, 560, 400)
}

// What GitHub's release notes use: headings, bullet lists, bold, italic,
// inline code and links.
func TestMarkdownRendersTheSubsetReleaseNotesUse(t *testing.T) {
	_, tt := newMarkdownApp(t, "## Heading two\n\n### Heading three\n\n* first **bold**\n* second *italic* and `code`\n* a [link](https://example.test/x)\n")
	for _, want := range []string{"Heading two", "Heading three", "first", "bold", "second", "italic", "code", "link"} {
		if !tt.HasText(want) {
			t.Errorf("lacks %q: %q", want, tt.Texts())
		}
	}
	for _, tx := range tt.Texts() {
		if strings.ContainsAny(tx, "#*`[]") {
			t.Errorf("unrendered Markdown syntax in %q", tx)
		}
	}
	if h, ok := tt.AccessByRole(ui.RoleHeading, "Heading two"); !ok || h.Level != 2 {
		t.Errorf("heading two: %+v, %v", h, ok)
	}
	if h, ok := tt.AccessByRole(ui.RoleHeading, "Heading three"); !ok || h.Level != 3 {
		t.Errorf("heading three: %+v, %v", h, ok)
	}
	if _, ok := tt.AccessByRole(ui.RoleLink, "link"); !ok {
		t.Error("the link is not a link")
	}
	items := 0
	for _, n := range tt.AccessAll() {
		if n.Role == ui.RoleListItem {
			items++
		}
	}
	if items != 3 {
		t.Errorf("%d list items, want 3", items)
	}
}

// The look is made from the tokens in use: another mode or theme remakes it,
// and the same ones keep it (the view's shaped text with it).
func TestMarkdownLookFollowsTheTokens(t *testing.T) {
	a, tt := newMarkdownApp(t, "A [link](https://example.test/x).")
	tt.Frame()
	light := a.md.opts.Theme
	if light == nil || light.Link != theme.Current().Color["ink"] {
		t.Fatalf("theme %+v: links are not in --ink", light)
	}
	tt.Frame()
	if a.md.opts.Theme != light {
		t.Error("the look is made again on a frame where nothing changed")
	}
	theme.SetMode(theme.ModeDark)
	tt.SetDark(true)
	tt.Frame()
	if a.md.opts.Theme == light || a.md.opts.Theme.Link != theme.Current().Color["ink"] || a.md.opts.Theme.Link == light.Link {
		t.Error("the look did not follow dark mode")
	}
}

// Text from a release can say anything: only the web's addresses open, and a
// Markdown with no way to open one has inert links.
func TestMarkdownLinksOpenOnlyWebAddresses(t *testing.T) {
	a, tt := newMarkdownApp(t, "[a](https://example.test/a) [b](http://example.test/b) [c](mailto:a@example.test) [d](file:///etc/passwd) [e](javascript:alert(1)) [f](C:\\Windows\\calc.exe) [g](relative/path) [h](#anchor)")
	for _, text := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		r, ok := tt.Find(text)
		if !ok {
			t.Fatalf("no %q on screen: %q", text, tt.Texts())
		}
		tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	}
	if got, want := strings.Join(a.opened, " "), "https://example.test/a http://example.test/b mailto:a@example.test"; got != want {
		t.Errorf("opened %q, want %q", got, want)
	}
	a.md = NewMarkdown(nil)
	a.opened = nil
	tt.Frame()
	r, _ := tt.Find("a")
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	if len(a.opened) != 0 {
		t.Errorf("a Markdown with no opener opened %q", a.opened)
	}
}
