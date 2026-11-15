package main

import "testing"

func TestIsTitleCase(t *testing.T) {
	for s, want := range map[string]bool{
		"Made for You": true, "Top Picks for You": true, "Songs in Your Library": true,
		"Singles and EPs in Your Library": true, "Top 25 Songs by City": true, "For You": true,
		"Delete “{name}” From Your Library?": true, "Finish Signing In in Your Browser": true,
		"Albums": true, "Sona Mini Player": true, "On This Computer": true,
		"Made for you": false, "Songs in your library": false, "Recently added": false,
		"the Queue": false, "Music videos": false, "Top 25 songs by city": false,
		"Delete {what} from your library?": false, "Of Montreal": true, "Welcome Home to": false,
	} {
		if got := isTitleCase(s); got != want {
			t.Errorf("isTitleCase(%q) = %v, want %v", s, got, want)
		}
	}
}

func TestCheckTitlesFindsOnlyTitles(t *testing.T) {
	r := writeTree(t, map[string]string{"internal/ui/features/page/page.go": `package page

import "github.com/coredev-uk/sona-go/internal/ui/i18n"

func view() {
	kit.PageHeader(c, i18n.T("Recently added"), i18n.T("Songs you added lately"), nil)
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("Made for You")}, nil)
	kit.Shelf(c, kit.ShelfOpts{Title: i18n.T("More for you")}, nil)
	_ = module.Route{Title: func(*ui.Route) string { return i18n.T("Top picks") }}
	kit.Button(c, i18n.T("Add to library"), o)
	kit.Confirm(kit.ConfirmOptions{Title: i18n.T("Delete the folder?"), ConfirmLabel: i18n.T("Delete folder")}, nil)
}
`})
	got, err := checkTitles(string(r))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`internal/ui/features/page/page.go:6: "Recently added"`,
		`internal/ui/features/page/page.go:8: "More for you"`,
		`internal/ui/features/page/page.go:9: "Top picks"`,
		`internal/ui/features/page/page.go:11: "Delete the folder?"`,
	}
	if len(got) != len(want) {
		t.Fatalf("flagged %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("flagged[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// Every title of the interface is in Title Case (docs/design.md, "Wording
// and case"). To cover a new heading helper, add it to titleSinks.
func TestUITitlesAreTitleCase(t *testing.T) {
	bad, err := checkTitles("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bad {
		t.Errorf("not in Title Case: %s", b)
	}
}
