package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/coredev-uk/sona-go/internal/app"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
	"github.com/coredev-uk/sona-go/internal/desktop/notify"
)

// Release reminder timing. The wait for the next release is capped at
// remindersCheck so a suspend or a clock change delays a reminder by at
// most that; a notification that can't be shown is tried again after
// remindersRetry.
const (
	remindersCheck   = time.Minute
	remindersRetry   = 5 * time.Minute
	remindersIdle    = 24 * time.Hour
	remindersTimeout = 15 * time.Second
	// maxReminderMessage bounds the notification body the UI sends.
	maxReminderMessage = 200
)

// RemindersService keeps release reminders (table release_reminders): a
// pre-release album the user asked about gets one desktop notification
// when it comes out, at its release (Album.ReleaseAt) or, when Sona wasn't
// running then, after the first paint of the next start. Clicking it
// opens the album ("app:open"). Before notifying it asks the catalog
// whether Apple moved the release later and, if so, waits for the new
// date. Reminders need the platform's notifications
// (Platform.Notifications, Capabilities.Notifications): without them Set
// fails with code "unsupported". They are user data, cleared on sign-out.
type RemindersService struct {
	e    *Env
	wake chan struct{}
	quit chan struct{}
	done chan struct{}

	mu      sync.Mutex
	started bool
	shown   []Notifier // fired reminders, kept to deliver their clicks
}

func newRemindersService(e *Env) *RemindersService {
	s := &RemindersService{e: e, wake: make(chan struct{}, 1), quit: make(chan struct{}), done: make(chan struct{})}
	if e.Platform.Notifications != nil {
		e.OnNetwork(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			select {
			case <-s.quit:
				return
			default:
			}
			s.started = true
			e.Go("release reminders", s.run)
		})
	}
	e.OnSignOut("reminders", func(ctx context.Context) error {
		if err := e.DB.Q.ClearReminders(ctx); err != nil {
			return err
		}
		s.changed(ctx)
		return nil
	})
	e.OnClose(s.close)
	return s
}

// List returns the reminders set, soonest release first.
func (s *RemindersService) List(ctx context.Context) ([]Reminder, error) {
	rows, err := s.e.DB.RQ.ListReminders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, len(rows))
	for i, r := range rows {
		out[i] = s.reminder(r)
	}
	return out, nil
}

// Set reminds the user when catalog album id comes out. message is the
// notification's body, in the interface's language (Go has no English of
// its own here). The album must be a pre-release with a full release date
// (invalid_argument otherwise); where the platform has no notifications
// it fails with unsupported.
func (s *RemindersService) Set(ctx context.Context, id, message string) (Reminder, error) {
	if s.e.Platform.Notifications == nil {
		return Reminder{}, fmt.Errorf("release reminders need notifications: %w", errors.ErrUnsupported)
	}
	if id == "" || !utf8.ValidString(message) || utf8.RuneCountInString(message) > maxReminderMessage {
		return Reminder{}, fmt.Errorf("%w: reminder for %q", ErrInvalidArgument, id)
	}
	a, err := s.e.Catalog.Album(ctx, id)
	if err != nil {
		return Reminder{}, s.e.Fail(err)
	}
	al := s.e.m.amAlbum(a)
	if !al.Prerelease || al.ReleaseAt == 0 {
		return Reminder{}, fmt.Errorf("%w: album %s is not a pre-release with a release date", ErrInvalidArgument, id)
	}
	row := dbq.PutReminderParams{
		AlbumID: al.CatalogID, Name: al.Name, ArtistName: al.Artist, ArtworkTemplate: a.Attributes.Artwork.Template(),
		ReleaseAt: al.ReleaseAt, Message: message, CreatedAt: s.e.Now().UnixMilli(),
	}
	if err := s.e.DB.Q.PutReminder(ctx, row); err != nil {
		return Reminder{}, err
	}
	s.changed(ctx)
	s.poke()
	return s.reminder(dbq.ReleaseReminder{
		AlbumID: row.AlbumID, Name: row.Name, ArtistName: row.ArtistName, ArtworkTemplate: row.ArtworkTemplate, ReleaseAt: row.ReleaseAt,
	}), nil
}

// Remove drops the reminder for catalog album id (nothing when there is
// none).
func (s *RemindersService) Remove(ctx context.Context, id string) error {
	n, err := s.e.DB.Q.DeleteReminder(ctx, id)
	if err != nil {
		return err
	}
	if n > 0 {
		s.changed(ctx)
		s.poke()
	}
	return nil
}

func (s *RemindersService) reminder(r dbq.ReleaseReminder) Reminder {
	return Reminder{AlbumID: r.AlbumID, Name: r.Name, Artist: r.ArtistName, ReleaseAt: r.ReleaseAt, Art: s.e.m.artOf(r.ArtworkTemplate)}
}

// changed emits every reminder as "reminders:changed".
func (s *RemindersService) changed(ctx context.Context) {
	if s.e.Emit == nil {
		return
	}
	list, err := s.List(ctx)
	if err != nil {
		s.e.Logger.Warn("reminders: list", "err", err)
		return
	}
	s.e.Emit.Emit(EventRemindersChanged, list)
}

// poke makes the loop look at the reminders again.
func (s *RemindersService) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// run notifies due reminders until close.
func (s *RemindersService) run() {
	defer close(s.done)
	for {
		t := time.NewTimer(s.fireDue())
		select {
		case <-s.quit:
			t.Stop()
			return
		case <-s.wake:
		case <-t.C:
		}
		t.Stop()
	}
}

// fireDue notifies every reminder whose release has come and returns how
// long to wait before looking again.
func (s *RemindersService) fireDue() time.Duration {
	ctx, cancel := context.WithTimeout(context.Background(), remindersTimeout)
	defer cancel()
	rows, err := s.e.DB.RQ.ListReminders(ctx)
	if err != nil {
		s.e.Logger.Warn("reminders: list", "err", err)
		return remindersRetry
	}
	if len(rows) == 0 {
		return remindersIdle
	}
	now := s.e.Now().UnixMilli()
	wait := remindersCheck
	changed := false
	for _, r := range rows {
		if r.ReleaseAt > now {
			wait = min(wait, time.Duration(r.ReleaseAt-now)*time.Millisecond)
			continue
		}
		if at := s.movedTo(ctx, r.AlbumID); at > now {
			s.e.Logger.Info("reminders: release moved", "album", r.AlbumID, "from", r.ReleaseAt, "to", at)
			if err := s.e.DB.Q.SetReminderRelease(ctx, dbq.SetReminderReleaseParams{ReleaseAt: at, AlbumID: r.AlbumID}); err != nil {
				s.e.Logger.Warn("reminders: move", "err", err)
			}
			changed = true
			continue
		}
		if err := s.show(ctx, r); err != nil {
			s.e.Logger.Info("reminders: notification not shown; trying again later", "album", r.AlbumID, "err", err)
			wait = min(wait, remindersRetry)
			continue
		}
		s.e.Logger.Info("reminders: notified", "album", r.AlbumID)
		if _, err := s.e.DB.Q.DeleteReminder(ctx, r.AlbumID); err != nil {
			s.e.Logger.Warn("reminders: delete", "err", err)
		}
		changed = true
	}
	if changed {
		s.changed(ctx)
	}
	return wait
}

// movedTo returns the album's release when Apple now lists it as a
// pre-release coming out later, 0 when it is out or can't be asked.
func (s *RemindersService) movedTo(ctx context.Context, id string) int64 {
	if s.e.Catalog == nil {
		return 0
	}
	a, err := s.e.Catalog.Album(ctx, id)
	if err != nil {
		s.e.Report(err)
		return 0
	}
	if al := s.e.m.amAlbum(a); al.Prerelease {
		return al.ReleaseAt
	}
	return 0
}

// show shows r's notification on its own notifier, which it keeps so a
// click opens the album.
func (s *RemindersService) show(ctx context.Context, r dbq.ReleaseReminder) error {
	var image string
	if r.ArtworkTemplate != "" && s.e.Art != nil {
		if p, err := s.e.Art.LocalPath(ctx, r.ArtworkTemplate, mprisArtSize, mprisArtSize); err == nil {
			image = p
		} else {
			s.e.Logger.Debug("reminders: artwork", "err", err)
		}
	}
	id := r.AlbumID
	n, err := s.e.Platform.Notifications(ctx, func(key string) {
		if key == notify.DefaultAction {
			s.open(id)
		}
	})
	if err != nil {
		return err
	}
	err = n.Show(ctx, notify.Notification{
		Summary: r.Name,
		Body:    r.Message,
		Image:   image,
		// The body click; its label is the album's name, not words of Sona's.
		Actions: []notify.Action{{Key: notify.DefaultAction, Label: r.Name}},
		Urgency: notify.UrgencyNormal,
	})
	if err != nil {
		n.Close()
		return err
	}
	s.mu.Lock()
	s.shown = append(s.shown, n)
	s.mu.Unlock()
	return nil
}

// open brings the window up on the album.
func (s *RemindersService) open(id string) {
	if s.e.Focus != nil {
		s.e.Focus()
	}
	if s.e.Emit != nil {
		s.e.Emit.Emit(EventAppOpen, OpenTarget{Kind: string(app.KindAlbum), ID: id})
	}
}

// close stops the loop. Shown notifications stay on screen.
func (s *RemindersService) close() {
	s.mu.Lock()
	close(s.quit)
	started := s.started
	shown := s.shown
	s.shown = nil
	s.mu.Unlock()
	if started {
		<-s.done
	}
	for _, n := range shown {
		n.Close()
	}
}
