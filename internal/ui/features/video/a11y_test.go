package video

import (
	"errors"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// walkLabels is the names of the Tab stops, in order.
func walkLabels(t *testing.T, tt *uitest.Session, limit int) []string {
	t.Helper()
	var out []string
	for _, s := range access.TabWalk(t, tt.Tester, limit) {
		out = append(out, s.Node.Label)
	}
	return out
}

type a11yNode struct {
	role  ui.Role
	label string
}

// TestA11yPage is the audit of the music video page and its states: no
// control nested in a button or a link, headings, lists and links named,
// Tab reaching the controls in reading order and never trapping.
func TestA11yPage(t *testing.T) {
	cases := []struct {
		name     string
		build    func(t *testing.T) *pageRig
		ready    []string
		headings map[string]int
		present  []a11yNode
		absent   []a11yNode
		tab      []string
	}{
		{
			name: "loaded",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, false, 1132, 1253)
			},
			ready:    []string{"Sky Video", "Notes", "More like this"},
			headings: map[string]int{"Sky Video": 1, "Notes": 2, "More by Mia Rivers": 2, "More like this": 2},
			present: []a11yNode{
				{ui.RoleLink, "Mia Rivers"}, {ui.RoleButton, "Play Sky Video"}, {ui.RoleButton, "Watch in Apple Music"},
				{ui.RoleButton, "Play the song"}, {ui.RoleMenuButton, "More for Sky Video"}, {ui.RoleLink, "Sky"},
				{ui.RoleLink, "See all"}, {ui.RoleTable, "More by Mia Rivers"}, {ui.RoleTable, "More like this"},
				{ui.RoleLink, "Rain Video"}, {ui.RoleButton, "Play Rain Video"}, {ui.RoleMenuButton, "More for Rain Video"},
				{ui.RoleLink, "Wind Video"}, {ui.RoleButton, "Play Wind Video"},
			},
			tab: []string{"Mia Rivers", "Play Sky Video", "Watch in Apple Music", "Play the song", "More for Sky Video", "Sky",
				"See all", "Rain Video", "Play Rain Video", "Snow Video", "Play Snow Video", "Wind Video", "Play Wind Video"},
		},
		{
			name: "videos do not play here",
			build: func(t *testing.T) *pageRig {
				p := newPageRig(t, false, 1132, 1253)
				p.caps.Video = false
				return p
			},
			ready:    []string{"Sky Video", "Notes", "Sona can't play music videos here: the video opens in Apple Music in your browser."},
			headings: map[string]int{"Sky Video": 1},
			present:  []a11yNode{{ui.RoleButton, "Watch in Apple Music"}, {ui.RoleLink, "Rain Video"}},
			absent: []a11yNode{
				{ui.RoleButton, "Play Sky Video"}, {ui.RoleMenuButton, "More for Sky Video"}, {ui.RoleButton, "Play Rain Video"},
			},
			tab: []string{"Mia Rivers", "Watch in Apple Music", "Play the song", "Sky", "See all", "Rain Video", "Snow Video", "Wind Video"},
		},
		{
			name: "error",
			build: func(t *testing.T) *pageRig {
				return newPageRig(t, false, 1132, 900, func(f *fakeCat) { f.err = errors.New("unreadable") })
			},
			ready:   []string{"Try again"},
			present: []a11yNode{{ui.RoleButton, "Try again"}},
			tab:     []string{"Try again"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.build(t)
			p.tt.Until("the page", p.tt.Shows(tc.ready...))
			p.tt.Settle()
			access.NoNestedControls(t, p.tt.Tester)
			tr := access.Read(t, p.tt.Tester)
			for label, level := range tc.headings {
				n, ok := tr.Find(ui.RoleHeading, label)
				switch {
				case !ok:
					t.Errorf("no heading %q:\n%s", label, tr.Dump())
				case n.Level != level:
					t.Errorf("heading %q is level %d, want %d", label, n.Level, level)
				}
			}
			for _, n := range tc.present {
				if _, ok := tr.Find(n.role, n.label); !ok {
					t.Errorf("the tree has no role %d %q:\n%s", n.role, n.label, tr.Dump())
				}
			}
			for _, n := range tc.absent {
				if _, ok := tr.Find(n.role, n.label); ok {
					t.Errorf("the tree has role %d %q", n.role, n.label)
				}
			}
			inOrder(t, walkLabels(t, p.tt, 100), tc.tab...)
		})
	}
}

// TestA11yPageLoadingAndErrorAreStatuses: the page that is still reading is
// a labelled status, and one that failed is a status read out once.
func TestA11yPageLoadingAndErrorAreStatuses(t *testing.T) {
	p := newPageRig(t, false, 1132, 900)
	p.tt.Frame()
	if _, ok := access.Read(t, p.tt.Tester).Find(ui.RoleStatus, "Loading"); !ok {
		t.Errorf("no Loading status while the page reads:\n%s", access.Read(t, p.tt.Tester).Dump())
	}

	f := newPageRig(t, false, 1132, 900, func(c *fakeCat) { c.err = errors.New("unreadable") })
	var heard []string
	f.tt.Until("the error", func() bool {
		heard = append(heard, f.tt.Announcements()...)
		return f.tt.HasText("Try again")
	})
	heard = append(heard, f.tt.Announcements()...)
	if len(heard) != 1 || !strings.HasPrefix(heard[0], "Couldn't load this music video") {
		t.Errorf("announced %q, want the error once", heard)
	}
	tr := access.Read(t, f.tt.Tester)
	if _, ok := tr.Find(ui.RoleStatus, "Couldn't load this music video"); !ok {
		t.Errorf("the error is not a status:\n%s", tr.Dump())
	}
}
