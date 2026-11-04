package search

import "testing"

// A right-click on the body of a result row opens the row's item menu (the
// row's open button takes the press, so it must pass it on), and choosing
// runs the action on that item without opening the page.
func TestRightClickOnAResultRowOpensItsItemMenu(t *testing.T) {
	r := newPageRig(t, "/search?q=blue", rigOpts{signedIn: true})
	ran := registerArtistAction(r)
	r.tt.Until("the results", r.tt.Shows("Top results", "Album · Blue Band"))
	if err := r.tt.RightClick("Blue Band"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the row's menu", r.tt.Shows("Do something"))
	r.click("Do something")
	r.tt.Until("the action", func() bool { return len(*ran) == 1 })
	if (*ran)[0] != "Blue Band" {
		t.Errorf("the action ran on %q", *ran)
	}
	if got := r.loc(); got != "/search?q=blue" {
		t.Errorf("the menu opened a page: %q", got)
	}
}
