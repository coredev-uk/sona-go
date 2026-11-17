package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// streamTransport answers calls from results and streams the statuses sent
// on statuses until end is closed (Sona quits).
type streamTransport struct {
	mu       sync.Mutex
	results  map[string]any
	calls    []Request
	statuses chan Status
	end      chan struct{}
}

func (f *streamTransport) Call(_ context.Context, req Request) (json.RawMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	return json.Marshal(f.results[req.Command])
}

func (f *streamTransport) Follow(ctx context.Context, each func(Status) error) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.end:
			return nil
		case s := <-f.statuses:
			if err := each(s); err != nil {
				return err
			}
		}
	}
}

func (f *streamTransport) Close() error { return nil }

func (f *streamTransport) called() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.calls...)
}

// connected runs `sona connect` over pipes: send writes a frame; next
// returns the first frame read (now or later) that a filter keeps, and
// leaves the others for later calls.
type connected struct {
	t       *testing.T
	in      *io.PipeWriter
	frames  chan Frame
	pending []Frame
	done    chan int
}

func runConnect(t *testing.T, tr Transport) *connected {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := &connected{t: t, in: inW, frames: make(chan Frame, 64), done: make(chan int, 1)}
	go func() {
		c.done <- Run(t.Context(), []string{"connect"}, Env{
			Stdin: inR, Stdout: outW, Stderr: io.Discard, Dial: func() (Transport, error) { return tr, nil },
		})
		_ = outW.Close()
	}()
	go func() {
		defer close(c.frames)
		r := bufio.NewReader(outR)
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				return
			}
			var f Frame
			if json.Unmarshal(line, &f) == nil {
				c.frames <- f
			}
		}
	}()
	t.Cleanup(func() { _ = inW.Close() })
	return c
}

func (c *connected) send(frame string) {
	c.t.Helper()
	if _, err := io.WriteString(c.in, frame+"\n"); err != nil {
		c.t.Fatal(err)
	}
}

func (c *connected) next(keep func(Frame) bool) Frame {
	c.t.Helper()
	for i, f := range c.pending {
		if keep(f) {
			c.pending = append(c.pending[:i], c.pending[i+1:]...)
			return f
		}
	}
	timeout := time.After(5 * time.Second)
	for {
		select {
		case f, ok := <-c.frames:
			if !ok {
				c.t.Fatal("connect ended")
			}
			if keep(f) {
				return f
			}
			c.pending = append(c.pending, f)
		case <-timeout:
			c.t.Fatal("no frame")
		}
	}
}

func byID(id int64) func(Frame) bool { return func(f Frame) bool { return f.ID == id } }

func event(name string) func(Frame) bool { return func(f Frame) bool { return f.Event == name } }

func TestConnectServesThePersonsProgram(t *testing.T) {
	playing := Status{Running: true, State: StatePlaying, Track: &Track{ID: "1", Title: "Song"}, Position: 3, Volume: 40, Repeat: "off", Private: true}
	tr := &streamTransport{
		results: map[string]any{
			"queue":   Queue{Index: 0, Tracks: []QueueTrack{{Track: *playing.Track}}},
			"private": Private{On: false, Source: SourceSetting},
			"like":    Like{Track: *playing.Track, Liked: true},
		},
		statuses: make(chan Status, 4), end: make(chan struct{}),
	}
	c := runConnect(t, tr)
	// Any ID, every permission but network.
	c.send(`{"id":1,"method":"hello","params":{"id":"streamdeck","protocol":{"min":"2.0","max":"2.0"},"permissions":["playback.read","playback.control","queue.read","like","private.read","network"]}}`)
	var hello HelloResult
	if f := c.next(byID(1)); f.Error != nil || json.Unmarshal(f.Result, &hello) != nil {
		t.Fatalf("hello: %+v", f)
	}
	if strings.Join(hello.Permissions, ",") != "playback.read,playback.control,queue.read,like,private.read" {
		t.Errorf("granted %v", hello.Permissions)
	}
	c.send(`{"id":2,"method":"subscribe","params":{"topics":["status","now-playing","queue"]}}`)
	c.next(byID(2))
	tr.statuses <- playing
	// The owner sees a private listen: the status and what plays, as the
	// command line does.
	st := c.next(event(TopicStatus))
	if !strings.Contains(string(st.Data), `"private":true`) || !strings.Contains(string(st.Data), `"title":"Song"`) {
		t.Errorf("status event %s", st.Data)
	}
	var np NowPlaying
	_ = json.Unmarshal(c.next(event(TopicNowPlaying)).Data, &np)
	if np.Track == nil || np.Track.ID != "1" || time.Since(time.UnixMilli(np.StartedAt)) < 3*time.Second {
		t.Errorf("now playing %+v", np)
	}
	if q := c.next(event(TopicQueue)); !strings.Contains(string(q.Data), `"index":0`) {
		t.Errorf("queue event %s", q.Data)
	}
	// Calls go to the running Sona; private listening may be switched.
	c.send(`{"id":3,"method":"call","params":{"command":"like","args":["toggle"]}}`)
	if f := c.next(byID(3)); f.Error != nil || !strings.Contains(string(f.Result), `"liked":true`) {
		t.Errorf("like: %+v %s", f.Error, f.Result)
	}
	c.send(`{"id":4,"method":"call","params":{"command":"private","args":["off"]}}`)
	if f := c.next(byID(4)); f.Error != nil {
		t.Errorf("private off: %+v", f.Error)
	}
	// Not the person's terminal commands.
	c.send(`{"id":5,"method":"call","params":{"command":"addons","args":["dev","/x"]}}`)
	if f := c.next(byID(5)); f.Error == nil || f.Error.Code != CodeUnknownCommand {
		t.Errorf("addons: %+v", f)
	}
	var sent []string
	for _, r := range tr.called() {
		sent = append(sent, r.Command+" "+strings.Join(r.Args, " "))
	}
	if !strings.Contains(strings.Join(sent, ";"), "like toggle;private off") {
		t.Errorf("Sona got %q", sent)
	}
	// Sona quits: the program hears shutdown, and connect exits with 3.
	close(tr.end)
	c.next(event(EventShutdown))
	select {
	case code := <-c.done:
		if code != ExitNotRunning {
			t.Errorf("exit %d after Sona quit", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connect kept running after Sona quit")
	}
}

func TestConnectEndsWhenTheProgramHangsUp(t *testing.T) {
	tr := &streamTransport{statuses: make(chan Status), end: make(chan struct{})}
	c := runConnect(t, tr)
	_ = c.in.Close()
	select {
	case code := <-c.done:
		if code != ExitOK {
			t.Errorf("exit %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("connect kept running with its input closed")
	}
}

func TestConnectNeedsSona(t *testing.T) {
	var out strings.Builder
	code := Run(t.Context(), []string{"connect"}, Env{
		Stdin: strings.NewReader(""), Stdout: &out, Stderr: io.Discard, Dial: func() (Transport, error) { return nil, ErrNotRunning },
	})
	if code != ExitNotRunning || out.Len() != 0 {
		t.Errorf("exit %d, stdout %q", code, out.String())
	}
}
