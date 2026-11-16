package cli

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

// fakeOwn is an add-on's own data: settings values and secrets.
type fakeOwn struct {
	mu      sync.Mutex
	hub     *Hub
	secrets map[string]string
	asked   []string
}

func newFakeOwn() *fakeOwn {
	o := &fakeOwn{hub: NewHub(), secrets: map[string]string{"token": "s3cr3t"}}
	o.hub.Publish(TopicSettings, SettingsValues{Values: map[string]string{"url": "https://example.com", "post": "true"}})
	return o
}

func (o *fakeOwn) WatchSettings() (<-chan any, func()) { return o.hub.Watch(TopicSettings) }

func (o *fakeOwn) Secret(key string) (string, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.asked = append(o.asked, key)
	v, ok := o.secrets[key]
	return v, ok
}

func TestAddonSettingsTopicNeedsNoPermission(t *testing.T) {
	own := newFakeOwn()
	a := startAddon(t, AddonConfig{Own: own}) // nothing granted
	a.hello()
	f := a.request(2, "subscribe", `{"topics":["settings"]}`)
	if f.Error != nil || string(f.Result) != `{"topics":["settings"]}` {
		t.Fatalf("subscribe = %+v", f)
	}
	ev := a.recv()
	if ev.Event != "settings" || !strings.Contains(string(ev.Data), `"url":"https://example.com"`) {
		t.Fatalf("event = %+v %s", ev, ev.Data)
	}
	own.hub.Publish(TopicSettings, SettingsValues{Values: map[string]string{"url": "https://example.org"}})
	if ev := a.recv(); !strings.Contains(string(ev.Data), "example.org") {
		t.Errorf("change = %s", ev.Data)
	}
	// Other topics still need theirs.
	wantCode(t, a.request(3, "subscribe", `{"topics":["status"]}`), AddonErrPermission)
}

func TestAddonSettingsTopicIsAbsentWithoutOwn(t *testing.T) {
	a := startAddon(t, AddonConfig{}, PermPlaybackRead)
	a.hello(PermPlaybackRead)
	wantCode(t, a.request(2, "subscribe", `{"topics":["settings"]}`), AddonErrUnknownTopic)
	wantCode(t, a.request(3, "secret", `{"key":"token"}`), AddonErrUnknownMethod)
}

func TestAddonSecret(t *testing.T) {
	own := newFakeOwn()
	a := startAddon(t, AddonConfig{Own: own})
	a.hello()
	f := a.request(2, "secret", `{"key":"token"}`)
	var res SecretResult
	if err := json.Unmarshal(f.Result, &res); err != nil || f.Error != nil || res.Value != "s3cr3t" {
		t.Fatalf("secret = %+v %s", f, f.Result)
	}
	wantCode(t, a.request(3, "secret", `{"key":"missing"}`), "not_found")
	for i, params := range []string{`{"key":""}`, `{"key":"../x"}`, `{"key":"Token"}`, `{"key":"` + strings.Repeat("a", 41) + `"}`, `[]`, `{"key":5}`} {
		wantCode(t, a.request(int64(10+i), "secret", params), AddonErrBadArguments)
	}
	own.mu.Lock()
	defer own.mu.Unlock()
	if len(own.asked) != 2 {
		t.Errorf("the host was asked for %v: a malformed key must never reach it", own.asked)
	}
}

func TestAddonSecretIsRateLimited(t *testing.T) {
	a := startAddon(t, AddonConfig{Own: newFakeOwn(), Rates: map[string]Rate{"": {PerSecond: 0.001, Burst: 2}}})
	a.hello()
	for i := range 2 {
		if f := a.request(int64(2+i), "secret", `{"key":"token"}`); f.Error != nil {
			t.Fatalf("call %d: %+v", i, f.Error)
		}
	}
	wantCode(t, a.request(9, "secret", `{"key":"token"}`), AddonErrRateLimited)
}

// The settings event and the secret answer are frames of the page's schema.
func TestAddonSettingsFramesMatchTheSchema(t *testing.T) {
	schema, _, _ := docBlocks(t)
	a := startAddon(t, AddonConfig{Own: newFakeOwn()})
	a.hello()
	a.send(`{"id":2,"method":"subscribe","params":{"topics":["settings"]}}`)
	a.send(`{"id":3,"method":"secret","params":{"key":"token"}}`)
	for range 3 {
		raw := a.recvRaw()
		if errs := validateDef(t, schema, "Frame", raw); len(errs) != 0 {
			t.Errorf("frame %s: %v", strings.TrimSpace(string(raw)), errs)
		}
	}
}
