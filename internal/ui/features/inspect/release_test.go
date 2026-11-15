//go:build !dev

package inspect

import (
	"os/exec"
	"strings"
	"testing"
)

// A release build holds no inspector: its binary's dependencies leave out
// this package (the only code that registers the action and draws the panel)
// and InspectService's file (services/inspect_dev.go) is not compiled.
func TestReleaseBuildHoldsNoInspector(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", "../../../../cmd/sona").Output()
	if err != nil {
		t.Skipf("go list: %v", err)
	}
	if deps := string(out); strings.Contains(deps, "/features/inspect") {
		t.Errorf("cmd/sona depends on the inspector:\n%s", deps)
	}
}
