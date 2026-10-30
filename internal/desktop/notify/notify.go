// Package notify shows desktop notifications through the
// org.freedesktop.Notifications D-Bus interface (Desktop Notifications
// Specification 1.2). A Notifier keeps one notification on screen: each
// Show replaces the previous one, and action buttons call back into the app.
package notify

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

const (
	busName = "org.freedesktop.Notifications"
	objPath = dbus.ObjectPath("/org/freedesktop/Notifications")
	iface   = busName

	sigActionInvoked = iface + ".ActionInvoked"
	sigClosed        = iface + ".NotificationClosed"
)

// DefaultAction is the action key the server invokes when the
// notification body itself is clicked.
const DefaultAction = "default"

// Notification is one notification's content.
type Notification struct {
	Summary string
	Body    string // plain text; escaped when the server renders markup
	// Image is a local image file shown as the notification's picture
	// (image-path hint); empty for none.
	Image   string
	Actions []Action // buttons in order; DefaultAction is the body click
}

// Action is one notification action.
type Action struct {
	Key   string // reported back to Options.OnAction
	Label string // button text
}

// Options configure a Notifier.
type Options struct {
	AppName string // app_name: the application's display name
	AppIcon string // app_icon: icon theme name, shown when there is no Image
	// DesktopEntry is the desktop file name without ".desktop" (the
	// desktop-entry hint), which lets servers group and badge by app.
	DesktopEntry string
	// OnAction runs, on the Notifier's signal goroutine, when the user
	// invokes an action of the notification currently shown.
	OnAction func(key string)
}

// Notifier shows one replaceable notification at a time.
type Notifier struct {
	conn *dbus.Conn
	obj  dbus.BusObject
	opts Options
	caps []string

	signals chan *dbus.Signal
	done    chan struct{}

	mu sync.Mutex
	id uint32 // the notification on screen; 0 when none
}

// New connects a Notifier to the notification server on conn: it reads the
// server's capabilities and subscribes to its action and close signals.
// conn stays owned by the caller (other users of conn.Signal also see these
// signals); Close removes the subscription.
func New(ctx context.Context, conn *dbus.Conn, opts Options) (*Notifier, error) {
	n := &Notifier{
		conn:    conn,
		obj:     conn.Object(busName, objPath),
		opts:    opts,
		signals: make(chan *dbus.Signal, 16),
		done:    make(chan struct{}),
	}
	if err := n.obj.CallWithContext(ctx, iface+".GetCapabilities", 0).Store(&n.caps); err != nil {
		return nil, fmt.Errorf("notify: capabilities: %w", err)
	}
	for _, member := range []string{"ActionInvoked", "NotificationClosed"} {
		if err := conn.AddMatchSignalContext(ctx, matchOpts(member)...); err != nil {
			return nil, fmt.Errorf("notify: subscribe %s: %w", member, err)
		}
	}
	conn.Signal(n.signals)
	go n.dispatch()
	return n, nil
}

func matchOpts(member string) []dbus.MatchOption {
	return []dbus.MatchOption{
		dbus.WithMatchObjectPath(objPath),
		dbus.WithMatchInterface(iface),
		dbus.WithMatchMember(member),
	}
}

// Has reports whether the server advertises capability (e.g. "actions").
func (n *Notifier) Has(capability string) bool { return slices.Contains(n.caps, capability) }

// Show displays msg, replacing the notification shown before (if the
// server still has it).
func (n *Notifier) Show(ctx context.Context, msg Notification) error {
	body := msg.Body
	if n.Has("body-markup") {
		body = escapeMarkup(body)
	}
	var actions []string
	if n.Has("actions") {
		for _, a := range msg.Actions {
			actions = append(actions, a.Key, a.Label)
		}
	}
	// No "transient" hint: some servers (Quickshell configs) drop transient
	// notifications without showing them. Replacing by ID already keeps a
	// single now-playing notification.
	hints := map[string]dbus.Variant{
		"urgency": dbus.MakeVariant(byte(0)), // low
	}
	if n.opts.DesktopEntry != "" {
		hints["desktop-entry"] = dbus.MakeVariant(n.opts.DesktopEntry)
	}
	if msg.Image != "" {
		hints["image-path"] = dbus.MakeVariant(fileURI(msg.Image))
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	var id uint32
	err := n.obj.CallWithContext(ctx, iface+".Notify", 0,
		n.opts.AppName, n.id, n.opts.AppIcon, msg.Summary, body,
		actions, hints, int32(-1),
	).Store(&id)
	if err != nil {
		return fmt.Errorf("notify: %w", err)
	}
	n.id = id
	return nil
}

// Dismiss closes the notification on screen, if any.
func (n *Notifier) Dismiss(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.id == 0 {
		return nil
	}
	id := n.id
	n.id = 0
	if err := n.obj.CallWithContext(ctx, iface+".CloseNotification", 0, id).Err; err != nil {
		return fmt.Errorf("notify: close: %w", err)
	}
	return nil
}

// Close stops listening for the server's signals; the notification on
// screen stays (call Dismiss first to remove it).
func (n *Notifier) Close() {
	n.conn.RemoveSignal(n.signals)
	for _, member := range []string{"ActionInvoked", "NotificationClosed"} {
		_ = n.conn.RemoveMatchSignal(matchOpts(member)...)
	}
	close(n.done)
}

// dispatch routes the server's signals until Close.
func (n *Notifier) dispatch() {
	var owner string // the server's unique name, resolved on demand
	for {
		select {
		case <-n.done:
			return
		case sig := <-n.signals:
			if sig == nil || sig.Path != objPath || len(sig.Body) < 2 {
				continue
			}
			id, ok := sig.Body[0].(uint32)
			if !ok {
				continue
			}
			n.mu.Lock()
			current := id != 0 && id == n.id
			n.mu.Unlock()
			if !current {
				continue
			}
			// Any client may emit these signals; act only on the server's.
			if sig.Sender != owner {
				owner = n.owner()
				if sig.Sender != owner {
					continue
				}
			}
			switch sig.Name {
			case sigClosed:
				n.mu.Lock()
				if n.id == id {
					n.id = 0
				}
				n.mu.Unlock()
			case sigActionInvoked:
				if key, ok := sig.Body[1].(string); ok && n.opts.OnAction != nil {
					n.opts.OnAction(key)
				}
			}
		}
	}
}

// owner returns the unique bus name currently owning the server's name.
func (n *Notifier) owner() string {
	var name string
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = n.conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.GetNameOwner", 0, busName).Store(&name)
	return name
}

// escapeMarkup escapes the characters the specification's body markup
// (a small XML subset) gives meaning to.
func escapeMarkup(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// fileURI turns a local path into the file:// URI the image-path hint takes.
func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: path}).String()
}
