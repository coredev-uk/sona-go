//go:build darwin

// Command glassspike is the driver of the macOS glass spike (#210 §6.1): it
// is run by .github/workflows/glass-spike.yml on a macOS runner and never
// merged. It starts the probe build on the fixture profile (no account, no
// network, the fake playback engine) in a private HOME, drives it through
// MyGo's automation socket to each surface the spike put glass on, and
// photographs the screen with screencapture, once for each combination of
// glass on or off, light or dark, focused or not, and Reduce Transparency.
//
//	glassspike shots   -app EXE -profile PROFILE -out DIR
//	glassspike measure -app EXE -profile PROFILE -uiprobe UIPROBE -out DIR
//
// A step that fails is logged and the run goes on: the report says what was
// and was not captured (DIR/report.txt).
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coredev-uk/sona-go/e2e/automation"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

var (
	appExe, profileBin, uiprobeBin, outDir string
	report                                 strings.Builder
	reportMu                               sync.Mutex
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: glassspike shots|measure -app EXE -profile PROFILE [-uiprobe UIPROBE] -out DIR")
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.StringVar(&appExe, "app", "", "the probe build's executable (inside Sona.app)")
	fs.StringVar(&profileBin, "profile", "", "e2e/profile's binary")
	fs.StringVar(&uiprobeBin, "uiprobe", "", "cmd/uiprobe's binary (measure)")
	fs.StringVar(&outDir, "out", "glass-out", "where the pictures, tables and the report go")
	reps := fs.Int("reps", 3, "measure: runs of each variant")
	_ = fs.Parse(os.Args[2:])
	if appExe == "" || profileBin == "" {
		fmt.Fprintln(os.Stderr, "glassspike: -app and -profile are required")
		os.Exit(2)
	}
	_ = os.MkdirAll(outDir, 0o755)
	meta()
	switch cmd {
	case "shots":
		shots()
	case "measure":
		measureAll(*reps)
	default:
		fmt.Fprintln(os.Stderr, "glassspike: shots or measure")
		os.Exit(2)
	}
	_ = os.WriteFile(filepath.Join(outDir, "report-"+cmd+".txt"), []byte(report.String()), 0o644)
}

func logf(format string, args ...any) {
	line := time.Now().Format("15:04:05 ") + fmt.Sprintf(format, args...)
	fmt.Println(line)
	reportMu.Lock()
	report.WriteString(line + "\n")
	reportMu.Unlock()
}

// sh runs a command and returns its combined output, trimmed.
func sh(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// meta records what the machine is: the figures are a VM's.
func meta() {
	var b strings.Builder
	for _, c := range [][]string{
		{"sw_vers"},
		{"uname", "-m"},
		{"sysctl", "-n", "machdep.cpu.brand_string"},
		{"sysctl", "-n", "hw.model"},
		{"sysctl", "-n", "hw.ncpu"},
		{"sysctl", "-n", "hw.memsize"},
		{"system_profiler", "SPDisplaysDataType"},
		{"defaults", "read", "-g", "AppleInterfaceStyle"},
		{"defaults", "read", "com.apple.universalaccess", "reduceTransparency"},
	} {
		out, err := sh(c[0], c[1:]...)
		fmt.Fprintf(&b, "$ %s\n%s\n", strings.Join(c, " "), out)
		if err != nil {
			fmt.Fprintf(&b, "(%v)\n", err)
		}
	}
	_ = os.WriteFile(filepath.Join(outDir, "machine.txt"), []byte(b.String()), 0o644)
}

// variant is one way to run the app.
type variant struct {
	name    string
	glass   string // SONA_GLASS
	dark    bool
	reduce  bool     // Reduce Transparency on
	topbar  string   // SONA_GLASS_TOPBAR
	only    []string // the scenes to take (nil: all)
	measure bool     // MYGO_FRAME_STATS=all
}

func (v variant) scene(name string) bool {
	if len(v.only) == 0 {
		return true
	}
	for _, o := range v.only {
		if o == name {
			return true
		}
	}
	return false
}

// session is a running app on a fixture profile.
type session struct {
	v      variant
	home   string
	cmd    *exec.Cmd
	done   chan struct{}
	c      *automation.Client
	paths  config.Paths
	covers *httptest.Server
	stderr *os.File
	dir    string
	shotN  int
	sock   string
}

// setAppearance sets the system's light or dark appearance before the app
// starts, so its windows and the system's glass take it from the start.
func setAppearance(dark bool) {
	script := fmt.Sprintf(`tell application "System Events" to tell appearance preferences to set dark mode to %t`, dark)
	if out, err := sh("osascript", "-e", script); err != nil {
		logf("appearance: osascript refused (%v: %s); using defaults", err, out)
		if dark {
			_, _ = sh("defaults", "write", "-g", "AppleInterfaceStyle", "Dark")
		} else {
			_, _ = sh("defaults", "delete", "-g", "AppleInterfaceStyle")
		}
	}
	time.Sleep(3 * time.Second)
	got, _ := sh("defaults", "read", "-g", "AppleInterfaceStyle")
	logf("appearance: dark=%t, AppleInterfaceStyle=%q", dark, got)
}

// setReduceTransparency tries to turn Reduce Transparency on or off; it
// reports what the preference reads afterwards.
func setReduceTransparency(on bool) bool {
	val := fmt.Sprint(on)
	for _, c := range [][]string{
		{"defaults", "write", "com.apple.universalaccess", "reduceTransparency", "-bool", val},
		{"sudo", "-n", "defaults", "write", "/Library/Preferences/com.apple.universalaccess", "reduceTransparency", "-bool", val},
	} {
		if out, err := sh(c[0], c[1:]...); err != nil {
			logf("reduce transparency: %s: %v %s", strings.Join(c, " "), err, out)
		}
	}
	_, _ = sh("killall", "cfprefsd")
	time.Sleep(3 * time.Second)
	got, _ := sh("defaults", "read", "com.apple.universalaccess", "reduceTransparency")
	logf("reduce transparency: wanted %t, preference reads %q", on, got)
	return got == "1" == on
}

func startSession(v variant) (*session, error) {
	home, err := os.MkdirTemp("", "gs-home")
	if err != nil {
		return nil, err
	}
	s := &session{v: v, home: home, done: make(chan struct{}), dir: filepath.Join(outDir, v.name)}
	_ = os.MkdirAll(s.dir, 0o755)
	// The XDG variables are only for the profile writer's guard; macOS paths
	// come from HOME.
	xdg := []string{"XDG_CONFIG_HOME=" + home + "/x/c", "XDG_DATA_HOME=" + home + "/x/d", "XDG_STATE_HOME=" + home + "/x/s", "XDG_CACHE_HOME=" + home + "/x/k"}
	prof := exec.Command(profileBin, "-theme", "system", "-lyrics")
	prof.Env = append(append(os.Environ(), "HOME="+home), xdg...)
	if out, err := prof.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("fixture profile: %v\n%s", err, out)
	}
	paths, err := config.ResolvePaths(func(k string) string {
		if k == "HOME" {
			return home
		}
		return ""
	})
	if err != nil {
		return nil, err
	}
	s.paths = paths
	s.covers = httptest.NewTLSServer(fixture.Handler())
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("gs-%d.sock", time.Now().UnixNano()%1e9))
	s.sock = sock
	s.stderr, _ = os.Create(filepath.Join(s.dir, "stderr.log"))
	env := append(os.Environ(),
		"HOME="+home, "MYGO_AUTOMATION="+sock, "SONA_E2E=1", "SONA_PRIVATE=1",
		"SONA_E2E_CDN="+s.covers.Listener.Addr().String(), "SONA_GLASS="+v.glass,
	)
	if v.topbar != "" {
		env = append(env, "SONA_GLASS_TOPBAR="+v.topbar)
	}
	if v.measure {
		env = append(env, "MYGO_FRAME_STATS=all")
	}
	s.cmd = exec.Command(appExe)
	s.cmd.Env = env
	s.cmd.Stdout, s.cmd.Stderr = s.stderr, s.stderr
	if err := s.cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = s.cmd.Wait(); close(s.done) }()
	s.c, err = automation.Dial(sock, 45*time.Second)
	if err != nil {
		s.stop()
		return nil, err
	}
	return s, nil
}

func (s *session) stop() {
	if s.c != nil {
		_ = s.c.Close()
	}
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-s.done:
		case <-time.After(10 * time.Second):
			_ = s.cmd.Process.Kill()
		}
	}
	// What the app logged about the glass, for the report.
	if b, err := os.ReadFile(s.paths.LogFile()); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.Contains(l, "glass spike") {
				logf("%s: app log: %s", s.v.name, l)
			}
		}
		_ = os.WriteFile(filepath.Join(s.dir, "sona.log"), b, 0o644)
	}
	if s.covers != nil {
		s.covers.Close()
	}
	if s.stderr != nil {
		_ = s.stderr.Close()
	}
	_ = os.RemoveAll(s.home)
}

// windows lists the app's windows.
func (s *session) windows() []automation.Window {
	ws, err := s.c.Windows()
	if err != nil {
		logf("%s: windows: %v", s.v.name, err)
	}
	return ws
}

// window is the window titled title ("" for the first content window).
func (s *session) window(title string) (automation.Window, bool) {
	for _, w := range s.windows() {
		if w.Visible && (title == "" || strings.Contains(w.Title, title)) && (title != "" || !strings.Contains(w.Title, "mini")) {
			return w, true
		}
	}
	return automation.Window{}, false
}

// shot photographs the screen around the window with screencapture, so what
// the system draws behind and around it (the glass, the shadow) is in the
// picture; when the screen cannot be captured it keeps the app's own frame.
func (s *session) shot(scene, title string) {
	if !s.v.scene(scene) {
		return
	}
	_ = s.c.Idle(3 * time.Second)
	time.Sleep(1200 * time.Millisecond) // blur, covers and the system's glass settle
	s.shotN++
	file := filepath.Join(s.dir, fmt.Sprintf("%02d-%s.png", s.shotN, scene))
	w, ok := s.window(title)
	if !ok {
		logf("%s/%s: no window %q to photograph", s.v.name, scene, title)
		return
	}
	const margin = 40
	x, y := max(0, w.Bounds.X-margin), max(0, w.Bounds.Y-margin)
	rect := fmt.Sprintf("%d,%d,%d,%d", int(x), int(y), int(w.Bounds.Width+2*margin), int(w.Bounds.Height+2*margin))
	if out, err := sh("screencapture", "-x", "-R", rect, file); err != nil {
		logf("%s/%s: screencapture -R %s: %v %s", s.v.name, scene, rect, err, out)
		s.ownFrame(scene, file)
		return
	}
	if fi, err := os.Stat(file); err != nil || fi.Size() == 0 || blank(file) {
		logf("%s/%s: screencapture wrote nothing usable (no Screen Recording permission?)", s.v.name, scene)
		s.ownFrame(scene, file)
		return
	}
	logf("%s/%s: captured %s (window at %.0f,%.0f %.0fx%.0f)", s.v.name, scene, filepath.Base(file), w.Bounds.X, w.Bounds.Y, w.Bounds.Width, w.Bounds.Height)
}

// ownFrame saves the window as the app itself draws it (no vibrancy or
// system glass: those are the window server's).
func (s *session) ownFrame(scene, file string) {
	_, data, err := s.c.Capture()
	if err != nil {
		logf("%s/%s: automation capture: %v", s.v.name, scene, err)
		return
	}
	own := strings.TrimSuffix(file, ".png") + "-appframe.png"
	if err := os.WriteFile(own, data, 0o644); err != nil {
		logf("%s/%s: %v", s.v.name, scene, err)
		return
	}
	logf("%s/%s: kept the app's own frame %s (no system glass in it)", s.v.name, scene, filepath.Base(own))
}

// blank reports a picture of one colour: a denied capture is a black or
// wallpaper-only picture.
func blank(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return true
	}
	defer func() { _ = f.Close() }()
	img, err := png.Decode(f)
	if err != nil {
		return true
	}
	return distinct(img) < 16
}

func distinct(img image.Image) int {
	seen := map[[3]uint32]bool{}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y += 5 {
		for x := b.Min.X; x < b.Max.X; x += 5 {
			r, g, bl, _ := img.At(x, y).RGBA()
			seen[[3]uint32{r >> 8, g >> 8, bl >> 8}] = true
		}
	}
	return len(seen)
}

// must logs a failed step and carries on.
func (s *session) must(step string, err error) bool {
	if err != nil {
		logf("%s: %s: %v", s.v.name, step, err)
		return false
	}
	return true
}

func (s *session) press(label string) bool {
	return s.must("press "+label, s.c.Press(20*time.Second, label))
}

func (s *session) pressRole(role, label string) bool {
	return s.must("press "+role+" "+label, s.c.PressRole(20*time.Second, role, label))
}

func (s *session) key(k string) { s.must("key "+k, s.c.Input(automation.Key(k))) }

// activate brings the app to the front, as a click on its Dock icon would:
// the binary was started from a shell, which does not make it the active app,
// and an inactive window draws its vibrancy greyed.
func (s *session) activate() {
	script := fmt.Sprintf(`tell application "System Events" to set frontmost of (first process whose unix id is %d) to true`, s.cmd.Process.Pid)
	if out, err := sh("osascript", "-e", script); err != nil {
		logf("%s: activate: %v %s", s.v.name, err, out)
	}
	time.Sleep(time.Second)
}

// pressPrefix presses the first button whose label starts with prefix.
func (s *session) pressPrefix(prefix string) bool {
	t, err := s.c.Wait(20*time.Second, "a button "+prefix+"…", func(t *automation.Tree) bool {
		for _, n := range t.Nodes {
			if n.Role == "button" && strings.HasPrefix(n.Label, prefix) {
				return true
			}
		}
		return false
	})
	if !s.must("find "+prefix, err) {
		return false
	}
	for _, n := range t.Nodes {
		if n.Role == "button" && strings.HasPrefix(n.Label, prefix) {
			x, y := n.Bounds.Centre()
			_ = s.c.Input(automation.PointerMove(x, y))
			_ = s.c.Idle(2 * time.Second)
			return s.must("click "+n.Label, s.c.Input(automation.Click(x, y)))
		}
	}
	return false
}

// wheel scrolls the page under the pointer by dy DIPs, in steps.
func (s *session) wheel(dy float64) {
	w, ok := s.window("")
	if !ok {
		return
	}
	x, y := w.ContentBounds.Width/2+100, w.ContentBounds.Height/2
	for range 10 {
		s.must("wheel", s.c.Input(automation.Event{"type": "wheel", "x": x, "y": y, "dx": 0, "dy": dy / 10, "precise": true}))
		time.Sleep(30 * time.Millisecond)
	}
}

// startPlaying opens the fixture's last album and plays it.
func (s *session) startPlaying() bool {
	albums := fixture.Albums()
	first := albums[len(albums)-1]
	if !s.pressRole("link", first.Name) {
		return false
	}
	if !s.press("Play " + first.Name) {
		return false
	}
	_, err := s.c.Wait(20*time.Second, "the player bar playing", func(t *automation.Tree) bool {
		_, ok := t.Under("", "Player").Find("button", "Pause")
		return ok
	})
	return s.must("playing", err)
}

// shotsVariants is every combination the issue asks for.
func shotsVariants() []variant {
	return []variant{
		{name: "light-off", glass: "0"},
		{name: "light-glass", glass: "1"},
		{name: "dark-off", glass: "0", dark: true},
		{name: "dark-glass", glass: "1", dark: true},
		{name: "light-glass-reduce-transparency", glass: "1", reduce: true},
		{name: "light-glass-topbar-strip", glass: "1", topbar: "strip", only: []string{"albums", "albums-scrolled"}},
		{name: "dark-glass-topbar-strip", glass: "1", topbar: "strip", dark: true, only: []string{"albums", "albums-scrolled"}},
	}
}

func shots() {
	for _, v := range shotsVariants() {
		logf("=== %s", v.name)
		setAppearance(v.dark)
		reduced := setReduceTransparency(v.reduce)
		if v.reduce && !reduced {
			logf("%s: Reduce Transparency could not be turned on here; the run goes on without it (its pictures are the glass as-is)", v.name)
		}
		runShots(v)
		if v.reduce {
			setReduceTransparency(false)
		}
	}
	setAppearance(false)
}

func runShots(v variant) {
	s, err := startSession(v)
	if err != nil {
		logf("%s: start: %v", v.name, err)
		return
	}
	defer s.stop()
	if _, err := s.c.Wait(60*time.Second, "the Albums page", func(t *automation.Tree) bool { return t.HasText("24 albums") }); !s.must("albums page", err) {
		return
	}
	s.activate()
	time.Sleep(3 * time.Second) // the covers
	s.shot("albums", "")
	s.wheel(260)
	s.shot("albums-scrolled", "")
	if !s.v.scene("albums-playing") && !s.v.scene("menu") && !s.v.scene("nowplaying-lyrics") {
		return
	}
	s.wheel(-1000)
	if !s.startPlaying() {
		return
	}
	s.press("Back")
	_, _ = s.c.Wait(20*time.Second, "the Albums page", func(t *automation.Tree) bool { return t.HasText("24 albums") })
	s.shot("albums-playing", "")

	// A menu: the album page's More button, with the song playing under it.
	albums := fixture.Albums()
	if s.pressRole("link", albums[len(albums)-2].Name) {
		if s.pressPrefix("More for") {
			s.shot("menu", "")
			s.key("Escape")
		}
		s.press("Back")
	}

	// Now Playing: the lyrics, then the queue.
	s.key("N")
	if _, err := s.c.Wait(20*time.Second, "Now Playing", func(t *automation.Tree) bool {
		_, ok := t.Find("button", "Close now playing (Escape)")
		return ok
	}); s.must("now playing", err) {
		if s.pressRole("tab", "Lyrics") {
			_, _ = s.c.Wait(10*time.Second, "the lyrics", func(t *automation.Tree) bool { return t.HasText("Streetlights hum a borrowed tune") })
		}
		s.shot("nowplaying-lyrics", "")
		if s.pressRole("tab", "Up next") {
			s.shot("nowplaying-queue", "")
		}
		s.key("Escape")
	}

	// The queue as the side panel, over the page.
	if s.pressRole("", "Queue") {
		s.shot("queue-aside", "")
		s.key("Escape")
	}

	// Not focused: another app in front.
	_, _ = sh("open", "-a", "Finder")
	time.Sleep(2 * time.Second)
	s.shot("unfocused", "")

	// The mini player, focused and not.
	if s.press("Mini player") {
		time.Sleep(2 * time.Second)
		s.shot("mini", "mini")
		_, _ = sh("open", "-a", "Finder")
		time.Sleep(2 * time.Second)
		s.shot("mini-unfocused", "mini")
	}
}

// autoTree is the automation client's tree, as the scenarios' waits take it.
type autoTree = *automation.Tree
