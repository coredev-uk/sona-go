package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

const deviceHelp = "PipeWire sink (node.name) to play to (default: the system default)"

// audioStream prints the child's PipeWire output stream as pw-dump
// reports it.
func audioStream(pid int) {
	out, err := exec.Command("pw-dump").Output()
	if err != nil {
		step("pipewire", "err", err)
		return
	}
	var objs []struct {
		ID   int    `json:"id"`
		Type string `json:"type"`
		Info struct {
			State string         `json:"state"`
			Props map[string]any `json:"props"`
		} `json:"info"`
	}
	if err := json.Unmarshal(out, &objs); err != nil {
		step("pipewire", "err", err)
		return
	}
	// Nodes name their client; the client carries the process ID.
	clients := map[string]bool{}
	for _, o := range objs {
		if o.Type == "PipeWire:Interface:Client" && fmt.Sprint(o.Info.Props["application.process.id"]) == strconv.Itoa(pid) {
			clients[strconv.Itoa(o.ID)] = true
		}
	}
	found := false
	for _, o := range objs {
		p := o.Info.Props
		if o.Type != "PipeWire:Interface:Node" || !clients[fmt.Sprint(p["client.id"])] {
			continue
		}
		found = true
		step("pipewire stream", "node", o.ID, "state", o.Info.State, "media.class", fmt.Sprintf("%q", p["media.class"]),
			"media.role", fmt.Sprintf("%q", p["media.role"]), "application.name", fmt.Sprintf("%q", p["application.name"]),
			"client", p["client.id"])
	}
	if !found {
		step("pipewire stream", "err", fmt.Sprintf("no node for pid %d", pid))
	}
}

// memory prints the proportional set size of this process and the host.
func memory(label string, child int) {
	parent := pss(os.Getpid())
	host := pss(child)
	step(label, "parent_pss", fmtKiB(parent), "host_pss", fmtKiB(host), "total", fmtKiB(parent+host))
}

func fmtKiB(k int) string { return fmt.Sprintf("%.1fMiB", float64(k)/1024) }

func pss(pid int) int {
	f, err := os.Open(fmt.Sprintf("/proc/%d/smaps_rollup", pid))
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "Pss:"); ok {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "kB")))
			return n
		}
	}
	return 0
}
