package song

import (
	"context"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// longUserQueue is v0.2.0's replace.ts LONG_USER_QUEUE: more upcoming songs
// the user queued by hand than this, and replacing the queue asks first
// (setting "confirm") or offers Undo (the default).
const longUserQueue = 3

// queueService is what the commands ask of PlaybackService.
type queueService interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
	UndoReplace(ctx context.Context) (bool, error)
}

// queueCommands is a stand-in for kit.QueueCommands (player-bar-queue PR):
// drop it for env.Queue at rebase. Only the methods the song page uses
// (PlaySongs, Replace) are here, with the same names and semantics.
type queueCommands struct {
	svc  queueService
	play *store.Playback
	post store.Poster
	// mode is the "queue.replaceConfirm" setting; called off the main
	// thread. Nil: undo.
	mode func() string
}

// playableRefs keeps the playable songs as track refs and maps start (an
// index in songs) to its place among them (v0.2.0's playable()).
func playableRefs(songs []services.Song, start int) (refs []services.TrackRef, index int) {
	for i, s := range songs {
		if !s.Playable {
			continue
		}
		if i == start {
			index = len(refs)
		}
		refs = append(refs, services.TrackRef{ID: s.ID, Source: s.Source})
	}
	return refs, index
}

// PlaySongs plays songs from start as the queue, from the collection from.
// Unplayable songs are dropped; none playable is an error toast.
func (c *queueCommands) PlaySongs(songs []services.Song, start int, from services.PlayFrom) {
	refs, index := playableRefs(songs, start)
	if len(refs) == 0 {
		kit.ToastErrorf(i18n.T("Nothing here can be played"), i18n.T("None of these songs is available to stream."))
		return
	}
	c.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return c.svc.PlayItems(ctx, refs, index, from)
	}, nil)
}

// addedUpcoming counts the songs after the current one the user queued by
// hand.
func addedUpcoming(q services.QueueState) int {
	n := 0
	for i := q.Index + 1; i < len(q.Items); i++ {
		if q.Items[i].Added {
			n++
		}
	}
	return n
}

// Replace starts a new queue with start, which runs off the main thread and
// returns its failure (shown here titled failTitle). Discarding a long
// queue the user built asks first, or by default offers Undo. done (may be
// nil) gets whether playback started, on the main thread.
func (c *queueCommands) Replace(failTitle string, start func(ctx context.Context) error, done func(started bool)) {
	finish := func(ok bool) {
		if done != nil {
			done(ok)
		}
	}
	added := addedUpcoming(c.play.Queue())
	run := func(mode string) {
		go func() {
			err := start(context.Background())
			c.post(func() {
				if err != nil {
					kit.ToastErrorf(failTitle, i18n.ErrorMessage(err))
					finish(false)
					return
				}
				if added > longUserQueue && mode != config.ReplaceConfirm {
					c.offerUndo(added)
				}
				finish(true)
			})
		}()
	}
	if added <= longUserQueue {
		run("")
		return
	}
	go func() {
		mode := config.ReplaceUndo
		if c.mode != nil {
			mode = c.mode()
		}
		c.post(func() {
			if mode != config.ReplaceConfirm {
				run(mode)
				return
			}
			kit.Confirm(kit.ConfirmOptions{
				Title:        i18n.T("Replace your queue?"),
				Body:         i18n.T("{added, plural, one {The # song you added to the queue will be removed.} other {The # songs you added to the queue will be removed.}}", "added", added),
				ConfirmLabel: i18n.T("Replace queue"),
			}, func(ok bool) {
				if !ok {
					finish(false)
					return
				}
				run(mode)
			})
		})
	}()
}

func (c *queueCommands) offerUndo(added int) {
	kit.ShowToast(kit.ToastSpec{
		ID:    "queue-replaced",
		Title: i18n.T("Queue replaced"),
		Description: i18n.T("{added, plural, one {# song you added was removed.} other {# songs you added were removed.}}",
			"added", added),
		Duration: 10 * time.Second,
		Action:   i18n.T("Undo"),
		OnAction: func() {
			go func() {
				if _, err := c.svc.UndoReplace(context.Background()); err != nil {
					kit.ToastErrorf(i18n.T("Couldn't bring back your queue"), i18n.ErrorMessage(err))
				}
			}()
		},
	})
}
