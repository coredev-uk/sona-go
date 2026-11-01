// Command uiprobe measures Sona's rendering on a running development build:
// frame times while scrolling, image decodes, DOM size and the styles that
// promote layers, per page. It drives a Sona built with `-tags mcp` (the
// Wails MCP server, never in release builds) through `js_eval`: it installs
// probe.js, opens each route, waits for its images, then scrolls the page
// from top to bottom and back at a fixed speed, one step per frame, and
// reads the requestAnimationFrame deltas. -firstpaint summarises
// "startup: first paint" from Sona's logs instead.
//
// Usage:
//
//	uiprobe [-addr 127.0.0.1:9322] [-pid PID] [-log sona.log] [-runs 3] [-speed 2000] [-pages empty=,home=/,new=/new,albums=/library/albums] [-css SHEET]
//	uiprobe -firstpaint run1/sona.log run2/sona.log …
//
// How to run it headless: CONTRIBUTING.md, "Measuring UI performance".
package main

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed probe.js
var probeJS string

type options struct {
	pid          int
	pages        string
	runs         int
	speed, maxPx int
	css          string
	log          string
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9322", "Sona's MCP server (WAILS_MCP_PORT)")
	var o options
	flag.IntVar(&o.runs, "runs", 3, "scroll passes per page")
	flag.IntVar(&o.speed, "speed", 2000, "scroll speed in CSS px per second")
	flag.IntVar(&o.maxPx, "max", 20000, "scroll at most this far down (CSS px)")
	flag.IntVar(&o.pid, "pid", 0, "Sona's process ID, to read its and its web processes' CPU time (Linux)")
	flag.StringVar(&o.pages, "pages", "empty=,home=/,new=/new,albums=/library/albums",
		"name=route pairs, comma separated; an empty route is the control, an empty page 20000 px tall")
	flag.StringVar(&o.css, "css", "", "a style sheet added to the page before measuring, to try a change without a build")
	flag.StringVar(&o.log, "log", "", "Sona's sona.log, to place the page's load after the process start")
	firstPaintOnly := flag.Bool("firstpaint", false, "summarise `startup: first paint` from the sona.log files given as arguments")
	flag.Parse()
	var err error
	if *firstPaintOnly {
		err = summariseFirstPaint(flag.Args())
	} else {
		err = probe(&client{url: "http://" + *addr + "/mcp"}, o)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "uiprobe:", err)
		os.Exit(1)
	}
}

type scrollResult struct {
	Interval   float64 `json:"interval"`
	Px         int     `json:"px"`
	Frames     int     `json:"frames"`
	P50        float64 `json:"p50"`
	P95        float64 `json:"p95"`
	P99        float64 `json:"p99"`
	Max        float64 `json:"max"`
	Dropped    int     `json:"dropped"`
	DroppedPct float64 `json:"droppedPct"`
	LongFrames int     `json:"longFrames"`
	Nodes      int     `json:"nodes"`
	Ms         int     `json:"ms"`
	// CPU time over the scroll's wall time, % of one core, from /proc (not the page).
	WebMainCPU float64 `json:"-"`
	WebCPU     float64 `json:"-"`
	UICPU      float64 `json:"-"`
	Images     struct {
		Loads     int     `json:"loads"`
		Distinct  int     `json:"distinct"`
		DecodedMP float64 `json:"decodedMP"`
		Ratio     float64 `json:"ratio"`
		Oversized int     `json:"oversized"`
	} `json:"images"`
}

type pageResult struct {
	Nodes  int     `json:"nodes"`
	Imgs   int     `json:"imgs"`
	Ratio  float64 `json:"ratio"`
	Height int     `json:"height"`
}

func probe(c *client, o options) error {
	if _, err := c.eval("const r = "+probeJS+"\nreturn r;", 10*time.Second); err != nil {
		return fmt.Errorf("installing the probe (is Sona built with -tags mcp and running?): %w", err)
	}
	if o.css != "" {
		js := fmt.Sprintf("const s = document.createElement('style'); s.textContent = %q; document.head.append(s); return 1", o.css)
		if _, err := c.eval(js, 10*time.Second); err != nil {
			return err
		}
	}
	var start struct {
		TimeOrigin       float64 `json:"timeOrigin"`
		ResponseEnd      int     `json:"responseEnd"`
		DOMContentLoaded int     `json:"domContentLoaded"`
		FCP              int     `json:"fcp"`
	}
	if err := c.evalInto("return __sonaProbe.startup()", 10*time.Second, &start); err != nil {
		return err
	}
	if o.log == "" {
		fmt.Printf("startup (ms since the page's navigation start): HTML received %d, DOMContentLoaded (scripts run) %d, first contentful paint %d\n",
			start.ResponseEnd, start.DOMContentLoaded, start.FCP)
	} else {
		paints, err := firstPaints(o.log)
		if err != nil {
			return err
		}
		last := paints[len(paints)-1]
		nav := int(time.UnixMilli(int64(start.TimeOrigin)).Sub(last.at.Add(-last.since)).Milliseconds())
		fmt.Printf("startup (ms since process start): page navigation %d, HTML received %d, DOMContentLoaded (scripts run) %d, first contentful paint %d, Go's first paint %d\n",
			nav, nav+start.ResponseEnd, nav+start.DOMContentLoaded, nav+start.FCP, last.since.Milliseconds())
	}
	// Frame times depend on everything else the computer is doing.
	if load, err := os.ReadFile("/proc/loadavg"); err == nil {
		fmt.Printf("load average: %s\n", bytes.Join(bytes.Fields(load)[:3], []byte(" ")))
	}
	fmt.Println()

	fmt.Println("| page | DOM nodes | layer styles (estimate) | frame p50 | frame p95 | dropped frames | frames > 50 ms | web main thread CPU | web process CPU | UI process CPU | images decoded | decoded MP | decoded / shown px |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, pair := range strings.Split(o.pages, ",") {
		if pair == "" {
			continue // -pages "": the startup line only
		}
		name, route, ok := strings.Cut(pair, "=")
		if !ok {
			return fmt.Errorf("-pages: %q is not name=route", pair)
		}
		var pg pageResult
		if err := c.evalInto(fmt.Sprintf("return await __sonaProbe.go(%q)", route), 60*time.Second, &pg); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		var layers map[string]int
		if err := c.evalInto("return __sonaProbe.layers()", 30*time.Second, &layers); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		var rs []scrollResult
		for range o.runs {
			if _, err := c.eval("return await __sonaProbe.top()", time.Minute); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			before := cpuTimes(o.pid)
			var r scrollResult
			js := fmt.Sprintf("return await __sonaProbe.scroll({speed: %d, maxPx: %d})", o.speed, o.maxPx)
			if err := c.evalInto(js, 3*time.Minute, &r); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
			after := cpuTimes(o.pid)
			if r.Ms > 0 && before.ok && after.ok {
				share := func(d time.Duration) float64 { return 100 * float64(d.Milliseconds()) / float64(r.Ms) }
				r.WebMainCPU = share(after.webMain - before.webMain)
				r.WebCPU = share(after.web - before.web)
				r.UICPU = share(after.ui - before.ui)
			}
			rs = append(rs, r)
		}
		m := median(rs)
		fmtCPU := func(pct float64) string {
			if o.pid == 0 {
				return "–"
			}
			return fmt.Sprintf("%.0f%%", pct)
		}
		fmt.Printf("| %s | %d | %s | %.1f ms | %.1f ms | %d of %d (%.1f%%) | %d | %s | %s | %s | %d | %.1f | %.2f |\n",
			name, pg.Nodes, formatLayers(layers), m.P50, m.P95,
			m.Dropped, m.Frames+m.Dropped, m.DroppedPct, m.LongFrames,
			fmtCPU(m.WebMainCPU), fmtCPU(m.WebCPU), fmtCPU(m.UICPU),
			m.Images.Loads, m.Images.DecodedMP, m.Images.Ratio)
		for i, r := range rs {
			fmt.Fprintf(os.Stderr, "%s run %d: %d px in %d ms, interval %.2f ms, p50 %.1f p95 %.1f max %.1f, dropped %d (%.1f%%), CPU web main %.0f%% web %.0f%% UI %.0f%%, images %d (%.1f MP, ratio %.2f, %d oversized), %d on screen at the top (ratio %.2f)\n",
				name, i+1, r.Px, r.Ms, r.Interval, r.P50, r.P95, r.Max, r.Dropped, r.DroppedPct, r.WebMainCPU, r.WebCPU, r.UICPU,
				r.Images.Loads, r.Images.DecodedMP, r.Images.Ratio, r.Images.Oversized, pg.Imgs, pg.Ratio)
		}
	}
	return nil
}

type cpu struct {
	webMain, web, ui time.Duration
	ok               bool
}

// stat reads a /proc stat file: parent, command name and CPU time used.
func stat(path string) (ppid int, comm string, t time.Duration, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, "", 0, false
	}
	open, end := bytes.IndexByte(data, '('), bytes.LastIndexByte(data, ')')
	if open < 0 || end < open {
		return 0, "", 0, false
	}
	f := strings.Fields(string(data[end+1:]))
	if len(f) < 13 {
		return 0, "", 0, false
	}
	ppid, _ = strconv.Atoi(f[1])
	utime, _ := strconv.Atoi(f[11])
	stime, _ := strconv.Atoi(f[12])
	// USER_HZ is 100 on every Linux architecture Sona builds for.
	return ppid, string(data[open+1 : end]), time.Duration(utime+stime) * 10 * time.Millisecond, true
}

// cpuTimes reads the CPU time Sona's UI process (pid) and its WebKit web
// processes (and their main threads, which run the page's script, style,
// layout and paint) have used so far, from /proc (Linux; zero elsewhere
// or without -pid). Unlike frame times, it barely changes with the load
// the rest of the computer puts on it.
func cpuTimes(pid int) cpu {
	if pid == 0 {
		return cpu{}
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return cpu{}
	}
	type proc struct {
		ppid int
		comm string
		t    time.Duration
	}
	procs := map[int]proc{}
	for _, e := range entries {
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if ppid, comm, t, ok := stat("/proc/" + e.Name() + "/stat"); ok {
			procs[n] = proc{ppid, comm, t}
		}
	}
	ui, ok := procs[pid]
	if !ok {
		return cpu{}
	}
	out := cpu{ui: ui.t, ok: true}
	for n, p := range procs {
		if !strings.HasPrefix(p.comm, "WebKitWebProces") {
			continue
		}
		for a := p.ppid; a > 1; a = procs[a].ppid {
			if a == pid {
				out.web += p.t
				if _, _, t, ok := stat(fmt.Sprintf("/proc/%d/task/%d/stat", n, n)); ok {
					out.webMain += t
				}
				break
			}
			if _, known := procs[a]; !known {
				break
			}
		}
	}
	return out
}

// median takes each field's median over the runs.
func median(rs []scrollResult) scrollResult {
	mid := func(f func(scrollResult) float64) float64 {
		xs := make([]float64, len(rs))
		for i, r := range rs {
			xs[i] = f(r)
		}
		slices.Sort(xs)
		return xs[len(xs)/2]
	}
	var m scrollResult
	m.P50 = mid(func(r scrollResult) float64 { return r.P50 })
	m.P95 = mid(func(r scrollResult) float64 { return r.P95 })
	m.P99 = mid(func(r scrollResult) float64 { return r.P99 })
	m.Frames = int(mid(func(r scrollResult) float64 { return float64(r.Frames) }))
	m.Dropped = int(mid(func(r scrollResult) float64 { return float64(r.Dropped) }))
	m.DroppedPct = mid(func(r scrollResult) float64 { return r.DroppedPct })
	m.WebMainCPU = mid(func(r scrollResult) float64 { return r.WebMainCPU })
	m.WebCPU = mid(func(r scrollResult) float64 { return r.WebCPU })
	m.UICPU = mid(func(r scrollResult) float64 { return r.UICPU })
	m.LongFrames = int(mid(func(r scrollResult) float64 { return float64(r.LongFrames) }))
	m.Images.Loads = int(mid(func(r scrollResult) float64 { return float64(r.Images.Loads) }))
	m.Images.DecodedMP = mid(func(r scrollResult) float64 { return r.Images.DecodedMP })
	m.Images.Ratio = mid(func(r scrollResult) float64 { return r.Images.Ratio })
	return m
}

func formatLayers(l map[string]int) string {
	if len(l) == 0 {
		return "none"
	}
	keys := make([]string, 0, len(l))
	for k := range l {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s %d", k, l[k])
	}
	return strings.Join(parts, ", ")
}

type firstPaint struct {
	at    time.Time     // when Go logged it
	since time.Duration // since the process started
}

// firstPaints reads the "startup: first paint" lines of Sona's JSON logs.
func firstPaints(files ...string) ([]firstPaint, error) {
	var out []firstPaint
	for _, name := range files {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			var line struct {
				Time  time.Time `json:"time"`
				Msg   string    `json:"msg"`
				Since int64     `json:"since_start_ms"`
			}
			if json.Unmarshal(sc.Bytes(), &line) == nil && line.Msg == "startup: first paint" {
				out = append(out, firstPaint{line.Time, time.Duration(line.Since) * time.Millisecond})
			}
		}
		err = sc.Err()
		_ = f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no `startup: first paint` lines")
	}
	return out, nil
}

// summariseFirstPaint prints the startup to first-paint times in Sona's JSON logs.
func summariseFirstPaint(files []string) error {
	if len(files) == 0 {
		return errors.New("-firstpaint: give one or more sona.log files")
	}
	paints, err := firstPaints(files...)
	if err != nil {
		return err
	}
	ms := make([]int64, len(paints))
	for i, p := range paints {
		ms[i] = p.since.Milliseconds()
	}
	slices.Sort(ms)
	fmt.Printf("first paint over %d starts: min %d ms, median %d ms, max %d ms\n",
		len(ms), ms[0], ms[len(ms)/2], ms[len(ms)-1])
	return nil
}

// client is a minimal MCP client for the Wails MCP server (streamable HTTP).
type client struct {
	url     string
	session string
	id      int
}

func (c *client) post(body any) ([]json.RawMessage, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.session != "" {
		req.Header.Set("Mcp-Session-Id", c.session)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if s := resp.Header.Get("Mcp-Session-Id"); s != "" {
		c.session = s
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("MCP %s: %s", resp.Status, bytes.TrimSpace(data))
	}
	var msgs []json.RawMessage
	for _, line := range bytes.Split(data, []byte("\n")) {
		if rest, ok := bytes.CutPrefix(line, []byte("data:")); ok {
			msgs = append(msgs, bytes.TrimSpace(rest))
		}
	}
	if len(msgs) == 0 && len(bytes.TrimSpace(data)) > 0 {
		msgs = append(msgs, data)
	}
	return msgs, nil
}

func (c *client) init() error {
	if c.session != "" {
		return nil
	}
	c.id++
	if _, err := c.post(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": "initialize", "params": map[string]any{
		"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "uiprobe", "version": "1"},
	}}); err != nil {
		return err
	}
	_, err := c.post(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return err
}

// eval runs js (an async function body) in Sona's main window and returns its JSON value.
func (c *client) eval(js string, timeout time.Duration) (json.RawMessage, error) {
	if err := c.init(); err != nil {
		return nil, err
	}
	c.id++
	msgs, err := c.post(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": "tools/call", "params": map[string]any{
		"name": "js_eval", "arguments": map[string]any{"js": js, "timeout_ms": timeout.Milliseconds()},
	}})
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		var r struct {
			Result *struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(m, &r); err != nil {
			continue
		}
		if r.Error != nil {
			return nil, errors.New(r.Error.Message)
		}
		if r.Result != nil && len(r.Result.Content) > 0 {
			text := r.Result.Content[0].Text
			if r.Result.IsError {
				return nil, errors.New(text)
			}
			return json.RawMessage(text), nil
		}
	}
	return nil, errors.New("js_eval: no result")
}

func (c *client) evalInto(js string, timeout time.Duration, out any) error {
	raw, err := c.eval(js, timeout)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}
