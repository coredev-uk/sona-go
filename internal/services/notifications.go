package services

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// noticesKeep is how long the notification centre keeps an entry; the
// query keeps the newest 100 besides.
const noticesKeep = 30 * 24 * time.Hour

// NotificationsService is the notification centre (table notifications):
// what happened without the person asking, kept until they dismiss it or
// for 30 days. Services post with Env.postNotice: a release they asked to be
// reminded of is out (RemindersService), new releases from their artists
// (ArtistTrackerService), an add-on switched off after crashing
// (AddonsService). An entry holds a kind and names, never words: the UI
// words it in its language. A second notice with the same key replaces the
// first and is unread again. What is under way or wrong right now (a sync,
// an update, the CDM, an add-on waiting for approval) is not stored: the UI
// reads it from those services' own state. Entries about the account are
// cleared on sign-out.
type NotificationsService struct {
	e *Env
}

func newNotificationsService(e *Env) *NotificationsService {
	e.OnSignOut("notifications", func(ctx context.Context) error {
		n, err := e.DB.Q.ClearAccountNotifications(ctx)
		if err != nil {
			return err
		}
		if n > 0 {
			e.noticesChanged(ctx)
		}
		return nil
	})
	return &NotificationsService{e: e}
}

// List returns the entries, newest first.
func (s *NotificationsService) List(ctx context.Context) (Notifications, error) {
	return s.e.listNotices(ctx)
}

// MarkRead marks every entry read (the centre was opened).
func (s *NotificationsService) MarkRead(ctx context.Context) error {
	n, err := s.e.DB.Q.MarkNotificationsRead(ctx)
	if err == nil && n > 0 {
		s.e.noticesChanged(ctx)
	}
	return err
}

// Dismiss drops entry id (nothing when there is none).
func (s *NotificationsService) Dismiss(ctx context.Context, id int64) error {
	n, err := s.e.DB.Q.DeleteNotification(ctx, id)
	if err == nil && n > 0 {
		s.e.noticesChanged(ctx)
	}
	return err
}

// Clear drops every entry.
func (s *NotificationsService) Clear(ctx context.Context) error {
	n, err := s.e.DB.Q.ClearNotifications(ctx)
	if err == nil && n > 0 {
		s.e.noticesChanged(ctx)
	}
	return err
}

// notice is an entry a service posts (Notification's fields; art is an
// artwork template).
type notice struct {
	kind, key                string
	subject, artist, variant string
	count                    int
	art                      string
	target                   OpenTarget
	// account: about the account, cleared on sign-out.
	account bool
}

// postNotice adds n to the centre, or replaces the entry with its key, and
// prunes what is past noticesKeep.
func (e *Env) postNotice(ctx context.Context, n notice) error {
	now := e.Now()
	account := int64(0)
	if n.account {
		account = 1
	}
	err := e.DB.Q.PutNotification(ctx, dbq.PutNotificationParams{
		Kind: n.kind, Key: n.key, Subject: n.subject, Artist: n.artist, Variant: n.variant, Count: int64(n.count),
		ArtworkTemplate: n.art, TargetKind: n.target.Kind, TargetID: n.target.ID, Account: account, CreatedAt: now.UnixMilli(),
	})
	if err != nil {
		return err
	}
	if err := e.DB.Q.PruneNotifications(ctx, now.Add(-noticesKeep).UnixMilli()); err != nil {
		e.Logger.Warn("notifications: prune", "err", err)
	}
	e.noticesChanged(ctx)
	return nil
}

// listNotices reads the centre's entries.
func (e *Env) listNotices(ctx context.Context) (Notifications, error) {
	rows, err := e.DB.RQ.ListNotifications(ctx)
	if err != nil {
		return Notifications{}, err
	}
	out := Notifications{Items: make([]Notification, 0, len(rows))}
	for _, r := range rows {
		n := Notification{
			ID: r.ID, Kind: r.Kind, Subject: r.Subject, Artist: r.Artist, Variant: r.Variant, Count: int(r.Count),
			Target: OpenTarget{Kind: r.TargetKind, ID: r.TargetID}, CreatedAt: r.CreatedAt, Read: r.Read != 0,
		}
		if r.ArtworkTemplate != "" {
			n.Art = e.m.artOf(r.ArtworkTemplate)
		}
		if !n.Read {
			out.Unread++
		}
		out.Items = append(out.Items, n)
	}
	return out, nil
}

// noticesChanged emits the centre's entries as "notifications:changed".
func (e *Env) noticesChanged(ctx context.Context) {
	if e.Emit == nil {
		return
	}
	list, err := e.listNotices(ctx)
	if err != nil {
		e.Logger.Warn("notifications: list", "err", err)
		return
	}
	e.Emit.Emit(EventNotificationsChanged, list)
}
