package main

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// motionOptions configures -motion.
type motionOptions struct {
	scenarios string
	window    time.Duration
	album     string
}

// A motionScenario is one way motion artwork can be on screen. setup
// brings the page there (motion is already on or off) and waits for the
// video when motion is on; teardown leaves it. A scenario with action
// measures the CPU over that action (a scroll) instead of over -window.
type motionScenario struct {
	name, what string
	setup      func(c *client, o motionOptions, on bool) error
	action     string
	teardown   string
	// plays: motion on, a video must play before the window starts.
	plays bool
}

// tileIndex is the Discover tile the tile scenario hovers: the first fully
// visible one with motion artwork, found with motion on and reused with it
// off.
var tileIndex = -1

func motionScenarios() []motionScenario {
	discover := func(c *client) error {
		if _, err := c.eval(`await __sonaProbe.go("/new"); return await __sonaProbe.top()`, 2*time.Minute); err != nil {
			return fmt.Errorf("opening Discover: %w", err)
		}
		return nil
	}
	return []motionScenario{
		{
			name: "idle", what: "Discover at rest (tiles play only on hover)",
			setup: func(c *client, _ motionOptions, _ bool) error { return discover(c) },
		},
		{
			name: "tile", what: "one Discover tile hovered, its video playing", plays: true,
			setup: func(c *client, _ motionOptions, on bool) error {
				if err := discover(c); err != nil {
					return err
				}
				var i int
				if err := c.evalInto(fmt.Sprintf("return await __sonaProbe.hoverTile(%d)", tileIndex), 10*time.Second, &i); err != nil {
					return err
				}
				if i < 0 {
					return errors.New("no tile with motion artwork on the page")
				}
				tileIndex = i
				return nil
			},
			teardown: "__sonaProbe.unhover(); return 1",
		},
		{
			name: "album", what: "an album page, its header playing", plays: true,
			setup: func(c *client, o motionOptions, _ bool) error {
				_, err := c.eval(fmt.Sprintf(`await __sonaProbe.go("/album/%s"); return await __sonaProbe.top()`, o.album), 2*time.Minute)
				return err
			},
		},
		{
			name: "nowplaying", what: "Now Playing open (its song paused) over an empty page", plays: true,
			setup:    nowPlayingOver(func(motionOptions) string { return "" }),
			teardown: "return await __sonaProbe.nowPlaying(false)",
		},
		{
			name: "nowplaying-album", what: "Now Playing open (its song paused) over the album page", plays: true,
			setup:    nowPlayingOver(func(o motionOptions) string { return "/album/" + o.album }),
			teardown: "return await __sonaProbe.nowPlaying(false)",
		},
		{
			name: "scroll", what: "Discover scrolled down and back at -speed under a resting pointer",
			setup:  func(c *client, _ motionOptions, _ bool) error { return discover(c) },
			action: "scroll",
		},
	}
}

// nowPlayingOver gets a song from -album into the player, paused, and opens
// Now Playing over the page at route ("": the empty control page).
func nowPlayingOver(route func(motionOptions) string) func(*client, motionOptions, bool) error {
	return func(c *client, o motionOptions, _ bool) error {
		var ok bool
		if err := c.evalInto(fmt.Sprintf("return await __sonaProbe.loadSong(%q)", o.album), 2*time.Minute, &ok); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("could not get a song from album %s into the player", o.album)
		}
		js := fmt.Sprintf("await __sonaProbe.go(%q); return await __sonaProbe.nowPlaying(true)", route(o))
		if err := c.evalInto(js, time.Minute, &ok); err != nil {
			return err
		}
		if !ok {
			return errors.New("now playing did not open")
		}
		return nil
	}
}

func motionScenarioNames() []string {
	var names []string
	for _, s := range motionScenarios() {
		names = append(names, s.name)
	}
	return names
}

// video is one <video> on the page (probe.js videos()).
type video struct {
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	BoxWidth  int     `json:"boxWidth"`
	BoxHeight int     `json:"boxHeight"`
	Paused    bool    `json:"paused"`
	Duration  float64 `json:"duration"`
	Frames    int     `json:"frames"`
	Dropped   int     `json:"dropped"`
}

type frameStats struct {
	Interval float64 `json:"interval"`
	P50      float64 `json:"p50"`
	P95      float64 `json:"p95"`
	Frames   int     `json:"frames"`
	Dropped  int     `json:"dropped"`
}

// motionSample is one scenario run: CPU in % of one core over the window,
// the videos that played and the page's frame times after it.
type motionSample struct {
	web, webMain, webMedia, ui float64
	playing                    int     // videos playing during the window
	fps                        float64 // video frames decoded per second, all videos
	droppedVideo               int
	size                       string // decoded → drawn device px of the first playing video
	frames                     frameStats
	threads                    map[string]float64 // web process threads, % of a core
}

func probeMotion(c *client, o options, m motionOptions) error {
	if o.pid == 0 {
		return fmt.Errorf("-motion needs -pid: it measures CPU time")
	}
	if err := install(c); err != nil {
		return err
	}
	var want []motionScenario
	all := motionScenarios()
	for name := range strings.SplitSeq(m.scenarios, ",") {
		i := slices.IndexFunc(all, func(s motionScenario) bool { return s.name == name })
		if i < 0 {
			return fmt.Errorf("-scenarios: unknown %q (know %s)", name, strings.Join(motionScenarioNames(), ", "))
		}
		want = append(want, all[i])
	}
	var env struct {
		DPR    float64 `json:"dpr"`
		Width  int     `json:"width"`
		Height int     `json:"height"`
	}
	if err := c.evalInto("return {dpr: devicePixelRatio, width: innerWidth, height: innerHeight}", 10*time.Second, &env); err != nil {
		return err
	}
	// The runs turn the setting on and off; it is left as it was.
	var was bool
	if err := c.evalInto("return await __sonaProbe.setMotion(true)", time.Minute, &was); err != nil {
		return fmt.Errorf("turning motion artwork on: %w", err)
	}
	defer func() { _, _ = c.eval(fmt.Sprintf("return await __sonaProbe.setMotion(%t)", was), time.Minute) }()
	if load, err := os.ReadFile("/proc/loadavg"); err == nil {
		fmt.Printf("load average: %s\n", bytes.Join(bytes.Fields(load)[:3], []byte(" ")))
	}
	fmt.Printf("window %d×%d CSS px at %g×; %s\n", env.Width, env.Height, env.DPR, renderer(o.pid))
	fmt.Printf("CPU: %% of one core over %s (scroll: over the scroll), median of %d runs (min–max); frame times over 3 s after it\n\n", m.window, o.runs)
	fmt.Println("| scenario | motion | web process | web main thread | GStreamer threads | UI process | videos playing | decoded → drawn px | video frames/s | frame p50 | frame p95 | dropped frames |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|---|---|")
	for _, s := range want {
		samples := map[bool][]motionSample{}
		for run := range o.runs {
			// On first: the tile scenario finds its tile with motion on.
			for _, on := range []bool{true, false} {
				r, err := motionRun(c, o, m, s, on)
				if err != nil {
					return fmt.Errorf("%s (motion %s, run %d): %w", s.name, onOff(on), run+1, err)
				}
				samples[on] = append(samples[on], r)
				fmt.Fprintf(os.Stderr, "%s motion %s run %d: web %.1f%% (main %.1f%%, GStreamer %.1f%%), UI %.1f%%, %d playing, %.1f video frames/s (%d dropped), %s, frames p50 %.1f p95 %.1f dropped %d of %d; busiest web threads: %s\n",
					s.name, onOff(on), run+1, r.web, r.webMain, r.webMedia, r.ui, r.playing, r.fps, r.droppedVideo, cmp.Or(r.size, "no video"),
					r.frames.P50, r.frames.P95, r.frames.Dropped, r.frames.Frames+r.frames.Dropped, busiest(r.threads, 5))
			}
		}
		for _, on := range []bool{true, false} {
			rs := samples[on]
			pick := func(f func(motionSample) float64) []float64 {
				xs := make([]float64, len(rs))
				for i, r := range rs {
					xs[i] = f(r)
				}
				return xs
			}
			sizes := ""
			for _, r := range rs {
				if r.size != "" {
					sizes = r.size
				}
			}
			fmt.Printf("| %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				s.name, onOff(on),
				spread(pick(func(r motionSample) float64 { return r.web }), "%"),
				spread(pick(func(r motionSample) float64 { return r.webMain }), "%"),
				spread(pick(func(r motionSample) float64 { return r.webMedia }), "%"),
				spread(pick(func(r motionSample) float64 { return r.ui }), "%"),
				spread(pick(func(r motionSample) float64 { return float64(r.playing) }), ""),
				cmp.Or(sizes, "–"),
				spread(pick(func(r motionSample) float64 { return r.fps }), ""),
				spread(pick(func(r motionSample) float64 { return r.frames.P50 }), " ms"),
				spread(pick(func(r motionSample) float64 { return r.frames.P95 }), " ms"),
				spread(pick(func(r motionSample) float64 { return float64(r.frames.Dropped) }), ""))
		}
	}
	fmt.Println()
	for _, s := range want {
		fmt.Printf("- %s: %s\n", s.name, s.what)
	}
	return nil
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// motionRun sets motion on or off, brings the page to scenario s and
// measures it once.
func motionRun(c *client, o options, m motionOptions, s motionScenario, on bool) (motionSample, error) {
	var r motionSample
	if _, err := c.eval(fmt.Sprintf("return await __sonaProbe.setMotion(%t)", on), time.Minute); err != nil {
		return r, fmt.Errorf("turning motion artwork %s: %w", onOff(on), err)
	}
	if err := s.setup(c, m, on); err != nil {
		return r, err
	}
	if on && s.plays {
		var n int
		if err := c.evalInto("return await __sonaProbe.playing()", time.Minute, &n); err != nil {
			return r, err
		}
		if n == 0 {
			return r, fmt.Errorf("no motion video played (Settings → Diagnostics → Motion artwork says why)")
		}
	}
	// Let the video's first loads and the page's transitions finish.
	time.Sleep(time.Second)
	var v0, v1 []video
	if err := c.evalInto("return __sonaProbe.videos()", 10*time.Second, &v0); err != nil {
		return r, err
	}
	before, start := cpuTimes(o.pid), time.Now()
	if s.action == "scroll" {
		var sr scrollResult
		js := fmt.Sprintf("return await __sonaProbe.scroll({speed: %d, maxPx: %d, hover: true})", o.speed, o.maxPx)
		if err := c.evalInto(js, 3*time.Minute, &sr); err != nil {
			return r, err
		}
		r.frames = frameStats{Interval: sr.Interval, P50: sr.P50, P95: sr.P95, Frames: sr.Frames, Dropped: sr.Dropped}
		r.playing = sr.Videos
	} else {
		time.Sleep(m.window)
	}
	after, wall := cpuTimes(o.pid), time.Since(start)
	if !before.ok || !after.ok {
		return r, fmt.Errorf("no process %d in /proc", o.pid)
	}
	share := func(d time.Duration) float64 { return 100 * d.Seconds() / wall.Seconds() }
	r.web, r.webMain, r.ui = share(after.web-before.web), share(after.webMain-before.webMain), share(after.ui-before.ui)
	// By thread ID: a video's threads come and go with its pipeline (the
	// time of one that ends inside the window is lost).
	r.threads = map[string]float64{}
	for id, t := range after.threads {
		d := t.t - before.threads[id].t
		r.threads[t.name] += share(d)
		if t.media {
			r.webMedia += share(d)
		}
	}
	if err := c.evalInto("return __sonaProbe.videos()", 10*time.Second, &v1); err != nil {
		return r, err
	}
	if s.action == "" {
		frames := 0
		for i, v := range v1 {
			if !v.Paused {
				r.playing++
			}
			if i < len(v0) {
				frames += v.Frames - v0[i].Frames
				r.droppedVideo += v.Dropped - v0[i].Dropped
			}
		}
		r.fps = float64(frames) / wall.Seconds()
		if err := c.evalInto("return await __sonaProbe.frames(3000)", time.Minute, &r.frames); err != nil {
			return r, err
		}
	}
	// The first playing video's sizes (a paused one under an overlay is not drawn).
	for _, v := range v1 {
		if !v.Paused {
			r.size = fmt.Sprintf("%d×%d → %d×%d", v.Width, v.Height, v.BoxWidth, v.BoxHeight)
			break
		}
	}
	if s.teardown != "" {
		if _, err := c.eval(s.teardown, time.Minute); err != nil {
			return r, err
		}
	}
	return r, nil
}

// spread formats a median with its range: "42% (40–45)".
func spread(xs []float64, unit string) string {
	if len(xs) == 0 {
		return "–"
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	f := func(x float64) string {
		if unit == " ms" {
			return fmt.Sprintf("%.1f", x)
		}
		return fmt.Sprintf("%.0f", x)
	}
	med := f(s[len(s)/2]) + unit
	if f(s[0]) == f(s[len(s)-1]) {
		return med
	}
	return fmt.Sprintf("%s (%s–%s)", med, f(s[0]), f(s[len(s)-1]))
}

// busiest lists the n web process threads that used the most CPU.
func busiest(threads map[string]float64, n int) string {
	names := make([]string, 0, len(threads))
	for name := range threads {
		names = append(names, name)
	}
	slices.SortFunc(names, func(a, b string) int { return cmp.Compare(threads[b], threads[a]) })
	parts := make([]string, 0, n)
	for _, name := range names[:min(n, len(names))] {
		parts = append(parts, fmt.Sprintf("%s %.1f%%", name, threads[name]))
	}
	return strings.Join(parts, ", ")
}

// renderer says how Sona's processes draw, from the devices and memory
// they have open: a GPU device (/dev/dri, /dev/nvidia*) or not, and
// Mesa's software rasteriser (llvmpipe's memfd). Without a GPU device the
// web process paints in software into shared memory and the UI process
// composites every frame, so both cost more than on a desktop.
func renderer(pid int) string {
	describe := func(p int) string {
		fds, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", p))
		if err != nil {
			return "unknown"
		}
		gpu, llvmpipe := "", false
		for _, fd := range fds {
			target, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", p, fd.Name()))
			if err != nil {
				continue
			}
			switch {
			case strings.HasPrefix(target, "/dev/nvidia"):
				gpu = "NVIDIA"
			case strings.HasPrefix(target, "/dev/dri/"):
				gpu = cmp.Or(gpu, "DRM render node")
			case strings.HasPrefix(target, "/memfd:lp_"):
				llvmpipe = true
			}
		}
		switch {
		case gpu != "" && llvmpipe:
			return "GPU (" + gpu + ") and llvmpipe (software GL) open"
		case gpu != "":
			return "GPU (" + gpu + ")"
		case llvmpipe:
			return "llvmpipe (software GL)"
		}
		return "no GPU device (software, shared memory)"
	}
	web := 0
	procs, _ := os.ReadDir("/proc")
	for _, e := range procs {
		var n int
		if _, err := fmt.Sscan(e.Name(), &n); err != nil {
			continue
		}
		if ppid, comm, _, ok := stat("/proc/" + e.Name() + "/stat"); ok && strings.HasPrefix(comm, "WebKitWebProces") {
			for a := ppid; a > 1; {
				if a == pid {
					web = n
					break
				}
				next, _, _, ok := stat(fmt.Sprintf("/proc/%d/stat", a))
				if !ok {
					break
				}
				a = next
			}
		}
	}
	return fmt.Sprintf("UI process: %s; web process: %s", describe(pid), describe(web))
}
