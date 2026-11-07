//go:build e2e

package e2e

import (
	"bytes"
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
	log  string   // Orca's debug file
	rd   *orcaLog // what Orca has logged so far
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

// What the walk reads from Orca's log besides what it says.
var (
	markMainLoop = []byte("ORCA: Starting Atspi main event loop")
	markRelease  = []byte("type=ATSPI_KEY_RELEASED_EVENT")
	markQueueing = []byte("EVENT MANAGER: Queueing ")
	markFinished = []byte("^^^^^ FINISHED PRIORITY-")
	// A value that moves as the song plays (the Seek slider, its time label)
	// is queued and finished several times a second: not a sign that Orca is
	// still answering a key.
	markTick     = []byte("object:property-change:")
	markTickDone = []byte("OBJECT EVENT OBJECT:PROPERTY-CHANGE:")
)

// orcaLog is Orca's debug file read as it grows. poll reads only what was
// appended since the last poll, and each complete entry is parsed once (the
// file is several MB by the end of the walk: reading and splitting all of it
// on every poll cost more than Orca's own answer to a key). An entry still
// being written is kept until it is finished: an entry is finished when the
// next one starts, or when it is a whole utterance ending its line.
type orcaLog struct {
	path string
	f    *os.File
	buf  []byte // what a read fills

	data    []byte // the file so far
	lineAt  int    // data[lineAt:] holds no complete line
	entryAt int    // data[entryAt:] starts at the first entry not parsed yet

	said     []string  // every utterance, in order
	releases int       // key releases Orca has been told about
	queued   int       // events Orca has queued
	finished int       // events Orca has finished processing
	started  bool      // Orca's main loop is running
	activeAt time.Time // when Orca last said or queued something (but a value tick)
}

// close closes the file.
func (l *orcaLog) close() {
	if l.f != nil {
		_ = l.f.Close()
	}
}

// poll reads what has been appended to the file and parses it.
func (l *orcaLog) poll() {
	if l.f == nil {
		f, err := os.Open(l.path)
		if err != nil {
			return // Orca has not created it yet
		}
		l.f = f
		l.buf = make([]byte, 64<<10)
	}
	for {
		n, err := l.f.Read(l.buf)
		l.data = append(l.data, l.buf[:n]...)
		if n == 0 || err != nil {
			break
		}
	}
	l.scan()
}

// feed appends bytes as if they were read from the file (the unit test writes
// a log in chunks).
func (l *orcaLog) feed(b []byte) {
	l.data = append(l.data, b...)
	l.scan()
}

// scan parses what is new in data.
func (l *orcaLog) scan() {
	now := time.Now()
	for {
		i := bytes.IndexByte(l.data[l.lineAt:], '\n')
		if i < 0 {
			break
		}
		l.count(l.data[l.lineAt:l.lineAt+i], now)
		l.lineAt += i + 1
	}
	// An utterance is one entry: its text up to the next time stamp. The
	// last entry is the one that may still be written; it counts once it is a
	// whole utterance on a finished line (Orca logs a time-stamped line right
	// after each utterance, so no later line is part of it).
	base := l.entryAt
	locs := logEntry.FindAllIndex(l.data[base:], -1)
	for i, loc := range locs {
		if i+1 < len(locs) {
			l.utter(l.data[base+loc[1]:base+locs[i+1][0]], now)
			continue
		}
		l.entryAt = base + loc[0]
		if l.data[len(l.data)-1] == '\n' && l.utter(l.data[base+loc[1]:], now) {
			l.entryAt = len(l.data)
		}
	}
}

// utter records the utterance an entry's text is, if it is one.
func (l *orcaLog) utter(entry []byte, now time.Time) bool {
	m := speech.FindSubmatch(entry)
	if m == nil {
		return false
	}
	l.said = append(l.said, string(m[1]))
	l.activeAt = now
	return true
}

// count reads one complete line.
func (l *orcaLog) count(line []byte, now time.Time) {
	switch {
	case bytes.Contains(line, markMainLoop):
		l.started = true
	case bytes.Contains(line, markRelease):
		l.releases++
	default:
		if i := bytes.Index(line, markQueueing); i >= 0 {
			l.queued++
			if !bytes.HasPrefix(line[i+len(markQueueing):], markTick) {
				l.activeAt = now
			}
		} else if bytes.Contains(line, markFinished) {
			l.finished++
			if !bytes.Contains(line, markTickDone) {
				l.activeAt = now
			}
		}
	}
}

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
	o.rd = &orcaLog{path: o.log}
	t.Cleanup(o.rd.close)
	if err := o.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() { _ = o.cmd.Wait(); close(o.done) }()
	t.Cleanup(func() {
		_ = o.cmd.Process.Kill()
		<-o.done
	})
	for deadline := time.Now().Add(30 * time.Second); len(o.rd.said) == 0; time.Sleep(pollEvery) {
		o.rd.poll()
		if len(o.rd.said) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for Orca to greet the window")
		}
	}
	// Orca's main loop is the last thing it starts, after the greeting; an
	// Orca that does not log it is not waited for.
	for deadline := time.Now().Add(5 * time.Second); !o.rd.started && time.Now().Before(deadline); time.Sleep(pollEvery) {
		o.rd.poll()
	}
	o.settle(settleQuiet)
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

// Waiting for Orca. The walk reads Orca's log every pollEvery. A key's
// answer is complete when Orca has said nothing and queued nothing (but a
// value tick, a slider moving as the song plays) for settleQuiet, and has
// finished every event it queued. On the CI runner the longest silence inside
// one key's answer, counting the one between the key's release and the first
// thing Orca did for it, is 0.15 s, and no answer took more than 0.34 s from
// the release to its last word (97 keys of the walk, 16 of the setup).
const (
	pollEvery   = 10 * time.Millisecond
	settleQuiet = 800 * time.Millisecond
)

// settle waits until Orca is done with what it was sent: its event queue is
// drained and it has said or queued nothing for quiet, at most 15 s. The
// quiet time counts from now at the earliest.
func (o *orca) settle(quiet time.Duration) {
	l := o.rd
	l.poll()
	l.activeAt = time.Now()
	finished, progressAt := l.finished, time.Now()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(pollEvery) {
		l.poll()
		if l.finished != finished || l.queued <= l.finished {
			finished, progressAt = l.finished, time.Now()
		} else if time.Since(progressAt) > lostEvent {
			// An event Orca logged as queued and never as finished (it
			// dropped it): stop waiting for it.
			o.t.Logf("Orca queued %d event(s) it never finished: not waiting for them", l.queued-l.finished)
			l.queued = l.finished
		}
		if l.queued <= l.finished && time.Since(l.activeAt) >= quiet {
			return
		}
	}
}

// lostEvent is how long a queued event may go unfinished, with Orca finishing
// nothing else, before settle takes it for dropped.
const lostEvent = 3 * time.Second

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
	o.rd.poll()
	before, n, from := len(o.rd.said), o.rd.releases, len(o.rd.data)
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
	for deadline := time.Now().Add(8 * time.Second); ; time.Sleep(pollEvery) {
		o.rd.poll()
		if o.rd.releases > n {
			break
		}
		if time.Now().After(deadline) {
			o.t.Logf("%s, %s: Orca never heard the key released", surface, chord)
			break
		}
	}
	o.settle(settleQuiet)
	spoken := slices.Clone(o.rd.said[before:])
	if len(spoken) > 0 && (keyEchoes[spoken[0]] || spoken[0] == key) {
		spoken = spoken[1:]
	}
	if m := focusMoved.FindAllSubmatch(o.rd.data[from:], -1); len(m) > 0 {
		o.focus = string(m[len(m)-1][1])
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
	o.rd.poll()
	n := len(o.rd.said)
	for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(pollEvery) {
		o.rd.poll()
	}
	o.rd.poll()
	if after := o.rd.said; len(after) > n {
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
