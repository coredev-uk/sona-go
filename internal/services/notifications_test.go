package services

import (
	"testing"
	"time"
)

// The centre's list: newest first, a notice with a key already there
// replaces it and is unread again, opening marks all read, an entry can be
// dismissed or all cleared, sign-out takes only the account's, and an entry
// past 30 days goes with the next post. Every change is announced.
func TestNotificationsKeepReplaceReadDismissAndClear(t *testing.T) {
	ctx := t.Context()
	h := newRemindersHarness(t, false)
	s := &NotificationsService{e: h.e}
	post := func(n notice) {
		t.Helper()
		if err := h.e.postNotice(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	day := localDay(t, "2026-11-01")
	h.clock.set(day)
	post(notice{kind: NoticeAddonOff, key: "addon-off:webhook", subject: "Webhook", target: OpenTarget{Kind: OpenSettings, ID: "addons"}})
	h.clock.set(day.Add(time.Hour))
	post(notice{kind: NoticeReleaseOut, key: "release-out:7", subject: "Album 7", artist: "Band", account: true})
	list, err := s.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || list.Unread != 2 || list.Items[0].Subject != "Album 7" || list.Items[1].Target.ID != "addons" {
		t.Fatalf("list = %+v", list)
	}
	if h.emit.events[len(h.emit.events)-1] != EventNotificationsChanged {
		t.Fatalf("events = %v", h.emit.events)
	}

	if err := s.MarkRead(ctx); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.List(ctx); list.Unread != 0 || !list.Items[0].Read {
		t.Fatalf("after MarkRead = %+v", list)
	}
	// The same add-on switched off again: one entry, the newest, unread.
	h.clock.set(day.Add(2 * time.Hour))
	post(notice{kind: NoticeAddonOff, key: "addon-off:webhook", subject: "Webhook 2"})
	if list, _ = s.List(ctx); len(list.Items) != 2 || list.Unread != 1 || list.Items[0].Subject != "Webhook 2" || list.Items[0].Read {
		t.Fatalf("after a repeat = %+v", list)
	}

	// Sign-out takes the release, which is about the account, and keeps the
	// add-on's.
	if err := h.e.Auth.SignOut(ctx); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.List(ctx); len(list.Items) != 1 || list.Items[0].Kind != NoticeAddonOff {
		t.Fatalf("after sign-out = %+v", list)
	}
	if err := s.Dismiss(ctx, list.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.List(ctx); len(list.Items) != 0 || list.Items == nil {
		t.Fatalf("after Dismiss = %#v", list)
	}

	// Past 30 days an entry goes with the next post.
	post(notice{kind: NoticeAddonOff, key: "addon-off:old", subject: "Old"})
	h.clock.set(day.Add(2*time.Hour + noticesKeep + time.Minute))
	post(notice{kind: NoticeAddonOff, key: "addon-off:new", subject: "New"})
	if list, _ = s.List(ctx); len(list.Items) != 1 || list.Items[0].Subject != "New" {
		t.Fatalf("after 30 days = %+v", list)
	}
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.List(ctx); len(list.Items) != 0 {
		t.Fatalf("after Clear = %+v", list)
	}
}
