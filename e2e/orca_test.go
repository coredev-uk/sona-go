//go:build e2e

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// orca is a real Orca screen reader running in the headless session, and
// the keyboard that drives the app the way a person does. Orca logs every
// utterance it generates ("SPEECH OUTPUT: '…'") in its debug file whether or
// not a speech server is there to say it, so the run needs no speech-dispatcher
// and no audio: the log is what Orca would speak.
type orca struct {
	t    *testing.T
	log  string // Orca's debug file
	done chan struct{}
	cmd  *exec.Cmd

	steps []orcaStep // everything pressed so far, for the transcript
	focus string     // where Orca last placed the focus: "[role: 'name' (0x…)]"
}

// orcaStep is one key press and what Orca spoke for it.
type orcaStep struct {
	surface string
	keys    string
	spoken  []string
	focus   string // the element that had the focus after the key
}

var (
	// An Orca log is a series of entries, each starting with a time stamp.
	// Orca logs where it has put the keyboard focus whenever it moves.
	focusMoved = regexp.MustCompile(`Locus of focus is \[([^\]]*?) \(0x[0-9a-f]+\)\]`)
	logEntry   = regexp.MustCompile(`(?m)^\d\d:\d\d:\d\d\.\d{6} - `)
	speech     = regexp.MustCompile(`(?s)^SPEECH OUTPUT: '(.*)' (?:\{.*\}|None)\s*$`)
)

// orcaBin is the Orca to run: E2E_ORCA_BIN, else `orca` on the PATH.
func orcaBin(t *testing.T) string {
	t.Helper()
	if os.Getenv("E2E_ORCA") != "1" {
		t.Skip("E2E_ORCA=1 runs the Orca walk (docs/accessibility.md, \"Listening with Orca\")")
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no Wayland display: run through `headless -- go test -tags e2e ./e2e/...`")
	}
	bin := os.Getenv("E2E_ORCA_BIN")
	if bin == "" {
		var err error
		if bin, err = exec.LookPath("orca"); err != nil {
			t.Fatal("E2E_ORCA=1 needs Orca: put `orca` on the PATH or set E2E_ORCA_BIN")
		}
	}
	if _, err := exec.LookPath("wtype"); err != nil {
		t.Fatal("E2E_ORCA=1 needs wtype on the PATH")
	}
	return bin
}

// startOrca starts Orca on the session's accessibility bus and waits until
// it has greeted the window. Start it after the app: Orca then finds the
// window already active.
func startOrca(t *testing.T) *orca {
	t.Helper()
	bin := orcaBin(t)
	o := &orca{t: t, done: make(chan struct{})}
	o.log = filepath.Join(outDir(t), strings.ReplaceAll(t.Name(), "/", "_")+"-orca.log")
	_ = os.Remove(o.log)
	// Orca's own directory (XDG_DATA_HOME/orca) is private to the run, and its
	// customizations file makes the debug log line-buffered: Orca writes the
	// file in blocks of 8 KB, so what it says would reach the log long after.
	data := t.TempDir()
	if err := os.MkdirAll(filepath.Join(data, "orca"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "orca", "orca-customizations.py"), []byte(orcaCustomizations), 0o644); err != nil {
		t.Fatal(err)
	}
	o.cmd = exec.Command(bin, "--debug-file="+o.log, "--debug")
	// devenv's LD_LIBRARY_PATH is the app's GTK stack: a Nix-built Orca has
	// its own. GSETTINGS_BACKEND=memory keeps Orca's settings out of dconf
	// (there is none in the session) and out of the user's.
	o.cmd.Env = append(envWithout(envWithout(os.Environ(), "LD_LIBRARY_PATH"), "XDG_DATA_HOME"),
		"GSETTINGS_BACKEND=memory", "XDG_DATA_HOME="+data, "LC_ALL=C.UTF-8", "LANGUAGE=en")
	if out, err := os.Create(strings.TrimSuffix(o.log, ".log") + "-stderr.log"); err == nil {
		o.cmd.Stdout, o.cmd.Stderr = out, out
		t.Cleanup(func() { _ = out.Close() })
	}
	if err := o.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = o.cmd.Wait(); close(o.done) }()
	t.Cleanup(func() {
		_ = o.cmd.Process.Kill()
		<-o.done
	})
	until(t, 30*time.Second, "Orca to greet the window", func() bool { return len(o.utterances()) > 0 })
	o.settle(2 * time.Second)
	return o
}

func envWithout(env []string, name string) []string {
	var out []string
	for _, e := range env {
		if !strings.HasPrefix(e, name+"=") {
			out = append(out, e)
		}
	}
	return out
}

// orcaCustomizations is Orca's orca-customizations.py: Orca runs it at start,
// before it speaks. The debug file becomes line-buffered.
const orcaCustomizations = `from orca import debug


class _Flushed:
    def __init__(self, f):
        self._f = f

    def writelines(self, lines):
        self._f.writelines(lines)
        self._f.flush()

    def write(self, text):
        self._f.write(text)
        self._f.flush()

    def __getattr__(self, name):
        return getattr(self._f, name)


if debug.debugFile:
    debug.debugFile = _Flushed(debug.debugFile)
`

// utterances is everything Orca has spoken so far, in order.
func (o *orca) utterances() []string {
	b, _ := os.ReadFile(o.log)
	var out []string
	entries := logEntry.Split(string(b), -1)
	for _, e := range entries {
		if m := speech.FindStringSubmatch(e); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

func (o *orca) logSize() int {
	b, _ := os.ReadFile(o.log)
	return len(b)
}

// logFrom is what Orca has logged since offset.
func (o *orca) logFrom(offset int) string {
	b, _ := os.ReadFile(o.log)
	if offset > len(b) {
		return ""
	}
	return string(b[offset:])
}

// keyReleases counts the key releases Orca has been told about: a key has
// reached Orca once its release has.
func (o *orca) keyReleases() int {
	b, _ := os.ReadFile(o.log)
	return strings.Count(string(b), "type=ATSPI_KEY_RELEASED_EVENT")
}

// settle waits until Orca has said nothing new for quiet, at most 15 s.
func (o *orca) settle(quiet time.Duration) {
	n, since := len(o.utterances()), time.Now()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if m := len(o.utterances()); m != n {
			n, since = m, time.Now()
		} else if time.Since(since) >= quiet {
			return
		}
	}
}

// keyEchoes are what Orca says for a key press itself, before what the key
// did: not a part of what the app announces.
var keyEchoes = map[string]bool{
	"Tab": true, "Return": true, "Escape": true, "Space": true, "Menu": true,
	"F10": true, "Delete": true, "Backspace": true, "Home": true, "End": true, "Enter": true,
}

// press sends one key chord ("Tab", "shift+Tab", "alt+Down") to the focused
// window through the compositor's virtual keyboard, as a keyboard does, waits
// for Orca to have it and to finish speaking, and returns what it spoke.
func (o *orca) press(surface, chord string) []string {
	o.t.Helper()
	before, n := len(o.utterances()), o.keyReleases()
	logFrom := o.logSize()
	parts := strings.Split(chord, "+")
	key, mods := parts[len(parts)-1], parts[:len(parts)-1]
	var args []string
	for _, m := range mods {
		args = append(args, "-M", m)
	}
	args = append(args, "-k", key)
	for i := len(mods) - 1; i >= 0; i-- {
		args = append(args, "-m", mods[i])
	}
	cmd := exec.Command("wtype", args...)
	cmd.Env = envWithout(os.Environ(), "LD_LIBRARY_PATH")
	if out, err := cmd.CombinedOutput(); err != nil {
		o.t.Fatalf("wtype %v: %v\n%s", args, err, out)
	}
	for deadline := time.Now().Add(8 * time.Second); o.keyReleases() <= n && time.Now().Before(deadline); {
		time.Sleep(50 * time.Millisecond)
	}
	o.settle(1200 * time.Millisecond)
	spoken := o.utterances()[before:]
	if len(spoken) > 0 && (keyEchoes[spoken[0]] || spoken[0] == key) {
		spoken = spoken[1:]
	}
	if m := focusMoved.FindAllStringSubmatch(o.logFrom(logFrom), -1); len(m) > 0 {
		o.focus = m[len(m)-1][1]
	}
	o.steps = append(o.steps, orcaStep{surface, chord, spoken, o.focus})
	o.t.Logf("%-16s %-12s %s   [focus %s]", surface, chord, strings.Join(spoken, " ¦ "), o.focus)
	// What no key may be answered with, on any surface: one thing said twice
	// in a row (a name and the text it is made of), or a flood (a control
	// whose value Orca reads as often as it changes).
	for i := 1; i < len(spoken); i++ {
		if spoken[i] == spoken[i-1] {
			o.t.Errorf("%s, %s: %q is said twice in a row: %q", surface, chord, spoken[i], spoken)
		}
	}
	if len(spoken) > maxUtterances {
		o.t.Errorf("%s, %s: Orca says %d things for one key: %q", surface, chord, len(spoken), spoken)
	}
	return spoken
}

// maxUtterances is the most one key may be answered with. The longest answer
// heard, a queue row that takes the place of a removed one, is a dozen.
const maxUtterances = 16

// want fails unless every phrase is in some utterance of spoken.
func (o *orca) want(surface, chord string, spoken []string, phrases ...string) {
	o.t.Helper()
	for _, p := range phrases {
		if !slices.ContainsFunc(spoken, func(u string) bool { return strings.Contains(u, p) }) {
			o.t.Errorf("%s, %s: Orca did not say %q: %q", surface, chord, p, spoken)
		}
	}
}

// heard is everything Orca said from step n on.
func (o *orca) heard(n int) []string {
	var all []string
	for _, s := range o.steps[n:] {
		all = append(all, s.spoken...)
	}
	return all
}

// wantOrder fails unless the utterances, from step n on, include each of
// names as a whole utterance, in this order.
func (o *orca) wantOrder(surface string, n int, names ...string) {
	o.t.Helper()
	all := o.heard(n)
	at := 0
	for _, u := range all {
		if at < len(names) && u == names[at] {
			at++
		}
	}
	if at < len(names) {
		o.t.Errorf("%s: Orca never said %q after %q in %q", surface, names[at], names[:at], all)
	}
}

// wantFocus fails unless Orca last put the focus on an element whose
// "role: 'name'" is this.
func (o *orca) wantFocus(surface, chord, want string) {
	o.t.Helper()
	if o.focus != want {
		o.t.Errorf("%s, %s: the focus is on [%s], want [%s]", surface, chord, o.focus, want)
	}
}

// wantOnce fails unless exactly one utterance of spoken contains phrase.
func (o *orca) wantOnce(surface, chord string, spoken []string, phrase string) {
	o.t.Helper()
	n := 0
	for _, u := range spoken {
		if strings.Contains(u, phrase) {
			n++
		}
	}
	if n != 1 {
		o.t.Errorf("%s, %s: Orca said %q %d times, want once: %q", surface, chord, phrase, n, spoken)
	}
}

// wantSilence fails if Orca says anything in the next d: what is focused
// must not be read again as the app changes under it.
func (o *orca) wantSilence(surface string, d time.Duration) {
	o.t.Helper()
	n := len(o.utterances())
	time.Sleep(d)
	if after := o.utterances(); len(after) > n {
		o.t.Errorf("%s: Orca goes on speaking with no key pressed: %q", surface, after[n:])
	}
}

// tabTo presses chord until what Orca spoke contains want (a substring of one
// utterance), at most limit presses, and returns what the last press said.
func (o *orca) tabTo(surface, chord, want string, limit int) []string {
	o.t.Helper()
	for range limit {
		spoken := o.press(surface, chord)
		if strings.Contains(strings.Join(spoken, "\n"), want) {
			return spoken
		}
	}
	o.t.Fatalf("%s pressed %d times and Orca never said %q", chord, limit, want)
	return nil
}

// transcript writes what was heard, per surface, as Markdown.
func (o *orca) transcript(name string) {
	var b strings.Builder
	last := ""
	for _, s := range o.steps {
		if s.surface != last {
			fmt.Fprintf(&b, "\n### %s\n\n", s.surface)
			last = s.surface
		}
		fmt.Fprintf(&b, "- `%s`: %s (focus: %s)\n", s.keys, strings.Join(s.spoken, " ¦ "), s.focus)
	}
	_ = os.WriteFile(filepath.Join(outDir(o.t), name), []byte(b.String()), 0o644)
}
