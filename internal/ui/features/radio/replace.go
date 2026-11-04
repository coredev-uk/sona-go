package radio

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
// the user queued by hand than this, and replacing the queue asks first (the
// "confirm" setting) or offers Undo (the default).
const longUserQueue = 3

// queueCommands is a stand-in for kit.QueueCommands (the player-bar-queue PR,
// which owns Env.Queue): only Replace, v0.2.0's replaceQueue. Drop it for
// env.Queue.Replace at rebase. The one difference is fail, which words a
// failed start: a station's own (v0.2.0's commands.ts `failed`), where
// kit.QueueCommands words every failure by its code.
type queueCommands struct {
	play *store.Playback
	post store.Poster
	svc  interface {
		UndoReplace(ctx context.Context) (bool, error)
	}
	// mode is the "queue.replaceConfirm" setting; it is read off the main
	// thread. Nil: undo.
	mode func() string
	// fail shows a failed command (what it was, then why); nil: the title and
	// Go's words for the error's code.
	fail func(title string, err error)
}

func (q *queueCommands) failed(title string, err error) {
	if q.fail != nil {
		q.fail(title, err)
		return
	}
	kit.ToastErrorf(title, i18n.ErrorText(services.CodeOf(err)))
}

// addedUpcoming counts the songs after the current one that the user queued
// by hand.
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
// returns its failure (nil: playback started; the failure is shown, titled
// failTitle). When that would discard a long queue the user built it asks
// first, or by default replaces it and offers Undo, which brings back the old
// queue at the same song and position. done (may be nil) gets whether
// playback started, on the main thread. Main thread only.
func (q *queueCommands) Replace(failTitle string, start func(ctx context.Context) error, done func(started bool)) {
	finish := func(ok bool) {
		if done != nil {
			done(ok)
		}
	}
	added := addedUpcoming(q.play.Queue())
	run := func(mode string) {
		go func() {
			err := start(context.Background())
			q.post(func() {
				if err != nil {
					q.failed(failTitle, err)
					finish(false)
					return
				}
				if added > longUserQueue && mode != config.ReplaceConfirm {
					q.offerUndo(added)
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
		if q.mode != nil {
			mode = q.mode()
		}
		q.post(func() {
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

// offerUndo says the queue was replaced and offers to bring it back.
func (q *queueCommands) offerUndo(added int) {
	kit.ShowToast(kit.ToastSpec{
		ID:    "queue-replaced",
		Title: i18n.T("Queue replaced"),
		Description: i18n.T("{added, plural, one {# song you added was removed.} other {# songs you added were removed.}}",
			"added", added),
		Duration: 10 * time.Second,
		Action:   i18n.T("Undo"),
		OnAction: func() {
			go func() {
				if _, err := q.svc.UndoReplace(context.Background()); err != nil {
					kit.ToastErrorf(i18n.T("Couldn't bring back your queue"), i18n.ErrorText(services.CodeOf(err)))
				}
			}()
		},
	})
}
