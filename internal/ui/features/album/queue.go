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

// queueAPI is what queueCommands asks of PlaybackService.
type queueAPI interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
	SetShuffle(on bool)
	UndoReplace(ctx context.Context) (bool, error)
}

// longUserQueue: more upcoming songs the user queued by hand than this, and
// replacing the queue asks first (the "confirm" setting) or offers Undo.
const longUserQueue = 3

// queueCommands is a stand-in for kit.QueueCommands (player-bar-queue PR):
// drop it for env.Queue at rebase. It is v0.2.0's player/commands.ts
// playSongs and shuffleSongs with replace.ts's replaceQueue; methods run on
// the main thread, the service calls on a goroutine of their own.
type queueCommands struct {
	api  queueAPI
	play *store.Playback
	mode func() string // the Replace queue setting; nil is "undo"
	pick func(n int) int
}

func newQueueCommands(api queueAPI, play *store.Playback, mode func() string) *queueCommands {
	return &queueCommands{api: api, play: play, mode: mode, pick: rand.IntN}
}

// playable is the songs that can be queued as track refs, and where start
// lands among them.
func playable(songs []services.Song, start int) (refs []services.TrackRef, index int) {
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
func (q *queueCommands) PlaySongs(songs []services.Song, start int, from services.PlayFrom) {
	refs, index := playable(songs, start)
	if len(refs) == 0 {
		kit.ShowToast(kit.ToastSpec{
			ID:          "nothing-to-play",
			Title:       i18n.T("Nothing here can be played"),
			Description: i18n.T("None of these songs is available to stream."),
			Kind:        kit.ToastError,
		})
		return
	}
	q.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return q.api.PlayItems(ctx, refs, index, from)
	}, nil)
}

// ShuffleSongs turns shuffle on and plays songs from a random playable one.
func (q *queueCommands) ShuffleSongs(songs []services.Song, from services.PlayFrom) {
	refs, _ := playable(songs, 0)
	if len(refs) == 0 {
		return
	}
	q.api.SetShuffle(true)
	start := q.pick(len(refs))
	q.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return q.api.PlayItems(ctx, refs, start, from)
	}, nil)
}

// PlayVideos plays music videos from start as the queue; opened runs once
// playback started (the video view opens).
func (q *queueCommands) PlayVideos(videos []services.MusicVideo, start int, opened func()) {
	if len(videos) == 0 {
		return
	}
	refs := make([]services.TrackRef, len(videos))
	for i, v := range videos {
		refs[i] = services.TrackRef{ID: v.ID, Source: v.Source, Kind: services.TrackKindVideo}
	}
	q.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return q.api.PlayItems(ctx, refs, start, services.PlayFrom{})
	}, func(started bool) {
		if started && opened != nil {
			opened()
		}
	})
}

// addedUpcoming is how many upcoming songs the user queued by hand.
func (q *queueCommands) addedUpcoming() int {
	if q.play == nil {
		return 0
	}
	qs := q.play.Queue()
	n := 0
	for i := qs.Index + 1; i < len(qs.Items); i++ {
		if qs.Items[i].Added {
			n++
		}
	}
	return n
}

// Replace starts a new queue with start, which reports its failure under the
// title failTitle; done (may be nil) says whether it started. When that
// would discard a long queue the user built it asks first or, by default,
// replaces it and offers Undo.
func (q *queueCommands) Replace(failTitle string, start func(ctx context.Context) error, done func(started bool)) {
	added := q.addedUpcoming()
	if added <= longUserQueue {
		go q.run(failTitle, start, done, 0)
		return
	}
	if q.mode != nil && q.mode() == config.ReplaceConfirm {
		kit.Confirm(kit.ConfirmOptions{
			Title: i18n.T("Replace your queue?"),
			Body: i18n.T("{added, plural, one {The # song you added to the queue will be removed.} other {The # songs you added to the queue will be removed.}}",
				"added", added),
			ConfirmLabel: i18n.T("Replace queue"),
		}, func(ok bool) {
			if !ok {
				if done != nil {
					done(false)
				}
				return
			}
			go q.run(failTitle, start, done, 0)
		})
		return
	}
	go q.run(failTitle, start, done, added)
}

// run calls start off the main thread; undoable is how many songs the user
// added that it removed, which the Undo toast says (0: no toast).
func (q *queueCommands) run(failTitle string, start func(ctx context.Context) error, done func(bool), undoable int) {
	err := start(context.Background())
	if done != nil {
		defer func() { done(err == nil) }()
	}
	if err != nil {
		kit.ToastErrorf(failTitle, i18n.ErrorMessage(err))
		return
	}
	if undoable == 0 {
		return
	}
	kit.ShowToast(kit.ToastSpec{
		ID:    "queue-replaced",
		Title: i18n.T("Queue replaced"),
		Description: i18n.T("{added, plural, one {# song you added was removed.} other {# songs you added were removed.}}",
			"added", undoable),
		Duration: 10 * time.Second,
		Action:   i18n.T("Undo"),
		OnAction: func() {
			go func() {
				if _, err := q.api.UndoReplace(context.Background()); err != nil {
					kit.ToastErrorf(i18n.T("Couldn't bring back your queue"), i18n.ErrorMessage(err))
				}
			}()
		},
	})
}
