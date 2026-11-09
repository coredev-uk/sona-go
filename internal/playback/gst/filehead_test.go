//go:build linux || (cgo && (darwin || windows))

package gst

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Each upload format reports its duration once prerolled, from the bytes
// fed so far and the file size.
func TestFilePipelineDuration(t *testing.T) {
	for _, c := range []struct {
		file string
		dur  time.Duration
	}{{"sine2s.mp3", 2069 * time.Millisecond}, {"sine2s.m4a", 2027 * time.Millisecond}, {"sine2s-alac.m4a", 2183 * time.Millisecond}} {
		data, err := os.ReadFile("testdata/" + c.file)
		if err != nil {
			t.Fatal(err)
		}
		p, err := NewFile(Options{Sink: "fakesink"}, int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Feed(0, data); err != nil {
			t.Fatal(err)
		}
		p.Complete()
		if err := p.Pause(); err != nil {
			t.Fatal(err)
		}
		var d time.Duration
		var ok bool
		for deadline := time.Now().Add(3 * time.Second); !ok && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			d, ok = p.Duration()
		}
		p.Close()
		if diff := d - c.dur; !ok || diff < -60*time.Millisecond || diff > 60*time.Millisecond {
			t.Errorf("%s: duration %v (%v), want ≈%v", c.file, d, ok, c.dur)
		}
	}
}

// nullSinks creates PipeWire null sinks named after the test process and
// removes them afterwards; it skips without PipeWire, its GStreamer plugin
// or pw-cli.
func nullSinks(t *testing.T, n int) []string {
	t.Helper()
	if _, err := os.Stat(filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "pipewire-0")); err != nil {
		t.Skip("no PipeWire daemon")
	}
	if !Have("pipewiresink") {
		t.Skip("no PipeWire GStreamer plugin")
	}
	pwcli, err := exec.LookPath("pw-cli")
	if err != nil {
		t.Skip("no pw-cli")
	}
	var names []string
	for i := range n {
		name := fmt.Sprintf("sona-test-%d-%d", os.Getpid(), i)
		out, err := exec.Command(pwcli, "create-node", "adapter",
			`{ factory.name=support.null-audio-sink node.name=`+name+` media.class=Audio/Sink object.linger=true audio.position=[FL FR] }`).CombinedOutput()
		if err != nil {
			t.Fatalf("pw-cli create-node: %v: %s", err, out)
		}
		t.Cleanup(func() { _ = exec.Command(pwcli, "destroy", name).Run() })
		names = append(names, name)
	}
	return names
}

// within runs f, failing the test if it takes longer than d (a deadlock in
// the sink: the test cannot recover from it, so it stops there).
func within(t *testing.T, d time.Duration, what string, f func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	case <-time.After(d):
		t.Fatalf("%s: still blocked after %v", what, d)
	}
}

// The output moves to another PipeWire sink at once in every state of an
// upload: playing, paused after playing, ended (also with nothing rendered:
// a seek past the end) and not yet played. Playback then carries on from
// where it was, and the pipeline closes. pipewiresink deadlocks when a
// stream it connected is stopped or flushed before rendering, so this
// needs the real sink.
func TestFilePipelineMovesOutputInEveryState(t *testing.T) {
	sinks := nullSinks(t, 2)
	data, err := os.ReadFile("testdata/sine2s.m4a")
	if err != nil {
		t.Fatal(err)
	}
	const bound = 3 * time.Second
	for _, c := range []struct {
		name string
		// before brings the pipeline to the state under test; it returns
		// the position playback resumes from after the move.
		before func(t *testing.T, p *FilePipeline) time.Duration
		seek   bool // seek to 0.5 s after the move (the stream had ended or not started)
	}{
		{"playing", func(t *testing.T, p *FilePipeline) time.Duration {
			within(t, bound, "play", p.Play)
			time.Sleep(300 * time.Millisecond)
			pos, _ := p.Position()
			return pos
		}, false},
		{"paused", func(t *testing.T, p *FilePipeline) time.Duration {
			within(t, bound, "play", p.Play)
			time.Sleep(500 * time.Millisecond)
			within(t, bound, "pause", p.Pause)
			pos, _ := p.Position()
			return pos
		}, false},
		{"ended", func(t *testing.T, p *FilePipeline) time.Duration {
			within(t, bound, "play", p.Play)
			waitFileEvent(t, p, EventEOS)
			return 500 * time.Millisecond
		}, true},
		{"ended unrendered", func(t *testing.T, p *FilePipeline) time.Duration {
			within(t, bound, "seek past the end", func() error { return p.Seek(20 * time.Second) })
			within(t, bound, "play", p.Play)
			waitFileEvent(t, p, EventEOS)
			return 500 * time.Millisecond
		}, true},
		{"paused at the end", func(t *testing.T, p *FilePipeline) time.Duration {
			within(t, bound, "play", p.Play)
			waitFileEvent(t, p, EventEOS)
			within(t, bound, "pause", p.Pause)
			return 500 * time.Millisecond
		}, true},
		{"not played", func(t *testing.T, p *FilePipeline) time.Duration {
			return 500 * time.Millisecond
		}, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			p, err := NewFile(Options{Sink: "pipewiresink", Device: sinks[0]}, int64(len(data)))
			if err != nil {
				t.Fatal(err)
			}
			defer within(t, bound, "close", func() error { p.Close(); return nil })
			if err := p.Feed(0, data); err != nil {
				t.Fatal(err)
			}
			p.Complete()
			within(t, bound, "pause", p.Pause)
			waitFileEvent(t, p, EventAsyncDone)
			from := c.before(t, p)
			within(t, bound, "move", func() error { return p.SetDevice(sinks[1], "") })
			if c.seek {
				within(t, bound, "seek", func() error { return p.Seek(from) })
			}
			within(t, bound, "play", p.Play)
			time.Sleep(400 * time.Millisecond)
			pos, ok := p.Position()
			if want := from + 400*time.Millisecond; !ok || pos < want-200*time.Millisecond || pos > want+200*time.Millisecond {
				t.Errorf("after the move playing at %v (%v), want ≈%v", pos, ok, want)
			}
		})
	}
}

func waitFileEvent(t *testing.T, p *FilePipeline, kind EventKind) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		select {
		case ev := <-p.Events():
			if ev.Kind == EventError {
				t.Fatal(ev.Err)
			}
			if ev.Kind == kind {
				return
			}
		case <-deadline:
			t.Fatalf("no event %v", kind)
		}
	}
}
