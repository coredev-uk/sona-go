//go:build darwin

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/egoist/mygo"

	"github.com/coredev-uk/sona-go/internal/desktop/notify"
	"github.com/coredev-uk/sona-go/internal/services"
)

// note is one notification as MyGo shows it: a title, a text, silence or the
// system sound, and a click. It has no artwork and no buttons.
type note struct {
	ID, Title, Body string
	Silent          bool
	OnClick         func()
}

// notificationCenter is MyGo's notifications (UNUserNotificationCenter), and
// a fake in the tests.
type notificationCenter interface {
	// Show shows n, replacing the notification of the same ID, and returns
	// what removes it. It waits for the user's answer to the system's first
	// "allow notifications?" question, and fails with
	// mygo.ErrNotificationsDenied when it is no.
	Show(ctx context.Context, n note) (remove func(), err error)
	// OnWillQuit calls fn as the app quits, while MyGo can still remove
	// notifications.
	OnWillQuit(fn func()) (off func())
}

type mygoCenter struct{}

func (mygoCenter) Show(ctx context.Context, n note) (func(), error) {
	m := mygo.NewNotification(mygo.NotificationOptions{ID: n.ID, Title: n.Title, Body: n.Body, Silent: n.Silent})
	m.OnClick(n.OnClick)
	if err := m.ShowContext(ctx); err != nil {
		return nil, err
	}
	return m.Close, nil
}

func (mygoCenter) OnWillQuit(fn func()) func() {
	return mygo.App.OnWillQuit(func(*mygo.QuitEvent) { fn() })
}

// macNotifier shows Sona's notifications through MyGo's Notification, which
// needs the app bundle (Sona.app). Of a notify.Notification only the Summary,
// the Body, the Urgency and the click (reported as notify.DefaultAction)
// arrive: MyGo's notifications carry no artwork and no buttons, which
// Platform.PlainNotifications tells Settings.
//
// One Notifier is one notification the system replaces in place: it shows
// under one identifier, so each Show replaces the one before, and every
// Notifier has its own (each release reminder keeps its own).
type macNotifier struct {
	center   notificationCenter
	id       string
	onAction func(key string)

	mu      sync.Mutex
	remove  func() // removes the last Show's; nil when none is shown
	closed  bool
	offQuit func() // the quit hook of a track notification; nil without one
}

// newNotifier is services.Platform.Notifications on macOS. The first
// notification asks the user whether to allow them; until they answer, Show
// waits (or ends with its ctx).
func newNotifier(c notificationCenter, onAction func(key string)) (*macNotifier, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, fmt.Errorf("notify: identifier: %w", err)
	}
	return &macNotifier{center: c, id: "dev.coredev.Sona.notification." + hex.EncodeToString(b[:]), onAction: onAction}, nil
}

// openNotifier is newNotifier over MyGo's notifications, where there are
// any.
func openNotifier(_ context.Context, onAction func(key string)) (services.Notifier, error) {
	if !mygo.NotificationsSupported() {
		return nil, fmt.Errorf("notifications need Sona.app: %w", errors.ErrUnsupported)
	}
	return newNotifier(mygoCenter{}, onAction)
}

// Show replaces the notification on screen with msg. A track notification
// (low urgency) is silent and goes when Sona quits, as the Linux one is
// dismissed; a reminder the user asked for (normal urgency) plays the
// system's sound and stays. It fails with services.ErrNotificationsDenied
// when the system lets Sona show none.
func (n *macNotifier) Show(ctx context.Context, msg notify.Notification) error {
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return os.ErrClosed
	}
	if msg.Urgency == notify.UrgencyLow && n.offQuit == nil {
		// MyGo can't remove a notification once its loop has stopped, which
		// is when the services close their notifier: do it as the app quits.
		n.offQuit = n.center.OnWillQuit(func() { _ = n.Dismiss(context.Background()) })
	}
	n.mu.Unlock()

	remove, err := n.center.Show(ctx, note{
		ID: n.id, Title: msg.Summary, Body: msg.Body,
		Silent:  msg.Urgency == notify.UrgencyLow,
		OnClick: n.clicked,
	})
	if err != nil {
		if errors.Is(err, mygo.ErrNotificationsDenied) {
			return fmt.Errorf("%w: %w", services.ErrNotificationsDenied, err)
		}
		return err
	}
	n.mu.Lock()
	n.remove = remove
	n.mu.Unlock()
	return nil
}

// clicked is the click on a notification (on the main thread).
func (n *macNotifier) clicked() {
	n.mu.Lock()
	closed := n.closed
	n.mu.Unlock()
	if !closed {
		n.onAction(notify.DefaultAction)
	}
}

// Dismiss removes the notification from the screen and from Notification
// Center.
func (n *macNotifier) Dismiss(context.Context) error {
	n.mu.Lock()
	remove := n.remove
	n.remove = nil
	n.mu.Unlock()
	if remove != nil {
		remove()
	}
	return nil
}

// Close stops reporting clicks.
func (n *macNotifier) Close() {
	n.mu.Lock()
	n.closed = true
	off := n.offQuit
	n.offQuit = nil
	n.mu.Unlock()
	if off != nil {
		off()
	}
}
