package procmem

import (
	"os"
	"testing"
)

func TestParseRollup(t *testing.T) {
	const rollup = "5584d4a00000-7ffd5f7ff000 ---p 00000000 00:00 0                          [rollup]\n" +
		"Rss:              130164 kB\nPss:               84192 kB\nPss_Dirty:         60616 kB\nShared_Clean:      70040 kB\n"
	u, err := parseRollup(rollup)
	if err != nil {
		t.Fatal(err)
	}
	if u.Resident != 130164<<10 || u.Footprint != 84192<<10 {
		t.Errorf("%+v, want 130164 kB resident and 84192 kB proportional", u)
	}
	if _, err := parseRollup("Rss: 5 kB\n"); err == nil {
		t.Error("a file without a Pss line was accepted")
	}
}

func TestOfReadsThisProcess(t *testing.T) {
	u, err := Of(os.Getpid())
	if err != nil {
		t.Skip("no smaps_rollup:", err) // a kernel without CONFIG_PROC_PAGE_MONITOR
	}
	if u.Resident == 0 || u.Footprint == 0 || u.Footprint > u.Resident {
		t.Errorf("%+v: a running process has pages, and its proportional share is at most its resident set", u)
	}
}
