package measure

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Usage is what a process and its children have used so far.
type Usage struct {
	// CPU is the CPU time (user and system) the tree has used.
	CPU time.Duration
	// RSS and PSS are the tree's resident and proportional memory, KiB.
	RSS, PSS int
	// Procs is the number of processes in the tree.
	Procs int
	// OK is false where /proc is unavailable or the pid unknown.
	OK bool
}

// clockTick is Linux's USER_HZ, which /proc/<pid>/stat counts CPU time in
// (100 on every architecture Sona runs on).
const clockTick = 100

// TreeUsage reads /proc for pid and all its descendants (the CDM host
// child while a song plays, say).
func TreeUsage(pid int) Usage {
	var u Usage
	if pid <= 0 {
		return u
	}
	seen := map[int]bool{}
	var walk func(pid int)
	walk = func(pid int) {
		if seen[pid] {
			return
		}
		seen[pid] = true
		cpu, ok := cpuTime(pid)
		if !ok {
			return
		}
		u.OK = true
		u.Procs++
		u.CPU += cpu
		rss, pss := memory(pid)
		u.RSS += rss
		u.PSS += pss
		tasks, _ := filepath.Glob(filepath.Join("/proc", strconv.Itoa(pid), "task", "*", "children"))
		for _, t := range tasks {
			data, err := os.ReadFile(t)
			if err != nil {
				continue
			}
			for _, f := range strings.Fields(string(data)) {
				if child, err := strconv.Atoi(f); err == nil {
					walk(child)
				}
			}
		}
	}
	walk(pid)
	return u
}

// cpuTime is the user and system CPU time of a process, from its stat file
// (fields after the command name, which may hold spaces and parentheses).
func cpuTime(pid int) (time.Duration, bool) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, false
	}
	s := string(data)
	i := strings.LastIndexByte(s, ')')
	if i < 0 {
		return 0, false
	}
	f := strings.Fields(s[i+1:])
	// f[0] is the state (field 3): utime is field 14, stime 15.
	if len(f) < 13 {
		return 0, false
	}
	ut, err1 := strconv.ParseInt(f[11], 10, 64)
	st, err2 := strconv.ParseInt(f[12], 10, 64)
	if err1 != nil || err2 != nil {
		return 0, false
	}
	return time.Duration(ut+st) * time.Second / clockTick, true
}

// memory is a process's RSS and PSS in KiB (smaps_rollup).
func memory(pid int) (rss, pss int) {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "smaps_rollup"))
	if err != nil {
		return 0, 0
	}
	for _, l := range strings.Split(string(data), "\n") {
		var v int
		if _, err := fmt.Sscanf(l, "Rss: %d kB", &v); err == nil {
			rss = v
		}
		if _, err := fmt.Sscanf(l, "Pss: %d kB", &v); err == nil {
			pss = v
		}
	}
	return rss, pss
}
