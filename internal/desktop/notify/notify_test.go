package notify

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/coredev-uk/sona-go/internal/app/dbustest"
)

// notifyCall is one Notify call the fake server received.
type notifyCall struct {
	replaces uint32
	icon     string
	summary  string
	body     string
	actions  []string
	hints    map[string]dbus.Variant
}

// fakeServer is a notification server on a private bus.
type fakeServer struct {
	conn  *dbus.Conn
	caps  []string
	calls chan notifyCall

	mu     sync.Mutex
	nextID uint32
	closed []uint32
}

func (f *fakeServer) GetCapabilities() ([]string, *dbus.Error) { return f.caps, nil }

func (f *fakeServer) Notify(_ string, replaces uint32, icon, summary, body string, actions []string, hints map[string]dbus.Variant, _ int32) (uint32, *dbus.Error) {
	f.calls <- notifyCall{replaces, icon, summary, body, actions, hints}
	f.mu.Lock()
	defer f.mu.Unlock()
	if replaces != 0 {
		return replaces, nil
	}
	f.nextID++
	return f.nextID, nil
}

func (f *fakeServer) CloseNotification(id uint32) *dbus.Error {
	f.mu.Lock()
	f.closed = append(f.closed, id)
	f.mu.Unlock()
	return nil
}

func (f *fakeServer) GetServerInformation() (string, string, string, string, *dbus.Error) {
	return "fake", "sona", "1", "1.2", nil
}

func (f *fakeServer) emit(member string, args ...any) {
	if err := f.conn.Emit(objPath, iface+"."+member, args...); err != nil {
		panic(err)
	}
}

func (f *fakeServer) next(t *testing.T) notifyCall {
	t.Helper()
	select {
	case c := <-f.calls:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("no Notify call")
		return notifyCall{}
	}
}

type harness struct {
	addr    string
	server  *fakeServer
	n       *Notifier
	actions chan string
}

func newHarness(t *testing.T, caps ...string) *harness {
	t.Helper()
	addr := dbustest.Start(t)
	srv := &fakeServer{conn: dbustest.Conn(t, addr), caps: caps, calls: make(chan notifyCall, 8)}
	if err := srv.conn.Export(srv, objPath, iface); err != nil {
		t.Fatal(err)
	}
	if reply, err := srv.conn.RequestName(busName, dbus.NameFlagDoNotQueue); err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("request name: %v %v", reply, err)
	}
	h := &harness{addr: addr, server: srv, actions: make(chan string, 4)}
	n, err := New(context.Background(), dbustest.Conn(t, addr), Options{
		AppName: "Sona", AppIcon: "sona", DesktopEntry: "dev.coredev.Sona",
		OnAction: func(key string) { h.actions <- key },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(n.Close)
	h.n = n
	return h
}

func (h *harness) show(t *testing.T, msg Notification) notifyCall {
	t.Helper()
	if err := h.n.Show(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	return h.server.next(t)
}

func (h *harness) action(t *testing.T) (string, bool) {
	t.Helper()
	select {
	case k := <-h.actions:
		return k, true
	case <-time.After(300 * time.Millisecond):
		return "", false
	}
}

var song = Notification{
	Summary: "Fish & Chips",
	Body:    "Artist <b> & Co",
	Image:   "/home/u/.cache/sona/art/ab cd/512.jpg",
	Actions: []Action{{DefaultAction, "Show Sona"}, {"next", "Next"}},
}

func TestShowReplacesPrevious(t *testing.T) {
	h := newHarness(t, "actions", "body", "body-markup")
	first := h.show(t, song)
	if first.replaces != 0 {
		t.Errorf("first notification replaces %d", first.replaces)
	}
	if first.summary != "Fish & Chips" || first.body != "Artist &lt;b&gt; &amp; Co" || first.icon != "sona" {
		t.Errorf("first = %+v", first)
	}
	if want := []string{"default", "Show Sona", "next", "Next"}; !reflect.DeepEqual(first.actions, want) {
		t.Errorf("actions = %v, want %v", first.actions, want)
	}
	if got := first.hints["image-path"].Value(); got != "file:///home/u/.cache/sona/art/ab%20cd/512.jpg" {
		t.Errorf("image-path = %v", got)
	}
	if got := first.hints["desktop-entry"].Value(); got != "dev.coredev.Sona" {
		t.Errorf("desktop-entry = %v", got)
	}
	if got := first.hints["urgency"].Value(); got != byte(0) {
		t.Errorf("urgency = %v, want low", got)
	}
	second := h.show(t, Notification{Summary: "Next song", Urgency: UrgencyNormal})
	if second.replaces != 1 {
		t.Errorf("second replaces %d, want 1", second.replaces)
	}
	if _, ok := second.hints["image-path"]; ok {
		t.Error("image-path sent without an image")
	}
	if got := second.hints["urgency"].Value(); got != byte(1) {
		t.Errorf("urgency = %v, want normal", got)
	}
}

func TestPlainServerGetsPlainBodyAndNoActions(t *testing.T) {
	h := newHarness(t, "body")
	c := h.show(t, song)
	if c.body != song.Body {
		t.Errorf("body = %q, want it unescaped", c.body)
	}
	if len(c.actions) != 0 {
		t.Errorf("actions sent to a server without the capability: %v", c.actions)
	}
}

func TestActionInvoked(t *testing.T) {
	h := newHarness(t, "actions")
	h.show(t, song)
	h.server.emit("ActionInvoked", uint32(1), "next")
	if k, ok := h.action(t); !ok || k != "next" {
		t.Fatalf("action = %q, %v", k, ok)
	}

	// Another client's notification, and a forged signal from a client
	// that is not the server, are ignored.
	h.server.emit("ActionInvoked", uint32(9), "next")
	forger := dbustest.Conn(t, h.addr)
	if err := forger.Emit(objPath, iface+".ActionInvoked", uint32(1), "next"); err != nil {
		t.Fatal(err)
	}
	if k, ok := h.action(t); ok {
		t.Fatalf("unexpected action %q", k)
	}
}

func TestClosedNotificationIsNotReplaced(t *testing.T) {
	h := newHarness(t, "actions")
	h.show(t, song)
	h.server.emit("NotificationClosed", uint32(1), uint32(2))
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.n.mu.Lock()
		id := h.n.id
		h.n.mu.Unlock()
		if id == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("NotificationClosed was not applied")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if c := h.show(t, song); c.replaces != 0 {
		t.Errorf("replaces %d after the notification closed", c.replaces)
	}
	// Its actions no longer reach the app.
	h.server.emit("ActionInvoked", uint32(1), "next")
	h.server.emit("ActionInvoked", uint32(2), "next")
	if k, ok := h.action(t); !ok || k != "next" {
		t.Fatalf("action on the new notification = %q, %v", k, ok)
	}
}

func TestDismiss(t *testing.T) {
	h := newHarness(t, "actions")
	h.show(t, song)
	if err := h.n.Dismiss(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.server.mu.Lock()
	closed := h.server.closed
	h.server.mu.Unlock()
	if !reflect.DeepEqual(closed, []uint32{1}) {
		t.Errorf("closed = %v, want [1]", closed)
	}
	if c := h.show(t, song); c.replaces != 0 {
		t.Errorf("replaces %d after Dismiss", c.replaces)
	}
}
