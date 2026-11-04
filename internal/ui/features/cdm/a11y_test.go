package cdm

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/uitest"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/access"
)

// The player bar's Widevine line: a link to Settings where the page exists
// (named by its words, with the problem as its description, as the tooltip
// says it), plain text where it does not; Tab reaches only the link.
func TestA11yNoticeLinkAndText(t *testing.T) {
	failed := services.CDMStatus{State: "failed", Code: "network"}
	const words = "Couldn't get Widevine · Retry in Settings"

	r := newRig(t, true, uitest.Bare())
	r.emit(services.CDMStatus{State: "downloading"})
	access.NoNestedControls(t, r.tt.Tester)
	tr := access.Read(t, r.tt.Tester)
	if n, ok := tr.Find(ui.RoleLink, "Waiting for Widevine to download"); !ok || !n.Has(access.Focusable) {
		t.Errorf("download link: %+v\n%s", n, tr.Dump())
	}
	r.emit(failed)
	tr = access.Read(t, r.tt.Tester)
	n, ok := tr.Find(ui.RoleLink, words)
	if !ok || n.Description != i18n.CDMProblem("network") || n.Description == "" {
		t.Errorf("failure link %+v\n%s", n, tr.Dump())
	}
	stops := access.TabWalk(t, r.tt.Tester, 5)
	if len(stops) != 1 || stops[0].Node.Label != words {
		t.Errorf("Tab stops %v", stops)
	}

	// Without the page the line is text and nothing takes the focus.
	r = newRig(t, false, uitest.Bare())
	r.emit(services.CDMStatus{State: "ready"})
	tr = access.Read(t, r.tt.Tester)
	if len(tr.Roles(ui.RoleLink)) != 0 || !r.tt.HasText("Widevine is ready. Press play to start.") {
		t.Errorf("ready line:\n%s", tr.Dump())
	}
}
