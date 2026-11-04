package probe

import (
	"slices"
	"testing"
	"time"
)

func TestParseTakesTheProbeFlagsOutOfTheArguments(t *testing.T) {
	o, rest, err := Parse([]string{"--hidden", "-probe", "-speed", "1500", "-runs=5", "sona://x", "-idle", "3s", "-max=9000", "-interval=16.67"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.Probe || o.Startup || !o.On() || o.Speed != 1500 || o.Runs != 5 || o.Idle != 3*time.Second || o.Max != 9000 || o.Interval != 16.67 {
		t.Errorf("options = %+v", o)
	}
	if want := []string{"--hidden", "sona://x"}; !slices.Equal(rest, want) {
		t.Errorf("rest = %v, want %v", rest, want)
	}
}

func TestParseDefaultsAndOff(t *testing.T) {
	o, rest, err := Parse([]string{"play"})
	if err != nil {
		t.Fatal(err)
	}
	if o.On() || o.Speed != 2000 || o.Max != 20000 || o.Runs != 3 || o.Idle != 10*time.Second || o.Interval != 0 || !slices.Equal(rest, []string{"play"}) {
		t.Errorf("options = %+v, rest %v", o, rest)
	}
	if o, _, _ := Parse([]string{"-startup"}); !o.On() || o.Probe {
		t.Errorf("-startup alone: %+v", o)
	}
	if o, _, _ := Parse([]string{"-probe=false"}); o.On() {
		t.Errorf("-probe=false: %+v", o)
	}
}

func TestParseRefusesBadValues(t *testing.T) {
	for _, args := range [][]string{{"-speed", "abc"}, {"-speed", "0"}, {"-runs", "0"}, {"-idle", "x"}, {"-max"}, {"-probe=maybe"}} {
		if _, _, err := Parse(args); err == nil {
			t.Errorf("Parse(%v) succeeded", args)
		}
	}
}
