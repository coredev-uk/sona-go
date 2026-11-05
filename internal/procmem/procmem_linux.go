package procmem

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Of reads /proc/<pid>/smaps_rollup.
func Of(pid int) (Usage, error) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/smaps_rollup")
	if err != nil {
		return Usage{}, err
	}
	return parseRollup(string(b))
}

// parseRollup takes the Rss and Pss lines (in kB) of a smaps_rollup file.
func parseRollup(s string) (Usage, error) {
	var u Usage
	var found int
	for _, l := range strings.Split(s, "\n") {
		key, rest, ok := strings.Cut(l, ":")
		if !ok || (key != "Rss" && key != "Pss") {
			continue
		}
		var kb uint64
		if _, err := fmt.Sscanf(strings.TrimSpace(rest), "%d kB", &kb); err != nil {
			return Usage{}, fmt.Errorf("procmem: %q: %w", l, err)
		}
		if key == "Rss" {
			u.Resident = kb << 10
		} else {
			u.Footprint = kb << 10
		}
		found++
	}
	if found != 2 {
		return Usage{}, fmt.Errorf("procmem: smaps_rollup has no Rss and Pss")
	}
	return u, nil
}
