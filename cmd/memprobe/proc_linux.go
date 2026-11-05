package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// tree is pid and all its descendants.
func tree(pid int) []int {
	seen := map[int]bool{}
	var out []int
	var walk func(int)
	walk = func(pid int) {
		if seen[pid] {
			return
		}
		seen[pid] = true
		out = append(out, pid)
		tasks, _ := filepath.Glob(filepath.Join("/proc", strconv.Itoa(pid), "task", "*", "children"))
		for _, t := range tasks {
			data, _ := os.ReadFile(t)
			for _, f := range strings.Fields(string(data)) {
				if child, err := strconv.Atoi(f); err == nil {
					walk(child)
				}
			}
		}
	}
	walk(pid)
	return out
}

func command(pid int) string {
	b, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	s := strings.ReplaceAll(strings.TrimRight(string(b), "\x00"), "\x00", " ")
	if len(s) > 90 {
		s = s[:90]
	}
	return s
}

type smaps struct {
	rss, pss, anon int
	byPath         map[string]int
	// regions are the mappings with the most anonymous memory (KiB).
	regions []region
}

// region is one mapping: its address range, size, anonymous memory and
// owner (empty for anonymous mappings).
type region struct {
	Range string `json:"range"`
	Size  int    `json:"size_kib"`
	Anon  int    `json:"anon_kib"`
	Name  string `json:"name,omitempty"`
}

// bigRegions is the n regions with the most anonymous memory, largest first.
func (m smaps) bigRegions(n int) []region {
	rs := slices.Clone(m.regions)
	sort.Slice(rs, func(i, j int) bool { return rs[i].Anon > rs[j].Anon })
	return rs[:min(n, len(rs))]
}

// top is the n mappings (grouped by file, anonymous ones together) with the
// most proportional memory.
func (m smaps) top(n int) map[string]int {
	type kv struct {
		k string
		v int
	}
	var all []kv
	for k, v := range m.byPath {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v > all[j].v })
	out := map[string]int{}
	for _, e := range slices.Clip(all[:min(n, len(all))]) {
		out[e.k] = e.v
	}
	return out
}

func readSmaps(pid int) (smaps, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/smaps", pid))
	if err != nil {
		return smaps{}, err
	}
	m := smaps{byPath: map[string]int{}}
	cur := ""
	idx := -1
	for _, l := range strings.Split(string(data), "\n") {
		f := strings.Fields(l)
		if len(f) == 0 {
			continue
		}
		if strings.Contains(f[0], "-") && !strings.HasSuffix(f[0], ":") {
			switch {
			case len(f) < 6:
				cur = "[anon]"
			case strings.HasPrefix(f[5], "["):
				cur = strings.Join(f[5:], " ") // [heap], [stack], [anon: Go: heap]
			default:
				cur = f[5]
				if strings.HasPrefix(cur, "/nix/store/") {
					if i := strings.Index(cur[len("/nix/store/"):], "/"); i >= 0 {
						cur = cur[len("/nix/store/")+i+1:] // the path inside the package
					}
				}
			}
			a, b, _ := strings.Cut(f[0], "-")
			lo, _ := strconv.ParseUint(a, 16, 64)
			hi, _ := strconv.ParseUint(b, 16, 64)
			name := ""
			if len(f) >= 6 {
				name = strings.Join(f[5:], " ")
			}
			m.regions = append(m.regions, region{Range: f[0], Size: int((hi - lo) >> 10), Name: name})
			idx = len(m.regions) - 1
			continue
		}
		v := 0
		if len(f) >= 2 {
			v, _ = strconv.Atoi(f[1])
		}
		switch f[0] {
		case "Rss:":
			m.rss += v
		case "Pss:":
			m.pss += v
			m.byPath[cur] += v
		case "Anonymous:":
			m.anon += v
			if idx >= 0 {
				m.regions[idx].Anon = v
			}
		}
	}
	return m, nil
}
