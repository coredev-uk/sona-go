package album

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/kit"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// This is a stand-in for kit.QueueCommands (the player-bar-queue PR, reached
// through Env.Queue): the same names and behaviour (v0.2.0's
// player/commands.ts playSongs and shuffleSongs over replace.ts's
// replaceQueue). Drop it for env.Queue when that merges.

// longUserQueue: more upcoming songs the user queued by hand than this, and
// replacing the queue asks first (setting "confirm") or offers Undo.
const longUserQueue = 3

// queueService is what the commands ask of PlaybackService.
type queueService interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
	UndoReplace(ctx context.Context) (bool, error)
	SetShuffle(on bool)
}

// queueCommands starts queues from lists of songs.
type queueCommands struct {
	svc  queueService
	play *store.Playback
	post store.Poster
	// mode is the Replace queue setting ("undo" or "confirm"), read off the
	// main thread; nil is "undo".
	mode func() string
	// pick is the random start of a shuffle (tests replace it).
	pick func(n int) int
}

func newQueueCommands(svc queueService, play *store.Playback, post store.Poster, mode func() string) *queueCommands {
	return &queueCommands{svc: svc, play: play, post: post, mode: mode, pick: rand.IntN}
}

// playableRefs keeps the playable songs as track refs and maps start (an
// index in songs) to its place among them.
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

func failed(what string, err error) { kit.ToastErrorf(what, i18n.ErrorText(services.CodeOf(err))) }

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
// returns its failure (shown titled failTitle). When that would discard a
// long queue the user built it asks first, or by default replaces it and
// offers Undo. done (may be nil) gets whether playback started, on the main
// thread.
func (q *queueCommands) Replace(failTitle string, start func(ctx context.Context) error, done func(started bool)) {
	finish := func(ok bool) {
		if done != nil {
			done(ok)
		}
	}
	added := 0
	if q.play != nil {
		added = addedUpcoming(q.play.Queue())
	}
	run := func(mode string) {
		go func() {
			err := start(context.Background())
			q.post(func() {
				if err != nil {
					failed(failTitle, err)
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
					failed(i18n.T("Couldn't bring back your queue"), err)
				}
			}()
		},
	})
}

// PlaySongs plays songs from start as the queue, from the collection from.
// Unplayable songs are dropped; none playable is an error toast.
func (q *queueCommands) PlaySongs(songs []services.Song, start int, from services.PlayFrom) {
	refs, index := playableRefs(songs, start)
	if len(refs) == 0 {
		kit.ToastErrorf(i18n.T("Nothing here can be played"), i18n.T("None of these songs is available to stream."))
		return
	}
	q.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return q.svc.PlayItems(ctx, refs, index, from)
	}, nil)
}

// ShuffleSongs turns shuffle on, then starts from a random playable song.
func (q *queueCommands) ShuffleSongs(songs []services.Song, from services.PlayFrom) {
	refs, _ := playableRefs(songs, 0)
	if len(refs) == 0 {
		return
	}
	q.svc.SetShuffle(true)
	if q.play != nil {
		q.play.Patch(func(s *services.PlaybackState) { s.Shuffle = true })
	}
	start := q.pick(len(refs))
	q.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return q.svc.PlayItems(ctx, refs, start, from)
	}, nil)
}
