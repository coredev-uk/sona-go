package kit

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

// controlsTester runs view, in light, in a window w×h under 20 DIPs of
// padding, as the button tests do.
func controlsTester(t *testing.T, w, h int, view func(c *ui.Context)) *ui.Tester {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() {
		theme.SetMode(theme.ModeSystem)
		theme.SetReducedMotion(false)
	})
	return ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		ui.Column(c).Padding(20).Gap(12).AlignItems(ui.Start).Children(func() { view(c) })
	}, w, h)
}

func controlsClick(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	if err := tt.Click(name); err != nil {
		t.Fatalf("click %q: %v", name, err)
	}
}

func controlsFind(t *testing.T, tt *ui.Tester, name string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(name)
	if !ok {
		t.Fatalf("no element %q among %q", name, tt.Texts())
	}
	return r
}

// --- Button, tooltip -------------------------------------------------------

func TestButtonVariantsAndSizes(t *testing.T) {
	for _, tc := range []struct {
		size ButtonSize
		want float32
	}{{SizeXS, 24}, {SizeSM, 32}, {SizeMD, 36}, {SizeLG, 40}} {
		for _, v := range []ButtonVariant{Default, Ghost, Outline, Play, Destructive, Secondary, Link} {
			tt := controlsTester(t, 200, 100, func(c *ui.Context) {
				IconButton(c, "plus", "Go", ButtonOpts{Variant: v, Size: tc.size})
			})
			if r := controlsFind(t, tt, "Go"); r.W != tc.want || r.H != tc.want {
				t.Errorf("variant %d, size %d: icon button %v×%v, want %v square", v, tc.size, r.W, r.H, tc.want)
			}
		}
	}
}

func TestSecondaryAndLinkFaces(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts ButtonOpts
		want string
	}{
		{"secondary is the secondary token", ButtonOpts{Variant: Secondary}, "secondary"},
		{"link has no face", ButtonOpts{Variant: Link}, "ground"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, tt := newButtonApp(t, "Label", tc.opts)
			tt.Advance(time.Second)
			if got, want := face(tt, "Label"), rgb(theme.C(tc.want)); got != want {
				t.Errorf("face %v, want %s %v", got, tc.want, want)
			}
		})
	}
}

func TestDisabledPlayDiscIsHoverToned(t *testing.T) {
	_, tt := newButtonApp(t, "", ButtonOpts{Variant: Play, Icon: "play", Filled: true, Disabled: true, A11yLabel: "Play"})
	tt.Advance(time.Second)
	tt.Frame()
	r := controlsFind(t, tt, "Play")
	got := tt.Image().RGBAAt(int(r.X)+4, int(r.Y+r.H/2))
	if want := rgb(theme.C("hover")); got != want {
		t.Errorf("disabled disc is %v, want --hover %v (and fully opaque)", got, want)
	}
}

func TestTooltipShowsOnHoverAndFocusAndEscapeHidesIt(t *testing.T) {
	clicks := 0
	tt := controlsTester(t, 300, 200, func(c *ui.Context) {
		if Button(c.Key("more"), "", ButtonOpts{Icon: "ellipsis", Variant: Outline, A11yLabel: "More actions", Tip: "Open the menu"}).Clicked() {
			clicks++
		}
		Button(c.Key("other"), "Elsewhere", ButtonOpts{Variant: Outline})
	})
	if tt.HasText("Open the menu") {
		t.Fatal("the tip shows before anything points at it")
	}
	r := controlsFind(t, tt, "More actions")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(time.Second)
	if !tt.HasText("Open the menu") {
		t.Fatalf("no tip after resting the pointer on the button: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(100 * time.Millisecond)
	if tt.HasText("Open the menu") {
		t.Error("Escape did not hide the tip")
	}
	// The keyboard focus brings it at once.
	tt.Move(250, 190)
	tt.Advance(time.Second)
	tt.Key(0, ui.KeyTab)
	tt.Advance(50 * time.Millisecond)
	if !tt.Focused("More actions") {
		t.Fatal("Tab did not focus the button")
	}
	if !tt.HasText("Open the menu") {
		t.Error("the keyboard focus did not show the tip")
	}
}

func TestTooltipDescribesItsAnchor(t *testing.T) {
	tt := controlsTester(t, 200, 100, func(c *ui.Context) {
		Tooltip(c, ui.Box(c).Size(30, 30).Focusable().Label("Thing"), "What the thing does")
	})
	if _, ok := tt.Find("Thing"); !ok {
		t.Fatal("no Thing")
	}
}

// --- Badge, marks, separator, skeleton --------------------------------------

func TestBadgeInteractive(t *testing.T) {
	n := 0
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		Badge(c.Key("static"), "Static", BadgeOpts{})
		if Badge(c.Key("link"), "Jazz", BadgeOpts{Variant: BadgeOutline, Interactive: true}).Clicked() {
			n++
		}
	})
	controlsClick(t, tt, "Jazz")
	if n != 1 {
		t.Errorf("clicked %d times", n)
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Jazz") {
		t.Error("an interactive badge does not take the focus")
	}
	if r := controlsFind(t, tt, "Static"); r.H > 20 {
		t.Errorf("badge is %v high", r.H)
	}
}

func TestExplicitMarkIsAnImageNamedExplicit(t *testing.T) {
	tt := controlsTester(t, 200, 100, func(c *ui.Context) { ExplicitMark(c) })
	r := controlsFind(t, tt, "Explicit")
	if r.W != 14 || r.H != 14 {
		t.Errorf("explicit mark is %v×%v, want 14×14", r.W, r.H)
	}
}

func TestTagsCarryTheirTitleAsDescription(t *testing.T) {
	tt := controlsTester(t, 400, 100, func(c *ui.Context) {
		ui.Row(c).Gap(6).Children(func() {
			PreviewMark(c.Key("p"))
			SDMark(c.Key("s"))
		})
	})
	for _, text := range []string{"Preview", "SD"} {
		controlsFind(t, tt, text)
	}
	r := controlsFind(t, tt, "Preview")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(time.Second)
	if !tt.HasText("Apple Music's 30-second preview, not the full song") {
		t.Errorf("the Preview tooltip is missing: %q", tt.Texts())
	}
}

func TestBadgesBuildNothingWithoutLabels(t *testing.T) {
	var shown bool
	tt := controlsTester(t, 400, 100, func(c *ui.Context) {
		shown = Badges(c, BadgesOpts{}).Valid()
	})
	_ = tt
	if shown {
		t.Error("Badges built an element with nothing to show")
	}
	tt = controlsTester(t, 500, 100, func(c *ui.Context) {
		Badges(c, BadgesOpts{Explicit: true, Traits: services.Traits{Atmos: true, Spatial: true, HiResLossless: true, Lossless: true, AppleDigitalMaster: true}, Extra: []string{"4K"}})
	})
	for _, want := range []string{"Explicit", "Dolby Atmos", "Hi-Res Lossless", "Apple Digital Master", "4K"} {
		controlsFind(t, tt, want)
	}
	for _, text := range tt.Texts() {
		if text == "Spatial Audio" || text == "Lossless" {
			t.Errorf("Badges shows %q beside the more specific label", text)
		}
	}
}

func TestTraitLabelsOrder(t *testing.T) {
	got := TraitLabels(services.Traits{Spatial: true, Lossless: true, AppleDigitalMaster: true})
	if strings.Join(got, "|") != "Spatial Audio|Lossless|Apple Digital Master" {
		t.Errorf("labels %q", got)
	}
}

func TestSeparatorIsAHairline(t *testing.T) {
	tt := controlsTester(t, 200, 100, func(c *ui.Context) {
		ui.Column(c).Width(100).Children(func() { Separator(c, false) })
	})
	img := tt.Image()
	if got, want := img.RGBAAt(60, 20), rgb(theme.C("border")); got != want {
		t.Errorf("separator pixel %v, want --border %v", got, want)
	}
}

func TestSkeletonPulsesUnlessMotionIsReduced(t *testing.T) {
	tt := controlsTester(t, 200, 100, func(c *ui.Context) { Skeleton(c, 100, 20) })
	px := func() [4]uint8 { p := tt.Image().RGBAAt(60, 30); return [4]uint8{p.R, p.G, p.B, p.A} }
	tt.Advance(time.Second) // half way through the 2 s pulse
	mid := px()
	tt.Advance(time.Second)
	full := px()
	if mid == full {
		t.Error("the skeleton does not pulse")
	}
	theme.SetReducedMotion(true)
	tt.Frame()
	a := px()
	tt.Advance(time.Second)
	if b := px(); a != b || tt.FramePending() {
		t.Errorf("a skeleton still moves under reduced motion: %v then %v (pending %v)", a, b, tt.FramePending())
	}
}

// --- Input, SearchField, Combobox -------------------------------------------

func TestInputTypesAndReportsChange(t *testing.T) {
	v, changed := "", 0
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		if Input(c, &v, InputOpts{A11yLabel: "Name", Placeholder: "Your name"}).Changed() {
			changed++
		}
	})
	controlsClick(t, tt, "Name")
	tt.Type("Thom")
	if v != "Thom" || changed == 0 {
		t.Errorf("value %q, changed %d", v, changed)
	}
	if r := controlsFind(t, tt, "Name"); r.H != 36 {
		t.Errorf("input is %v high, want 36", r.H)
	}
}

func TestInputBorderFollowsFocusAndInvalid(t *testing.T) {
	v := ""
	invalid := false
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		Input(c, &v, InputOpts{A11yLabel: "Name", Invalid: invalid})
	})
	tt.Advance(time.Second)
	r := controlsFind(t, tt, "Name")
	edge := func() [3]uint8 {
		tt.Frame()
		tt.Advance(time.Second)
		p := tt.Image().RGBAAt(int(r.X)+40, int(r.Y)) // the top border
		return [3]uint8{p.R, p.G, p.B}
	}
	rest := edge()
	if want := rgb(theme.C("input")); rest != [3]uint8{want.R, want.G, want.B} {
		t.Errorf("resting border %v, want --input %v", rest, want)
	}
	controlsClick(t, tt, "Name")
	focused := edge()
	if want := rgb(theme.C("ring")); focused == rest {
		t.Errorf("the focused border is still %v (ring is %v)", focused, want)
	}
	invalid = true
	danger := edge()
	if want := rgb(theme.C("destructive")); danger != [3]uint8{want.R, want.G, want.B} {
		t.Errorf("invalid border %v, want destructive %v", danger, want)
	}
}

func TestDisabledInputTakesNothing(t *testing.T) {
	v := "x"
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		Input(c, &v, InputOpts{A11yLabel: "Name", Disabled: true})
	})
	_ = tt.Click("Name")
	tt.Key(0, ui.KeyTab)
	tt.Type("abc")
	if v != "x" || tt.Focused("Name") {
		t.Errorf("a disabled input took %q (focused %v)", v, tt.Focused("Name"))
	}
}

func TestSearchFieldTypesClearsAndSubmits(t *testing.T) {
	q, changed, submitted := "", 0, 0
	tt := controlsTester(t, 320, 100, func(c *ui.Context) {
		in := SearchField(c, &q, SearchFieldOpts{Placeholder: "Search your library"})
		if in.Changed() {
			changed++
		}
		if in.Submitted() {
			submitted++
		}
	})
	if !tt.HasText("Search your library") {
		t.Error("no placeholder")
	}
	if _, ok := tt.Find("Clear"); ok {
		t.Error("a Clear button on an empty field")
	}
	controlsClick(t, tt, "Search your library")
	tt.Type("kid")
	if q != "kid" || changed == 0 {
		t.Fatalf("query %q, changed %d", q, changed)
	}
	tt.Key(0, ui.KeyEnter)
	if submitted == 0 {
		t.Error("Enter was not reported as Submitted")
	}
	controlsClick(t, tt, "Clear")
	if q != "" {
		t.Errorf("Clear left %q", q)
	}
	if !tt.Focused("Search your library") {
		t.Error("the focus did not return to the field after Clear")
	}
	tt.Type("a")
	tt.Key(0, ui.KeyEscape)
	if q != "" {
		t.Errorf("Escape left %q", q)
	}
}

func TestComboboxFiltersAndChooses(t *testing.T) {
	choice := "France"
	changed := 0
	opts := []string{"France", "Frankfurt", "Freetown", "Germany", "Spain"}
	tt := controlsTester(t, 360, 300, func(c *ui.Context) {
		if Combobox(c, &choice, opts, ComboboxOpts{A11yLabel: "Place", Width: 240}).Changed() {
			changed++
		}
	})
	controlsClick(t, tt, "Place")
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("ge")
	if !tt.HasText("Germany") || tt.HasText("Spain") {
		t.Errorf("typing did not filter: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEnter)
	if choice != "Germany" || changed == 0 {
		t.Errorf("Enter chose %q (changed %d), want Germany", choice, changed)
	}
	// Down shows every option; Down again and Enter choose the second.
	tt.Key(0, ui.KeyDown)
	if !tt.HasText("Spain") {
		t.Errorf("Down did not show all: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if choice == "Germany" {
		t.Errorf("an arrow and Enter chose nothing: %q", choice)
	}
	// Escape closes, and what was typed goes back to the choice on blur.
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("zzz")
	tt.Key(0, ui.KeyEscape)
	if tt.HasText("Spain") {
		t.Error("Escape left the options open")
	}
	before := choice
	tt.Key(0, ui.KeyTab)
	tt.Frame()
	if choice != before {
		t.Errorf("blur changed the choice to %q", choice)
	}
}

// --- Switch ----------------------------------------------------------------

func TestSwitchTogglesByClickAndSpace(t *testing.T) {
	on, changed := false, 0
	tt := controlsTester(t, 200, 100, func(c *ui.Context) {
		if Switch(c, &on, SwitchOpts{A11yLabel: "Crossfade"}).Changed() {
			changed++
		}
	})
	r := controlsFind(t, tt, "Crossfade")
	if r.W != 32 || r.H < 18 || r.H > 19 {
		t.Errorf("switch is %v×%v, want 32×18.4", r.W, r.H)
	}
	controlsClick(t, tt, "Crossfade")
	if !on || changed != 1 {
		t.Fatalf("click: on %v, changed %d", on, changed)
	}
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeySpace)
	if on {
		t.Error("Space did not turn it off")
	}
}

func TestSwitchColoursAndThumb(t *testing.T) {
	on := true
	tt := controlsTester(t, 200, 100, func(c *ui.Context) {
		Switch(c, &on, SwitchOpts{A11yLabel: "A"})
	})
	tt.Advance(time.Second)
	r := controlsFind(t, tt, "A")
	at := func(dx float32) [3]uint8 {
		p := tt.Image().RGBAAt(int(r.X+dx), int(r.Y+r.H/2))
		return [3]uint8{p.R, p.G, p.B}
	}
	ink, ground := rgb(theme.C("primary")), rgb(theme.C("background"))
	if got := at(5); got != [3]uint8{ink.R, ink.G, ink.B} {
		t.Errorf("checked track %v, want ink %v", got, ink)
	}
	if got := at(r.W - 9); got != [3]uint8{ground.R, ground.G, ground.B} {
		t.Errorf("checked thumb (at the end) %v, want ground %v", got, ground)
	}
	on = false
	tt.Frame()
	tt.Advance(time.Second)
	off := rgb(theme.C("input"))
	if got := at(r.W - 5); got != [3]uint8{off.R, off.G, off.B} {
		t.Errorf("unchecked track %v, want control-line %v", got, off)
	}
	if got := at(9); got != [3]uint8{ground.R, ground.G, ground.B} {
		t.Errorf("unchecked thumb (at the start) %v, want ground %v", got, ground)
	}
}

func TestSwitchSlidesUnlessMotionIsReduced(t *testing.T) {
	on := false
	tt := controlsTester(t, 200, 100, func(c *ui.Context) {
		Switch(c, &on, SwitchOpts{A11yLabel: "A"})
	})
	r := controlsFind(t, tt, "A")
	thumbAt := func() int {
		tt.Frame()
		row := int(r.Y + r.H/2)
		for x := int(r.X) + 2; x < int(r.X+r.W)-1; x++ {
			if p := tt.Image().RGBAAt(x, row); p.R > 230 && p.G > 230 && p.B > 230 {
				return x
			}
		}
		return -1
	}
	rest := thumbAt()
	on = true
	tt.Frame()
	tt.Advance(60 * time.Millisecond)
	mid := thumbAt()
	tt.Advance(time.Second)
	end := thumbAt()
	if rest >= mid || mid >= end {
		t.Errorf("the thumb did not slide: %d, %d, %d", rest, mid, end)
	}
	theme.SetReducedMotion(true)
	on = false
	tt.Frame()
	if got := thumbAt(); got != rest {
		t.Errorf("reduced motion: the thumb is at %d at once, want %d", got, rest)
	}
}

func TestDisabledSwitchIsInert(t *testing.T) {
	on := false
	tt := controlsTester(t, 200, 100, func(c *ui.Context) {
		Switch(c, &on, SwitchOpts{A11yLabel: "A", Disabled: true})
	})
	_ = tt.Click("A")
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeySpace)
	if on || tt.Focused("A") {
		t.Errorf("a disabled switch changed (%v) or took the focus", on)
	}
}

// --- Select ----------------------------------------------------------------

func controlsPlanItems() []SelectItem {
	return []SelectItem{
		SelectLabel("Plans"), SelectOption("free", "Free"), SelectOption("pro", "Pro"), SelectSeparator(),
		SelectLabel("Teams"), {Value: "ent", Label: "Enterprise", Disabled: true}, SelectOption("team", "Team"),
	}
}

func TestSelectOpensChoosesAndReturnsTheFocus(t *testing.T) {
	v, changed := "free", 0
	tt := controlsTester(t, 360, 360, func(c *ui.Context) {
		if Select(c, &v, controlsPlanItems(), SelectOpts{A11yLabel: "Plan"}).Changed() {
			changed++
		}
	})
	if !tt.HasText("Free") || tt.HasText("Pro") {
		t.Fatalf("closed select shows %q", tt.Texts())
	}
	controlsClick(t, tt, "Plan")
	for _, want := range []string{"Plans", "Teams", "Pro", "Team"} {
		if !tt.HasText(want) {
			t.Errorf("open list lacks %q: %q", want, tt.Texts())
		}
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if v != "pro" || changed == 0 {
		t.Fatalf("choice %q, changed %d", v, changed)
	}
	if tt.HasText("Teams") {
		t.Error("the list stays open after a choice")
	}
	if !tt.Focused("Plan") {
		t.Error("the focus did not return to the trigger")
	}
	// Escape closes without choosing.
	tt.Key(0, ui.KeyEnter)
	if !tt.HasText("Teams") {
		t.Fatalf("Enter on the trigger did not open it: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEscape)
	if v != "pro" || tt.HasText("Teams") || !tt.Focused("Plan") {
		t.Errorf("Escape: choice %q, open %v, focus %v", v, tt.HasText("Teams"), tt.Focused("Plan"))
	}
	// A click on a row chooses it.
	controlsClick(t, tt, "Plan")
	controlsClick(t, tt, "Team")
	if v != "team" {
		t.Errorf("a click chose %q", v)
	}
}

func TestSelectArrowsSkipDisabledOptions(t *testing.T) {
	v := "pro"
	tt := controlsTester(t, 360, 360, func(c *ui.Context) {
		Select(c, &v, controlsPlanItems(), SelectOpts{A11yLabel: "Plan"})
	})
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyEnter) // open, on Pro
	tt.Key(0, ui.KeyDown)  // Enterprise is disabled: Team is next
	tt.Key(0, ui.KeyEnter)
	if v != "team" {
		t.Errorf("Down from Pro chose %q, want team", v)
	}
}

func TestSelectKeepsTheKeyboardInTheOpenList(t *testing.T) {
	v := "free"
	tt := controlsTester(t, 360, 360, func(c *ui.Context) {
		Select(c.Key("a"), &v, controlsPlanItems(), SelectOpts{A11yLabel: "Plan"})
		Button(c.Key("b"), "After", ButtonOpts{})
	})
	controlsClick(t, tt, "Plan")
	tt.Key(0, ui.KeyTab)
	if tt.Focused("After") || !tt.Focused("Plan") {
		t.Error("Tab left the open list")
	}
}

func TestSelectDisabledOptionCannotBeChosen(t *testing.T) {
	v := "free"
	tt := controlsTester(t, 360, 360, func(c *ui.Context) {
		Select(c, &v, controlsPlanItems(), SelectOpts{A11yLabel: "Plan"})
	})
	controlsClick(t, tt, "Plan")
	_ = tt.Click("Enterprise")
	if v == "ent" {
		t.Error("a disabled option was chosen")
	}
}

func TestSelectShowsAPlaceholderAndSizes(t *testing.T) {
	v := ""
	tt := controlsTester(t, 360, 200, func(c *ui.Context) {
		Select(c.Key("a"), &v, controlsPlanItems(), SelectOpts{A11yLabel: "Plan", Placeholder: "Pick one"})
		Select(c.Key("b"), &v, controlsPlanItems(), SelectOpts{A11yLabel: "Small", Small: true})
	})
	if !tt.HasText("Pick one") {
		t.Errorf("no placeholder: %q", tt.Texts())
	}
	if a, b := controlsFind(t, tt, "Plan"), controlsFind(t, tt, "Small"); a.H != 36 || b.H != 32 {
		t.Errorf("heights %v and %v, want 36 and 32", a.H, b.H)
	}
}

func TestSelectLongListScrollsWithinTheWindow(t *testing.T) {
	var items []SelectItem
	for i := 1; i <= 60; i++ {
		items = append(items, SelectOption(fmt.Sprint(i), fmt.Sprintf("Language %d", i)))
	}
	v := "1"
	tt := controlsTester(t, 300, 260, func(c *ui.Context) {
		Select(c, &v, items, SelectOpts{A11yLabel: "Language"})
	})
	controlsClick(t, tt, "Language")
	if !tt.HasText("Language 3") {
		t.Fatalf("no options: %q", tt.Texts())
	}
	if r := controlsFind(t, tt, "Language 60"); r.Y < 260 {
		t.Errorf("the last option is at %v inside a window 260 high: the list is not capped", r.Y)
	}
	tt.Key(0, ui.KeyEnd)
	tt.Frame()
	tt.Advance(time.Second)
	if r := controlsFind(t, tt, "Language 60"); r.Y+r.H > 260 {
		t.Errorf("End did not scroll the last option into view: at %v", r)
	}
}

// --- Segmented -------------------------------------------------------------

func controlsSegments() []SegmentOption[string] {
	return []SegmentOption[string]{{"list", "List"}, {"grid", "Grid"}, {"table", "Table"}}
}

func TestSegmentedChoosesAndReportsEveryClick(t *testing.T) {
	v, changed := "list", 0
	tt := controlsTester(t, 360, 100, func(c *ui.Context) {
		if Segmented(c, &v, controlsSegments(), "View").Changed() {
			changed++
		}
	})
	if r := controlsFind(t, tt, "View"); r.H != 32 {
		t.Errorf("control is %v high, want 32", r.H)
	}
	controlsClick(t, tt, "Grid")
	if v != "grid" || changed == 0 {
		t.Fatalf("click: %q, changed %d", v, changed)
	}
	changed = 0
	controlsClick(t, tt, "Grid")
	if v != "grid" || changed == 0 {
		t.Errorf("clicking the chosen segment reported nothing (changed %d)", changed)
	}
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyRight)
	if v != "table" {
		t.Errorf("the arrow chose %q, want table", v)
	}
}

func TestSegmentedRaisesTheChosen(t *testing.T) {
	v := "grid"
	tt := controlsTester(t, 360, 100, func(c *ui.Context) { Segmented(c, &v, controlsSegments(), "View") })
	tt.Advance(time.Second)
	on, off := controlsFind(t, tt, "Grid"), controlsFind(t, tt, "List")
	img := tt.Image()
	px := func(r ui.Rect) [3]uint8 { p := img.RGBAAt(int(r.X)-6, int(r.Y+r.H/2)); return [3]uint8{p.R, p.G, p.B} }
	if got, want := px(on), ([3]uint8{255, 255, 255}); got != want {
		t.Errorf("chosen segment face %v, want raised %v", got, want)
	}
	hover := rgb(theme.C("hover"))
	if got := px(off); got != [3]uint8{hover.R, hover.G, hover.B} {
		t.Errorf("others sit on the --hover track: %v, want %v", got, hover)
	}
}

// --- Slider ----------------------------------------------------------------

func TestSliderKeysDragAndLabel(t *testing.T) {
	v, changed := 50.0, 0
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		if Slider(c, &v, 0, 100, SliderOpts{A11yLabel: "Volume", ValueText: "50%", Width: 200}).Changed() {
			changed++
		}
	})
	r := controlsFind(t, tt, "Volume")
	if r.H != 16 {
		t.Errorf("slider is %v high, want 16", r.H)
	}
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Volume") {
		t.Fatal("Tab did not focus the slider")
	}
	tt.Key(0, ui.KeyRight)
	if v != 51 || changed == 0 {
		t.Errorf("Right: %v, changed %d", v, changed)
	}
	tt.Key(0, ui.KeyPageUp)
	if v != 61 {
		t.Errorf("Page Up: %v, want a tenth of the range more", v)
	}
	tt.Key(0, ui.KeyHome)
	tt.Key(0, ui.KeyEnd)
	if v != 100 {
		t.Errorf("End: %v", v)
	}
	// Pressing the track sets the value from the pointer.
	tt.ClickAt(r.X+6+(r.W-12)*0.25, r.Y+r.H/2)
	if v < 24 || v > 26 {
		t.Errorf("a press at a quarter set %v", v)
	}
}

func TestDisabledSliderIgnoresPointerAndKeys(t *testing.T) {
	v := 30.0
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		Slider(c, &v, 0, 100, SliderOpts{A11yLabel: "Volume", Disabled: true, Width: 200})
	})
	r := controlsFind(t, tt, "Volume")
	tt.ClickAt(r.X+r.W-4, r.Y+r.H/2)
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyRight)
	if v != 30 {
		t.Errorf("a disabled slider moved to %v", v)
	}
}

func TestSliderThumbAndRange(t *testing.T) {
	v := 50.0
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		Slider(c, &v, 0, 100, SliderOpts{A11yLabel: "Volume", Width: 200, Range: theme.C("amber")})
	})
	r := controlsFind(t, tt, "Volume")
	img := tt.Image()
	mid := int(r.Y + r.H/2)
	ink := rgb(theme.C("ink"))
	if got := img.RGBAAt(int(r.X+6+(r.W-12)*0.5), mid); got != ink {
		t.Errorf("thumb at the value is %v, want ink %v", got, ink)
	}
	amber := rgb(theme.C("amber"))
	if got := img.RGBAAt(int(r.X+6+(r.W-12)*0.25), mid); got != amber {
		t.Errorf("range before the thumb is %v, want %v", got, amber)
	}
	if got := img.RGBAAt(int(r.X+6+(r.W-12)*0.8), mid); got == amber || got == ink {
		t.Errorf("track after the thumb is %v: it should be the faint track", got)
	}
}

func TestVerticalSlider(t *testing.T) {
	v := 0.0
	tt := controlsTester(t, 100, 300, func(c *ui.Context) {
		Slider(c, &v, -12, 12, SliderOpts{A11yLabel: "Gain", Vertical: true, Step: 1})
	})
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyUp)
	if v != 1 {
		t.Errorf("Up on a vertical stepped slider: %v, want 1", v)
	}
	if r := controlsFind(t, tt, "Gain"); r.H < 176 {
		t.Errorf("vertical slider is %v high, want at least 176", r.H)
	}
}

// --- Tabs ------------------------------------------------------------------

func TestTabsArrowsHomeEndAndPanel(t *testing.T) {
	sel, changed := 0, 0
	labels := []string{"General", "Playback", "Advanced"}
	tt := controlsTester(t, 400, 200, func(c *ui.Context) {
		if Tabs(c, &sel, labels, TabsOpts{A11yLabel: "Settings"}).Changed() {
			changed++
		}
		TabPanel(c, labels[sel])
		ui.Text(c, "Page "+labels[sel])
	})
	controlsClick(t, tt, "Playback")
	if sel != 1 || changed == 0 {
		t.Fatalf("click: %d, changed %d", sel, changed)
	}
	if !tt.HasText("Page Playback") {
		t.Error("the panel did not follow")
	}
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyRight)
	if sel != 2 || !tt.Focused("Advanced") {
		t.Errorf("Right: tab %d, focus on Advanced %v", sel, tt.Focused("Advanced"))
	}
	tt.Key(0, ui.KeyHome)
	if sel != 0 || !tt.Focused("General") {
		t.Errorf("Home: tab %d", sel)
	}
	tt.Key(0, ui.KeyEnd)
	if sel != 2 {
		t.Errorf("End: tab %d", sel)
	}
	if r := controlsFind(t, tt, "Settings"); r.H != 36 {
		t.Errorf("tab list is %v high, want 36", r.H)
	}
}

func TestTabsVerticalAndLine(t *testing.T) {
	sel := 0
	tt := controlsTester(t, 300, 300, func(c *ui.Context) {
		Tabs(c, &sel, []string{"One", "Two", "Three"}, TabsOpts{Vertical: true, Line: true, A11yLabel: "Pages"})
	})
	one, two := controlsFind(t, tt, "One"), controlsFind(t, tt, "Two")
	if two.Y <= one.Y || two.X != one.X {
		t.Errorf("vertical tabs are not stacked: %v then %v", one, two)
	}
	tt.Key(0, ui.KeyTab)
	tt.Key(0, ui.KeyDown)
	if sel != 1 {
		t.Errorf("Down: %d", sel)
	}
}

// --- ProgressBar -----------------------------------------------------------

func TestProgressBarValueAndIndeterminate(t *testing.T) {
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		ui.Column(c).Width(200).Gap(10).Children(func() {
			ProgressBar(c.Key("a"), 0.5, ProgressOpts{A11yLabel: "Download", ValueText: "50%, 3 MB of 6 MB"})
			ProgressBar(c.Key("b"), -1, ProgressOpts{A11yLabel: "Starting"})
		})
	})
	tt.Advance(time.Second)
	a := controlsFind(t, tt, "Download")
	img := tt.Image()
	muted, track := rgb(theme.C("ink-muted")), img.RGBAAt(int(a.X+a.W-4), int(a.Y+a.H/2))
	if got := img.RGBAAt(int(a.X+a.W/4), int(a.Y+a.H/2)); got != muted {
		t.Errorf("fill %v, want ink-muted %v", got, muted)
	}
	if track == muted {
		t.Errorf("the empty half is filled")
	}
	if a.H != 4 {
		t.Errorf("bar is %v high, want 4", a.H)
	}
	b := controlsFind(t, tt, "Starting")
	if got := img.RGBAAt(int(b.X+b.W/4), int(b.Y+b.H/2)); got == muted {
		t.Error("an indeterminate bar is filled")
	}
	if tt.FramePending() {
		t.Error("the bars keep asking for frames")
	}
}

// --- RatingButtons, LibraryButton, PlayButtons ------------------------------

func TestRatingButtons(t *testing.T) {
	rating, pending, compact := 0, false, false
	var got RatingAction
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		if a := RatingButtons(c, RatingOpts{Name: "OK Computer", Rating: rating, Pending: pending, Compact: compact}); a != RateNone {
			got = a
		}
	})
	r := controlsFind(t, tt, "Favourite OK Computer")
	if r.W != 32 {
		t.Errorf("button is %v wide, want 32", r.W)
	}
	controlsClick(t, tt, "Favourite OK Computer")
	if got != RateFavorite {
		t.Errorf("click asked for %q", got)
	}
	controlsClick(t, tt, "Suggest less like OK Computer")
	if got != RateSuggestLess {
		t.Errorf("click asked for %q", got)
	}
	rating = 1
	tt.Frame()
	controlsClick(t, tt, "Undo favourite for OK Computer")
	if got != RateUnfavorite {
		t.Errorf("click on a favourite asked for %q", got)
	}
	rating = -1
	tt.Frame()
	controlsClick(t, tt, "Undo suggest less for OK Computer")
	if got != RateUndoSuggestLess {
		t.Errorf("click on a suggested-less item asked for %q", got)
	}
	pending, compact = true, true
	tt.Frame()
	got = RateNone
	_ = tt.Click("Favourite OK Computer")
	if got != RateNone {
		t.Errorf("a pending button reported %q", got)
	}
	if r := controlsFind(t, tt, "Favourite OK Computer"); r.W != 24 {
		t.Errorf("compact button is %v wide, want 24", r.W)
	}
}

func TestRatingButtonsHideSuggestLess(t *testing.T) {
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		RatingButtons(c, RatingOpts{Name: "X", HideSuggestLess: true})
	})
	if _, ok := tt.Find("Suggest less like X"); ok {
		t.Error("Suggest Less shows although hidden")
	}
	controlsFind(t, tt, "Favourite X")
}

func TestRatingButtonsPressedState(t *testing.T) {
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		ui.Row(c).Children(func() { RatingButtons(c, RatingOpts{Name: "X", Rating: 1}) })
	})
	tt.Advance(time.Second)
	// The star is filled ink when it is a favourite.
	r := controlsFind(t, tt, "Undo favourite for X")
	img := tt.Image()
	ink := rgb(theme.C("ink"))
	if got := img.RGBAAt(int(r.X+r.W/2), int(r.Y+r.H/2)); got != ink {
		t.Errorf("favourite star centre %v, want filled ink %v", got, ink)
	}
}

func TestLibraryButton(t *testing.T) {
	in, pending, playlist := false, false, false
	var got LibraryAction
	tt := controlsTester(t, 400, 100, func(c *ui.Context) {
		if a := LibraryButton(c, LibraryOpts{Name: "OK Computer", InLibrary: in, Pending: pending, Playlist: playlist}); a != LibraryNone {
			got = a
		}
	})
	controlsClick(t, tt, "Add")
	if got != LibraryAdd {
		t.Errorf("Add asked for %v", got)
	}
	pending = true
	tt.Frame()
	got = LibraryNone
	_ = tt.Click("Add")
	if got != LibraryNone {
		t.Error("a pending Add reported a click")
	}
	pending, in = false, true
	tt.Frame()
	controlsClick(t, tt, "In library. Delete “OK Computer” from your library")
	if got != LibraryRemove {
		t.Errorf("In library asked for %v", got)
	}
	playlist = true
	tt.Frame()
	got = LibraryNone
	_ = tt.Click("In library")
	if got != LibraryNone {
		t.Error("the playlist's label took a click")
	}
	if _, ok := tt.Find("In library. Delete “OK Computer” from your library"); ok {
		t.Error("a playlist offers deletion")
	}
}

func TestLibraryButtonAddLabel(t *testing.T) {
	tt := controlsTester(t, 400, 100, func(c *ui.Context) {
		LibraryButton(c, LibraryOpts{Name: "X", AddLabel: "Pre-add"})
	})
	controlsFind(t, tt, "Pre-add")
}

func TestPlayButtons(t *testing.T) {
	var play, shuffle bool
	offer, disabled := true, false
	opts := PlayButtonsOpts{Label: "OK Computer"}
	tt := controlsTester(t, 400, 100, func(c *ui.Context) {
		o := opts
		o.Shuffle, o.Disabled = offer, disabled
		ui.Row(c).Gap(8).Children(func() {
			p, s := PlayButtons(c, o)
			play, shuffle = play || p, shuffle || s
		})
	})
	r := controlsFind(t, tt, "Play OK Computer")
	if r.W != 40 || r.H != 40 {
		t.Errorf("disc is %v×%v, want 40", r.W, r.H)
	}
	controlsClick(t, tt, "Play OK Computer")
	if !play || shuffle {
		t.Errorf("play %v, shuffle %v", play, shuffle)
	}
	play = false
	controlsClick(t, tt, "Shuffle")
	if play || !shuffle {
		t.Errorf("play %v, shuffle %v", play, shuffle)
	}
	offer = false
	tt.Frame()
	if _, ok := tt.Find("Shuffle"); ok {
		t.Error("Shuffle shows for a single song")
	}
	opts = PlayButtonsOpts{PlayLabel: "Play your songs"}
	tt.Frame()
	controlsFind(t, tt, "Play your songs")
	disabled = true
	tt.Frame()
	play = false
	_ = tt.Click("Play your songs")
	if play {
		t.Error("a disabled disc was clicked")
	}
}

func TestPlayButtonIsAnAmberDisc(t *testing.T) {
	n := 0
	tt := controlsTester(t, 200, 120, func(c *ui.Context) {
		if PlayButton(c, "Play Abbey Road", 48).Clicked() {
			n++
		}
	})
	r := controlsFind(t, tt, "Play Abbey Road")
	if r.W != 48 || r.H != 48 {
		t.Errorf("disc is %v", r)
	}
	tt.Advance(time.Second)
	if got, want := tt.Image().RGBAAt(int(r.X)+6, int(r.Y)+24), rgb(theme.C("amber")); got != want {
		t.Errorf("disc %v, want amber %v", got, want)
	}
	controlsClick(t, tt, "Play Abbey Road")
	if n != 1 {
		t.Errorf("clicked %d times", n)
	}
}

// --- EmptyState, ErrorBlock, DetailStates ----------------------------------

func TestEmptyStateActionRuns(t *testing.T) {
	ran := 0
	tt := controlsTester(t, 500, 300, func(c *ui.Context) {
		EmptyState(c, "No albums yet", "Add music and it appears here.", EmptyOpts{Action: &EmptyAction{Label: "Browse", Run: func() { ran++ }}})
	})
	controlsFind(t, tt, "No albums yet")
	controlsFind(t, tt, "Add music and it appears here.")
	controlsClick(t, tt, "Browse")
	if ran != 1 {
		t.Errorf("action ran %d times", ran)
	}
	// A heading, for the page's outline.
	if r := controlsFind(t, tt, "Browse"); r.H > 32 {
		t.Errorf("action is %v high, want the small size", r.H)
	}
}

func TestEmptyStateCoversDrawsOneRowOfSlots(t *testing.T) {
	tt := controlsTester(t, 760, 400, func(c *ui.Context) {
		ui.Column(c).Width(720).Children(func() {
			EmptyState(c, "No albums yet", "Add music.", EmptyOpts{Frame: FrameCovers})
		})
	})
	tt.Advance(time.Second)
	img := tt.Image()
	// 720 wide: (720+24)/(160+24) = 4 slots, each 162 wide, 24 apart, and
	// their dashed top edges are the only thing on the row.
	ground := rgb(theme.C("ground"))
	inked := func(x0, x1 int) bool {
		for y := 20; y <= 21; y++ {
			for x := x0; x < x1; x++ {
				if img.RGBAAt(x, y) != ground {
					return true
				}
			}
		}
		return false
	}
	for k := 0; k < 4; k++ {
		if x := 20 + k*186; !inked(x+2, x+160) {
			t.Errorf("slot %d has no dashed edge", k)
		}
		if x := 20 + k*186 + 164; k < 3 && inked(x, x+20) {
			t.Errorf("something is drawn between slots %d and %d", k, k+1)
		}
	}
}

func TestErrorBlockWordsGoFailureAndRetries(t *testing.T) {
	retries, retrying := 0, false
	tt := controlsTester(t, 600, 200, func(c *ui.Context) {
		ErrorBlock(c, "Couldn't load this album", &net.DNSError{Err: "no such host"}, ErrorBlockOpts{OnRetry: func() { retries++ }, Retrying: retrying})
	})
	controlsFind(t, tt, "Couldn't load this album")
	if !tt.HasText("Sona couldn't reach the network. Check your connection and try again.") {
		t.Errorf("no reason: %q", tt.Texts())
	}
	if strings.Contains(strings.Join(tt.Texts(), " "), "no such host") {
		t.Error("Go's own error text reached the screen")
	}
	if got := tt.Announcements(); len(got) != 1 || !strings.Contains(got[0], "Couldn't load this album") {
		t.Errorf("announcements %q, want the failure once", got)
	}
	controlsClick(t, tt, "Try again")
	if retries != 1 {
		t.Errorf("retried %d times", retries)
	}
	retrying = true
	tt.Frame()
	controlsFind(t, tt, "Retrying…")
	_ = tt.Click("Retrying…")
	if retries != 1 {
		t.Error("the retrying button took a click")
	}
	if got := tt.Announcements(); len(got) != 0 {
		t.Errorf("the failure was announced again: %q", got)
	}
}

func TestErrorBlockWithoutRetryHasNoButton(t *testing.T) {
	tt := controlsTester(t, 600, 200, func(c *ui.Context) {
		ErrorBlock(c, "Couldn't load this", errors.New("boom"), ErrorBlockOpts{})
	})
	if _, ok := tt.Find("Try again"); ok {
		t.Error("a Try again button without OnRetry")
	}
}

func TestDetailSkeletonIsABusyStatus(t *testing.T) {
	tt := controlsTester(t, 900, 700, func(c *ui.Context) {
		ui.Column(c).Width(800).Children(func() { DetailSkeleton(c, true) })
	})
	controlsFind(t, tt, "Loading")
}

func TestDetailErrorByCode(t *testing.T) {
	var err error
	back, signIn, retry := 0, 0, 0
	tt := controlsTester(t, 700, 400, func(c *ui.Context) {
		DetailError(c, NounArtist, err, DetailErrorOpts{
			Retry: func() { retry++ }, Back: EmptyAction{Label: "Back to Artists", Run: func() { back++ }}, SignIn: func() { signIn++ },
		})
	})
	err = fmt.Errorf("artist: %w", applemusic.ErrNotFound)
	tt.Frame()
	controlsFind(t, tt, "This artist isn't available")
	controlsClick(t, tt, "Back to Artists")
	if back != 1 {
		t.Errorf("back ran %d times", back)
	}
	err = auth.ErrNotSignedIn
	tt.Frame()
	controlsFind(t, tt, "Sign in to see this")
	controlsFind(t, tt, "Apple Music needs a signed-in account to show this artist.")
	controlsClick(t, tt, "Sign in")
	if signIn != 1 {
		t.Errorf("sign in ran %d times", signIn)
	}
	err = &net.DNSError{Err: "x"}
	tt.Frame()
	controlsFind(t, tt, "Couldn't load this artist")
	controlsClick(t, tt, "Try again")
	if retry != 1 {
		t.Errorf("retry ran %d times", retry)
	}
}

func TestDetailErrorNouns(t *testing.T) {
	for noun, want := range map[DetailNoun]string{
		NounAlbum: "album", NounArtist: "artist", NounPlaylist: "playlist", NounSong: "song", NounMusicVideo: "music video",
	} {
		tt := controlsTester(t, 700, 300, func(c *ui.Context) {
			DetailError(c, noun, applemusic.ErrNotFound, DetailErrorOpts{Back: EmptyAction{Label: "Back"}})
		})
		controlsFind(t, tt, "This "+want+" isn't available")
	}
}

// --- PageHeader, CollectionHeader, Notes, ArtistLinks ------------------------

func TestPageHeader(t *testing.T) {
	n := 0
	tt := controlsTester(t, 700, 200, func(c *ui.Context) {
		PageHeader(c, "Albums", "128 albums", func() {
			if Button(c, "Sort", ButtonOpts{Variant: Outline, Size: SizeSM}).Clicked() {
				n++
			}
		})
	})
	controlsFind(t, tt, "Albums")
	controlsFind(t, tt, "128 albums")
	controlsClick(t, tt, "Sort")
	if n != 1 {
		t.Error("the page's action did not run")
	}
	title, sort := controlsFind(t, tt, "Albums"), controlsFind(t, tt, "Sort")
	if sort.X < title.X+title.W {
		t.Errorf("actions at %v are not at the end of the row (title %v)", sort, title)
	}
}

func TestCollectionHeaderBuildsItsParts(t *testing.T) {
	l := newArtLoader(&fakeArt{}, nil, 1)
	t.Cleanup(l.Close)
	var built []string
	tt := controlsTester(t, 1100, 400, func(c *ui.Context) {
		CollectionHeader(c, CollectionHeaderOpts{
			Title: "OK Computer", Loader: l,
			Byline:      func() { built = append(built, "byline"); ui.Text(c, "Radiohead") },
			Meta:        "Album · 1997",
			Badges:      func() { Badges(c, BadgesOpts{Explicit: true}) },
			Description: "A long description.",
			Status:      func() { ui.Text(c, "Out in 3 days") },
			Actions:     func() { PlayButtons(c, PlayButtonsOpts{Label: "OK Computer"}) },
			Backdrop:    func() { built = append(built, "backdrop") },
		})
	})
	for _, want := range []string{"OK Computer", "Radiohead", "Album · 1997", "Explicit", "A long description.", "Out in 3 days", "Play OK Computer"} {
		controlsFind(t, tt, want)
	}
	if len(built) < 2 {
		t.Errorf("built %q", built)
	}
	// 1100 wide: the 200px cover; narrow windows get the smaller one.
	title, play := controlsFind(t, tt, "OK Computer"), controlsFind(t, tt, "Play OK Computer")
	if title.X < 20+200+28-1 {
		t.Errorf("title at %v: the cover is not 200 wide", title.X)
	}
	_ = play
	tt.SetSize(900, 400)
	tt.Frame()
	if title := controlsFind(t, tt, "OK Computer"); title.X > 20+136+28+1 {
		t.Errorf("narrow title at %v: the cover is not 136 wide", title.X)
	}
}

func TestNotesClampsAndExpands(t *testing.T) {
	long := strings.Repeat("A sentence that goes on and on about nothing in particular. ", 12) + "\n\nSecond paragraph."
	short := "Just one line."
	text := long
	tt := controlsTester(t, 700, 600, func(c *ui.Context) { Notes(c, text, 0) })
	tt.Advance(100 * time.Millisecond)
	more, ok := tt.Find("Read more")
	if !ok {
		t.Fatalf("no Read more for long text: %q", tt.Texts())
	}
	if tt.HasText("Second paragraph.") {
		t.Error("the clamp shows the second paragraph")
	}
	_ = more
	controlsClick(t, tt, "Read more")
	tt.Advance(100 * time.Millisecond)
	if !tt.HasText("Second paragraph.") || !tt.HasText("Show less") {
		t.Errorf("not expanded: %q", tt.Texts())
	}
	controlsClick(t, tt, "Show less")
	tt.Advance(100 * time.Millisecond)
	if tt.HasText("Second paragraph.") || !tt.HasText("Read more") {
		t.Errorf("not collapsed: %q", tt.Texts())
	}
	text = short
	tt.Frame()
	tt.Advance(100 * time.Millisecond)
	if tt.HasText("Read more") {
		t.Error("Read more shows for text that fits")
	}
}

func TestNotesParagraphs(t *testing.T) {
	got := notesParagraphs("one\nstill one\n\ntwo\n\n\nthree")
	want := []string{"one\nstill one", "two", "three"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("paragraphs %q, want %q", got, want)
	}
}

func TestArtistLinksJoinWithCommasAndAnAmpersand(t *testing.T) {
	artists := []services.Artist{{ID: "1", Name: "Daft Punk"}, {ID: "2", Name: "Pharrell Williams"}, {ID: "3", Name: "Nile Rodgers"}}
	tt := controlsTester(t, 700, 100, func(c *ui.Context) { ArtistLinks(c, artists, "x", ui.Color{}) })
	for _, a := range artists {
		controlsFind(t, tt, a.Name)
	}
	joined := strings.Join(tt.Texts(), "")
	if !strings.Contains(joined, ", ") || !strings.Contains(joined, " & ") {
		t.Errorf("texts %q: want \"A, B & C\"", tt.Texts())
	}
	tt = controlsTester(t, 700, 100, func(c *ui.Context) { ArtistLinks(c, nil, "Daft Punk & Friends", ui.Color{}) })
	controlsFind(t, tt, "Daft Punk & Friends")
}

func TestArtistLinksGoToTheArtistPage(t *testing.T) {
	artists := []services.Artist{{ID: "r.42", Name: "Radiohead"}}
	var path string
	router := ui.NewRouter("/")
	tt := ui.NewTester(func(c *ui.Context) {
		theme.Apply(c)
		router.View(c, func(r *ui.Route) {
			if r.Match("/artist/{id}") {
				path = r.Param("id")
				ui.Text(c, "Artist page")
				return
			}
			ArtistLinks(c, artists, "x", ui.Color{})
		})
	}, 400, 200)
	r := controlsFind(t, tt, "Radiohead") // the paragraph's box: the link is at its start
	tt.ClickAt(r.X+10, r.Y+r.H/2)
	if path != "r.42" || !tt.HasText("Artist page") {
		t.Errorf("went to %q (%q)", path, tt.Texts())
	}
}

// --- Focus rings, more keys ------------------------------------------------

// controlsRingAround reports whether anything but the ground is drawn within off
// DIPs above the top edge of r, in the middle.
func controlsRingAround(tt *ui.Tester, r ui.Rect, off int) bool {
	tt.Frame()
	for d := 1; d <= off; d++ {
		if tt.Image().RGBAAt(int(r.X+r.W/2), int(r.Y)-d) != rgb(theme.C("ground")) {
			return true
		}
	}
	return false
}

func TestFocusRingShowsForTheKeyboardOnly(t *testing.T) {
	for name, build := range map[string]func(c *ui.Context){
		"button": func(c *ui.Context) { Button(c.Key("b"), "Target", ButtonOpts{Variant: Outline, A11yLabel: "Target"}) },
		"switch": func(c *ui.Context) { on := false; Switch(c.Key("b"), &on, SwitchOpts{A11yLabel: "Target"}) },
		"tab": func(c *ui.Context) {
			sel := 0
			Tabs(c.Key("b"), &sel, []string{"Target", "Other"}, TabsOpts{Line: true, A11yLabel: "Tabs"})
		},
	} {
		t.Run(name, func(t *testing.T) {
			newTester := func() *ui.Tester {
				return controlsTester(t, 300, 100, func(c *ui.Context) {
					ui.Column(c).Fill().Background(theme.C("ground")).Padding(20).Children(func() { build(c) })
				})
			}
			off := 2
			if name == "tab" {
				off = 12 // the label sits inside the tab's padding
			}
			tt := newTester()
			r := controlsFind(t, tt, "Target")
			controlsClick(t, tt, "Target")
			tt.Move(250, 90)
			tt.Advance(time.Second)
			if controlsRingAround(tt, r, off) {
				t.Error("a ring after a mouse click")
			}
			tt = newTester()
			tt.Key(0, ui.KeyTab)
			for i := 0; i < 3 && !tt.Focused("Target"); i++ {
				tt.Key(0, ui.KeyTab)
			}
			tt.Advance(time.Second)
			if !tt.Focused("Target") {
				t.Fatal("Tab never reached the control")
			}
			if !controlsRingAround(tt, r, off) {
				t.Error("no ring while the keyboard has the focus")
			}
		})
	}
}

func TestSliderThumbRingShowsOnKeyboardFocus(t *testing.T) {
	v := 50.0
	tt := controlsTester(t, 300, 100, func(c *ui.Context) {
		Slider(c, &v, 0, 100, SliderOpts{A11yLabel: "Volume", Width: 200})
	})
	r := controlsFind(t, tt, "Volume")
	x, y := int(r.X+6+(r.W-12)*0.5), int(r.Y+r.H/2)
	probe := func() [3]uint8 { tt.Frame(); p := tt.Image().RGBAAt(x+8, y); return [3]uint8{p.R, p.G, p.B} } // 2px past the thumb: inside the 4px ring
	rest := probe()
	tt.Key(0, ui.KeyTab)
	focused := probe()
	if rest == focused {
		t.Errorf("no ring beside the thumb after Tab: %v", rest)
	}
	ring := theme.C("ring").Over(theme.C("ground"))
	if got := (rgb(ring)); focused != [3]uint8{got.R, got.G, got.B} && focused == rest {
		t.Errorf("ring colour %v, want %v", focused, got)
	}
}

func TestSelectOpensWithSpaceAndArrow(t *testing.T) {
	v := "free"
	tt := controlsTester(t, 360, 360, func(c *ui.Context) {
		Select(c, &v, controlsPlanItems(), SelectOpts{A11yLabel: "Plan"})
	})
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Plan") {
		t.Fatal("Tab did not focus the trigger")
	}
	tt.Key(0, ui.KeySpace)
	if !tt.HasText("Teams") {
		t.Errorf("Space did not open the list: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Key(0, ui.KeyDown)
	if !tt.HasText("Teams") {
		t.Errorf("Down did not open the list: %q", tt.Texts())
	}
}

func TestPlayButtonsTooltipIsPlay(t *testing.T) {
	tt := controlsTester(t, 400, 100, func(c *ui.Context) {
		ui.Row(c).Children(func() { PlayButtons(c, PlayButtonsOpts{Label: "OK Computer"}) })
	})
	r := controlsFind(t, tt, "Play OK Computer")
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(time.Second)
	found := false
	for _, s := range tt.Texts() {
		found = found || s == "Play"
	}
	if !found {
		t.Errorf("no \"Play\" tooltip: %q", tt.Texts())
	}
}

func TestLabelledButtonHeights(t *testing.T) {
	for _, tc := range []struct {
		size ButtonSize
		want float32
	}{{SizeXS, 24}, {SizeSM, 32}, {SizeMD, 36}, {SizeLG, 40}} {
		tt := controlsTester(t, 300, 100, func(c *ui.Context) {
			Button(c, "Label", ButtonOpts{Variant: Outline, Size: tc.size, A11yLabel: "Box"})
		})
		if r := controlsFind(t, tt, "Box"); r.H != tc.want || r.W < 40 {
			t.Errorf("size %d: labelled button %v, want %v high", tc.size, r, tc.want)
		}
	}
}

func TestScrollAreaTakesTheKeyboard(t *testing.T) {
	tt := controlsTester(t, 300, 200, func(c *ui.Context) {
		ScrollArea(c).Size(240, 100).Label("List").Children(func() {
			for i := 0; i < 40; i++ {
				ui.Text(c.Key(i), fmt.Sprintf("Row %d", i)).Height(24).Width(200)
			}
		})
	})
	first := controlsFind(t, tt, "Row 0")
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("List") {
		t.Fatal("Tab did not focus the scroll area")
	}
	tt.Key(0, ui.KeyPageDown)
	tt.Advance(time.Second)
	// A row scrolled out of the viewport is laid out with no box.
	if r, ok := tt.Find("Row 0"); ok && r.W != 0 && r.Y >= first.Y {
		t.Errorf("Page Down did not scroll: Row 0 still at %v", r)
	}
}
