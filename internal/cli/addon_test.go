package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const wait = 5 * time.Second

// fakeHost answers add-on calls with call (or a plain status) and serves
// topics from a hub.
type fakeHost struct {
	hub *Hub

	mu    sync.Mutex
	calls []Request
	call  func(context.Context, Request) (any, error)
}

func newFakeHost() *fakeHost { return &fakeHost{hub: NewHub()} }

func (h *fakeHost) Call(ctx context.Context, req Request) (any, error) {
	h.mu.Lock()
	h.calls = append(h.calls, req)
	fn := h.call
	h.mu.Unlock()
	if fn != nil {
		return fn(ctx, req)
	}
	return Status{Running: true, State: StatePlaying, Repeat: "off"}, nil
}

func (h *fakeHost) Watch(topic string) (<-chan any, func(), error) {
	if _, ok := TopicPermission(topic); !ok {
		return nil, nil, errors.New("no such topic")
	}
	ch, cancel := h.hub.Watch(topic)
	return ch, cancel, nil
}

func (h *fakeHost) called() []Request {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.calls)
}

// grants is the set of permissions an add-on holds, changed while it runs.
type grants struct {
	mu   sync.Mutex
	set  map[string]bool
	asks map[string]int
}

func newGrants(perms ...string) *grants {
	g := &grants{set: map[string]bool{}, asks: map[string]int{}}
	g.grant(perms...)
	return g
}

func (g *grants) grant(perms ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range perms {
		g.set[p] = true
	}
}

func (g *grants) revoke(perms ...string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, p := range perms {
		delete(g.set, p)
	}
}

func (g *grants) allowed(p string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.asks[p]++
	return g.set[p]
}

// asked is how many times the session has asked whether p is held.
func (g *grants) asked(p string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.asks[p]
}

// testAddon is a Session served over an in-memory pipe, and the add-on's end.
type testAddon struct {
	t      *testing.T
	s      *Session
	host   *fakeHost
	grants *grants
	conn   net.Conn
	r      *bufio.Reader
	served chan error
}

func startAddon(t *testing.T, cfg AddonConfig, perms ...string) *testAddon {
	t.Helper()
	a := &testAddon{t: t, host: newFakeHost(), grants: newGrants(perms...), served: make(chan error, 1)}
	if cfg.ID == "" {
		cfg.ID = "demo"
	}
	cfg.Host, cfg.Allowed = a.host, a.grants.allowed
	a.s = NewSession(cfg)
	server, client := net.Pipe()
	a.conn, a.r = client, bufio.NewReader(client)
	go func() { a.served <- a.s.Serve(t.Context(), server) }()
	t.Cleanup(func() {
		_ = client.Close()
		select {
		case <-a.s.Done():
		case <-time.After(wait):
			t.Error("session did not end")
		}
	})
	return a
}

func (a *testAddon) send(line string) {
	a.t.Helper()
	_ = a.conn.SetWriteDeadline(time.Now().Add(wait))
	if _, err := io.WriteString(a.conn, line+"\n"); err != nil {
		a.t.Fatalf("write %.60q: %v", line, err)
	}
}

func (a *testAddon) recv() Frame {
	a.t.Helper()
	_ = a.conn.SetReadDeadline(time.Now().Add(wait))
	line, err := a.r.ReadBytes('\n')
	if err != nil {
		a.t.Fatalf("read: %v", err)
	}
	var f Frame
	if err := json.Unmarshal(line, &f); err != nil {
		a.t.Fatalf("frame %q: %v", line, err)
	}
	return f
}

// closed reports that the session ended the channel (after any frames
// still queued, which it returns).
func (a *testAddon) closed() []Frame {
	a.t.Helper()
	var rest []Frame
	for {
		_ = a.conn.SetReadDeadline(time.Now().Add(wait))
		line, err := a.r.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				return rest
			}
			a.t.Fatalf("the channel is still open: %v", err)
		}
		var f Frame
		_ = json.Unmarshal(line, &f)
		rest = append(rest, f)
	}
}

func (a *testAddon) hello(perms ...string) HelloResult {
	a.t.Helper()
	b, _ := json.Marshal(perms)
	a.send(`{"id":1,"method":"hello","params":{"id":"demo","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":` + string(b) + `}}`)
	f := a.recv()
	if f.ID != 1 || f.Error != nil {
		a.t.Fatalf("hello answered %+v", f)
	}
	var r HelloResult
	if err := json.Unmarshal(f.Result, &r); err != nil {
		a.t.Fatal(err)
	}
	return r
}

// request sends a request and returns its answer; events and other
// requests in between are skipped.
func (a *testAddon) request(id int64, method, params string) Frame {
	a.t.Helper()
	line := `{"id":` + itoa(id) + `,"method":"` + method + `"`
	if params != "" {
		line += `,"params":` + params
	}
	a.send(line + "}")
	return a.answer(id)
}

func (a *testAddon) answer(id int64) Frame {
	a.t.Helper()
	for {
		f := a.recv()
		if f.ID == id && f.Method == "" {
			return f
		}
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func wantCode(t *testing.T, f Frame, code string) {
	t.Helper()
	if f.Error == nil || f.Error.Code != code {
		t.Fatalf("answer = %+v (result %s), want error %q", f.Error, f.Result, code)
	}
	if f.Error.Message != addonMessages[code] || f.Error.Message == "" {
		t.Errorf("message = %q, want the fixed %q", f.Error.Message, addonMessages[code])
	}
}

func TestAddonHelloGrantsAndSubscribes(t *testing.T) {
	a := startAddon(t, AddonConfig{}, PermPlaybackRead)
	res := a.hello(PermPlaybackRead, PermPlaybackControl)
	if res.ID != "demo" || res.Protocol != "2.0.0" || !slices.Equal(res.Permissions, []string{PermPlaybackRead}) || res.Limits.MaxFrame != AddonMaxFrame {
		t.Fatalf("hello result = %+v", res)
	}
	f := a.request(2, "subscribe", `{"topics":["status","now-playing"]}`)
	if f.Error != nil || string(f.Result) != `{"topics":["status","now-playing"]}` {
		t.Fatalf("subscribe = %+v", f)
	}
	a.host.hub.Publish(TopicNowPlaying, NowPlaying{Track: &Track{ID: "1", Title: "Song"}, StartedAt: 5})
	ev := a.recv()
	if ev.Event != "now-playing" || !strings.Contains(string(ev.Data), `"title":"Song"`) || ev.ID != 0 {
		t.Fatalf("event = %+v %s", ev, ev.Data)
	}
	if f := a.request(3, "subscribe", `{"topics":["queue"]}`); f.Error == nil || f.Error.Code != AddonErrPermission {
		t.Fatalf("queue without queue.read: %+v", f)
	}
}

// A client that skips hello, or sends a v1 command, gets an error and
// nothing else, and the host never hears of it.
func TestAddonRequiresHello(t *testing.T) {
	for name, line := range map[string]string{
		"call":         `{"id":1,"method":"call","params":{"command":"status"}}`,
		"subscribe":    `{"id":1,"method":"subscribe","params":{"topics":["status"]}}`,
		"v1 run":       `{"command":"status"}`,
		"not json":     `status`,
		"hello no id":  `{"method":"hello","params":{"id":"demo","protocol":{"min":"2.0","max":"2.0"}}}`,
		"empty object": `{}`,
	} {
		t.Run(name, func(t *testing.T) {
			a := startAddon(t, AddonConfig{}, PermPlaybackRead, PermPlaybackControl)
			a.host.hub.Publish(TopicStatus, Status{Running: true})
			a.send(line)
			f := a.recv()
			wantCode(t, f, AddonErrHelloRequired)
			if rest := a.closed(); len(rest) != 0 {
				t.Errorf("more frames after the refusal: %+v", rest)
			}
			if n := len(a.host.called()); n != 0 {
				t.Errorf("the host ran %d commands", n)
			}
		})
	}
}

func TestAddonHelloMustNameItsAddon(t *testing.T) {
	for name, params := range map[string]string{
		"another id":         `{"id":"other","protocol":{"min":"2.0","max":"2.0"},"permissions":[]}`,
		"no id":              `{"protocol":{"min":"2.0","max":"2.0"},"permissions":[]}`,
		"unknown permission": `{"id":"demo","protocol":{"min":"2.0","max":"2.0"},"permissions":["root"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			a := startAddon(t, AddonConfig{})
			a.send(`{"id":1,"method":"hello","params":` + params + `}`)
			wantCode(t, a.recv(), AddonErrBadHello)
			a.closed()
		})
	}
}

func TestNegotiateVersion(t *testing.T) {
	for _, tc := range []struct {
		min, max string
		want     string
	}{
		{"2.0.0", "2.0.0", "2.0.0"},
		{"2.0", "2.9.4", "2.0.0"},        // the highest both speak: Sona's
		{"1.0.0", "2.0.0", "2.0.0"},      // an add-on that also speaks v1
		{"2.1.0", "2.5.0", ""},           // its minimum is above Sona's highest
		{"3.0.0", "3.2.0", ""},           // another major
		{"1.0.0", "1.9.0", ""},           // below Sona's lowest
		{"2.0.0", "1.0.0", ""},           // an empty range
		{"two", "2.0.0", ""},             // not a version
		{"2.0.0-rc1", "2.0.0", ""},       // no pre-releases
		{"2.0.0", "", ""},                // missing
		{"v2.0.0", "v2.0.0", ""},         // no v prefix
		{"02.0.0", "2.0.0", ""},          // no leading zeros
		{"2.0.0.0", "2.0.0.0", ""},       // too many parts
		{"2.0.0", "2.0.0+build5", ""},    // no build metadata
		{"2.0", "2.0", "2.0.0"},          // major.minor is enough
		{"0.9", "99999.0", "2.0.0"},      // large but sane
		{"2.0", "9999999.0", ""},         // beyond the digit cap
		{"2.0.0", "2.0.0 ", ""},          // whitespace
		{"0.0.0", "2.0.0", "2.0.0"},      // a range that starts early
		{"2.0.1", "2.0.9", "2.0.0"},      // patches are ignored
		{"1.0.0", "3.0.0", "2.0.0"},      // a range spanning ours
		{"2.0.0", "2.0.0\n", ""},         // a newline
		{"\u0662.0.0", "2.0.0", ""},      // non-ASCII digits
		{"2.0.0", "2.0.0-0", ""},         // pre-release
		{"2.00.0", "2.0.0", ""},          // leading zero in minor
		{"2.0.00", "2.0.0", ""},          // leading zero in patch
		{"-2.0.0", "2.0.0", ""},          // sign
		{"2.0.0", "2.0.0.0", ""},         // four parts
		{"2", "2", ""},                   // major alone
		{"2.0.0", "2.1", "2.0.0"},        // mixed forms
		{"2.0.0", "2.0.0000001", ""},     // padded patch
		{"1.99999", "2.0", "2.0.0"},      // top of the minor range
		{"2.0.0", "1000000.0.0", ""},     // seven digits
		{"2.0.0", "100000.0.0", "2.0.0"}, // six digits
	} {
		got, ok := NegotiateVersion(VersionRange{Min: tc.min, Max: tc.max})
		if (tc.want == "") == ok || got != tc.want {
			t.Errorf("%q..%q = %q, %v; want %q", tc.min, tc.max, got, ok, tc.want)
		}
	}
}

func TestAddonRefusesAVersionItCannotSpeak(t *testing.T) {
	a := startAddon(t, AddonConfig{})
	a.send(`{"id":1,"method":"hello","params":{"id":"demo","protocol":{"min":"3.0.0","max":"3.1.0"},"permissions":[]}}`)
	wantCode(t, a.recv(), AddonErrVersion)
	a.closed()
}

func TestAddonHelloOnce(t *testing.T) {
	a := startAddon(t, AddonConfig{})
	a.hello()
	wantCode(t, a.request(2, "hello", `{"id":"demo"}`), AddonErrAlreadyHello)
}

// Every command the running Sona answers maps to one permission an add-on
// can hold; the local ones (help, completion) to none.
func TestEveryCommandMapsToAPermission(t *testing.T) {
	for _, c := range commands {
		perm, ok := RequestPermission(Request{Command: c.name})
		if c.local || c.ownerOnly {
			if ok {
				t.Errorf("local or owner-only command %s has permission %q", c.name, perm)
			}
			continue
		}
		if !ok || !KnownPermission(perm) {
			t.Errorf("command %s: permission %q (known %v)", c.name, perm, ok)
		}
	}
	for name := range commandPermission {
		if lookup(name) == nil {
			t.Errorf("permission for %q, which is no command", name)
		}
	}
	for _, topic := range Topics {
		// "" is the settings topic: the add-on's own data needs none.
		if perm, ok := TopicPermission(topic); !ok || (perm != "" && !KnownPermission(perm)) || (perm == "" && topic != TopicSettings) {
			t.Errorf("topic %s: permission %q", topic, perm)
		}
	}
	// Listing the queue reads it; its verbs change it.
	if p, _ := RequestPermission(Request{Command: "queue"}); p != PermQueueRead {
		t.Errorf("queue: %q", p)
	}
	for _, verb := range [][]string{{"clear"}, {"jump", "2"}} {
		if p, _ := RequestPermission(Request{Command: "queue", Args: verb}); p != PermQueueControl {
			t.Errorf("queue %v: %q", verb, p)
		}
	}
	guarded := []string{PermQueueControl}
	for _, q := range commandPermission {
		guarded = append(guarded, q)
	}
	for _, q := range topicPermission {
		guarded = append(guarded, q)
	}
	for _, p := range Permissions {
		if p == PermNetwork {
			continue // the OS sandbox's, not the protocol's
		}
		if !slices.Contains(guarded, p) {
			t.Errorf("permission %s guards nothing", p)
		}
	}
}

// Permissions are asked at every call: revoking one refuses the very next
// call, and events stop at the next change.
func TestAddonPermissionsCheckedPerCall(t *testing.T) {
	a := startAddon(t, AddonConfig{}, PermPlaybackControl, PermPlaybackRead)
	a.hello(PermPlaybackControl, PermPlaybackRead)
	if f := a.request(2, "call", `{"command":"play"}`); f.Error != nil {
		t.Fatalf("play with permission: %+v", f.Error)
	}
	a.grants.revoke(PermPlaybackControl)
	wantCode(t, a.request(3, "call", `{"command":"play"}`), AddonErrPermission)
	wantCode(t, a.request(4, "call", `{"command":"next"}`), AddonErrPermission)
	if f := a.request(5, "call", `{"command":"status"}`); f.Error != nil {
		t.Fatalf("status keeps its permission: %+v", f.Error)
	}
	if n := len(a.host.called()); n != 2 {
		t.Errorf("host ran %d commands, want play and status", n)
	}
	// Events follow the grant too.
	if f := a.request(6, "subscribe", `{"topics":["status"]}`); f.Error != nil {
		t.Fatal(f.Error)
	}
	a.grants.revoke(PermPlaybackRead)
	asked := a.grants.asked(PermPlaybackRead)
	a.host.hub.Publish(TopicStatus, Status{Running: true, Volume: 1})
	// Wait until the session has looked at that event, and dropped it.
	for deadline := time.Now().Add(wait); a.grants.asked(PermPlaybackRead) == asked; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the session never looked at the event")
		}
	}
	wantCode(t, a.request(7, "call", `{"command":"status"}`), AddonErrPermission)
	a.grants.grant(PermPlaybackRead)
	a.host.hub.Publish(TopicStatus, Status{Running: true, Volume: 2})
	ev := a.recv()
	if ev.Event != "status" || !strings.Contains(string(ev.Data), `"volume":2`) {
		t.Fatalf("first event after the revoked one = %+v %s", ev, ev.Data)
	}
}

func TestAddonCannotSwitchPrivateModeOff(t *testing.T) {
	a := startAddon(t, AddonConfig{}, PermPrivateRead)
	a.hello(PermPrivateRead)
	a.host.call = func(_ context.Context, r Request) (any, error) { return Private{On: true, Source: SourceSetting}, nil }
	for i, args := range []string{`["off"]`, `["on"]`, `["toggle"]`, `["status","off"]`, `["bogus"]`} {
		wantCode(t, a.request(int64(10+i), "call", `{"command":"private","args":`+args+`}`), AddonErrPermission)
	}
	for i, params := range []string{`{"command":"private"}`, `{"command":"private","args":["status"]}`} {
		if f := a.request(int64(20+i), "call", params); f.Error != nil {
			t.Errorf("%s: %+v", params, f.Error)
		}
	}
	if got := a.host.called(); len(got) != 2 {
		t.Errorf("host ran %+v, want only the two reads", got)
	}
}

func TestAddonSpatialPermissions(t *testing.T) {
	// playback.read alone reads Spatial Audio; switching it needs playback.control.
	a := startAddon(t, AddonConfig{}, PermPlaybackRead)
	a.hello(PermPlaybackRead)
	a.host.call = func(_ context.Context, r Request) (any, error) { return Spatial{On: true}, nil }
	for i, args := range []string{`["on"]`, `["off"]`, `["toggle"]`, `["bogus"]`, `["status","on"]`} {
		wantCode(t, a.request(int64(10+i), "call", `{"command":"spatial","args":`+args+`}`), AddonErrPermission)
	}
	for i, params := range []string{`{"command":"spatial"}`, `{"command":"spatial","args":["status"]}`} {
		if f := a.request(int64(20+i), "call", params); f.Error != nil {
			t.Errorf("%s: %+v", params, f.Error)
		}
	}
	if got := a.host.called(); len(got) != 2 {
		t.Errorf("host ran %+v, want only the two reads", got)
	}

	c := startAddon(t, AddonConfig{}, PermPlaybackControl)
	c.hello(PermPlaybackControl)
	c.host.call = func(_ context.Context, r Request) (any, error) { return Spatial{On: true}, nil }
	if f := c.request(30, "call", `{"command":"spatial","args":["toggle"]}`); f.Error != nil {
		t.Errorf("toggle with playback.control: %+v", f.Error)
	}
	wantCode(t, c.request(31, "call", `{"command":"spatial"}`), AddonErrPermission)
}

func TestAddonUnknownCommands(t *testing.T) {
	a := startAddon(t, AddonConfig{}, Permissions...)
	a.hello()
	for i, params := range []string{`{"command":"help"}`, `{"command":"completion","args":["bash"]}`, `{"command":"hello"}`, `{"command":""}`, `{}`, `{"command":"play;rm"}`} {
		wantCode(t, a.request(int64(10+i), "call", params), CodeUnknownCommand)
	}
	wantCode(t, a.request(30, "frobnicate", ""), AddonErrUnknownMethod)
	wantCode(t, a.request(31, "invoke", `{"action":"addon.demo.x"}`), AddonErrUnknownMethod) // only Sona invokes
	wantCode(t, a.request(32, "call", `"play"`), AddonErrBadArguments)
	wantCode(t, a.request(33, "call", `{"command":"volume","args":["loud"]}`), AddonErrBadArguments)
	wantCode(t, a.request(34, "call", `{"command":"search"}`), AddonErrBadArguments)
	if n := len(a.host.called()); n != 0 {
		t.Errorf("host ran %d refused commands", n)
	}
}

// An error reply carries a code and the fixed message: never the text of
// the error, which here holds an Apple URL, a token and a path.
func TestAddonErrorsNeverCarryErrorText(t *testing.T) {
	leak := `Get "https://amp-api.music.apple.com/v1/catalog/gb/search?term=x&token=eyJabc": dial tcp 17.1.2.3:443: connect: refused (/home/me/.config/sona/secret)`
	a := startAddon(t, AddonConfig{}, PermSearch, PermLike)
	a.hello()
	for i, err := range []error{
		&Error{Code: "network", Message: leak},
		&Error{Code: "something_new", Message: leak},
		&Error{Code: CodeFailed, Message: leak},
		errors.New(leak),
		context.DeadlineExceeded,
	} {
		a.host.call = func(context.Context, Request) (any, error) { return nil, err }
		f := a.request(int64(10+i), "call", `{"command":"search","args":["x"]}`)
		if f.Error == nil {
			t.Fatalf("no error for %v", err)
		}
		raw, _ := json.Marshal(f)
		for _, bad := range []string{"apple", "token", "17.1.2.3", "/home", "dial"} {
			if strings.Contains(string(raw), bad) {
				t.Errorf("error %v leaked %q: %s", err, bad, raw)
			}
		}
		if f.Error.Message != addonMessages[f.Error.Code] {
			t.Errorf("message %q is not the fixed one for %q", f.Error.Message, f.Error.Code)
		}
		if i == 0 && f.Error.Code != "network" {
			t.Errorf("network error became %q", f.Error.Code)
		}
		if i > 0 && i < 4 && f.Error.Code != AddonErrFailed {
			t.Errorf("error %d became %q, want failed", i, f.Error.Code)
		}
		if i == 4 && f.Error.Code != AddonErrTimeout {
			t.Errorf("deadline became %q, want timeout", f.Error.Code)
		}
	}
	// A bad usage keeps its meaning without its text.
	a.host.call = func(context.Context, Request) (any, error) {
		return nil, Usage(msgBadVolume, "https://amp-api.music.apple.com")
	}
	f := a.request(40, "call", `{"command":"like"}`)
	wantCode(t, f, AddonErrBadArguments)
	for _, code := range AddonErrorCodes() {
		if addonMessages[code] == "" || strings.ContainsAny(addonMessages[code], "%\n") {
			t.Errorf("message of %q = %q", code, addonMessages[code])
		}
	}
}

func TestAddonFrameSizeLimit(t *testing.T) {
	a := startAddon(t, AddonConfig{Limits: AddonLimits{MaxFrame: 1024}}, PermPlaybackRead)
	a.hello(PermPlaybackRead)
	// Exactly the limit passes.
	pad := strings.Repeat("a", 1024-len(`{"id":2,"method":"call","params":{"command":"status","args":[""]}}`))
	a.send(`{"id":2,"method":"call","params":{"command":"status","args":["` + pad + `"]}}`)
	if f := a.answer(2); f.Error == nil || f.Error.Code != AddonErrBadArguments {
		// status takes no arguments: refused for that, not for its size.
		t.Fatalf("a frame at the limit: %+v", f)
	}
	a.send(`{"id":3,"method":"call","params":{"command":"status","args":["` + strings.Repeat("a", 1100) + `"]}}`)
	wantCode(t, a.recv(), AddonErrFrameTooLarge)
	a.closed()
	if n := len(a.host.called()); n != 0 {
		t.Errorf("host ran %d commands", n)
	}
}

func TestAddonDefaultFrameLimitIsOneMiB(t *testing.T) {
	if AddonMaxFrame != 1<<20 || (AddonLimits{}).withDefaults().MaxFrame != 1<<20 {
		t.Fatalf("default frame limit = %d", AddonMaxFrame)
	}
	a := startAddon(t, AddonConfig{}, PermSearch)
	a.hello()
	// A 1 MiB result that doesn't fit is an error, not a truncated frame.
	a.host.call = func(context.Context, Request) (any, error) {
		return SearchResults{Term: strings.Repeat("x", AddonMaxFrame)}, nil
	}
	wantCode(t, a.request(2, "call", `{"command":"search","args":["x"]}`), AddonErrResultTooLarge)
	go func() { // the session stops reading at the limit, so this write fails
		_, _ = io.WriteString(a.conn, `{"id":3,"method":"call","params":{"command":"search","args":["`+strings.Repeat("a", AddonMaxFrame)+`"]}}`+"\n")
	}()
	wantCode(t, a.recv(), AddonErrFrameTooLarge)
	a.closed()
}

func TestAddonCallsInFlightLimit(t *testing.T) {
	a := startAddon(t, AddonConfig{Limits: AddonLimits{Calls: 2}}, PermPlaybackRead)
	a.hello()
	release := make(chan struct{})
	started := make(chan struct{}, 8)
	a.host.call = func(ctx context.Context, _ Request) (any, error) {
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return Status{Running: true}, nil
	}
	a.send(`{"id":2,"method":"call","params":{"command":"status"}}`)
	a.send(`{"id":3,"method":"call","params":{"command":"status"}}`)
	<-started
	<-started
	a.send(`{"id":4,"method":"call","params":{"command":"status"}}`)
	wantCode(t, a.recv(), AddonErrBusy)
	close(release)
	got := map[int64]bool{}
	for range 2 {
		f := a.recv()
		got[f.ID] = f.Error == nil
	}
	if !got[2] || !got[3] {
		t.Errorf("the two calls in flight answered %v", got)
	}
	// Room again.
	if f := a.request(5, "call", `{"command":"status"}`); f.Error != nil {
		t.Errorf("after they finished: %+v", f.Error)
	}
}

func TestAddonArgumentLimits(t *testing.T) {
	a := startAddon(t, AddonConfig{Limits: AddonLimits{Args: 3, ArgBytes: 16}}, PermSearch)
	a.hello()
	wantCode(t, a.request(2, "call", `{"command":"search","args":["a","b","c","d"]}`), AddonErrBadArguments)
	wantCode(t, a.request(3, "call", `{"command":"search","args":["`+strings.Repeat("x", 17)+`"]}`), AddonErrBadArguments)
	if f := a.request(4, "call", `{"command":"search","args":["a","b","`+strings.Repeat("x", 16)+`"]}`); f.Error != nil {
		t.Errorf("at the limits: %+v", f.Error)
	}
	if n := len(a.host.called()); n != 1 {
		t.Errorf("host ran %d commands, want the one inside the limits", n)
	}
}

func TestAddonSubscriptionLimits(t *testing.T) {
	a := startAddon(t, AddonConfig{Limits: AddonLimits{Subscriptions: 2}}, PermPlaybackRead, PermQueueRead)
	a.hello()
	wantCode(t, a.request(2, "subscribe", `{"topics":["status","now-playing","queue"]}`), AddonErrTooMany)
	if f := a.request(3, "subscribe", `{"topics":["status","queue"]}`); f.Error != nil {
		t.Fatalf("two topics: %+v", f.Error)
	}
	// A third one is more than are allowed at once; one already held is not.
	wantCode(t, a.request(4, "subscribe", `{"topics":["now-playing"]}`), AddonErrTooMany)
	if f := a.request(5, "subscribe", `{"topics":["status"]}`); f.Error != nil || string(f.Result) != `{"topics":["status","queue"]}` {
		t.Fatalf("again: %+v %s", f.Error, f.Result)
	}
	wantCode(t, a.request(6, "subscribe", `{"topics":["nope"]}`), AddonErrUnknownTopic)
	if f := a.request(7, "unsubscribe", `{"topics":["queue"]}`); f.Error != nil || string(f.Result) != `{"topics":["status"]}` {
		t.Fatalf("unsubscribe: %+v %s", f.Error, f.Result)
	}
	if f := a.request(8, "subscribe", `{"topics":["now-playing"]}`); f.Error != nil {
		t.Fatalf("after unsubscribing: %+v", f.Error)
	}
	// An unsubscribed topic sends nothing more.
	a.host.hub.Publish(TopicQueue, Queue{Index: -1})
	a.host.hub.Publish(TopicStatus, Status{Running: true})
	if ev := a.recv(); ev.Event != "status" {
		t.Fatalf("got %s after unsubscribing from queue", ev.Event)
	}
}

// An add-on that sends and never reads doesn't make Sona queue without
// end: past the limit its session ends.
func TestAddonQueueLimit(t *testing.T) {
	a := startAddon(t, AddonConfig{Limits: AddonLimits{Queue: 8}})
	a.hello()
	// Nothing is read from now on: the pipe holds no data, so the writer
	// stalls on the first reply and the rest pile up.
	go func() {
		for i := range 100 {
			_ = a.conn.SetWriteDeadline(time.Now().Add(wait))
			if _, err := io.WriteString(a.conn, `{"id":`+itoa(int64(i+2))+`,"method":"nope"}`+"\n"); err != nil {
				return
			}
		}
	}()
	select {
	case <-a.s.Done():
	case <-time.After(wait):
		t.Fatal("a session whose add-on doesn't read didn't end")
	}
}

// A slow reader gets the latest status, and the publisher never waits.
func TestAddonSlowReaderGetsTheLatest(t *testing.T) {
	a := startAddon(t, AddonConfig{}, PermPlaybackRead)
	a.hello()
	if f := a.request(2, "subscribe", `{"topics":["status"]}`); f.Error != nil {
		t.Fatal(f.Error)
	}
	const n = 500
	start := time.Now()
	for i := 1; i <= n; i++ {
		a.host.hub.Publish(TopicStatus, Status{Running: true, Volume: i})
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("publishing %d statuses to a reader that doesn't read took %v", n, d)
	}
	var seen []int
	for {
		ev := a.recv()
		var st Status
		if err := json.Unmarshal(ev.Data, &st); err != nil {
			t.Fatal(err)
		}
		seen = append(seen, st.Volume)
		if st.Volume == n {
			break
		}
	}
	if len(seen) > 5 {
		t.Errorf("the slow reader got %d events (%v …), want the latest and at most a few before it", len(seen), seen[:5])
	}
}

func TestAddonRateLimits(t *testing.T) {
	var mu sync.Mutex
	now := time.Unix(1000, 0)
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	a := startAddon(t, AddonConfig{
		Now:   clock,
		Rates: map[string]Rate{PermSearch: {PerSecond: 1, Burst: 2}, PermLike: {PerSecond: 0.5, Burst: 1}, "": {PerSecond: 100, Burst: 100}},
	}, PermSearch, PermLike, PermPlaybackRead)
	a.hello()
	search := func(id int64) Frame { return a.request(id, "call", `{"command":"search","args":["x"]}`) }
	if search(10).Error != nil || search(11).Error != nil {
		t.Fatal("a burst of two searches is refused")
	}
	wantCode(t, search(12), AddonErrRateLimited)
	// The like limit is its own: searches don't spend it.
	if f := a.request(13, "call", `{"command":"like"}`); f.Error != nil {
		t.Fatalf("like: %+v", f.Error)
	}
	wantCode(t, a.request(14, "call", `{"command":"like"}`), AddonErrRateLimited)
	mu.Lock()
	now = now.Add(1100 * time.Millisecond)
	mu.Unlock()
	if f := search(15); f.Error != nil {
		t.Fatalf("a second later: %+v", f.Error)
	}
	wantCode(t, search(16), AddonErrRateLimited)
	// Other commands are not held to the search or like rates.
	for i := range 20 {
		if f := a.request(int64(30+i), "call", `{"command":"status"}`); f.Error != nil {
			t.Fatalf("status %d: %+v", i, f.Error)
		}
	}
	if n := len(a.host.called()); n != 4+20 { // 3 searches, 1 like, 20 statuses
		t.Errorf("host ran %d commands, want 24", n)
	}
}

func TestDefaultRatesLimitSearchLikeAndOpen(t *testing.T) {
	for _, p := range []string{PermSearch, PermLike, PermOpen} {
		r, ok := defaultRates[p]
		if !ok || r.Burst < 1 || r.PerSecond <= 0 || r.PerSecond > 2 {
			t.Errorf("rate of %s = %+v", p, r)
		}
	}
	if defaultRates[""].Burst < 1 {
		t.Error("no rate for the other commands")
	}
}

// Events and answers never carry an Apple API host: artwork only from the
// public CDN, links only on music.apple.com.
func TestAddonEventsCarryOnlyPublicURLs(t *testing.T) {
	cdn := "https://is1-ssl.mzstatic.com/image/thumb/Music/a/600x600bb.jpg"
	for in, want := range map[string]string{
		cdn: cdn,
		"https://amp-api.music.apple.com/v1/catalog/gb/songs/1/artwork.jpg": "",
		"http://is1-ssl.mzstatic.com/a.jpg":                                 "",
		"https://user:pw@is1-ssl.mzstatic.com/a.jpg":                        "",
		"https://is1-ssl.mzstatic.com/a.jpg?token=x":                        "",
		"https://is1-ssl.mzstatic.com:8443/a.jpg":                           "",
		"https://evil.example/mzstatic.com/a.jpg":                           "",
		"https://mzstatic.com/a.jpg":                                        "",
		"https://notmzstatic.com/a.jpg":                                     "",
		"file:///etc/passwd":                                                "",
		"/art/abc/600x600.jpg":                                              "",
		"":                                                                  "",
	} {
		if got := PublicArtwork(in); got != want {
			t.Errorf("PublicArtwork(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{
		"https://music.apple.com/gb/album/abbey-road/1441164426": "https://music.apple.com/gb/album/abbey-road/1441164426",
		"https://amp-api.music.apple.com/v1/catalog/gb/albums/1": "",
		"https://music.apple.com.evil.example/gb/album/x/1":      "",
		"https://user@music.apple.com/gb/album/x/1":              "",
		"http://music.apple.com/gb/album/x/1":                    "",
		"music://music.apple.com/gb/album/x/1":                   "",
		"https://api.music.apple.com/v1/x":                       "",
	} {
		if got := PublicShareURL(in); got != want {
			t.Errorf("PublicShareURL(%q) = %q, want %q", in, got, want)
		}
	}

	a := startAddon(t, AddonConfig{}, PermPlaybackRead, PermSearch)
	a.hello()
	if f := a.request(2, "subscribe", `{"topics":["status"]}`); f.Error != nil {
		t.Fatal(f.Error)
	}
	api := "https://amp-api.music.apple.com/v1/x.jpg"
	a.host.hub.Publish(TopicStatus, Status{Running: true, Track: &Track{ID: "1", Artwork: api}})
	ev := a.recv()
	var st Status
	_ = json.Unmarshal(ev.Data, &st)
	if st.Track == nil || st.Track.Artwork != "" || st.Track.ID != "1" {
		t.Errorf("event track = %+v", st.Track)
	}
	a.host.call = func(context.Context, Request) (any, error) {
		return SearchResults{Results: []SearchResult{
			{Kind: "album", ID: "1", URL: "https://music.apple.com/gb/album/x/1"},
			{Kind: "album", ID: "2", URL: "https://amp-api.music.apple.com/v1/catalog/gb/albums/2"},
		}}, nil
	}
	f := a.request(3, "call", `{"command":"search","args":["x"]}`)
	var res SearchResults
	_ = json.Unmarshal(f.Result, &res)
	if len(res.Results) != 2 || res.Results[0].URL == "" || res.Results[1].URL != "" {
		t.Errorf("search results = %+v", res.Results)
	}
}

func TestAddonQueueWindow(t *testing.T) {
	q := Queue{Index: 1500, Tracks: make([]QueueTrack, 3000)}
	for i := range q.Tracks {
		q.Tracks[i].ID = itoa(int64(i))
	}
	w := windowQueue(q)
	if len(w.Tracks) != addonMaxQueueTracks || w.Tracks[w.Index].ID != "1500" {
		t.Fatalf("window = %d tracks, index %d → %q", len(w.Tracks), w.Index, w.Tracks[w.Index].ID)
	}
	q.Index = 3
	if w := windowQueue(q); w.Index != 3 || w.Tracks[0].ID != "0" {
		t.Errorf("window near the start = index %d, first %q", w.Index, w.Tracks[0].ID)
	}
	q.Index = 2999
	if w := windowQueue(q); w.Tracks[w.Index].ID != "2999" || len(w.Tracks) != addonMaxQueueTracks {
		t.Errorf("window at the end = index %d", w.Index)
	}
	q.Index = -1
	if w := windowQueue(q); w.Index != -1 {
		t.Errorf("no playing song: index %d", w.Index)
	}
	if w := windowQueue(Queue{Index: -1}); w.Tracks == nil {
		t.Error("an empty queue encodes as null, not []")
	}
}

func TestAddonInvoke(t *testing.T) {
	a := startAddon(t, AddonConfig{}, PermPlaybackRead)
	if _, err := a.s.Invoke(t.Context(), "addon.demo.x", nil); AsError(err).Code != AddonErrNotRunning {
		t.Errorf("before hello: %v", err)
	}
	a.hello()

	type out struct {
		res InvokeResult
		err error
	}
	invoke := func(ctx context.Context, action string) chan out {
		ch := make(chan out, 1)
		go func() {
			r, err := a.s.Invoke(ctx, action, json.RawMessage(`{"kind":"album","id":"1"}`))
			ch <- out{r, err}
		}()
		return ch
	}
	next := func() Frame {
		for {
			f := a.recv()
			if f.Method == AddonMethodInvoke {
				return f
			}
		}
	}

	// An answer comes back, and the add-on was told the deadline Sona set.
	got := invoke(t.Context(), "addon.demo.copy")
	req := next()
	var p InvokeParams
	if err := json.Unmarshal(req.Params, &p); err != nil || p.Action != "addon.demo.copy" || string(p.Target) != `{"kind":"album","id":"1"}` {
		t.Fatalf("invoke params = %+v (%v)", p, err)
	}
	if p.DeadlineMs <= 0 || p.DeadlineMs > DefaultInvokeDeadline.Milliseconds() {
		t.Errorf("deadline = %d ms, want within %v", p.DeadlineMs, DefaultInvokeDeadline)
	}
	a.send(`{"id":` + itoa(req.ID) + `,"result":{"toast":"Copied","url":"https://song.link/x"}}`)
	if o := <-got; o.err != nil || o.res.Toast != "Copied" || o.res.URL != "https://song.link/x" {
		t.Fatalf("invoke = %+v", o)
	}

	// A caller's deadline shortens it; a long one is capped.
	ctx, cancel := context.WithTimeout(t.Context(), time.Hour)
	got = invoke(ctx, "addon.demo.long")
	req = next()
	_ = json.Unmarshal(req.Params, &p)
	if p.DeadlineMs > MaxInvokeDeadline.Milliseconds() {
		t.Errorf("deadline = %d ms, above the cap %v", p.DeadlineMs, MaxInvokeDeadline)
	}
	cancel()
	if o := <-got; AsError(o.err).Code != AddonErrTimeout {
		t.Errorf("cancelled invoke = %v", o.err)
	}

	// An add-on that doesn't answer costs the caller the deadline and
	// nothing else; its late answer is dropped.
	ctx, cancel = context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	got = invoke(ctx, "addon.demo.hang")
	late := next()
	if o := <-got; AsError(o.err).Code != AddonErrTimeout {
		t.Fatalf("hung invoke = %v", o.err)
	}
	a.send(`{"id":` + itoa(late.ID) + `,"result":{"toast":"too late"}}`)
	if f := a.request(2, "call", `{"command":"status"}`); f.Error != nil {
		t.Errorf("the session after a late answer: %+v", f.Error)
	}

	// An error from the add-on keeps its code, not its words.
	got = invoke(t.Context(), "addon.demo.fail")
	req = next()
	a.send(`{"id":` + itoa(req.ID) + `,"error":{"code":"made_up","message":"see https://evil.example"}}`)
	if o := <-got; AsError(o.err).Code != AddonErrFailed || strings.Contains(o.err.Error(), "evil") {
		t.Errorf("add-on error = %v", o.err)
	}

	// Answers that break the rules are refused, not passed on.
	for name, result := range map[string]string{
		"javascript url": `{"url":"javascript:alert(1)"}`,
		"file url":       `{"url":"file:///etc/passwd"}`,
		"credentials":    `{"url":"https://user:pw@example.com/"}`,
		"long toast":     `{"toast":"` + strings.Repeat("x", MaxToast+1) + `"}`,
		"not an object":  `"hi"`,
	} {
		got = invoke(t.Context(), "addon.demo.bad")
		req = next()
		a.send(`{"id":` + itoa(req.ID) + `,"result":` + result + `}`)
		if o := <-got; AsError(o.err).Code != AddonErrFailed {
			t.Errorf("%s: invoke = %+v, %v", name, o.res, o.err)
		}
	}
}

func TestAddonInvokesInFlightLimit(t *testing.T) {
	a := startAddon(t, AddonConfig{Limits: AddonLimits{Invokes: 2}})
	a.hello()
	ctx, cancel := context.WithTimeout(t.Context(), wait)
	defer cancel()
	for range 2 {
		go func() { _, _ = a.s.Invoke(ctx, "addon.demo.slow", nil) }()
	}
	for range 2 {
		for a.recv().Method != AddonMethodInvoke {
		}
	}
	if _, err := a.s.Invoke(ctx, "addon.demo.third", nil); AsError(err).Code != AddonErrBusy {
		t.Fatalf("third invoke = %v, want busy", err)
	}
}

func TestAddonCloseSaysShutdown(t *testing.T) {
	a := startAddon(t, AddonConfig{})
	a.hello()
	a.s.Close()
	if ev := a.recv(); ev.Event != EventShutdown {
		t.Fatalf("after Close: %+v", ev)
	}
	a.closed()
	if _, err := a.s.Invoke(t.Context(), "addon.demo.x", nil); AsError(err).Code != AddonErrNotRunning {
		t.Errorf("invoke after close: %v", err)
	}
}

// The v1 handler (D-Bus Run, the socket's run frame) has no v2 method:
// hello and the rest are not commands, whatever add-on ID they name.
func TestV1RefusesAddonMethods(t *testing.T) {
	srv := &countingServer{}
	for _, raw := range []string{
		`{"command":"hello","args":["demo"]}`,
		`{"id":1,"method":"hello","params":{"id":"demo","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":["playback.read"]}}`,
		`{"command":"subscribe","args":["status"]}`,
		`{"command":"call","args":["status"]}`,
		`{"command":"invoke","args":["addon.demo.x"]}`,
	} {
		r := Handle(t.Context(), srv, []byte(raw))
		if r.Error == nil || r.Error.Code != CodeUnknownCommand || r.Result != nil {
			t.Errorf("%s → %+v", raw, r)
		}
	}
	if srv.calls != 0 {
		t.Errorf("the server ran %d commands", srv.calls)
	}
}

// After hello a v1-shaped frame (a command, not a method) is refused and
// runs nothing.
func TestAddonRefusesV1FramesAfterHello(t *testing.T) {
	a := startAddon(t, AddonConfig{}, PermPlaybackRead, PermPlaybackControl)
	a.hello(PermPlaybackRead, PermPlaybackControl)
	for _, raw := range []string{
		`{"command":"status"}`,
		`{"id":7,"command":"pause","args":[]}`,
		`{"command":"call","args":["status"]}`,
	} {
		a.send(raw)
		f := a.recv()
		wantCode(t, f, AddonErrBadFrame)
	}
	a.host.mu.Lock()
	defer a.host.mu.Unlock()
	if len(a.host.calls) != 0 {
		t.Errorf("the host ran %+v", a.host.calls)
	}
}

type countingServer struct{ calls int }

func (s *countingServer) Call(context.Context, Request) (any, error) { s.calls++; return nil, nil }
func (s *countingServer) Watch() (<-chan Status, func())             { return nil, func() {} }

func TestHubDropsRepeatsAndKeepsTheLatest(t *testing.T) {
	h := NewHub()
	h.Publish("t", 1)
	ch, cancel := h.Watch("t")
	if v := <-ch; v != 1 {
		t.Fatalf("current = %v", v)
	}
	h.Publish("t", 2)
	h.Publish("t", 2) // a repeat
	h.Publish("t", 3) // replaces the 2 nobody took
	if v := <-ch; v != 3 {
		t.Fatalf("latest = %v", v)
	}
	select {
	case v := <-ch:
		t.Fatalf("extra value %v", v)
	default:
	}
	cancel()
	if _, ok := <-ch; ok {
		t.Error("channel open after cancel")
	}
	h.Close()
	if _, ok := <-func() <-chan any { c, _ := h.Watch("t"); return c }(); ok {
		t.Error("a watch after Close is open")
	}
}
