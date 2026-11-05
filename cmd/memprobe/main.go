//go:build linux

// Command memprobe measures the memory of a running Sona probe build, the
// app's process and each child (the CDM host while a song plays), while a
// scripted sequence of steps drives the app through MyGo's automation
// socket:
//
//	memprobe -pid <app pid> -socket <MYGO_AUTOMATION> -dir <SONA_MEMPROFILE_DIR> \
//	    link:Discover wait:20s measure:discover press:"Play Perfecth" wait:12s measure:preview
//
// Steps: wait:<duration>; link:<label>, press:<label>, button:<label>
// toggle:<label>, tab:<label> (press an element through the accessibility tree); wheel:<dy> (scroll
// the page by dy DIPs); scroll:<n>x<dy> (n wheel steps of dy DIPs, 700 ms apart); measure:<name> (print the RSS and PSS of every
// process, what its mappings are made of, and the Go runtime's figures, and
// keep the heap profiles); dump (print the tree's labels); shot:<file>
// (capture the window).
//
// It needs a probe build started with SONA_MEMPROFILE_DIR=<dir> (the Go
// heap is written there on SIGUSR1, by the app and its child alike) and
// reads /proc, so it measures on Linux only.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/coredev-uk/sona-go/e2e/automation"
)

func main() {
	pid := flag.Int("pid", 0, "the app's process ID")
	socket := flag.String("socket", os.Getenv("MYGO_AUTOMATION"), "the app's automation socket")
	dir := flag.String("dir", os.Getenv("SONA_MEMPROFILE_DIR"), "where the app writes its heap profiles on SIGUSR1")
	out := flag.String("o", "", "keep the heap profiles of each measure here, under <name>/ (default: leave them in -dir)")
	flag.Parse()
	if *pid == 0 {
		fmt.Fprintln(os.Stderr, "usage: memprobe -pid PID [-socket PATH] [-dir DIR] step…")
		os.Exit(2)
	}
	// Without a socket only wait and measure work (a process with no window:
	// cmd/playspike).
	var c *automation.Client
	if *socket != "" {
		var err error
		if c, err = automation.Dial(*socket, 30*time.Second); err != nil {
			fatal(err)
		}
		if err := waitWindow(c); err != nil {
			fatal(err)
		}
	}
	for _, step := range flag.Args() {
		verb, arg, _ := strings.Cut(step, ":")
		fmt.Fprintf(os.Stderr, "-- %s\n", step)
		var err error
		switch verb {
		case "wait":
			var d time.Duration
			if d, err = time.ParseDuration(arg); err == nil {
				time.Sleep(d)
			}
		case "link":
			err = c.PressRole(20*time.Second, "link", arg)
		case "button":
			err = c.PressRole(20*time.Second, "button", arg)
		case "toggle":
			err = c.PressRole(20*time.Second, "toggleButton", arg)
		case "tab":
			err = c.PressRole(20*time.Second, "tab", arg)
		case "press":
			err = c.Press(20*time.Second, arg)
		case "wheel":
			var dy float64
			if dy, err = strconv.ParseFloat(arg, 64); err == nil {
				err = c.Input(automation.Event{"type": "wheel", "x": 600.0, "y": 600.0, "dx": 0.0, "dy": dy, "precise": true})
			}
		case "scroll":
			// scroll:<steps>x<dy>: that many wheel steps of dy DIPs, 700 ms apart
			var n int
			var dy float64
			if _, err = fmt.Sscanf(arg, "%dx%f", &n, &dy); err == nil {
				for range n {
					if err = c.Input(automation.Event{"type": "wheel", "x": 600.0, "y": 600.0, "dx": 0.0, "dy": dy, "precise": true}); err != nil {
						break
					}
					time.Sleep(700 * time.Millisecond)
				}
			}
		case "measure":
			err = measure(*pid, arg, *dir, *out)
		case "dump":
			err = dump(c)
		case "shot":
			err = shot(c, arg)
		default:
			err = fmt.Errorf("unknown step %q", step)
		}
		if err != nil {
			fatal(fmt.Errorf("%s: %w", step, err))
		}
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "memprobe:", err)
	os.Exit(1)
}

func waitWindow(c *automation.Client) error {
	for end := time.Now().Add(60 * time.Second); time.Now().Before(end); time.Sleep(200 * time.Millisecond) {
		if ws, err := c.Windows(); err == nil && len(ws) > 0 && ws[0].Visible {
			return nil
		}
	}
	return fmt.Errorf("no window")
}

func dump(c *automation.Client) error {
	t, err := c.Tree()
	if err != nil {
		return err
	}
	for _, n := range t.Nodes {
		if n.Label != "" {
			fmt.Printf("%s %q %.0f,%.0f %.0fx%.0f\n", n.Role, n.Label, n.Bounds.X, n.Bounds.Y, n.Bounds.Width, n.Bounds.Height)
		}
	}
	return nil
}

func shot(c *automation.Client, file string) error {
	_, png, err := c.Capture()
	if err != nil {
		return err
	}
	return os.WriteFile(file, png, 0o644)
}

// measure signals every process of the tree to write its heap profile,
// then prints each one's memory.
func measure(root int, name, dir, keep string) error {
	pids := tree(root)
	for _, p := range pids {
		_ = syscall.Kill(p, syscall.SIGUSR1)
	}
	time.Sleep(1500 * time.Millisecond)
	type row struct {
		Name    string         `json:"name"`
		PID     int            `json:"pid"`
		Command string         `json:"command"`
		RSSKiB  int            `json:"rss_kib"`
		PSSKiB  int            `json:"pss_kib"`
		AnonKiB int            `json:"anon_kib"`
		Maps    map[string]int `json:"pss_by_mapping_kib"`
		Regions []region       `json:"biggest_anon_regions,omitempty"`
		Go      map[string]any `json:"go,omitempty"`
	}
	var total row
	total.Name = name
	for _, p := range pids {
		m, err := readSmaps(p)
		if err != nil {
			continue
		}
		r := row{Name: name, PID: p, Command: command(p), RSSKiB: m.rss, PSSKiB: m.pss, AnonKiB: m.anon, Maps: m.top(8), Regions: m.bigRegions(6)}
		if dir != "" {
			if b, err := os.ReadFile(fmt.Sprintf("%s/%d.memstats.json", dir, p)); err == nil {
				var ms map[string]any
				if json.Unmarshal(b, &ms) == nil {
					r.Go = map[string]any{}
					for _, k := range []string{"HeapAlloc", "HeapInuse", "HeapSys", "HeapIdle", "HeapReleased", "StackInuse", "Sys", "NumGC"} {
						r.Go[k] = ms[k]
					}
				}
			}
		}
		b, _ := json.Marshal(r)
		fmt.Println(string(b))
		total.RSSKiB += r.RSSKiB
		total.PSSKiB += r.PSSKiB
	}
	fmt.Printf("{\"name\":%q,\"total_rss_kib\":%d,\"total_pss_kib\":%d,\"procs\":%d}\n", name, total.RSSKiB, total.PSSKiB, len(pids))
	if keep != "" && dir != "" {
		dst := keep + "/" + name
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		for _, p := range pids {
			for _, ext := range []string{"heap.pb.gz", "memstats.json"} {
				if b, err := os.ReadFile(fmt.Sprintf("%s/%d.%s", dir, p, ext)); err == nil {
					_ = os.WriteFile(fmt.Sprintf("%s/%d.%s", dst, p, ext), b, 0o644)
				}
			}
		}
	}
	return nil
}
