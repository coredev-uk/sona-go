package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLineAtPosition(t *testing.T) {
	// Three lines close together, one after a long rest, and a last one.
	song := []LyricLine{
		{Start: 10, End: 11.8, Text: "one"},
		{Start: 12, End: 13.8, Text: "two"},
		{Start: 14, End: 15.8, Text: "three"},
		{Start: 25, End: 27, Text: "four"},
		{Start: 40, End: 42, Text: "five"},
	}
	inf := math.Inf(1)
	for _, tc := range []struct {
		name  string
		lines []LyricLine
		pos   float64
		want  lineAt
	}{
		{"no lines", nil, 5, lineAt{-1, -1, inf}},
		{"before the first", song, 0, lineAt{-1, 0, 9.75}},
		{"negative", song, -3, lineAt{-1, 0, 9.75}},
		{"just before the lead", song, 9.74, lineAt{-1, 0, 9.75}},
		{"the first, a quarter second early", song, 9.75, lineAt{0, 1, 11.75}},
		{"in the first", song, 10.5, lineAt{0, 1, 11.75}},
		{"the second", song, 11.75, lineAt{1, 2, 13.75}},
		{"the line stays through a short gap", song, 11.9, lineAt{1, 2, 13.75}},
		{"the third, a rest follows", song, 13.75, lineAt{2, 3, 16.05}},
		{"still lit in the grace after its end", song, 16.0, lineAt{2, 3, 16.05}},
		{"resting", song, 16.05, lineAt{-1, 3, 24.75}},
		{"resting, a long while", song, 20, lineAt{-1, 3, 24.75}},
		{"the fourth", song, 24.75, lineAt{3, 4, 27.25}},
		{"resting before the last", song, 30, lineAt{-1, 4, 39.75}},
		{"the last", song, 41, lineAt{4, -1, 42.25}},
		{"after the last", song, 42.25, lineAt{-1, -1, inf}},
		{"past the end of the song", song, 500, lineAt{-1, -1, inf}},
		{"equal starts: the later line", []LyricLine{{Start: 5, End: 6}, {Start: 5, End: 7}, {Start: 5, End: 8}}, 5, lineAt{2, -1, 8.25}},
		{"equal starts, then one after", []LyricLine{{Start: 5, End: 6}, {Start: 5, End: 7}, {Start: 6, End: 8}}, 4.75, lineAt{1, 2, 5.75}},
		{"lines without an end never rest", []LyricLine{{Start: 10}, {Start: 20}}, 100, lineAt{1, -1, inf}},
		{"unsorted: the line begun last, not the last listed", []LyricLine{{Start: 20, End: 22}, {Start: 10, End: 12}, {Start: 15, End: 16}}, 10.5, lineAt{1, 2, 12.25}},
		{"unsorted: later", []LyricLine{{Start: 20, End: 22}, {Start: 10, End: 12}, {Start: 15, End: 16}}, 19.9, lineAt{0, -1, 22.25}},
		// Word-timed lines span their words (the server widens them), so a
		// line may run on under the next: the line begun last is sung.
		{"overlapping lines", []LyricLine{{Start: 10, End: 14}, {Start: 12, End: 15}}, 12, lineAt{1, -1, 15.25}},
	} {
		got := lineAtPosition(tc.lines, tc.pos)
		if got.Index != tc.want.Index || got.Next != tc.want.Next || math.Abs(got.Until-tc.want.Until) > 1e-9 && !(math.IsInf(got.Until, 1) && math.IsInf(tc.want.Until, 1)) {
			t.Errorf("%s: lineAtPosition(%v) = %+v, want %+v", tc.name, tc.pos, got, tc.want)
		}
	}
}

// The answer holds until the position it says: that position gives another
// one, and a hair before it the same.
func TestLineAtPositionUntil(t *testing.T) {
	lines := []LyricLine{{Start: 1, End: 2}, {Start: 2.5, End: 3}, {Start: 9, End: 11}, {Start: 9, End: 12}, {Start: 30, End: 31}}
	for pos := 0.0; pos < 40; pos += 0.037 {
		at := lineAtPosition(lines, pos)
		if math.IsInf(at.Until, 1) {
			continue
		}
		if at.Until <= pos-0.0005 {
			t.Fatalf("lineAtPosition(%v).Until = %v, not after", pos, at.Until)
		}
		before, after := lineAtPosition(lines, at.Until-0.0011), lineAtPosition(lines, at.Until)
		if before.Index != at.Index || before.Next != at.Next || (after.Index == at.Index && after.Next == at.Next) {
			t.Errorf("at %v (until %v): before %+v, at it %+v; was %+v", pos, at.Until, before, after, at)
		}
	}
}

func TestVerseStarts(t *testing.T) {
	lines := []LyricLine{
		{Start: 1, End: 2, Part: "Verse"},
		{Start: 2, End: 3, Part: "Verse"},
		{Start: 3, End: 4, Part: "Chorus"}, // the part changes
		{Start: 4, End: 5},                 // no part named
		{Start: 9, End: 10},                // a 4 s pause
		{Start: 11, End: 12},               // a short one
		{Start: 20},                        // 8 s, but the line before has no end... it has: 12 → 8 s
	}
	got := verseStarts(lines)
	want := []bool{false, false, true, false, true, false, true}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("verseStarts = %v, want %v", got, want)
			break
		}
	}
	// Plain lyrics have no times: only the parts break them.
	plain := []LyricLine{{Text: "a", Part: "A"}, {Text: "b", Part: "A"}, {Text: "c", Part: "B"}}
	if got := verseStarts(plain); got[0] || got[1] || !got[2] {
		t.Errorf("plain verseStarts = %v", got)
	}
}

var lyricsSong = &Track{ID: "1441164430", Title: "Come Together", Artist: "The Beatles", Album: "Abbey Road", Duration: 259.9}

// timedLyrics is a short song: two verses, one line with its localizations.
func timedLyrics() Lyrics {
	return Lyrics{
		Track: *lyricsSong, State: StatePlaying, Position: 12.5, Provider: "apple", Timing: "word", Synced: true,
		Lines: []LyricLine{
			{Start: 10, End: 11.8, Text: "Here come old flat top", Agent: "v1", Part: "Verse"},
			{Start: 12.34, End: 13.8, Text: "He come groovin' up slowly", Background: "(oh)", Agent: "v1", Part: "Verse", Translation: "Ahí viene el viejo", Transliteration: "Hia kamu"},
			{Start: 20, End: 21, Text: "", Background: "(ooh)", Part: "Chorus"},
			{Start: 3725.5, End: 3726, Text: "Last\nline"},
		},
	}
}

func plainLyrics() Lyrics {
	return Lyrics{
		Track: *lyricsSong, State: StatePaused, Provider: "musixmatch", Timing: "none", Synced: false,
		Lines: []LyricLine{{Text: "one", Part: "Verse"}, {Text: "two", Part: "Verse"}, {Text: "three", Part: "Chorus"}},
	}
}

func TestRunLyrics(t *testing.T) {
	tr := &fakeTransport{results: map[string]any{"lyrics": timedLyrics()}}
	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"lyrics"}, "Here come old flat top\nHe come groovin' up slowly\n\n(ooh)\n\nLast line\n"},
		{[]string{"lyrics", "--translation"}, "Here come old flat top\nAhí viene el viejo\n\n(ooh)\n\nLast line\n"},
		{[]string{"lyrics", "--transliteration"}, "Here come old flat top\nHia kamu\n\n(ooh)\n\nLast line\n"},
		{[]string{"lyrics", "--lrc"}, "[ti:Come Together]\n[ar:The Beatles]\n[al:Abbey Road]\n[00:10.00]Here come old flat top\n[00:12.34]He come groovin' up slowly\n[00:20.00](ooh)\n[62:05.50]Last line\n"},
		{[]string{"lyrics", "--lrc", "--translation"}, "[ti:Come Together]\n[ar:The Beatles]\n[al:Abbey Road]\n[00:10.00]Here come old flat top\n[00:12.34]Ahí viene el viejo\n[00:20.00](ooh)\n[62:05.50]Last line\n"},
	} {
		code, out, errOut := run(t, tr, tc.argv...)
		if code != ExitOK || out != tc.want || errOut != "" {
			t.Errorf("%q: %d, %q, %q\nwant %q", tc.argv, code, out, errOut, tc.want)
		}
	}
	if len(tr.calls) != 5 || tr.calls[0].Command != "lyrics" || len(tr.calls[0].Args) != 0 {
		t.Errorf("calls = %+v", tr.calls)
	}

	// --json is the reply as received.
	code, out, _ := run(t, tr, "lyrics", "--json")
	var got Lyrics
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != ExitOK || !strings.HasSuffix(out, "\n") || len(got.Lines) != 4 || got.Lines[1].Translation != "Ahí viene el viejo" {
		t.Errorf("lyrics --json: %d, %q, %v", code, out, err)
	}

	// Plain lyrics: lines, a blank between parts; no LRC for them.
	tr.results["lyrics"] = plainLyrics()
	if code, out, errOut := run(t, tr, "lyrics"); code != ExitOK || out != "one\ntwo\n\nthree\n" || errOut != "" {
		t.Errorf("plain: %d, %q, %q", code, out, errOut)
	}
	if code, out, errOut := run(t, tr, "lyrics", "--lrc"); code != ExitFailed || out != "" ||
		errOut != "sona: these lyrics have no timing, so they can't be written as an LRC file; leave out --lrc for the plain text\n" {
		t.Errorf("plain --lrc: %d, %q, %q", code, out, errOut)
	}
}

func TestRunLyricsErrors(t *testing.T) {
	tr := &fakeTransport{errs: map[string]error{"lyrics": &Error{Code: CodeNoLyrics}}}
	if code, out, errOut := run(t, tr, "lyrics"); code != ExitFailed || out != "" || errOut != "sona: this song has no lyrics\n" {
		t.Errorf("no lyrics: %d, %q, %q", code, out, errOut)
	}
	tr.errs["lyrics"] = &Error{Code: CodeNothingPlaying}
	if code, out, errOut := run(t, tr, "lyrics", "--lrc"); code != ExitFailed || out != "" || errOut != "sona: nothing is playing\n" {
		t.Errorf("nothing playing: %d, %q, %q", code, out, errOut)
	}
	tr.errs["lyrics"] = &Error{Code: "network", Message: "no route"}
	if code, _, errOut := run(t, tr, "lyrics"); code != ExitFailed || !strings.Contains(errOut, "couldn't reach the network") {
		t.Errorf("network: %d, %q", code, errOut)
	}
	if code, out, errOut := run(t, nil, "lyrics"); code != ExitNotRunning || out != "" || errOut != "sona: Sona isn't running; start it first\n" {
		t.Errorf("not running: %d, %q, %q", code, out, errOut)
	}
}

func TestParseLyrics(t *testing.T) {
	in, err := parse([]string{"lyrics", "--follow", "--translation", "--format", "{{.text}}"})
	if err != nil || !in.follow || !in.translation || in.format != "{{.text}}" || in.textMode() != showTranslation {
		t.Errorf("parse = %+v, %v", in, err)
	}
	if in, err := parse([]string{"lyrics", "--lrc", "--transliteration"}); err != nil || !in.lrc || in.textMode() != showTransliteration {
		t.Errorf("--lrc --transliteration = %+v, %v", in, err)
	}
	if in, err := parse([]string{"lyrics", "--follow", "--json", "--translation"}); err != nil || !in.json || !in.translation {
		t.Errorf("--follow --json --translation = %+v, %v", in, err)
	}
	if got := mustParse(t, "lyrics", "--follow").request(); got.Command != "lyrics" || len(got.Args) != 0 {
		t.Errorf("request = %+v", got)
	}
	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"lyrics", "now"}, "lyrics: too many arguments"},
		{[]string{"lyrics", "--loud"}, "lyrics: unknown option --loud"},
		{[]string{"lyrics", "--lrc=1"}, "lyrics: --lrc takes no value"},
		{[]string{"lyrics", "--format"}, "lyrics: --format needs a value"},
		{[]string{"lyrics", "--json", "--lrc"}, "lyrics: --lrc and --json can't be used together"},
		{[]string{"lyrics", "--lrc", "--follow"}, "lyrics: --lrc and --follow can't be used together"},
		{[]string{"lyrics", "--lrc", "--follow", "--format", "x"}, "lyrics: --lrc and --follow can't be used together"},
		{[]string{"lyrics", "--follow", "--json", "--format", "x"}, "lyrics: --json and --format can't be used together"},
		{[]string{"lyrics", "--format", "x"}, "lyrics: --format needs --follow"},
		{[]string{"lyrics", "--translation", "--transliteration"}, "lyrics: --translation and --transliteration can't be used together"},
		{[]string{"lyrics", "--json", "--translation"}, "lyrics: --json and --translation can't be used together"},
		{[]string{"lyrics", "--json", "--transliteration"}, "lyrics: --json and --transliteration can't be used together"},
		{[]string{"lyrics", "--follow", "--format", "{{.text"}, "lyrics: --format: "},
	} {
		_, err := parse(tc.argv)
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeUsage || !strings.HasPrefix(e.Message, tc.want) {
			t.Errorf("parse(%q) err = %v, want usage %q…", tc.argv, err, tc.want)
		}
	}
	// The running Sona checks its own side too.
	if err := checkRequest(Request{Command: "lyrics"}); err != nil {
		t.Errorf("checkRequest(lyrics) = %v", err)
	}
	if err := checkRequest(Request{Command: "lyrics", Args: []string{"x"}}); err == nil {
		t.Error("checkRequest accepted an argument")
	}
}

func mustParse(t *testing.T, argv ...string) *invocation {
	t.Helper()
	in, err := parse(argv)
	if err != nil {
		t.Fatal(err)
	}
	return in
}

func TestLyricsSchemas(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want string
	}{
		{Lyrics{}, "lines position provider state synced timing track"},
		{LyricLine{}, "agent background end part start text translation transliteration"},
		{LyricsNow{}, "index line lyrics next position provider running state synced text timing track"},
	} {
		if got := strings.Join(keys(t, tc.v), " "); got != tc.want {
			t.Errorf("%T keys = %q, want %q", tc.v, got, tc.want)
		}
	}
	b, _ := json.Marshal(lyricsGone())
	if string(b) != `{"running":false,"state":"idle","track":null,"position":0,"lyrics":"none","provider":"","timing":"","synced":false,"index":-1,"line":null,"next":null,"text":""}` {
		t.Errorf("gone = %s", b)
	}
}

func TestLRCStamp(t *testing.T) {
	for _, tc := range []struct {
		sec  float64
		want string
	}{
		{0, "[00:00.00]"}, {0.004, "[00:00.00]"}, {0.019, "[00:00.01]"}, {83.4, "[01:23.40]"}, {59.999, "[00:59.99]"},
		{3600, "[60:00.00]"}, {6000.5, "[100:00.50]"}, {-2, "[00:00.00]"},
	} {
		if got := lrcStamp(tc.sec); got != tc.want {
			t.Errorf("lrcStamp(%v) = %q, want %q", tc.sec, got, tc.want)
		}
	}
}

// fakeClock is time that only the test moves.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers map[int]*fakeTimer
	seq    int
}

type fakeTimer struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Unix(1_000_000, 0), timers: map[int]*fakeTimer{}}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) (<-chan time.Time, func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	id := c.seq
	tm := &fakeTimer{at: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers[id] = tm
	return tm.ch, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.timers, id)
	}
}

// advance moves time on by d, firing the timers it reaches.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for id, tm := range c.timers {
		if !tm.at.After(c.now) {
			tm.ch <- c.now
			delete(c.timers, id)
		}
	}
}

// pending is how many timers are set.
func (c *fakeClock) pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// next waits for the one timer the follower sets and moves time on to it.
func (c *fakeClock) next(t *testing.T) time.Duration {
	t.Helper()
	waitFor(t, "a timer", func() bool { return c.pending() == 1 })
	c.mu.Lock()
	var d time.Duration
	for _, tm := range c.timers {
		d = tm.at.Sub(c.now)
	}
	c.mu.Unlock()
	c.advance(d)
	return d
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

// lyricsServer is Sona for `lyrics --follow`: the statuses it sends, the
// lyrics it has by song, and the calls it got.
type lyricsServer struct {
	statuses chan Status
	mu       sync.Mutex
	lyrics   map[string]Lyrics
	errs     map[string]error
	playing  string // the song `lyrics` answers for
	calls    int
	ended    error // what Follow returns once statuses closes
}

func newLyricsServer() *lyricsServer {
	return &lyricsServer{statuses: make(chan Status), lyrics: map[string]Lyrics{}, errs: map[string]error{}}
}

func (s *lyricsServer) Call(_ context.Context, req Request) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if req.Command != "lyrics" {
		return nil, &Error{Code: CodeUnknownCommand}
	}
	if err := s.errs[s.playing]; err != nil {
		return nil, err
	}
	return json.Marshal(s.lyrics[s.playing])
}

func (s *lyricsServer) Follow(ctx context.Context, each func(Status) error) error {
	for {
		select {
		case st, ok := <-s.statuses:
			if !ok {
				return s.ended
			}
			if err := each(st); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *lyricsServer) Close() error { return nil }

func (s *lyricsServer) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// send plays song id: the lyrics are asked for it from now.
func (s *lyricsServer) send(id string, state string, pos float64) {
	s.mu.Lock()
	s.playing = id
	s.mu.Unlock()
	st := Status{Running: true, State: state, Position: pos, Repeat: "off"}
	if id != "" {
		st.Track = &Track{ID: id, Title: "Song " + id, Duration: 100}
	}
	s.statuses <- st
}

// lineSink delivers each line written to it.
type lineSink struct{ lines chan string }

func (w *lineSink) Write(p []byte) (int, error) {
	w.lines <- string(p)
	return len(p), nil
}

// syncBuffer is a buffer the follower writes while the test reads.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func (s *syncBuffer) Len() int { return len(s.String()) }

// follower is a running `lyrics --follow`.
type follower struct {
	srv   *lyricsServer
	clk   *fakeClock
	out   *lineSink
	errs  *syncBuffer
	code  chan int
}

func startFollow(t *testing.T, srv *lyricsServer, argv ...string) *follower {
	t.Helper()
	f := &follower{srv: srv, clk: newFakeClock(), out: &lineSink{make(chan string, 100)}, errs: &syncBuffer{}, code: make(chan int, 1)}
	env := Env{Stdout: f.out, Stderr: f.errs, Clock: f.clk, Dial: func() (Transport, error) { return srv, nil }}
	go func() { f.code <- Run(context.Background(), append([]string{"lyrics", "--follow"}, argv...), env) }()
	return f
}

// line waits for the next line printed.
func (f *follower) line(t *testing.T) string {
	t.Helper()
	select {
	case l := <-f.out.lines:
		return l
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a line")
		return ""
	}
}

func (f *follower) want(t *testing.T, lines ...string) {
	t.Helper()
	for _, want := range lines {
		if got := f.line(t); got != want {
			t.Fatalf("line = %q, want %q", got, want)
		}
	}
}

// quit ends the stream as Sona quitting does and returns the exit status.
func (f *follower) quit(t *testing.T) int {
	t.Helper()
	close(f.srv.statuses)
	select {
	case code := <-f.code:
		return code
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not end")
		return -1
	}
}

func songLyrics(id string, synced bool, lines ...LyricLine) Lyrics {
	l := Lyrics{Track: Track{ID: id, Title: "Song " + id, Duration: 100}, Provider: "apple", Timing: "line", Synced: synced, Lines: lines}
	if !synced {
		l.Timing = "none"
	}
	return l
}

var threeLines = []LyricLine{
	{Start: 10, End: 11.8, Text: "one"},
	{Start: 12, End: 13.8, Text: "two"},
	{Start: 14, End: 15.8, Text: "three"},
}

func TestFollowLyricsLines(t *testing.T) {
	srv := newLyricsServer()
	srv.lyrics["1"] = songLyrics("1", true, threeLines...)
	f := startFollow(t, srv)

	srv.send("1", StatePlaying, 0)
	f.want(t, "\n") // a line now: nothing is sung yet
	// Each change of the line is one line, at the time it begins (a quarter
	// of a second early); the last rests.
	for _, tc := range []struct {
		line string
		at   time.Duration
	}{
		{"one\n", 9750 * time.Millisecond},
		{"two\n", 2000 * time.Millisecond},
		{"three\n", 2000 * time.Millisecond},
		{"\n", 2300 * time.Millisecond},
	} {
		if d := f.clk.next(t); d != tc.at {
			t.Fatalf("before %q the timer was %v, want %v", tc.line, d, tc.at)
		}
		f.want(t, tc.line)
	}
	waitFor(t, "the timer to stop", func() bool { return f.clk.pending() == 0 })
	if n := srv.callCount(); n != 1 {
		t.Errorf("lyrics asked %d times, want once", n)
	}
	if code := f.quit(t); code != ExitOK {
		t.Errorf("exit %d", code)
	}
	f.want(t, "\n") // the last empty line
	if f.errs.Len() != 0 {
		t.Errorf("stderr = %q", f.errs.String())
	}
}

func TestFollowLyricsPauseAndSeek(t *testing.T) {
	srv := newLyricsServer()
	srv.lyrics["1"] = songLyrics("1", true, threeLines...)
	f := startFollow(t, srv)

	srv.send("1", StatePlaying, 10.5)
	f.want(t, "\n", "one\n") // the song's lyrics are fetched first
	waitFor(t, "a timer", func() bool { return f.clk.pending() == 1 })

	// The clock moves a second, then it pauses: no timer runs while paused
	// and time passing prints nothing.
	f.clk.advance(500 * time.Millisecond)
	srv.send("1", StatePaused, 11)
	waitFor(t, "the timer to stop", func() bool { return f.clk.pending() == 0 })
	f.clk.advance(time.Hour)

	// A seek while paused re-syncs at once.
	srv.send("1", StatePaused, 14.5)
	f.want(t, "three\n")
	// Resuming keeps the line and starts the timer to the next boundary
	// (the rest after the last line).
	srv.send("1", StatePlaying, 14.5)
	if d := f.clk.next(t); d != 1550*time.Millisecond {
		t.Errorf("resumed timer = %v, want 1.55s", d)
	}
	f.want(t, "\n")

	// A seek while playing, back to the first line and to before it.
	srv.send("1", StatePlaying, 10.1)
	f.want(t, "one\n")
	srv.send("1", StatePlaying, 2)
	f.want(t, "\n")
	if d := f.clk.next(t); d != 7750*time.Millisecond {
		t.Errorf("timer after the seek = %v, want 7.75s", d)
	}
	f.want(t, "one\n")
	// A status that only moves the position changes nothing.
	srv.send("1", StatePlaying, 10.3)
	srv.send("1", StatePlaying, 10.4)
	if code := f.quit(t); code != ExitOK {
		t.Errorf("exit %d", code)
	}
	f.want(t, "\n")
	select {
	case l := <-f.out.lines:
		t.Errorf("extra line %q", l)
	default:
	}
}

func TestFollowLyricsTrackChange(t *testing.T) {
	srv := newLyricsServer()
	srv.lyrics["1"] = songLyrics("1", true, threeLines...)
	srv.lyrics["2"] = songLyrics("2", true, LyricLine{Start: 1, End: 2, Text: "alpha"})
	srv.errs["3"] = &Error{Code: CodeNoLyrics}
	srv.lyrics["4"] = songLyrics("4", false, LyricLine{Text: "plain"})
	f := startFollow(t, srv, "--json")

	type event struct {
		Running  bool
		Lyrics   string
		Index    int
		State    string
		Track    *Track
		Synced   bool
		Provider string
		Line     *LyricLine
		Next     *LyricLine
	}
	read := func() event {
		var e event
		if err := json.Unmarshal([]byte(f.line(t)), &e); err != nil {
			t.Fatal(err)
		}
		return e
	}

	srv.send("1", StatePlaying, 12.5)
	if e := read(); e.Lyrics != lyricsLoading || e.Track == nil || e.Track.ID != "1" || e.Line != nil {
		t.Errorf("loading = %+v", e)
	}
	if e := read(); e.Lyrics != lyricsReady || e.Index != 1 || e.Line == nil || e.Line.Text != "two" || e.Next == nil || e.Next.Text != "three" || !e.Synced || e.Provider != "apple" {
		t.Errorf("ready = %+v (line %+v)", e, e.Line)
	}
	// The same song again (the queue repeats it) asks nothing.
	srv.send("1", StatePlaying, 0)
	if e := read(); e.Index != -1 || e.Next == nil || e.Next.Text != "one" {
		t.Errorf("restart = %+v", e)
	}
	if n := srv.callCount(); n != 1 {
		t.Errorf("lyrics asked %d times for one song", n)
	}
	// Another song: its own lyrics.
	srv.send("2", StatePlaying, 0)
	if e := read(); e.Lyrics != lyricsLoading || e.Track.ID != "2" {
		t.Errorf("loading 2 = %+v", e)
	}
	if e := read(); e.Lyrics != lyricsReady || e.Index != -1 || e.Next == nil || e.Next.Text != "alpha" {
		t.Errorf("ready 2 = %+v", e)
	}
	if d := f.clk.next(t); d != 750*time.Millisecond {
		t.Errorf("timer = %v", d)
	}
	if e := read(); e.Index != 0 || e.Line.Text != "alpha" {
		t.Errorf("alpha = %+v", e)
	}
	// A song with no lyrics, then plain lyrics: nothing is ever sung.
	srv.send("3", StatePlaying, 0)
	if e := read(); e.Lyrics != lyricsLoading {
		t.Errorf("loading 3 = %+v", e)
	}
	if e := read(); e.Lyrics != lyricsNone || e.Index != -1 || e.Line != nil {
		t.Errorf("none = %+v", e)
	}
	srv.send("4", StatePlaying, 5)
	read() // loading
	if e := read(); e.Lyrics != lyricsReady || e.Synced || e.Index != -1 || e.Line != nil {
		t.Errorf("plain = %+v", e)
	}
	waitFor(t, "no timer for plain lyrics", func() bool { return f.clk.pending() == 0 })
	// Nothing playing.
	srv.send("", StateIdle, 0)
	if e := read(); e.Lyrics != lyricsNone || e.Track != nil || e.State != StateIdle || !e.Running {
		t.Errorf("idle = %+v", e)
	}
	if code := f.quit(t); code != ExitOK {
		t.Errorf("exit %d", code)
	}
	if e := read(); e.Running || e.State != StateIdle || e.Lyrics != lyricsNone {
		t.Errorf("last = %+v", e)
	}
	if f.errs.Len() != 0 {
		t.Errorf("stderr = %q", f.errs.String())
	}
}

// A song changing while its lyrics are fetched drops the stale answer.
func TestFollowLyricsStaleFetch(t *testing.T) {
	srv := newLyricsServer()
	// The server answers for song 2 whatever was asked: the status for
	// song 1 is old.
	srv.lyrics["2"] = songLyrics("2", true, LyricLine{Start: 0, End: 5, Text: "alpha"})
	f := startFollow(t, srv)
	srv.mu.Lock()
	srv.playing = "2"
	srv.mu.Unlock()
	srv.statuses <- Status{Running: true, State: StatePlaying, Track: &Track{ID: "1"}}
	f.want(t, "\n")
	srv.send("2", StatePlaying, 0)
	f.want(t, "alpha\n")
	f.quit(t)
}

func TestFollowLyricsFormatAndLocalization(t *testing.T) {
	srv := newLyricsServer()
	srv.lyrics["1"] = songLyrics("1", true,
		LyricLine{Start: 1, End: 2, Text: "hola", Translation: "hello", Transliteration: "ola"},
		LyricLine{Start: 2, End: 3, Text: "adios"})
	f := startFollow(t, srv, "--translation", "--format", `{{.lyrics}}|{{.index}}|{{.text}}|{{with .next}}{{.text}}{{end}}|{{clock .position}}`)
	srv.send("1", StatePlaying, 1.5)
	f.want(t, "loading|-1|||0:01\n", "ready|0|hello|adios|0:01\n")
	f.clk.next(t)
	f.want(t, "ready|1|adios||0:01\n") // no translation: the line itself
	f.clk.next(t)
	f.want(t, "ready|-1|||0:03\n")
	f.quit(t)
	f.want(t, "none|-1|||0:00\n")
}

func TestFollowLyricsNotRunning(t *testing.T) {
	code, out, errOut := run(t, nil, "lyrics", "--follow")
	if code != ExitNotRunning || out != "\n" || errOut != "sona: Sona isn't running; start it first\n" {
		t.Errorf("text: %d, %q, %q", code, out, errOut)
	}
	code, out, _ = run(t, nil, "lyrics", "--follow", "--json")
	if code != ExitNotRunning || !strings.HasPrefix(out, `{"running":false,"state":"idle"`) || strings.Count(out, "\n") != 1 {
		t.Errorf("json: %d, %q", code, out)
	}
	// The stream failing, or Sona not ready.
	srv := newLyricsServer()
	srv.ended = errors.New("connection reset by peer")
	f := startFollow(t, srv)
	srv.send("", StateIdle, 0)
	f.want(t, "\n")
	if code := f.quit(t); code != ExitNotRunning || !strings.Contains(f.errs.String(), "lost the connection to Sona: connection reset by peer") {
		t.Errorf("broken stream: %d, %q", code, f.errs.String())
	}
	srv = newLyricsServer()
	srv.ended = &Error{Code: CodeStarting}
	f = startFollow(t, srv)
	if code := f.quit(t); code != ExitNotRunning || f.errs.String() != "sona: Sona is starting; try again in a moment\n" {
		t.Errorf("starting: %d, %q", code, f.errs.String())
	}
}

// A lyrics fetch that fails otherwise than for lack of lyrics is said on
// standard error, once, and the song shows nothing.
func TestFollowLyricsFetchFails(t *testing.T) {
	srv := newLyricsServer()
	srv.errs["1"] = &Error{Code: "network", Message: "no route"}
	srv.lyrics["2"] = songLyrics("2", true, LyricLine{Start: 0, End: 5, Text: "alpha"})
	f := startFollow(t, srv)
	srv.send("1", StatePlaying, 3)
	f.want(t, "\n")
	waitFor(t, "the failure to be said", func() bool { return f.errs.Len() > 0 })
	srv.send("1", StatePaused, 3)
	srv.send("2", StatePlaying, 1)
	f.want(t, "alpha\n")
	if got := f.errs.String(); got != "sona: Sona couldn't reach the network; check your connection and try again\n" {
		t.Errorf("stderr = %q", got)
	}
	f.quit(t)
}

func TestFeedJump(t *testing.T) {
	clk := time.Unix(1000, 0)
	f := NewFeed(func() Status { return Status{} })
	f.now = func() time.Time { return clk }
	ch, _ := f.Watch()
	<-ch
	song := &Track{ID: "1", Duration: 200}
	playing := func(pos float64) Status {
		return Status{Running: true, State: StatePlaying, Track: song, Position: pos}
	}
	f.Publish(playing(10))
	if s := <-ch; s.Position != 10 {
		t.Fatalf("first = %+v", s)
	}
	published := func() bool {
		select {
		case <-ch:
			return true
		default:
			return false
		}
	}
	// Playing on is no change: the position is where the clock says.
	clk = clk.Add(30 * time.Second)
	f.Publish(playing(40.2))
	if published() {
		t.Error("playing on was published")
	}
	// A seek forwards or back is, and playing resumes from it.
	f.Publish(playing(100))
	if !published() {
		t.Error("a seek forwards was dropped")
	}
	clk = clk.Add(time.Second)
	f.Publish(playing(101))
	if published() {
		t.Error("playing on after the seek was published")
	}
	f.Publish(playing(3))
	if !published() {
		t.Error("a seek back was dropped")
	}
	// A song started again: the same status, the position back at 0.
	clk = clk.Add(190 * time.Second) // played on to the end
	f.Publish(playing(0))
	if !published() {
		t.Error("a restart was dropped")
	}
	// While paused the position stays put, so any move is a seek.
	paused := playing(50)
	paused.State = StatePaused
	f.Publish(paused)
	<-ch
	clk = clk.Add(time.Hour)
	paused.Position = 50.1
	f.Publish(paused)
	if published() {
		t.Error("a paused position within the jump was published")
	}
	paused.Position = 70
	f.Publish(paused)
	if !published() {
		t.Error("a seek while paused was dropped")
	}
}
