package kit

import (
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

type paletteApp struct {
	p       Palette
	ran     []string
	popOpen bool
}

func (a *paletteApp) source(q string) []PaletteSection {
	cmds := []PaletteEntry{
		{Key: "c:queue", Label: "Show or hide the queue", Icon: "list-music", Keys: []string{"Ctrl+Q"}, Run: a.run("queue")},
		{Key: "c:next", Label: "Next track", Icon: "fast-forward", Keys: []string{"Ctrl+ArrowRight", "N"}, Run: a.run("next")},
		{Key: "c:albums", Label: "Library · Albums", Icon: "disc-3", Run: a.run("albums")},
	}
	if q == "" {
		return []PaletteSection{{Title: "Playback", Entries: cmds}}
	}
	ranked := RankCommands(q, cmds, func(e PaletteEntry) string { return e.Label }, nil)
	var out []PaletteSection
	if len(ranked) > 0 {
		out = append(out, PaletteSection{Title: "Commands", Entries: ranked})
	}
	return append(out, PaletteSection{Title: "Apple Music", Entries: []PaletteEntry{
		{Key: "search", Label: "Search Apple Music for “" + q + "”", Icon: "search", Run: a.run("search:" + q)},
	}})
}

func (a *paletteApp) run(s string) func() { return func() { a.ran = append(a.ran, s) } }

func (a *paletteApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(20).Gap(12).AlignItems(ui.Start).Children(func() {
		open := Button(c.Key("open"), "Open palette", ButtonOpts{})
		if open.Clicked() {
			a.p.Open()
		}
		pop := Button(c.Key("pop"), "Details", ButtonOpts{})
		if pop.Clicked() {
			a.popOpen = !a.popOpen
		}
		Popover(c, pop, &a.popOpen, PopoverOpts{}, func() {
			ui.Text(c, "Popover body")
			Button(c.Key("pb"), "Inside", ButtonOpts{})
		})
	})
	a.p.Show(c, a.source)
}

func newPaletteApp(t *testing.T) (*paletteApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	a := &paletteApp{}
	return a, ui.NewTester(a.view, 800, 600)
}

func TestPaletteOpensFocusingFieldAndListsGroups(t *testing.T) {
	a, tt := newPaletteApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	if !a.p.IsOpen() {
		t.Fatal("not open")
	}
	for _, s := range []string{"Playback", "Show or hide the queue", "Next track", "Library · Albums", "Move", "Run", "Close"} {
		if !tt.HasText(s) {
			t.Errorf("%q missing", s)
		}
	}
	if !tt.Focused("Find a page, action or music in your library") {
		t.Error("the field does not have the focus")
	}
}

func TestPaletteTypingRanksAndEnterRunsAfterClosingWithFocusBack(t *testing.T) {
	a, tt := newPaletteApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Type("next")
	if !tt.HasText("Commands") || !tt.HasText("Search Apple Music for “next”") {
		t.Fatal("results did not follow the typing")
	}
	tt.Key(0, ui.KeyEnter)
	tt.Advance(300 * time.Millisecond)
	if a.p.IsOpen() {
		t.Fatal("Enter did not close the palette")
	}
	if !slices.Equal(a.ran, []string{"next"}) {
		t.Errorf("ran %v, want [next] (ranked first)", a.ran)
	}
	if !tt.Focused("Open palette") {
		t.Error("focus did not return to the opener")
	}
}

func TestPaletteArrowsMoveAndWrap(t *testing.T) {
	a, tt := newPaletteApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyUp) // wraps to the last
	tt.Key(0, ui.KeyEnter)
	tt.Advance(300 * time.Millisecond)
	if !slices.Equal(a.ran, []string{"albums"}) {
		t.Fatalf("Up from the first: %v", a.ran)
	}
	a.ran = nil
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	tt.Advance(300 * time.Millisecond)
	if !slices.Equal(a.ran, []string{"next"}) {
		t.Errorf("Down: %v", a.ran)
	}
}

func TestPaletteEscapeClosesAndRunsNothing(t *testing.T) {
	a, tt := newPaletteApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	tt.Key(0, ui.KeyEscape)
	tt.Advance(300 * time.Millisecond)
	if a.p.IsOpen() || len(a.ran) != 0 {
		t.Errorf("open %v ran %v", a.p.IsOpen(), a.ran)
	}
	if !tt.Focused("Open palette") {
		t.Error("focus did not return to the opener")
	}
}

func TestPaletteClickRunsEntry(t *testing.T) {
	a, tt := newPaletteApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	if err := tt.Click("Library · Albums"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(300 * time.Millisecond)
	if !slices.Equal(a.ran, []string{"albums"}) {
		t.Errorf("ran %v", a.ran)
	}
}

func TestPaletteShowsKeysOfEntries(t *testing.T) {
	_, tt := newPaletteApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter)
	for _, s := range []string{"Ctrl", "Q", "→"} {
		if !tt.HasText(s) {
			t.Errorf("keycap %q missing", s)
		}
	}
}

func TestPopoverOpensBelowAnchorEscapeClosesAndFocusReturns(t *testing.T) {
	a, tt := newPaletteApp(t)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Details") {
		t.Fatal("anchor not focused")
	}
	tt.Key(0, ui.KeyEnter)
	if !a.popOpen || !tt.HasText("Popover body") {
		t.Fatal("popover did not open")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(300 * time.Millisecond)
	if a.popOpen {
		t.Fatal("Escape did not close the popover")
	}
	if !tt.Focused("Details") {
		t.Error("focus did not return to the anchor")
	}
}

func TestPopoverPressOutsideCloses(t *testing.T) {
	a, tt := newPaletteApp(t)
	a.popOpen = true
	tt.Frame()
	tt.ClickAt(700, 500)
	if a.popOpen {
		t.Error("a press outside did not close the popover")
	}
}
