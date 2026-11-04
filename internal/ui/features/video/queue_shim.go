package video

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

// queueAPI is what the shim asks of PlaybackService.
type queueAPI interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
	UndoReplace(ctx context.Context) (bool, error)
}

// queueCommands is the subset of kit.QueueCommands the video feature plays
// through. Drop this file for env.Queue (kit.QueueCommands) when the
// player-bar-queue PR merges: PlayVideos has the same name and meaning.
type queueCommands struct {
	svc  queueAPI
	play *store.Playback
	post store.Poster
	// mode is the "queue.replaceConfirm" setting, read off the main thread.
	mode func() string
}

// Replace is kit.QueueCommands.Replace: it starts a new queue with start,
// which runs off the main thread and returns its failure (shown here, titled
// failTitle). When that would discard a long queue the user built, it asks
// first, or by default replaces it and offers Undo. done gets whether
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
					kit.ToastErrorf(failTitle, i18n.ErrorText(services.CodeOf(err)))
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

// PlayVideos plays music videos from start as the queue (videos are never
// reported as plays: no PlayFrom collection; a library copy plays its
// catalogue video); opened runs, on the main thread, once playback started:
// the video view opens. v0.2.0's playVideos.
func (q *queueCommands) PlayVideos(videos []services.MusicVideo, start int, opened func()) {
	if len(videos) == 0 {
		return
	}
	refs := make([]services.TrackRef, len(videos))
	for i, v := range videos {
		refs[i] = services.TrackRef{ID: v.ID, Source: v.Source, Kind: services.TrackKindVideo}
	}
	q.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return q.svc.PlayItems(ctx, refs, start, services.PlayFrom{})
	}, func(ok bool) {
		if ok && opened != nil {
			opened()
		}
	})
}

// PlaySongs plays songs from start as the queue, from the collection from (a
// kind "" with a label for anything else). Unplayable songs are dropped;
// none playable is an error toast. v0.2.0's playSongs.
func (q *queueCommands) PlaySongs(songs []services.Song, start int, from services.PlayFrom) {
	var refs []services.TrackRef
	index := 0
	for i, s := range songs {
		if !s.Playable {
			continue
		}
		if i == start {
			index = len(refs)
		}
		refs = append(refs, services.TrackRef{ID: s.ID, Source: s.Source})
	}
	if len(refs) == 0 {
		kit.ToastErrorf(i18n.T("Nothing here can be played"), i18n.T("None of these songs is available to stream."))
		return
	}
	q.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return q.svc.PlayItems(ctx, refs, index, from)
	}, nil)
}
