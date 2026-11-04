package library

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

// longUserQueue is v0.2.0's replace.ts LONG_USER_QUEUE: more upcoming songs
// the user queued by hand than this, and replacing the queue asks first
// (setting "confirm") or offers Undo (the default).
const longUserQueue = 3

// queueService is what the queue commands ask of PlaybackService.
type queueService interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
	UndoReplace(ctx context.Context) (bool, error)
	SetShuffle(on bool)
}

// queueCommands is a stand-in for kit.QueueCommands (player-bar-queue PR);
// drop it for env.Queue at rebase. Methods and semantics are the same:
// features/player/commands.ts and replace.ts.
type queueCommands struct {
	svc  queueService
	play *store.Playback
	post store.Poster
	// replaceMode is the queue.replaceConfirm setting, read off the main
	// thread; nil: undo.
	replaceMode func() string
}

func queueFail(what string, err error) {
	kit.ToastErrorf(what, i18n.ErrorText(services.CodeOf(err)))
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

func videoRef(v services.MusicVideo) services.TrackRef {
	return services.TrackRef{ID: v.ID, Source: v.Source, Kind: services.TrackKindVideo}
}

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
// returns its failure (nil: playback started; the failure is shown titled
// failTitle). When that would discard a long queue the user built it asks
// first, or by default replaces it and offers Undo. done (may be nil) gets
// whether playback started, on the main thread. Main thread only.
func (c *queueCommands) Replace(failTitle string, start func(ctx context.Context) error, done func(started bool)) {
	finish := func(ok bool) {
		if done != nil {
			done(ok)
		}
	}
	added := 0
	if c.play != nil {
		added = addedUpcoming(c.play.Queue())
	}
	run := func(mode string) {
		go func() {
			err := start(context.Background())
			c.post(func() {
				if err != nil {
					queueFail(failTitle, err)
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
		if c.replaceMode != nil {
			mode = c.replaceMode()
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
					queueFail(i18n.T("Couldn't bring back your queue"), err)
				}
			}()
		},
	})
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

// ShuffleSongs turns shuffle on, then starts from a random playable song.
func (c *queueCommands) ShuffleSongs(songs []services.Song, from services.PlayFrom) {
	refs, _ := playableRefs(songs, 0)
	if len(refs) == 0 {
		return
	}
	start := rand.IntN(len(refs))
	c.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		c.svc.SetShuffle(true)
		return c.svc.PlayItems(ctx, refs, start, from)
	}, nil)
}

// PlayVideos plays videos from start as the queue; opened runs once
// playback has started (it opens the video view).
func (c *queueCommands) PlayVideos(videos []services.MusicVideo, start int, opened func()) {
	if len(videos) == 0 {
		return
	}
	refs := make([]services.TrackRef, len(videos))
	for i, v := range videos {
		refs[i] = videoRef(v)
	}
	c.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return c.svc.PlayItems(ctx, refs, start, services.PlayFrom{})
	}, func(started bool) {
		if started && opened != nil {
			opened()
		}
	})
}
