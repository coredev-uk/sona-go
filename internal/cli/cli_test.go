package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
)

func TestIsCommand(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"private", "on"}, true},
		{[]string{"status", "--follow"}, true},
		{[]string{"help"}, true},
		{[]string{"--help"}, true},
		{[]string{"-h"}, true},
		{nil, false},
		{[]string{"--hidden"}, false},
		{[]string{"music://music.apple.com/us/album/x/1"}, false},
		{[]string{"--hidden", "private"}, false}, // only the first argument names a command
		{[]string{"dance"}, false},
	} {
		if got := IsCommand(tc.args); got != tc.want {
			t.Errorf("IsCommand(%q) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		argv []string
		want Request
	}{
		{[]string{"play"}, Request{Command: "play"}},
		{[]string{"private"}, Request{Command: "private"}},
		{[]string{"private", "toggle", "--json"}, Request{Command: "private", Args: []string{"toggle"}}},
		{[]string{"volume", "-10"}, Request{Command: "volume", Args: []string{"-10"}}},
		{[]string{"volume", "+5"}, Request{Command: "volume", Args: []string{"+5"}}},
		{[]string{"volume", "--", "70"}, Request{Command: "volume", Args: []string{"70"}}},
		{[]string{"search", "--type", "album", "abbey", "road"}, Request{Command: "search", Args: []string{"abbey road"}, Kind: "album", Limit: 5}},
		{[]string{"search", "--limit=20", "x"}, Request{Command: "search", Args: []string{"x"}, Limit: 20}},
		{[]string{"search", "--", "--weird"}, Request{Command: "search", Args: []string{"--weird"}, Limit: 5}},
		{[]string{"open", "https://music.apple.com/us/album/x/1"}, Request{Command: "open", Args: []string{"https://music.apple.com/us/album/x/1"}}},
		{[]string{"status", "--follow", "--format", "{{.state}}"}, Request{Command: "status"}},
	} {
		in, err := parse(tc.argv)
		if err != nil {
			t.Errorf("parse(%q): %v", tc.argv, err)
			continue
		}
		if got := in.request(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parse(%q) = %+v, want %+v", tc.argv, got, tc.want)
		}
	}

	in, err := parse([]string{"status", "--json", "--follow"})
	if err != nil || !in.json || !in.follow {
		t.Errorf("status --json --follow = %+v, %v", in, err)
	}
	for _, argv := range [][]string{{"--help"}, {"-h"}, {"queue", "--help"}, {"volume", "500", "-h"}} {
		if in, err := parse(argv); err != nil || !in.help && in.cmd.name != "help" {
			t.Errorf("parse(%q) = %+v, %v; want help", argv, in, err)
		}
	}

	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"dance"}, `unknown command "dance"`},
		{[]string{"play", "now"}, "play: too many arguments"},
		{[]string{"play", "--loud"}, "play: unknown option --loud"},
		{[]string{"play", "-x"}, "play: unknown option -x"},
		{[]string{"play", "--json=yes"}, "play: --json takes no value"},
		{[]string{"open"}, "open: missing link"},
		{[]string{"search"}, "search: missing term"},
		{[]string{"search", "x", "--type"}, "search: --type needs a value"},
		{[]string{"search", "x", "--type", "podcast"}, "search: --type must be one of song, album"},
		{[]string{"search", "x", "--limit", "0"}, "search: --limit must be a number from 1 to 20"},
		{[]string{"search", "x", "--limit", "21"}, "search: --limit must be a number from 1 to 20"},
		{[]string{"private", "maybe"}, `private: "maybe" isn't one of on, off, toggle, status`},
		{[]string{"like", "on", "off"}, "like: too many arguments"},
		{[]string{"volume", "101"}, `volume: "101" isn't 0 to 100, +N or -N`},
		{[]string{"volume", "loud"}, `volume: "loud" isn't 0 to 100, +N or -N`},
		{[]string{"status", "--json", "--format", "x"}, "status: --json and --format can't be used together"},
		{[]string{"status", "--format", "{{.state"}, "status: --format: "},
		{[]string{"help", "dance"}, `help: "dance" isn't one of play`},
		{[]string{"completion"}, "completion: missing shell"},
		{[]string{"completion", "tcsh"}, `completion: "tcsh" isn't one of bash, zsh, fish`},
	} {
		_, err := parse(tc.argv)
		var e *Error
		if !errors.As(err, &e) || e.Code != CodeUsage || !strings.HasPrefix(e.Message, tc.want) {
			t.Errorf("parse(%q) err = %v, want usage %q…", tc.argv, err, tc.want)
		}
	}
}

func TestParseVolume(t *testing.T) {
	for _, tc := range []struct {
		arg      string
		n        int
		relative bool
		ok       bool
	}{
		{"0", 0, false, true},
		{"100", 100, false, true},
		{"55", 55, false, true},
		{"+5", 5, true, true},
		{"-100", -100, true, true},
		{"+150", 150, true, true},
		{"101", 0, false, false},
		{"", 0, false, false},
		{"5%", 0, false, false},
		{"1000", 0, false, false},
	} {
		n, rel, err := ParseVolume(tc.arg)
		if (err == nil) != tc.ok || (tc.ok && (n != tc.n || rel != tc.relative)) {
			t.Errorf("ParseVolume(%q) = %d, %v, %v", tc.arg, n, rel, err)
		}
	}
}

// keys returns the JSON object keys of v, sorted.
func keys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// The JSON schemas are a contract (docs/cli.md): every field is always
// present, under these names. Adding one means adding it here and there.
func TestJSONSchemas(t *testing.T) {
	for _, tc := range []struct {
		v    any
		want string
	}{
		{Status{}, "error position preview private repeat running shuffle state track volume"},
		{Track{}, "album artist artwork duration explicit id title"},
		{Queue{}, "index tracks"},
		{QueueTrack{}, "album artist artwork autoPlay duration explicit id title"},
		{SearchResults{}, "results term"},
		{SearchResult{}, "explicit id kind name subtitle url"},
		{Volume{}, "volume"},
		{Like{}, "liked track"},
		{Private{}, "on saved source"},
		{Opened{}, "id kind storefront"},
	} {
		if got := strings.Join(keys(t, tc.v), " "); got != tc.want {
			t.Errorf("%T keys = %q, want %q", tc.v, got, tc.want)
		}
	}
	b, _ := json.Marshal(NotRunning())
	if string(b) != `{"running":false,"state":"idle","track":null,"position":0,"volume":0,"shuffle":false,"repeat":"off","private":false,"preview":false,"error":""}` {
		t.Errorf("not running = %s", b)
	}
}

// fakeTransport answers from fixed results and statuses.
type fakeTransport struct {
	results   map[string]any   // by command
	errs      map[string]error // by command
	statuses  []Status         // what Follow delivers
	followErr error
	calls     []Request
	closed    bool
}

func (f *fakeTransport) Call(_ context.Context, req Request) (json.RawMessage, error) {
	f.calls = append(f.calls, req)
	if err := f.errs[req.Command]; err != nil {
		return nil, err
	}
	b, err := json.Marshal(f.results[req.Command])
	return b, err
}

func (f *fakeTransport) Follow(_ context.Context, each func(Status) error) error {
	for _, s := range f.statuses {
		if err := each(s); err != nil {
			return err
		}
	}
	return f.followErr
}

func (f *fakeTransport) Close() error { f.closed = true; return nil }

func run(t *testing.T, tr Transport, argv ...string) (code int, stdout, stderr string) {
	t.Helper()
	return runIn(t, nil, tr, argv...)
}

// runIn runs a command line with p's text.
func runIn(t *testing.T, p *l10n.Printer, tr Transport, argv ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = Run(context.Background(), argv, Env{Stdout: &out, Stderr: &errOut, Printer: p, Dial: func() (Transport, error) {
		if tr == nil {
			return nil, ErrNotRunning
		}
		return tr, nil
	}})
	return code, out.String(), errOut.String()
}

var song = &Track{ID: "1441164430", Title: "Come Together", Artist: "The Beatles", Album: "Abbey Road", Duration: 259.9}

func TestRunNotRunning(t *testing.T) {
	for _, argv := range [][]string{{"play"}, {"volume", "5"}, {"private", "on"}, {"open", "music://x"}} {
		code, out, errOut := run(t, nil, argv...)
		if code != ExitNotRunning || out != "" || errOut != "sona: Sona isn't running; start it first\n" {
			t.Errorf("%q: %d, %q, %q", argv, code, out, errOut)
		}
	}
	// status reports it, in every form.
	for _, tc := range []struct {
		argv []string
		out  string
	}{
		{[]string{"status"}, "Sona isn't running\n"},
		{[]string{"status", "--json"}, `{"running":false,"state":"idle","track":null,"position":0,"volume":0,"shuffle":false,"repeat":"off","private":false,"preview":false,"error":""}` + "\n"},
		{[]string{"status", "--follow"}, "not running\n"},
		{[]string{"status", "--follow", "--format", "{{if .running}}on{{else}}off{{end}}"}, "off\n"},
	} {
		code, out, errOut := run(t, nil, tc.argv...)
		if code != ExitNotRunning || out != tc.out || errOut != "" {
			t.Errorf("%q: %d, %q, %q; want %q", tc.argv, code, out, errOut, tc.out)
		}
	}
	// Local commands need no Sona.
	for _, argv := range [][]string{{"help"}, {"help", "status"}, {"help", "--man"}, {"--help"}, {"completion", "fish"}, {"queue", "--help"}} {
		if code, out, _ := run(t, nil, argv...); code != ExitOK || out == "" {
			t.Errorf("%q: exit %d, %d bytes", argv, code, len(out))
		}
	}
}

func TestRunOutput(t *testing.T) {
	playing := Status{Running: true, State: StatePlaying, Track: song, Position: 83.4, Volume: 40, Repeat: "all", Private: true}
	tr := &fakeTransport{results: map[string]any{
		"status": playing,
		"pause":  Status{Running: true, State: StatePaused, Track: song, Preview: true},
		"next":   Status{Running: true, State: StateIdle, Repeat: "off"},
		"queue": Queue{Index: 1, Tracks: []QueueTrack{
			{Track: Track{Title: "Something", Artist: "The Beatles"}},
			{Track: *song},
			{Track: Track{Title: "Radio song", Artist: "Someone"}, AutoPlay: true},
		}},
		"search": SearchResults{Term: "abbey", Results: []SearchResult{
			{Kind: "album", Name: "Abbey Road", Subtitle: "The Beatles", URL: "https://music.apple.com/gb/album/abbey-road/1441164426"},
			{Kind: "album", Name: "Abbey Road (Remastered)", Subtitle: "The Beatles"},
			{Kind: "song", Name: "Abbey Road Medley", Subtitle: "Someone"},
		}},
		"volume":  Volume{Volume: 55},
		"like":    Like{Track: *song, Liked: true},
		"private": Private{On: true, Source: SourceEnvironment},
		"open":    Opened{Kind: "album", ID: "1441164426", Storefront: "gb"},
	}}
	for _, tc := range []struct {
		argv []string
		want string
	}{
		{[]string{"status"}, "state: playing\ntitle: Come Together\nartist: The Beatles\nalbum: Abbey Road\nposition: 1:23 / 4:19\nvolume: 40%\nshuffle: off\nrepeat: all\nprivate listening: on\n"},
		{[]string{"status", "--format", "{{.state}} {{with .track}}{{.artist}} – {{.title}} {{clock .duration}}{{end}} {{json .volume}}"}, "playing The Beatles – Come Together 4:19 40\n"},
		{[]string{"pause"}, "paused: Come Together — The Beatles (preview)\n"},
		{[]string{"next"}, "stopped\n"},
		{[]string{"queue"}, "  1  Something — The Beatles\n▶ 2  Come Together — The Beatles\n  3  Radio song — Someone (AutoPlay)\n"},
		{[]string{"search", "abbey"}, "Albums\n  Abbey Road — The Beatles  https://music.apple.com/gb/album/abbey-road/1441164426\n  Abbey Road (Remastered) — The Beatles\nSongs\n  Abbey Road Medley — Someone\n"},
		{[]string{"volume", "+15"}, "volume: 55%\n"},
		{[]string{"like"}, "favourite: on (Come Together — The Beatles)\n"},
		{[]string{"private"}, "private listening: on (forced by SONA_PRIVATE=1)\n"},
		{[]string{"open", "https://music.apple.com/gb/album/abbey-road/1441164426"}, "opening album 1441164426\n"},
		{[]string{"volume", "--json"}, `{"volume":55}` + "\n"},
	} {
		code, out, errOut := run(t, tr, tc.argv...)
		if code != ExitOK || out != tc.want || errOut != "" {
			t.Errorf("%q: %d, %q, %q\nwant %q", tc.argv, code, out, errOut, tc.want)
		}
	}
	if !tr.closed {
		t.Error("transport not closed")
	}
	if got := privateLine(nil, Private{On: true, Source: SourceDevelopment}); got != "private listening: on (development build: this session only; saved setting off)" {
		t.Errorf("development line = %q", got)
	}
	tr.results["queue"] = Queue{Index: -1, Tracks: []QueueTrack{}}
	tr.results["search"] = SearchResults{Term: "zzz", Results: []SearchResult{}}
	for argv, want := range map[string]string{"queue": "the queue is empty\n", "search zzz": "no results for \"zzz\"\n"} {
		if _, out, _ := run(t, tr, strings.Fields(argv)...); out != want {
			t.Errorf("%s = %q, want %q", argv, out, want)
		}
	}
}

func TestRunErrors(t *testing.T) {
	tr := &fakeTransport{errs: map[string]error{
		"like":    &Error{Code: CodeNothingPlaying},
		"private": &Error{Code: "private_forced", Message: "private mode is forced on"},
		"next":    &Error{Code: "network", Message: "dial tcp: no route to host"},
		"prev":    &Error{Code: CodeFailed, Message: "the queue file is damaged"},
		"open":    &Error{Code: CodeBadLink, Message: "unsupported Apple Music URL"},
		"play":    &Error{Code: CodeStarting},
		"queue":   &Error{Code: CodeUnknownCommand, Message: "queue"},
		"pause":   errors.New("connection reset by peer"),
		"toggle":  &Error{Code: "unauthorized", Message: "403 Forbidden"},
		"search":  &Error{Code: CodeAccountLoading},
		"volume":  &Error{Code: CodeUsage},
	}}
	for _, tc := range []struct {
		argv   []string
		code   int
		stderr string
	}{
		{[]string{"like"}, ExitFailed, "sona: nothing is playing\n"},
		{[]string{"toggle"}, ExitFailed, "sona: Apple Music needs you signed in to Sona for this\n"},
		{[]string{"search", "x"}, ExitFailed, "sona: Sona is still reading your Apple Music account; try again in a moment\n"},
		{[]string{"private", "off"}, ExitFailed, "sona: private listening is forced on for this run (SONA_PRIVATE=1)\n"},
		{[]string{"next"}, ExitFailed, "sona: Sona couldn't reach the network; check your connection and try again\n"},
		{[]string{"prev"}, ExitFailed, "sona: Sona couldn't do that: the queue file is damaged\n"},
		{[]string{"open", "https://example.com"}, ExitUsage, "sona: that isn't an Apple Music link Sona can open\n"},
		{[]string{"play"}, ExitNotRunning, "sona: Sona is starting; try again in a moment\n"},
		{[]string{"queue"}, ExitFailed, "sona: the running Sona is a different version; quit and start it again\n"},
		{[]string{"pause"}, ExitNotRunning, "sona: can't reach the running Sona: connection reset by peer\n"},
		{[]string{"volume", "4"}, ExitUsage, "sona: the command line is wrong: an unknown command, argument or option\n"},
		{[]string{"volume", "400"}, ExitUsage, "sona: volume: \"400\" isn't 0 to 100, +N or -N\n"},
		{[]string{"plya"}, ExitUsage, "sona: unknown command \"plya\" (run `sona help`)\n"},
	} {
		code, out, errOut := run(t, tr, tc.argv...)
		if code != tc.code || out != "" || errOut != tc.stderr {
			t.Errorf("%q: %d, %q, %q; want %d, %q", tc.argv, code, out, errOut, tc.code, tc.stderr)
		}
	}
	// A reply this version can't read.
	tr = &fakeTransport{results: map[string]any{"status": []int{1}}}
	if code, _, errOut := run(t, tr, "status"); code != ExitFailed || !strings.Contains(errOut, "different version") {
		t.Errorf("bad reply: %d, %q", code, errOut)
	}
}

func TestRunFollow(t *testing.T) {
	paused := Status{Running: true, State: StatePaused, Track: song, Position: 10}
	tr := &fakeTransport{statuses: []Status{
		{Running: true, State: StatePlaying, Track: song, Position: 1},
		{Running: true, State: StatePlaying, Track: song, Position: 2}, // only the position moved
		paused,
		{Running: true, State: StatePaused, Track: song, Position: 10, Volume: 70}, // the text line doesn't show it
	}}
	code, out, errOut := run(t, tr, "status", "--follow")
	want := "playing: Come Together — The Beatles\npaused: Come Together — The Beatles\nnot running\n"
	if code != ExitOK || out != want || errOut != "" {
		t.Errorf("follow: %d, %q, %q\nwant %q", code, out, errOut, want)
	}
	code, out, _ = run(t, tr, "status", "--follow", "--json")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != ExitOK || len(lines) != 4 || !strings.HasPrefix(lines[0], `{"running":true,"state":"playing"`) || !strings.HasPrefix(lines[3], `{"running":false`) {
		t.Errorf("follow --json: %d, %q", code, out)
	}
	code, out, _ = run(t, tr, "status", "--follow", "--format", `{"text": {{with .track}}{{json .title}}{{else}}""{{end}}, "class": {{json .state}}}`)
	want = `{"text": "Come Together", "class": "playing"}` + "\n" + `{"text": "Come Together", "class": "paused"}` + "\n" + `{"text": "", "class": "idle"}` + "\n"
	if code != ExitOK || out != want {
		t.Errorf("follow --format: %d, %q\nwant %q", code, out, want)
	}

	// The connection breaking is not Sona quitting.
	tr.followErr = errors.New("connection reset by peer")
	code, out, errOut = run(t, tr, "status", "--follow")
	if code != ExitNotRunning || strings.Contains(out, "not running") || errOut != "sona: can't reach the running Sona: lost the connection to Sona: connection reset by peer\n" {
		t.Errorf("broken stream: %d, %q, %q", code, out, errOut)
	}
	tr = &fakeTransport{followErr: &Error{Code: CodeStarting}}
	if code, _, errOut := run(t, tr, "status", "--follow"); code != ExitNotRunning || errOut != "sona: Sona is starting; try again in a moment\n" {
		t.Errorf("starting: %d, %q", code, errOut)
	}
}

func TestFeed(t *testing.T) {
	current := Status{Running: true, State: StateIdle}
	f := NewFeed(func() Status { return current })
	a, cancelA := f.Watch()
	if s := <-a; s.State != StateIdle {
		t.Fatalf("first = %+v, want the current status", s)
	}
	playing := Status{Running: true, State: StatePlaying, Track: &Track{ID: "1"}, Position: 1}
	f.Publish(playing)
	moved := playing
	moved.Position = 30
	moved.Track = &Track{ID: "1"} // equal, not the same pointer
	f.Publish(moved)
	if s := <-a; s.Position != 1 {
		t.Errorf("got %+v", s)
	}
	select {
	case s := <-a:
		t.Errorf("a position-only change was published: %+v", s)
	default:
	}
	// A slow follower gets the latest status, not a backlog.
	f.Publish(Status{Running: true, State: StatePaused})
	f.Publish(Status{Running: true, State: StatePlaying, Volume: 5})
	if s := <-a; s.Volume != 5 {
		t.Errorf("latest = %+v", s)
	}
	b, _ := f.Watch()
	cancelA()
	cancelA() // twice is fine
	if _, ok := <-a; ok {
		t.Error("cancelled channel still open")
	}
	<-b
	f.Close()
	if _, ok := <-b; ok {
		t.Error("Close left a follower open")
	}
	c, _ := f.Watch()
	if _, ok := <-c; ok {
		t.Error("Watch after Close delivered a status")
	}
}

type echoServer struct{}

func (echoServer) Call(_ context.Context, req Request) (any, error) {
	if req.Command == "like" {
		return nil, errors.New("boom")
	}
	return req, nil
}

func (echoServer) Watch() (<-chan Status, func()) { return nil, func() {} }

func TestHandle(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		srv  Server
		req  string
		code string
	}{
		{nil, `{"command":"status"}`, CodeStarting},
		{echoServer{}, `not json`, CodeIncompatible},
		{echoServer{}, `{"command":"completion","args":["bash"]}`, CodeUnknownCommand},
		{echoServer{}, `{"command":"private","args":["on","off"]}`, CodeUsage},
		{echoServer{}, `{"command":"search","args":["x"],"limit":99}`, CodeUsage},
		{echoServer{}, `{"command":"search","args":["x"],"kind":"podcast"}`, CodeUsage},
		{echoServer{}, `{"command":"like"}`, CodeFailed},
	} {
		r := Handle(ctx, tc.srv, []byte(tc.req))
		if r.Error == nil || r.Error.Code != tc.code {
			t.Errorf("Handle(%s) = %+v, want %s", tc.req, r, tc.code)
		}
	}
	r := Handle(ctx, echoServer{}, []byte(`{"command":"volume","args":["+5"]}`))
	if r.Error != nil || string(r.Result) != `{"command":"volume","args":["+5"]}` {
		t.Errorf("valid request: %+v", r)
	}
	if raw, err := DecodeReply(StatusReply(Status{Running: true})); err != nil || !strings.HasPrefix(string(raw), `{"running":true`) {
		t.Errorf("StatusReply round trip: %s, %v", raw, err)
	}
}

// Help, printed lines and errors are in the Printer's language; JSON and
// the command line's syntax are not.
func TestRunTranslated(t *testing.T) {
	fr := l10n.NewPrinter("fr", map[l10n.Message]string{
		msgUsage:          "utilisation",
		msgPlaySummary:    "Reprendre la lecture",
		msgStatePlaying:   "lecture",
		msgFieldLine:      "%s : %s",
		msgTrackByArtist:  "%[2]s – %[1]s",
		msgNothingPlaying: "rien n'est en cours de lecture",
		msgUnknownCommand: "commande inconnue %q",
		msgNotRunning:     "Sona n'est pas lancé",
	})
	if _, out, _ := runIn(t, fr, nil, "help"); !strings.HasPrefix(out, "utilisation: sona <command>") || !strings.Contains(out, "Reprendre la lecture") {
		t.Errorf("help:\n%s", out)
	}
	if _, out, _ := runIn(t, fr, nil, "completion", "fish"); !strings.Contains(out, "'Reprendre la lecture'") {
		t.Error("completion descriptions untranslated")
	}
	playing := Status{Running: true, State: StatePlaying, Track: song}
	tr := &fakeTransport{
		results: map[string]any{"play": playing, "status": playing},
		errs:    map[string]error{"like": &Error{Code: CodeNothingPlaying}},
	}
	if _, out, _ := runIn(t, fr, tr, "play"); out != "lecture : The Beatles – Come Together\n" {
		t.Errorf("play = %q", out)
	}
	if _, out, _ := runIn(t, fr, tr, "status", "--json"); !strings.HasPrefix(out, `{"running":true,"state":"playing"`) {
		t.Errorf("status --json = %q", out)
	}
	if _, _, errOut := runIn(t, fr, tr, "like"); errOut != "sona: rien n'est en cours de lecture\n" {
		t.Errorf("like = %q", errOut)
	}
	if _, _, errOut := runIn(t, fr, tr, "plya"); errOut != "sona: commande inconnue \"plya\"\n" {
		t.Errorf("unknown command = %q", errOut)
	}
	if _, _, errOut := runIn(t, fr, nil, "play"); errOut != "sona: Sona n'est pas lancé\n" {
		t.Errorf("not running = %q", errOut)
	}
}
