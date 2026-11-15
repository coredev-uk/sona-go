package services

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// docs/addons.md states how the supervisor runs an add-on; the numbers are
// the code's.
func TestAddonsDocStatesTheSupervisorsTimings(t *testing.T) {
	b, err := os.ReadFile("../../docs/addons.md")
	if err != nil {
		t.Fatal(err)
	}
	o := defaultAddonOptions()
	var secs []string
	for _, d := range o.backoff {
		secs = append(secs, fmt.Sprint(int(d.Seconds())))
	}
	list := strings.Join(secs[:len(secs)-1], ", ") + " and then " + secs[len(secs)-1] + " seconds"
	for _, want := range []string{
		"after " + list,
		fmt.Sprintf("After %d stops in a row, with no run of a minute", o.maxCrashes),
		fmt.Sprintf("A hello must arrive within %d seconds", int(o.helloTimeout.Seconds())),
		"gives it a second, asks it to terminate, gives it a second more",
	} {
		if !strings.Contains(strings.ReplaceAll(string(b), "**", ""), want) && !strings.Contains(string(b), want) {
			t.Errorf("docs/addons.md lacks %q", want)
		}
	}
	if o.stable.Seconds() != 60 || o.grace.Seconds() != 1 {
		t.Errorf("stable %v, grace %v: update the page", o.stable, o.grace)
	}
}
