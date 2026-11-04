package video

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

func brief(tr access.Tree) string {
	var b strings.Builder
	depth := make([]int, len(tr.Nodes))
	for i, n := range tr.Nodes {
		if n.Parent >= 0 {
			depth[i] = depth[n.Parent] + 1
		}
		if n.Has(access.Offscreen) || n.Role == ui.RoleCell {
			continue
		}
		fmt.Fprintf(&b, "%s%s", strings.Repeat(" ", depth[i]), n)
		if n.States != 0 {
			fmt.Fprintf(&b, " %s", n.States)
		}
		if n.Level != 0 {
			fmt.Fprintf(&b, " L%d", n.Level)
		}
		if n.Description != "" {
			fmt.Fprintf(&b, " desc=%q", n.Description)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestZZDump(t *testing.T) {
	p := newPageRig(t, false, 1132, 1253)
	p.loaded()
	p.tt.Settle()
	access.NoNestedControls(t, p.tt.Tester)
	tr := access.Read(t, p.tt.Tester)
	t.Log("\n" + brief(tr))
	var s []string
	for _, st := range access.TabWalk(t, p.tt.Tester, 100) {
		s = append(s, st.Node.String())
	}
	t.Log("stops ", strings.Join(s, " | "))
}

func TestZZView(t *testing.T) {
	r := viewRig(t)
	r.play("playing")
	r.tt.Frame()
	r.tt.SetFocused(true)
	r.tt.Key(0, ui.KeyTab)
	t.Logf("opener focus: %v", r.tt.Focused("Page behind"))
	r.env.Video.Open()
	r.tt.Frame()
	r.tt.Until("the view", r.tt.Shows("Sky Video", "Mia Rivers"))
	r.tt.Advance(fadeIn)
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)
	t.Log("\n" + brief(tr))
	t.Logf("focus %v", tr.Focus)
	var s []string
	for _, st := range access.TabWalk(t, r.tt.Tester, 30) {
		s = append(s, st.Node.String())
	}
	t.Log("stops ", strings.Join(s, " | "))
	r.tt.Key(0, ui.KeyEscape)
	r.tt.Drain()
	tr = access.Read(t, r.tt.Tester)
	t.Logf("after Escape focus %v, page behind focused %v", tr.Focus, r.tt.Focused("Page behind"))
}
