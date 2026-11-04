package kit

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/theme"
)

type dialogApp struct {
	open    bool
	opts    DialogOpts
	clicked int
	behind  int
	footer  bool // the footer carries its own Close button
}

func (a *dialogApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(20).Gap(12).AlignItems(ui.Start).Children(func() {
		if Button(c.Key("opener"), "Rename…", ButtonOpts{}).Clicked() {
			a.open = true
		}
		if Button(c.Key("behind"), "Behind", ButtonOpts{}).Clicked() {
			a.behind++
		}
	})
	Dialog(c, &a.open, a.opts, func() {
		if Button(c.Key("inside"), "Save", ButtonOpts{}).Clicked() {
			a.clicked++
		}
		DialogFooter(c, &a.open, a.footer, func() {})
	})
}

func newDialogApp(t *testing.T, o DialogOpts) (*dialogApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	a := &dialogApp{opts: o}
	return a, ui.NewTester(a.view, 640, 480)
}

func TestDialogOpensAndEscapeClosesReturningFocus(t *testing.T) {
	a, tt := newDialogApp(t, DialogOpts{Title: "Rename playlist", Description: "Pick a new name."})
	tt.Key(0, ui.KeyTab)
	if !tt.Focused("Rename…") {
		t.Fatal("opener not focused")
	}
	tt.Key(0, ui.KeyEnter)
	if !a.open || !tt.HasText("Rename playlist") || !tt.HasText("Pick a new name.") {
		t.Fatal("dialog did not open with its header")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(300 * time.Millisecond)
	if a.open {
		t.Fatal("Escape did not close the dialog")
	}
	if !tt.Focused("Rename…") {
		t.Error("focus did not return to the opener")
	}
}

func TestTakeDialogsBuiltCountsOpenDialogsPerFrame(t *testing.T) {
	a, tt := newDialogApp(t, DialogOpts{Title: "Rename"})
	TakeDialogsBuilt()
	tt.Frame()
	if n := TakeDialogsBuilt(); n != 0 {
		t.Errorf("a closed dialog counted %d", n)
	}
	a.open = true
	tt.Frame()
	if n := TakeDialogsBuilt(); n != 1 {
		t.Errorf("an open dialog counted %d, want 1", n)
	}
	if n := TakeDialogsBuilt(); n != 0 {
		t.Errorf("the count was not reset: %d", n)
	}
}

func TestDialogCloseButtonCloses(t *testing.T) {
	a, tt := newDialogApp(t, DialogOpts{Title: "Rename"})
	a.open = true
	tt.Frame()
	if err := tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	if a.open {
		t.Error("the X did not close the dialog")
	}
}

func TestDialogFooterCloseCloses(t *testing.T) {
	a, tt := newDialogApp(t, DialogOpts{Title: "Rename", HideClose: true})
	a.footer, a.open = true, true
	tt.Frame()
	if err := tt.Click("Close"); err != nil {
		t.Fatal(err)
	}
	if a.open {
		t.Error("the footer's Close did not close the dialog")
	}
}

func TestDialogHideCloseDropsTheX(t *testing.T) {
	a, tt := newDialogApp(t, DialogOpts{Title: "Rename", HideClose: true})
	a.open = true
	tt.Frame()
	if _, ok := tt.Find("Close"); ok {
		t.Error("HideClose left a Close button")
	}
}

func TestDialogScrimClickCloses(t *testing.T) {
	a, tt := newDialogApp(t, DialogOpts{Title: "Rename"})
	a.open = true
	tt.Frame()
	tt.ClickAt(5, 5)
	if a.open {
		t.Error("a click on the scrim did not close the dialog")
	}
}

func TestDialogPageBehindIsInert(t *testing.T) {
	a, tt := newDialogApp(t, DialogOpts{Title: "Rename"})
	a.open = true
	tt.Frame()
	for range 4 {
		tt.Key(0, ui.KeyTab)
		if tt.Focused("Behind") || tt.Focused("Rename…") {
			t.Fatal("Tab left the dialog")
		}
	}
	_ = tt.Click("Behind")
	if a.behind != 0 {
		t.Error("the page behind a dialog took a click")
	}
}

func TestDialogContentWorks(t *testing.T) {
	a, tt := newDialogApp(t, DialogOpts{Title: "Rename"})
	a.open = true
	tt.Frame()
	if err := tt.Click("Save"); err != nil {
		t.Fatal(err)
	}
	if a.clicked != 1 || !a.open {
		t.Errorf("clicked %d, open %v", a.clicked, a.open)
	}
}

// confirmApp mounts the confirm host and asks on a button.
type confirmApp struct {
	answers []bool
	o       ConfirmOptions
}

func (a *confirmApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(20).Children(func() {
		if Button(c.Key("ask"), "Delete…", ButtonOpts{}).Clicked() {
			Confirm(a.o, func(ok bool) { a.answers = append(a.answers, ok) })
		}
	})
	ConfirmHost(c)
}

func newConfirmApp(t *testing.T) (*confirmApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem); pendingConfirm = nil })
	a := &confirmApp{o: ConfirmOptions{
		Title: "Delete “OK Computer” from your library?", Body: "It also leaves any playlists it's in.",
		ConfirmLabel: "Delete", Destructive: true,
	}}
	return a, ui.NewTester(a.view, 640, 480)
}

func TestConfirmShowsQuestionFocusesConfirmAndSettlesTrue(t *testing.T) {
	a, tt := newConfirmApp(t)
	_ = tt.Click("Delete…")
	for _, s := range []string{a.o.Title, a.o.Body, "Cancel", "Delete"} {
		if !tt.HasText(s) {
			t.Fatalf("%q missing", s)
		}
	}
	if _, ok := tt.Find("Close"); ok {
		t.Error("a confirm dialog has no X button")
	}
	if !tt.Focused("Delete") {
		t.Error("the confirm button is not focused")
	}
	tt.Key(0, ui.KeyEnter)
	if len(a.answers) != 1 || !a.answers[0] {
		t.Errorf("answers %v, want [true]", a.answers)
	}
	if ConfirmPending() {
		t.Error("the question is still pending")
	}
}

func TestConfirmCancelAndEscapeSettleFalse(t *testing.T) {
	a, tt := newConfirmApp(t)
	_ = tt.Click("Delete…")
	_ = tt.Click("Cancel")
	_ = tt.Click("Delete…")
	tt.Key(0, ui.KeyEscape)
	if len(a.answers) != 2 || a.answers[0] || a.answers[1] {
		t.Errorf("answers %v, want [false false]", a.answers)
	}
}

func TestConfirmAskingAgainSettlesThePendingOneFalse(t *testing.T) {
	var got []bool
	t.Cleanup(func() { pendingConfirm = nil })
	Confirm(ConfirmOptions{Title: "one"}, func(ok bool) { got = append(got, ok) })
	Confirm(ConfirmOptions{Title: "two"}, func(ok bool) { got = append(got, ok) })
	if len(got) != 1 || got[0] {
		t.Fatalf("got %v, want the first settled false", got)
	}
	settleConfirm(true)
	if len(got) != 2 || !got[1] {
		t.Errorf("got %v", got)
	}
}

// toastApp mounts the toaster.
type toastApp struct{ show func() }

func (a *toastApp) view(c *ui.Context) {
	theme.Apply(c)
	ui.Column(c).Padding(20).Children(func() {
		if Button(c.Key("go"), "Notify", ButtonOpts{}).Clicked() && a.show != nil {
			a.show()
		}
	})
	Toaster(c)
}

func newToastApp(t *testing.T) (*toastApp, *ui.Tester) {
	t.Helper()
	theme.SetMode(theme.ModeLight)
	t.Cleanup(func() { theme.SetMode(theme.ModeSystem) })
	a := &toastApp{}
	return a, ui.NewTester(a.view, 640, 480)
}

func TestToastShowsAnnouncesAndExpires(t *testing.T) {
	a, tt := newToastApp(t)
	a.show = func() { ToastErrorf("Couldn't favourite “Airbag”", "Check your connection.") }
	_ = tt.Click("Notify")
	if !tt.HasText("Couldn't favourite “Airbag”") || !tt.HasText("Check your connection.") {
		t.Fatal("the toast is not shown")
	}
	if !tt.HasText("Notifications") {
		t.Error("the region is not named Notifications")
	}
	found := false
	for _, s := range tt.Announcements() {
		if s == "Couldn't favourite “Airbag”" {
			found = true
		}
	}
	if !found {
		t.Errorf("the toast was not announced: %v", tt.Announcements())
	}
	tt.Advance(10 * time.Second)
	if tt.HasText("Couldn't favourite “Airbag”") {
		t.Error("the toast stayed past its time")
	}
}

func TestUnavailableNeverStacks(t *testing.T) {
	a, tt := newToastApp(t)
	a.show = func() { Unavailable("Not available while listening to a live station") }
	for range 4 {
		_ = tt.Click("Notify")
	}
	n := 0
	for _, s := range tt.Texts() {
		if s == "Not available while listening to a live station" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d copies of the message on screen, want 1", n)
	}
}

func TestToastActionRunsAndCloses(t *testing.T) {
	a, tt := newToastApp(t)
	ran := 0
	a.show = func() { ShowToast(ToastSpec{ID: "undo", Title: "Removed", Action: "Undo", OnAction: func() { ran++ }}) }
	_ = tt.Click("Notify")
	if err := tt.Click("Undo"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if ran != 1 {
		t.Errorf("action ran %d times", ran)
	}
	if tt.HasText("Removed") {
		t.Error("the toast stayed after its action")
	}
}

func TestShowToastAsksForAFrame(t *testing.T) {
	var posts int
	SetToastPoster(func(fn func()) { posts++; fn() })
	t.Cleanup(func() { SetToastPoster(nil) })
	ShowToast(ToastSpec{ID: "p", Title: "x"})
	CloseToast("p")
	if posts != 2 {
		t.Errorf("%d frames asked for, want 2", posts)
	}
	toasts.mu.Lock()
	toasts.queue, toasts.closed = nil, nil
	toasts.mu.Unlock()
}
