package measure

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Usage is what a process and its children have used so far.
type Usage struct {
	// CPU is the CPU time (user and system) the tree has used.
	CPU time.Duration
	// RSS and PSS are the tree's resident memory, KiB (macOS has no
	// proportional figure here: PSS is RSS).
	RSS, PSS int
	// Procs is the number of processes in the tree.
	Procs int
	// OK is false where ps fails or the pid is unknown.
	OK bool
}

// TreeUsage reads `ps` for pid and all its descendants. ps counts CPU time
// in hundredths of a second.
func TreeUsage(pid int) Usage {
	var u Usage
	if pid <= 0 {
		return u
	}
	out, err := exec.Command("ps", "-axo", "pid=,ppid=,time=,rss=").Output()
	if err != nil {
		return u
	}
	type proc struct {
		cpu time.Duration
		rss int
	}
	procs := map[int]proc{}
	kids := map[int][]int{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 4 {
			continue
		}
		p, e1 := strconv.Atoi(f[0])
		pp, e2 := strconv.Atoi(f[1])
		rss, e3 := strconv.Atoi(f[3])
		cpu, ok := psTime(f[2])
		if e1 != nil || e2 != nil || e3 != nil || !ok {
			continue
		}
		procs[p] = proc{cpu, rss}
		kids[pp] = append(kids[pp], p)
	}
	var walk func(int)
	walk = func(p int) {
		pr, ok := procs[p]
		if !ok {
			return
		}
		u.OK = true
		u.Procs++
		u.CPU += pr.cpu
		u.RSS += pr.rss
		u.PSS += pr.rss
		for _, k := range kids[p] {
			walk(k)
		}
	}
	walk(pid)
	return u
}

// psTime parses ps's TIME column: [[dd-]hh:]mm:ss.cc.
func psTime(s string) (time.Duration, bool) {
	var days int
	if d, rest, ok := strings.Cut(s, "-"); ok {
		n, err := strconv.Atoi(d)
		if err != nil {
			return 0, false
		}
		days, s = n, rest
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	secs, err := strconv.ParseFloat(parts[len(parts)-1], 64)
	if err != nil {
		return 0, false
	}
	total := time.Duration(secs * float64(time.Second))
	mult := time.Minute
	for i := len(parts) - 2; i >= 0; i-- {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return 0, false
		}
		total += time.Duration(n) * mult
		mult *= 60
	}
	return total + time.Duration(days)*24*time.Hour, true
}
