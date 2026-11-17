package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// docs/addons.md is the protocol's documentation and its JSON schema. These
// tests keep the two true to each other and to the code: every example in
// the page validates against the schema, every frame the session sends
// does too, and the page names every code, permission, topic and command.

const addonsDoc = "../../docs/addons.md"

// docBlocks returns the fenced blocks of the page whose info string starts
// with "json ": kind ("schema" or "example") and the name after it.
func docBlocks(t *testing.T) (schema map[string]any, examples []docExample, text string) {
	t.Helper()
	b, err := os.ReadFile(addonsDoc)
	if err != nil {
		t.Fatal(err)
	}
	text = string(b)
	fence := regexp.MustCompile("(?s)```json (schema|example)([^\n]*)\n(.*?)\n```")
	for _, m := range fence.FindAllStringSubmatch(text, -1) {
		switch m[1] {
		case "schema":
			if schema != nil {
				t.Fatal("two schemas in the page")
			}
			if err := json.Unmarshal([]byte(m[3]), &schema); err != nil {
				t.Fatalf("the schema is not JSON: %v", err)
			}
		case "example":
			examples = append(examples, docExample{name: strings.TrimSpace(m[2]), body: m[3]})
		}
	}
	if schema == nil {
		t.Fatal("no schema in the page")
	}
	return schema, examples, text
}

type docExample struct{ name, body string }

// validate checks v (decoded JSON) against the subset of JSON Schema the
// page uses; a keyword outside the subset is an error, so the page cannot
// quietly rely on one that nothing checks.
type validator struct{ root map[string]any }

func (vl validator) check(schema map[string]any, v any, path string) []string {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, path+": "+fmt.Sprintf(format, args...)) }
	for kw, arg := range schema {
		switch kw {
		case "$ref":
			ref := arg.(string)
			def, ok := vl.root["$defs"].(map[string]any)[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
			if !ok || !strings.HasPrefix(ref, "#/$defs/") {
				add("unresolved $ref %q", ref)
				continue
			}
			errs = append(errs, vl.check(def, v, path)...)
		case "type":
			if !typeMatches(arg.(string), v) {
				add("is not a %s: %v", arg, v)
			}
		case "enum":
			if !slices.ContainsFunc(arg.([]any), func(o any) bool { return o == v }) {
				add("%v is not one of %v", v, arg)
			}
		case "const":
			if arg != v {
				add("%v is not %v", v, arg)
			}
		case "pattern":
			if s, ok := v.(string); ok && !regexp.MustCompile(arg.(string)).MatchString(s) {
				add("%q doesn't match %s", s, arg)
			}
		case "minLength", "maxLength":
			if s, ok := v.(string); ok {
				n := float64(len(s))
				if (kw == "minLength" && n < arg.(float64)) || (kw == "maxLength" && n > arg.(float64)) {
					add("length %v breaks %s %v", n, kw, arg)
				}
			}
		case "minimum", "maximum":
			if n, ok := v.(float64); ok {
				if (kw == "minimum" && n < arg.(float64)) || (kw == "maximum" && n > arg.(float64)) {
					add("%v breaks %s %v", n, kw, arg)
				}
			}
		case "minItems", "maxItems", "uniqueItems", "items":
			a, ok := v.([]any)
			if !ok {
				continue
			}
			switch kw {
			case "minItems":
				if float64(len(a)) < arg.(float64) {
					add("%d items, at least %v", len(a), arg)
				}
			case "maxItems":
				if float64(len(a)) > arg.(float64) {
					add("%d items, at most %v", len(a), arg)
				}
			case "uniqueItems":
				for i := range a {
					for j := range a[:i] {
						if arg == true && fmt.Sprint(a[i]) == fmt.Sprint(a[j]) {
							add("items %d and %d are equal", j, i)
						}
					}
				}
			case "items":
				for i, e := range a {
					errs = append(errs, vl.check(arg.(map[string]any), e, fmt.Sprintf("%s[%d]", path, i))...)
				}
			}
		case "required", "properties", "additionalProperties":
			o, ok := v.(map[string]any)
			if !ok {
				continue
			}
			props, _ := schema["properties"].(map[string]any)
			switch kw {
			case "required":
				for _, k := range arg.([]any) {
					if _, ok := o[k.(string)]; !ok {
						add("missing %q", k)
					}
				}
			case "properties":
				for k, sub := range props {
					if e, ok := o[k]; ok {
						errs = append(errs, vl.check(sub.(map[string]any), e, path+"."+k)...)
					}
				}
			case "additionalProperties":
				if arg == false {
					for k := range o {
						if _, ok := props[k]; !ok {
							add("unexpected %q", k)
						}
					}
				}
			}
		case "oneOf":
			n := 0
			for _, sub := range arg.([]any) {
				if len(vl.check(sub.(map[string]any), v, path)) == 0 {
					n++
				}
			}
			if n != 1 {
				add("matches %d of the oneOf alternatives, want 1", n)
			}
		case "$schema", "$id", "$defs":
		default:
			add("the page's schema uses %q, which the test doesn't check", kw)
		}
	}
	return errs
}

func typeMatches(typ string, v any) bool {
	switch typ {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		n, ok := v.(float64)
		return ok && n == float64(int64(n))
	}
	return false
}

// validateDef checks raw JSON against the page's definition name.
func validateDef(t *testing.T, schema map[string]any, name string, raw []byte) []string {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return []string{"not JSON: " + err.Error()}
	}
	return validator{schema}.check(map[string]any{"$ref": "#/$defs/" + name}, v, "$")
}

func TestAddonDocExamplesValidate(t *testing.T) {
	schema, examples, _ := docBlocks(t)
	if len(examples) < 15 {
		t.Fatalf("only %d examples in the page", len(examples))
	}
	for _, ex := range examples {
		def := ex.name
		if strings.HasPrefix(def, "Manifest") {
			def = "Manifest"
		}
		if errs := validateDef(t, schema, def, []byte(ex.body)); len(errs) != 0 {
			t.Errorf("example %s %s: %v", ex.name, ex.body, errs)
		}
		if strings.HasPrefix(ex.name, "Manifest") {
			continue // a manifest is a file, not a frame
		}
		if errs := validateDef(t, schema, "Frame", []byte(ex.body)); len(errs) != 0 {
			t.Errorf("example %s is not a frame: %v", ex.name, errs)
		}
	}
}

// The validator must refuse what the rules forbid, or the tests above
// prove nothing.
func TestAddonDocSchemaRefusesWhatItForbids(t *testing.T) {
	schema, _, _ := docBlocks(t)
	for name, tc := range map[string]struct{ def, frame string }{
		"artwork on an Apple API host":          {"NowPlayingEvent", `{"event":"now-playing","data":{"track":{"id":"1","title":"t","artist":"a","album":"b","duration":1,"explicit":false,"artwork":"https://amp-api.music.apple.com/x.jpg"},"startedAt":1}}`},
		"private flag set":                      {"StatusEvent", `{"event":"status","data":{"running":true,"state":"idle","track":null,"position":0,"volume":1,"shuffle":false,"repeat":"off","private":true,"preview":false,"error":""}}`},
		"unknown permission":                    {"HelloRequest", `{"id":1,"method":"hello","params":{"id":"demo","protocol":{"min":"2.0","max":"2.0"},"permissions":["root"]}}`},
		"toast of a script URL":                 {"InvokeResponse", `{"id":1,"result":{"url":"file:///etc/passwd"}}`},
		"a share link on another host":          {"SearchResponse", `{"id":1,"result":{"term":"x","results":[{"kind":"song","id":"1","name":"n","subtitle":"","explicit":false,"url":"https://evil.example/music.apple.com"}]}}`},
		"extra field":                           {"ShutdownEvent", `{"event":"shutdown","x":1}`},
		"a frame of no kind":                    {"Frame", `{"hello":1}`},
		"a manifest with a field":               {"Manifest", `{"id":"demo","name":"d","version":"1.0.0","command":"run","permissions":[],"sandbox":"wasm"}`},
		"a manifest command up":                 {"Manifest", `{"id":"demo","name":"d","version":"1.0.0","command":"/bin/sh","permissions":[]}`},
		"a manifest command with a drive":       {"Manifest", `{"id":"demo","name":"d","version":"1.0.0","command":"C:run","permissions":[]}`},
		"a wasm manifest with a native command": {"Manifest", `{"id":"demo","name":"d","version":"1.0.0","command":"run","runtime":"wasm","permissions":[]}`},
		"a manifest with another runtime":       {"Manifest", `{"id":"demo","name":"d","version":"1.0.0","command":"run","runtime":"docker","permissions":[]}`},
		"a wasm manifest asking for network":    {"Manifest", `{"id":"demo","name":"d","version":"1.0.0","command":"run.wasm","runtime":"wasm","permissions":["network"]}`},
	} {
		if errs := validateDef(t, schema, tc.def, []byte(tc.frame)); len(errs) == 0 {
			t.Errorf("%s: the schema accepted %s", name, tc.frame)
		}
	}
}

// The page names everything the code defines.
func TestAddonDocCoversTheCode(t *testing.T) {
	schema, _, text := docBlocks(t)
	for _, c := range AddonErrorCodes() {
		if !strings.Contains(text, "| `"+c+"` |") {
			t.Errorf("error code %q is not in the page's table", c)
		}
	}
	rows := regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\| ").FindAllStringSubmatch(text, -1)
	codes := AddonErrorCodes()
	for _, r := range rows {
		if !slices.Contains(codes, r[1]) && !slices.Contains(Permissions, r[1]) && !slices.Contains(Topics, r[1]) && r[1] != EventShutdown &&
			!slices.Contains([]string{"id", "name", "version", "description", "command", "runtime", "permissions", "contributions", "strings", "toast", "url"}, r[1]) { // the manifest's fields and an invoke result's
			t.Errorf("the page's table has %q, which is not a code", r[1])
		}
	}
	for _, p := range Permissions {
		if !strings.Contains(text, "| `"+p+"` |") {
			t.Errorf("permission %q is not in the page's table", p)
		}
	}
	for _, topic := range Topics {
		if !strings.Contains(text, "`"+topic+"`") {
			t.Errorf("topic %q is not in the page", topic)
		}
	}
	defs := schema["$defs"].(map[string]any)
	enumOf := func(path ...string) []string {
		var v any = defs
		for _, k := range path {
			v = v.(map[string]any)[k]
		}
		var out []string
		for _, e := range v.([]any) {
			out = append(out, e.(string))
		}
		return out
	}
	if got := enumOf("Permission", "enum"); !slices.Equal(got, Permissions) {
		t.Errorf("schema permissions = %v, code %v", got, Permissions)
	}
	if got := enumOf("Topic", "enum"); !slices.Equal(got, Topics) {
		t.Errorf("schema topics = %v, code %v", got, Topics)
	}
	var commands []string
	for c := range commandPermission {
		commands = append(commands, c)
	}
	got := enumOf("CallRequest", "properties", "params", "properties", "command", "enum")
	slices.Sort(commands)
	slices.Sort(got)
	if !slices.Equal(got, commands) {
		t.Errorf("schema commands = %v, code %v", got, commands)
	}
	l := AddonLimits{}.withDefaults()
	for _, want := range []string{
		fmt.Sprintf("| Frame size | %d MiB |", l.MaxFrame>>20),
		fmt.Sprintf("| Calls in flight | %d |", l.Calls),
		fmt.Sprintf("| Invokes awaiting an answer | %d |", l.Invokes),
		fmt.Sprintf("| Subscriptions | %d topics |", l.Subscriptions),
		fmt.Sprintf("| Arguments of a call | %d, each at most %d bytes |", l.Args, l.ArgBytes),
		fmt.Sprintf("| Unread frames queued for the add-on | %d |", l.Queue),
		fmt.Sprintf("| Call rate | %.0f per second, burst %d |", defaultRates[""].PerSecond, defaultRates[""].Burst),
		fmt.Sprintf("| `search` | %.0f per second, burst %d |", defaultRates[PermSearch].PerSecond, defaultRates[PermSearch].Burst),
		fmt.Sprintf("| `like`, `open` | %.0f per second, burst %d |", defaultRates[PermLike].PerSecond, defaultRates[PermLike].Burst),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the page's limits lack %q", want)
		}
	}
	if defaultRates[PermLike] != defaultRates[PermOpen] {
		t.Error("like and open no longer share a rate; the page's row says they do")
	}
	if !strings.Contains(text, "never more than 30") || MaxInvokeDeadline != 30*time.Second || DefaultInvokeDeadline != 10*time.Second {
		t.Error("the invoke deadlines moved; update the page")
	}
	if !strings.Contains(text, "Permissions are advisory until the OS sandbox (#304) lands") {
		t.Error("the page must say permissions are advisory")
	}
}

// recvRaw reads the add-on's next line unparsed.
func (a *testAddon) recvRaw() []byte {
	a.t.Helper()
	_ = a.conn.SetReadDeadline(time.Now().Add(wait))
	line, err := a.r.ReadBytes('\n')
	if err != nil {
		a.t.Fatalf("read: %v", err)
	}
	return line
}

// Every frame a session sends, for every kind of message, is a frame of the
// page's schema, and a call's result is the schema of its command.
func TestAddonSessionFramesMatchTheSchema(t *testing.T) {
	schema, _, _ := docBlocks(t)
	a := startAddon(t, AddonConfig{}, Permissions...)
	track := Track{ID: "1", Title: "Song", Artist: "Artist", Album: "Album", Duration: 200, Artwork: "https://is1-ssl.mzstatic.com/a/600x600bb.jpg"}
	results := map[string]any{
		"status":  Status{Running: true, State: StatePlaying, Track: &track, Volume: 50, Repeat: "off"},
		"queue":   Queue{Index: 0, Tracks: []QueueTrack{{Track: track}}},
		"search":  SearchResults{Term: "x", Results: []SearchResult{{Kind: "album", ID: "1", Name: "n", URL: "https://music.apple.com/gb/album/n/1"}}},
		"lyrics":  Lyrics{Track: track, State: StatePlaying, Timing: "line", Synced: true, Lines: []LyricLine{{Start: 1, End: 2, Text: "la"}}},
		"like":    Like{Track: track, Liked: true},
		"volume":  Volume{Volume: 30},
		"private": Private{On: true, Source: SourceSetting, Saved: true},
		"open":    Opened{Kind: "album", ID: "1", Storefront: "gb"},
	}
	a.host.call = func(_ context.Context, req Request) (any, error) {
		if v, ok := results[req.Command]; ok {
			return v, nil
		}
		return results["status"], nil
	}
	defs := map[string]string{
		"status": "Status", "play": "Status", "queue": "Queue", "search": "SearchResults", "lyrics": "Lyrics",
		"like": "Like", "volume": "Volume", "private": "Private", "open": "Opened",
	}
	var raws [][]byte
	next := func() []byte { raw := a.recvRaw(); raws = append(raws, raw); return raw }

	a.send(`{"id":1,"method":"hello","params":{"id":"demo","protocol":{"min":"2.0.0","max":"2.0.0"},"permissions":["` + strings.Join(Permissions, `","`) + `"]}}`)
	next()
	a.send(`{"id":2,"method":"subscribe","params":{"topics":["status","now-playing","queue"]}}`)
	a.host.hub.Publish(TopicStatus, results["status"])
	a.host.hub.Publish(TopicNowPlaying, NowPlaying{Track: &track, StartedAt: 9})
	a.host.hub.Publish(TopicQueue, results["queue"])
	id := int64(2)
	for _, cmd := range []string{"status", "play", "queue", "search", "lyrics", "like", "volume", "private", "open"} {
		id++
		args := map[string]string{"search": `["abbey"]`, "open": `["https://music.apple.com/gb/album/n/1"]`}[cmd]
		if args == "" {
			args = "[]"
		}
		a.send(`{"id":` + itoa(id) + `,"method":"call","params":{"command":"` + cmd + `","args":` + args + `}}`)
		var res struct {
			ID     int64
			Result json.RawMessage
		}
		for {
			raw := next()
			res.ID, res.Result = 0, nil
			if err := json.Unmarshal(raw, &res); err != nil {
				t.Fatal(err)
			}
			if res.ID == id {
				break
			}
		}
		if errs := validateDef(t, schema, defs[cmd], res.Result); len(errs) != 0 {
			t.Errorf("%s result %s: %v", cmd, res.Result, errs)
		}
	}
	a.send(`{"id":20,"method":"unsubscribe","params":{"topics":["queue"]}}`)
	a.send(`{"id":21,"method":"call","params":{"command":"private","args":["off"]}}`)
	a.send(`{"id":22,"method":"nope"}`)
	a.send(`not json`)                 // bad_frame, answering no request: id 0
	a.send(`{"command":"status"}`)     // a v1 frame after hello: bad_frame
	a.send(`{"id":0,"method":"call"}`) // a request without an id: bad_frame

	// An invoke, and its answer.
	invoked := make(chan InvokeResult, 1)
	go func() {
		r, err := a.s.Invoke(context.Background(), "addon.demo.send", json.RawMessage(`{"kind":"album","id":"1","name":"Abbey Road"}`))
		if err != nil {
			t.Errorf("invoke: %v", err)
		}
		invoked <- r
	}()
	var inv Frame
	for inv.Method != AddonMethodInvoke {
		if err := json.Unmarshal(next(), &inv); err != nil {
			t.Fatal(err)
		}
	}
	a.send(`{"id":` + itoa(inv.ID) + `,"result":{"toast":"Sent","url":"https://example.com/x"}}`)
	if r := <-invoked; r.Toast != "Sent" {
		t.Errorf("invoke result = %+v", r)
	}
	a.s.Close()
	for {
		_ = a.conn.SetReadDeadline(time.Now().Add(wait))
		line, err := a.r.ReadBytes('\n')
		if len(line) > 0 {
			raws = append(raws, line)
		}
		if err != nil {
			break
		}
	}
	kinds := map[string]int{}
	for _, raw := range raws {
		if errs := validateDef(t, schema, "Frame", raw); len(errs) != 0 {
			t.Errorf("frame %s: %v", strings.TrimSpace(string(raw)), errs)
		}
		var f Frame
		_ = json.Unmarshal(raw, &f)
		switch {
		case f.Event != "":
			kinds["event "+f.Event]++
		case f.Error != nil:
			kinds["error"]++
		case f.Method != "":
			kinds[f.Method]++
		default:
			kinds["result"]++
		}
	}
	for _, want := range []string{"event status", "event now-playing", "event queue", "event shutdown", "error", "invoke", "result"} {
		if kinds[want] == 0 {
			t.Errorf("no %s frame was checked (%v)", want, kinds)
		}
	}
}

// The frames that end a session before or without a request, and the
// bad_frame answers, carry an id (0 when there is none) too.
func TestAddonErrorFramesWithoutARequestMatchTheSchema(t *testing.T) {
	schema, _, _ := docBlocks(t)
	for name, send := range map[string]func(a *testAddon){
		"no hello":        func(a *testAddon) { a.send(`{"method":"call","params":{"command":"status"}}`) },
		"v1 before hello": func(a *testAddon) { a.send(`{"command":"status"}`) },
		"too large":       func(a *testAddon) { a.send(strings.Repeat("x", 2048)) },
		"bad after hello": func(a *testAddon) { a.hello(PermPlaybackRead); a.send(`nope`) },
	} {
		t.Run(name, func(t *testing.T) {
			a := startAddon(t, AddonConfig{Limits: AddonLimits{MaxFrame: 1024}}, PermPlaybackRead)
			send(a)
			_ = a.conn.SetReadDeadline(time.Now().Add(wait))
			raw, err := a.r.ReadBytes('\n')
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"id":`) {
				t.Errorf("error frame without an id: %s", raw)
			}
			if errs := validateDef(t, schema, "Frame", raw); len(errs) != 0 {
				t.Errorf("frame %s: %v", raw, errs)
			}
		})
	}
}
