package search

import (
	"context"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
)

// failingRecent is the search service whose recent-search writes fail.
type failingRecent struct {
	searchAPI
	err error
}

func (f failingRecent) ForgetRecent(context.Context, string) error { return f.err }
func (f failingRecent) ClearRecent(context.Context) error          { return f.err }

func TestRecentRemoveFailureToastsAndKeepsTheEntry(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{wrap: func(api searchAPI) searchAPI {
		return failingRecent{api, context.DeadlineExceeded}
	}})
	seedRecent(t, r)
	r.s.refreshRecent()
	r.tt.Until("the entries", r.tt.Shows("alpha", "Blue Band"))
	rect, _ := r.tt.Find("alpha")
	r.tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	r.tt.Frame()
	r.click("Remove alpha from recent searches")
	r.tt.Until("the toast", r.tt.Shows("Couldn't remove the search", i18n.ErrorText(services.CodeTimeout)))
	if !r.tt.HasText("alpha") {
		t.Errorf("the entry went although the service refused: %q", r.tt.Texts())
	}
	if !slices.Contains(r.recentKinds(), "term:catalog:alpha") {
		t.Error("the service forgot alpha although it failed")
	}
}

func TestRecentClearFailureToastsAndKeepsTheShelf(t *testing.T) {
	r := newPageRig(t, "/search", rigOpts{wrap: func(api searchAPI) searchAPI {
		return failingRecent{api, context.DeadlineExceeded}
	}})
	seedRecent(t, r)
	r.s.refreshRecent()
	r.tt.Until("the entries", r.tt.Shows("Clear", "alpha"))
	r.click("Clear")
	r.tt.Until("the toast", r.tt.Shows("Couldn't clear recent searches", i18n.ErrorText(services.CodeTimeout)))
	if !r.tt.HasText("Recently searched") || !r.tt.HasText("alpha") {
		t.Errorf("Clear emptied the shelf although the service refused: %q", r.tt.Texts())
	}
}

// registerArtistAction gives artists a menu action that records the item it
// ran on.
func registerArtistAction(r *pageRig) *[]string {
	var ran []string
	r.h.Env.Actions.Register(kit.ActionDef{
		ID: "test.artist", Target: kit.KindArtist, Group: kit.GroupItem,
		Label: func(*kit.ActionContext) string { return "Do something" },
		Run:   func(x *kit.ActionContext) { ran = append(ran, x.Item().Name()) },
	})
	return &ran
}

// A result row of a kind with actions has the item menu: its "More for"
// button (shown on hover) opens it, and choosing runs the action on that item
// without opening the page.
func TestResultRowOpensItsItemMenuAndRunsAnAction(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	ran := registerArtistAction(r)
	r.tt.Until("the results", r.tt.Shows("Top results", "Album · Blue Band"))

	rect, ok := r.tt.Find("Blue Band")
	if !ok {
		t.Fatalf("no Blue Band row: %q", r.tt.Texts())
	}
	r.tt.Move(rect.X+rect.W/2, rect.Y+rect.H/2)
	r.tt.Frame()
	r.click("More for Blue Band")
	r.tt.Until("the button's menu", r.tt.Shows("Do something"))
	r.click("Do something")
	r.tt.Until("the action", func() bool { return len(*ran) == 1 })
	if (*ran)[0] != "Blue Band" {
		t.Errorf("the action ran on %q", *ran)
	}
	if r.tt.HasText("Do something") {
		t.Error("the menu stayed open after choosing")
	}
	if got := r.loc(); got != "/search?q=blue" {
		t.Errorf("the menu opened a page: %q", got)
	}
}

// Shift+F10 on the focused row opens the same menu, on its first row.
func TestResultRowOpensItsItemMenuFromTheKeyboard(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	ran := registerArtistAction(r)
	r.tt.Until("the results", r.tt.Shows("Top results", "Album · Blue Band"))
	for range 30 {
		if r.tt.Focused("Blue Band") {
			break
		}
		r.tt.Key(0, ui.KeyTab)
	}
	if !r.tt.Focused("Blue Band") {
		t.Fatal("Tab never reached the Blue Band row")
	}
	r.tt.Key(ui.Shift, ui.KeyF10)
	r.tt.Until("the keyboard menu", r.tt.Shows("Do something"))
	r.tt.Key(0, ui.KeyEnter)
	r.tt.Until("the action", func() bool { return len(*ran) == 1 })
	if got := r.loc(); got != "/search?q=blue" {
		t.Errorf("the menu opened a page: %q", got)
	}
}
