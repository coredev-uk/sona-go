package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptTransport plays the running Sona's side of `addons dev`.
type scriptTransport struct {
	mu    sync.Mutex
	calls []Request
	start func() (AddonDev, error)
	logs  []AddonDevLog // one per poll; the last repeats
	polls int
	// onStop is called by dev-stop.
	onStop func()
}

func (s *scriptTransport) Call(_ context.Context, req Request) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, req)
	switch req.Args[0] {
	case addonsDev:
		d, err := s.start()
		if err != nil {
			return nil, err
		}
		return json.Marshal(d)
	case addonsDevLog:
		l := s.logs[min(s.polls, len(s.logs)-1)]
		s.polls++
		return json.Marshal(l)
	case addonsDevStop:
		if s.onStop != nil {
			s.onStop()
		}
	}
	return json.Marshal(struct{}{})
}

func (s *scriptTransport) Follow(context.Context, func(Status) error) error { return nil }
func (s *scriptTransport) Close() error                                     { return nil }

func (s *scriptTransport) requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.calls...)
}

func runDev(t *testing.T, ctx context.Context, tr Transport, terminal bool, argv ...string) (code int, stdout, stderr string) {
	t.Helper()
	devPollEvery = time.Millisecond
	var out, errOut bytes.Buffer
	code = Run(ctx, argv, Env{Stdout: &out, Stderr: &errOut, Terminal: terminal, Dial: func() (Transport, error) { return tr, nil }})
	return code, out.String(), errOut.String()
}

func devStarted() (AddonDev, error) {
	return AddonDev{ID: "demo", Name: "Demo", Version: "1.0.0", Token: "tok", Permissions: []string{"playback.read"}}, nil
}

func TestAddonsDevPrintsTheLogUntilTheAddonEnds(t *testing.T) {
	tr := &scriptTransport{start: devStarted, logs: []AddonDevLog{
		{Lines: []string{"first"}, Next: 1},
		{Lines: []string{}, Next: 1},
		{Lines: []string{"second", "third"}, Next: 3, Ended: true, Exit: "exit status 1"},
	}}
	code, out, errOut := runDev(t, context.Background(), tr, false, "addons", "dev", "some/folder")
	if code != ExitOK || out != "first\nsecond\nthird\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if !strings.Contains(errOut, "Running Demo (demo) from ") || !strings.HasSuffix(strings.Split(errOut, "\n")[0], "some/folder. Press Ctrl+C to stop.") || !strings.Contains(errOut, "Demo ended: exit status 1.") {
		t.Errorf("stderr %q", errOut)
	}
	reqs := tr.requests()
	// The folder is made absolute by the client; the polls carry the token and the line after the last.
	if reqs[0].Command != "addons" || reqs[0].Args[0] != "dev" || !strings.HasPrefix(reqs[0].Args[1], "/") {
		t.Errorf("start %+v", reqs[0])
	}
	if reqs[1].Args[0] != "dev-log" || reqs[1].Args[1] != "tok" || reqs[1].Args[2] != "0" || reqs[len(reqs)-1].Args[2] != "1" {
		t.Errorf("polls %+v", reqs[1:])
	}
}

func TestAddonsDevCtrlCEndsTheRunBeforeLeaving(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	tr := &scriptTransport{start: devStarted, logs: []AddonDevLog{{Lines: []string{"hello"}, Next: 1}, {Lines: []string{}, Next: 1}}}
	var once sync.Once
	go func() {
		for {
			if tr.pollCount() >= 2 {
				once.Do(cancel)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	code, out, errOut := runDev(t, ctx, tr, false, "addons", "dev", "/tmp/x")
	if code != ExitOK || out != "hello\n" || !strings.Contains(errOut, "Stopped Demo.") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	reqs := tr.requests()
	if last := reqs[len(reqs)-1]; last.Args[0] != "dev-stop" || last.Args[1] != "tok" {
		t.Errorf("the last request is %+v, want dev-stop", last)
	}
}

func (s *scriptTransport) pollCount() int { s.mu.Lock(); defer s.mu.Unlock(); return s.polls }

// When the person doesn't confirm, or the add-on is refused, the command
// fails with the reason and prints no log.
func TestAddonsDevFailsWhenNotConfirmedOrRefused(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
		code int
	}{
		{"denied", &Error{Code: CodeDenied}, "sona: you didn't confirm it in Sona's window\n", ExitFailed},
		{"refused", &Error{Code: "invalid_argument", Message: "invalid argument: demo is an installed add-on's ID"}, "demo is an installed add-on's ID", ExitFailed},
		{"not running", ErrNotRunning, "sona: Sona isn't running; start it first\n", ExitNotRunning},
		{"unsupported", &Error{Code: "unsupported"}, "sona:", ExitFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := &scriptTransport{start: func() (AddonDev, error) { return AddonDev{}, tc.err }}
			code, out, errOut := runDev(t, context.Background(), tr, false, "addons", "dev", "/tmp/x")
			if code != tc.code || out != "" || !strings.Contains(errOut, tc.want) {
				t.Errorf("exit %d, stdout %q, stderr %q", code, out, errOut)
			}
			if len(tr.requests()) != 1 {
				t.Errorf("requests after a refusal: %+v", tr.requests())
			}
		})
	}
}

// On a terminal what the add-on wrote can't drive it: escape sequences and
// control characters are stripped from every line and from the names; piped, the
// lines are printed as Sona sent them.
func TestAddonsDevStripsEscapeSequencesOnATerminal(t *testing.T) {
	evil := []string{
		"\x1b[31mred\x1b[0m text",
		"title \x1b]0;pwned\x07done",
		"clear \x1b[2J\x1b[Hscreen",
		"c1 \u009b31mred\u009d0;x\u009cend",
		"bell\x07 and \x08backspace and\ttab \r overwritten",
		"bidi \u202Eevil\u202C and \u200Bzero width",
		"dcs \x1bPpayload\x1b\\ after",
		"unterminated \x1b]0;never ends",
	}
	tr := func() *scriptTransport {
		return &scriptTransport{
			start: func() (AddonDev, error) { return AddonDev{ID: "demo", Name: "De\x1b[31mmo", Token: "tok"}, nil },
			logs:  []AddonDevLog{{Lines: evil, Next: int64(len(evil)), Ended: true, Exit: "gone \x1b[0m"}},
		}
	}
	code, out, errOut := runDev(t, context.Background(), tr(), true, "addons", "dev", "/tmp/x")
	if code != ExitOK {
		t.Fatalf("exit %d: %q", code, errOut)
	}
	for _, s := range []string{out, errOut} {
		for _, r := range s {
			if r != '\n' && (r < 0x20 || (r >= 0x7f && r < 0xa0) || r == 0x202E || r == 0x200B) {
				t.Errorf("the output keeps %U: %q", r, s)
			}
		}
	}
	want := []string{
		"red text",
		"title done",
		"clear screen",
		"c1 redend",
		"bell and backspace and tab overwritten",
		"bidi evil and zero width",
		"dcs after",
		"unterminated ",
	}
	got := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	for i, w := range want {
		// Runs of spaces are what control characters become.
		if i >= len(got) || strings.Join(strings.Fields(got[i]), " ") != strings.Join(strings.Fields(w), " ") {
			t.Errorf("line %d = %q, want %q", i, got[i], w)
		}
	}
	if !strings.Contains(errOut, "Running Demo (demo)") || !strings.Contains(errOut, "Demo ended: gone .") {
		t.Errorf("stderr %q", errOut)
	}
	// Piped: the lines are the log's, as Sona sent them (Sona's own log
	// already replaced the controls).
	_, piped, _ := runDev(t, context.Background(), tr(), false, "addons", "dev", "/tmp/x")
	if !strings.Contains(piped, "\x1b[31mred") {
		t.Errorf("piped output was altered: %q", piped)
	}
}

func TestTerminalSafe(t *testing.T) {
	for in, want := range map[string]string{
		"plain text, ünïcode 日本語": "plain text, ünïcode 日本語",
		"\x1b[1;31mbold\x1b[m":    "bold",
		"a\x1bcb":                 "ab",
		"a\x1b(Bb":                "ab",
		"x\xffy":                  "x\uFFFDy",
		"line\r\nbreak":           "line  break",
		"a\u2028b":                "a b",
		"\x1b":                    "",
		"\x1b[":                   "",
		"ok\x1b]8;;http://x\x1b\\link\x1b]8;;\x1b\\": "oklink",
	} {
		if got := TerminalSafe(in); got != want {
			t.Errorf("TerminalSafe(%q) = %q, want %q", in, got, want)
		}
	}
}

// The person may type `dev` and a folder; the client's own polls are requests
// only, and the running Sona checks them as it does the command line.
func TestAddonsCommandLineAndRequestChecks(t *testing.T) {
	for _, argv := range [][]string{
		{"addons"},
		{"addons", "dev"},
		{"addons", "install", "x"},
		{"addons", "dev-log", "tok", "0"},
		{"addons", "dev-stop", "tok"},
		{"addons", "dev", "a", "b"},
		{"addons", "dev", ""},
	} {
		if _, err := parse(argv); err == nil {
			t.Errorf("%q was accepted", argv)
		}
	}
	if in, err := parse([]string{"addons", "dev", "./x"}); err != nil || !in.cmd.ownerOnly {
		t.Errorf("addons dev ./x: %v", err)
	}
	for _, tc := range []struct {
		req Request
		ok  bool
	}{
		{Request{Command: "addons", Args: []string{"dev", "/abs/folder"}}, true},
		{Request{Command: "addons", Args: []string{"dev", "relative"}}, false},
		{Request{Command: "addons", Args: []string{"dev", "/abs\x00x"}}, false},
		{Request{Command: "addons", Args: []string{"dev-log", "tok", "12"}}, true},
		{Request{Command: "addons", Args: []string{"dev-log", "tok", "-1"}}, false},
		{Request{Command: "addons", Args: []string{"dev-log", "tok", "x"}}, false},
		{Request{Command: "addons", Args: []string{"dev-log", "tok"}}, false},
		{Request{Command: "addons", Args: []string{"dev-stop", "tok"}}, true},
		{Request{Command: "addons", Args: []string{"dev-stop"}}, false},
		{Request{Command: "addons", Args: []string{"nope", "x"}}, false},
		{Request{Command: "addons"}, false},
	} {
		err := checkRequest(tc.req)
		if (err == nil) != tc.ok {
			t.Errorf("%+v: %v", tc.req, err)
		}
	}
	if _, ok := RequestPermission(Request{Command: "addons"}); ok {
		t.Error("an add-on may send `addons`")
	}
	var ce *Error
	if r := Handle(context.Background(), nil, []byte(`{"command":"addons","args":["dev","/x"]}`)); !errors.As(r.Error, &ce) || ce.Code != CodeStarting {
		t.Errorf("a server that is starting: %+v", r)
	}
}
