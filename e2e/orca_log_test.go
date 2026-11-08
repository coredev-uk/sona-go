//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// orcaLogSample is a piece of an Orca debug file as the walk reads it: time
// stamped entries (some of several lines), the untimestamped blocks Orca
// writes for a key and for each event it processes, utterances (one of them
// of two lines), and the lines the walk counts. Orca logs a time-stamped line
// after every utterance, and the sample does too, but for the last one.
func orcaLogSample() string {
	var b strings.Builder
	n := 0
	entry := func(format string, args ...any) {
		n++
		fmt.Fprintf(&b, "00:39:%02d.%06d - %s\n", n/1000000, n%1000000, fmt.Sprintf(format, args...))
	}
	voice := "{'established': False, 'family': {'name': '', 'gender': None, 'lang': 'C', 'dialect': '', 'variant': None}}"
	entry("INFO: Preparing to launch.")
	entry("ORCA: Starting Atspi main event loop")
	entry("SPEECH OUTPUT: 'Screen reader on.' {'established': False}")
	entry("SPEECH DISPATCHER: Speaking 'Screen reader on.' as string")
	for i := range 12 {
		b.WriteString("\nKEYBOARD_EVENT:  type=ATSPI_KEY_PRESSED_EVENT\n                 id=65289\n                 text='Tab'\n\n")
		b.WriteString("vvvvv PROCESS ATSPI_KEY_PRESSED_EVENT: 'Tab' (9) vvvvv\n")
		entry("SCRIPT: dev.coredev.Sona (module=orca.scripts.toolkits.gtk.script)")
		entry("SPEECH OUTPUT: 'Tab' %s", voice)
		entry("SPEECH DISPATCHER: Speaking '<speak><mark name=\"0:3\"/>Tab</speak>' \n                  ORCA rate 50.0, pitch 5.0, volume 10.0\n                  SD rate (client not available)")
		b.WriteString("^^^^^ PROCESS ATSPI_KEY_PRESSED_EVENT: 'Tab' (9) ^^^^^\n\n")
		b.WriteString("KEYBOARD_EVENT:  type=ATSPI_KEY_RELEASED_EVENT\n                 id=65289\n\n")
		b.WriteString("vvvvv PROCESS ATSPI_KEY_RELEASED_EVENT: 'Tab' (9) vvvvv\n")
		entry("GSETTINGS REGISTRY: speech/enable using default value = True")
		b.WriteString("^^^^^ PROCESS ATSPI_KEY_RELEASED_EVENT: 'Tab' (9) ^^^^^\n\n\n")
		entry("EVENT MANAGER: Queueing object:state-changed:focused for [link: 'For you' (0x7f)]  in dev.coredev.Sona")
		entry("EVENT MANAGER: Queueing object:property-change:accessible-value for [slider: 'Seek' (0x7f)]  in dev.coredev.Sona")
		b.WriteString("\nvvvvv START PRIORITY-3 OBJECT EVENT OBJECT:STATE-CHANGED:FOCUSED (queue size: 1) vvvvv\n")
		entry("EVENT SOURCE:\n                  app='dev.coredev.Sona (gtk 3.24.52)' name='For you' role='link'\n                  states='enabled, focusable, focused'\n                  path=[-1, 0, 0, 0, 0, 1]\n                  ")
		entry("SPEECH OUTPUT: 'Main' %s", voice)
		if i == 5 {
			// An utterance with a line break in it.
			entry("SPEECH OUTPUT: 'Up next:\nDust on the Dial' %s", voice)
		}
		entry("SPEECH OUTPUT: 'link.' {'established': False}")
		entry("SPEECH DISPATCHER: Speaking 'link.' as string")
		b.WriteString("^^^^^ FINISHED PRIORITY-3 OBJECT EVENT OBJECT:STATE-CHANGED:FOCUSED ^^^^^\n\n")
		b.WriteString("vvvvv START PRIORITY-4 OBJECT EVENT OBJECT:PROPERTY-CHANGE:ACCESSIBLE-VALUE (queue size: 0) vvvvv\n")
		entry("SPEECH PRESENTER: Adjusting 'x' from None start_offset: None")
		b.WriteString("^^^^^ FINISHED PRIORITY-4 OBJECT EVENT OBJECT:PROPERTY-CHANGE:ACCESSIBLE-VALUE ^^^^^\n\n")
	}
	entry("SPEECH OUTPUT: 'Now playing: The Long Way Round' None")
	return b.String()
}

// fullParse is what the walk did before: split the whole file into entries
// and take the utterances.
func fullParse(b []byte) []string {
	var out []string
	for _, e := range logEntry.Split(string(b), -1) {
		if m := speech.FindStringSubmatch(e); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}

func (l *orcaLog) counts() [4]int { return [4]int{l.releases, l.queued, l.finished, len(l.said)} }

func TestOrcaLogIncremental(t *testing.T) {
	log := []byte(orcaLogSample())
	want := fullParse(log)
	wantCounts := [4]int{
		bytes.Count(log, []byte("type=ATSPI_KEY_RELEASED_EVENT")),
		bytes.Count(log, []byte("EVENT MANAGER: Queueing ")),
		bytes.Count(log, []byte("^^^^^ FINISHED PRIORITY-")),
		len(want),
	}
	if wantCounts != [4]int{12, 24, 24, len(want)} || len(want) != 39 || !slices.Contains(want, "Up next:\nDust on the Dial") {
		t.Fatalf("the sample is not what the test assumes: %v, %d utterances", wantCounts, len(want))
	}

	check := func(t *testing.T, l *orcaLog, what string) {
		t.Helper()
		if !slices.Equal(l.said, want) {
			t.Errorf("%s: utterances differ:\n got %q\nwant %q", what, l.said, want)
		}
		if l.counts() != wantCounts {
			t.Errorf("%s: releases, queued, finished, utterances = %v, want %v", what, l.counts(), wantCounts)
		}
		if !l.started {
			t.Errorf("%s: the main loop was not seen", what)
		}
	}

	t.Run("whole", func(t *testing.T) {
		l := &orcaLog{}
		l.feed(log)
		check(t, l, "one chunk")
	})
	t.Run("a byte at a time", func(t *testing.T) {
		l := &orcaLog{}
		for i := range log {
			l.feed(log[i : i+1])
		}
		check(t, l, "bytes")
	})
	t.Run("random chunks", func(t *testing.T) {
		rng := rand.New(rand.NewPCG(1, 2))
		for run := range 200 {
			l := &orcaLog{}
			for rest := log; len(rest) > 0; {
				n := min(len(rest), 1+rng.IntN(1+rng.IntN(400)))
				l.feed(rest[:n])
				rest = rest[n:]
			}
			check(t, l, fmt.Sprintf("run %d", run))
		}
	})
	t.Run("a partial entry waits", func(t *testing.T) {
		l := &orcaLog{}
		l.feed(log[:len(log)-1]) // the last utterance without its line break
		if got := len(l.said); got != len(want)-1 {
			t.Fatalf("%d utterances before the last line is finished, want %d", got, len(want)-1)
		}
		l.feed(log[len(log)-1:])
		check(t, l, "finished")
		// An utterance cut inside its voice is not one yet.
		l = &orcaLog{}
		l.feed([]byte("00:00:01.000001 - SPEECH OUTPUT: 'Tab' {'established': False, 'fam"))
		l.feed([]byte("ily': {'name': ''}}\n"))
		if !slices.Equal(l.said, []string{"Tab"}) {
			t.Errorf("said %q, want just Tab", l.said)
		}
	})
}

// A value tick (the Seek slider moving as the song plays) is not Orca still
// answering a key; a focus change is.
func TestOrcaLogActivity(t *testing.T) {
	l := &orcaLog{}
	l.feed([]byte("00:00:01.000001 - EVENT MANAGER: Queueing object:property-change:accessible-value for [slider: 'Seek']\n" +
		"^^^^^ FINISHED PRIORITY-4 OBJECT EVENT OBJECT:PROPERTY-CHANGE:ACCESSIBLE-VALUE ^^^^^\n"))
	if !l.activeAt.IsZero() || l.queued != 1 || l.finished != 1 {
		t.Fatalf("a value tick counts as activity: %v, queued %d, finished %d", l.activeAt, l.queued, l.finished)
	}
	l.feed([]byte("00:00:01.000002 - EVENT MANAGER: Queueing object:state-changed:focused for [link: 'For you']\n"))
	if l.activeAt.IsZero() || l.queued != 2 || l.finished != 1 {
		t.Fatalf("a focus change is not activity: %v, queued %d, finished %d", l.activeAt, l.queued, l.finished)
	}
}
