package kit

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/i18n"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

// LongUserQueue is v0.2.0's replace.ts LONG_USER_QUEUE: more upcoming songs
// the user queued by hand than this, and replacing the queue asks first
// (setting "confirm") or offers Undo (the default).
const LongUserQueue = 3

// QueueService is what the queue commands ask of PlaybackService
// (*services.PlaybackService).
type QueueService interface {
	PlayItems(ctx context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error
	PlayNext(ctx context.Context, refs []services.TrackRef) error
	Append(ctx context.Context, refs []services.TrackRef) error
	Insert(ctx context.Context, at int, refs []services.TrackRef) error
	UndoReplace(ctx context.Context) (bool, error)
	SetShuffle(on bool)
}

// QueueDeps is everything QueueCommands reads and calls.
type QueueDeps struct {
	Svc  QueueService
	Play *store.Playback
	// Post hands a result back to the main thread (Env.Post).
	Post store.Poster
	// ReplaceMode is the "queue.replaceConfirm" setting ("confirm" or
	// "undo"); called off the main thread. Nil: undo.
	ReplaceMode func() string
	// Tracks is the songs an album or a playlist holds, in order; called off
	// the main thread.
	Tracks func(ctx context.Context, it Item) ([]services.Song, error)
	// Unavailable is why the queue takes nothing now (a live station), "" when
	// it does; any thread.
	Unavailable func() string
}

// QueueCommands is features/player/commands.ts and replace.ts: starting a
// queue (replacing one the user built, with a confirm or an Undo), shuffling
// songs, playing videos and queueing items. Every feature that plays
// something uses it, not the service, so a long queue of the user's is never
// lost without a word. Its callbacks and Replace run on the main thread; the
// commands start goroutines.
type QueueCommands struct {
	d QueueDeps
}

// NewQueueCommands returns the commands over d.
func NewQueueCommands(d QueueDeps) *QueueCommands { return &QueueCommands{d: d} }

// PlayableRefs keeps the playable songs as track refs and maps start (an
// index in songs) to its place among them (v0.2.0's playable()).
func PlayableRefs(songs []services.Song, start int) (refs []services.TrackRef, index int) {
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

// VideoRef is a music video's queue entry (TrackRef kind "music-video").
func VideoRef(v services.MusicVideo) services.TrackRef {
	return services.TrackRef{ID: v.ID, Source: v.Source, Kind: services.TrackKindVideo}
}

// fail shows a failed command: what it was, then Go's words for the code.
func fail(what string, err error) {
	ToastErrorf(what, i18n.ErrorText(services.CodeOf(err)))
}

func nothingPlayable() {
	ToastErrorf(i18n.T("Nothing here can be played"), i18n.T("None of these songs is available to stream."))
}

// AddedUpcoming counts the songs after the current one that the user
// queued by hand.
func AddedUpcoming(q services.QueueState) int {
	n := 0
	for i := q.Index + 1; i < len(q.Items); i++ {
		if q.Items[i].Added {
			n++
		}
	}
	return n
}

// ReplaceOption changes how Replace behaves.
type ReplaceOption func(*replaceOptions)

type replaceOptions struct {
	failure func(err error) (title, description string)
}

// WithFailure words a failed start: the toast's title and description for err
// (instead of Replace's failTitle and the error code's text).
func WithFailure(f func(err error) (title, description string)) ReplaceOption {
	return func(o *replaceOptions) { o.failure = f }
}

// Replace starts a new queue with start, which runs off the main thread and
// returns its failure (nil: playback started; the failure is shown here
// titled failTitle). When that would discard a long queue the user built it
// asks first, or by default replaces it and offers Undo, which brings back
// the old queue at the same song and position. done (may be nil) gets whether
// playback started, on the main thread. Main thread only.
//
// A failure is worded by default as failTitle over the words of its error
// code; WithFailure replaces that (a station with its own wording).
func (c *QueueCommands) Replace(failTitle string, start func(ctx context.Context) error, done func(started bool), opts ...ReplaceOption) {
	var o replaceOptions
	for _, f := range opts {
		f(&o)
	}
	finish := func(ok bool) {
		if done != nil {
			done(ok)
		}
	}
	added := AddedUpcoming(c.d.Play.Queue())
	run := func(mode string) {
		go func() {
			err := start(context.Background())
			c.d.Post(func() {
				if err != nil {
					if o.failure != nil {
						title, text := o.failure(err)
						ToastErrorf(title, text)
					} else {
						fail(failTitle, err)
					}
					finish(false)
					return
				}
				if added > LongUserQueue && mode != config.ReplaceConfirm {
					c.offerUndo(added)
				}
				finish(true)
			})
		}()
	}
	if added <= LongUserQueue {
		run("")
		return
	}
	go func() {
		mode := config.ReplaceUndo
		if c.d.ReplaceMode != nil {
			mode = c.d.ReplaceMode()
		}
		c.d.Post(func() {
			if mode != config.ReplaceConfirm {
				run(mode)
				return
			}
			Confirm(ConfirmOptions{
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

func (c *QueueCommands) offerUndo(added int) {
	ShowToast(ToastSpec{
		ID:    "queue-replaced",
		Title: i18n.T("Queue replaced"),
		Description: i18n.T("{added, plural, one {# song you added was removed.} other {# songs you added were removed.}}",
			"added", added),
		Duration: 10 * time.Second,
		Action:   i18n.T("Undo"),
		OnAction: func() {
			go func() {
				if _, err := c.d.Svc.UndoReplace(context.Background()); err != nil {
					fail(i18n.T("Couldn't bring back your queue"), err)
				}
			}()
		},
	})
}

// PlaySongs plays songs from start as the queue, from the collection from
// (a kind "" with a label for anything else: "search"). Unplayable songs are
// dropped; none playable is an error toast.
func (c *QueueCommands) PlaySongs(songs []services.Song, start int, from services.PlayFrom) {
	refs, index := PlayableRefs(songs, start)
	if len(refs) == 0 {
		nothingPlayable()
		return
	}
	c.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return c.d.Svc.PlayItems(ctx, refs, index, from)
	}, nil)
}

// ShuffleSongs turns shuffle on, then starts from a random playable song.
func (c *QueueCommands) ShuffleSongs(songs []services.Song, from services.PlayFrom) {
	refs, _ := PlayableRefs(songs, 0)
	if len(refs) == 0 {
		return
	}
	c.d.Play.Patch(func(s *services.PlaybackState) { s.Shuffle = true })
	start := rand.IntN(len(refs))
	// Shuffle first, then the start (the service is never called on the main thread).
	go func() {
		c.d.Svc.SetShuffle(true)
		c.d.Post(func() {
			c.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
				return c.d.Svc.PlayItems(ctx, refs, start, from)
			}, nil)
		})
	}()
}

// PlayVideos plays music videos from start as the queue (videos are never
// reported as plays: no PlayFrom collection); opened runs when playback
// started (the video view opens).
func (c *QueueCommands) PlayVideos(videos []services.MusicVideo, start int, opened func()) {
	if len(videos) == 0 {
		return
	}
	refs := make([]services.TrackRef, len(videos))
	for i, v := range videos {
		refs[i] = VideoRef(v)
	}
	c.Replace(i18n.T("Couldn't start playback"), func(ctx context.Context) error {
		return c.d.Svc.PlayItems(ctx, refs, start, services.PlayFrom{})
	}, func(ok bool) {
		if ok && opened != nil {
			opened()
		}
	})
}

// QueuePlace is where QueueItems puts songs (v0.2.0's "next" | "last" |
// number): QueueNext, QueueLast or QueueAt(slot).
type QueuePlace struct {
	kind queuePlaceKind
	slot int
}

type queuePlaceKind int

const (
	placeNext queuePlaceKind = iota
	placeLast
	placeAt
)

// QueueNext is right after the current item.
var QueueNext = QueuePlace{kind: placeNext}

// QueueLast is after the songs the user queued.
var QueueLast = QueuePlace{kind: placeLast}

// QueueAt is before queue position slot (a drop on the queue panel's gap).
func QueueAt(slot int) QueuePlace { return QueuePlace{kind: placeAt, slot: slot} }

// QueueItems queues songs, music videos, albums and playlists (their tracks),
// in order, at where. On a live station it says why and does nothing. Main
// thread only; the service calls run on a goroutine.
func (c *QueueCommands) QueueItems(items []Item, where QueuePlace) {
	if c.d.Unavailable != nil {
		if reason := c.d.Unavailable(); reason != "" {
			Unavailable(reason)
			return
		}
	}
	go func() {
		refs, err := c.refs(items)
		if err != nil {
			ToastErrorf(i18n.T("Couldn't add to the queue"), i18n.ErrorMessage(err))
			return
		}
		if len(refs) == 0 {
			nothingPlayable()
			return
		}
		ctx := context.Background()
		switch where.kind {
		case placeNext:
			err = c.d.Svc.PlayNext(ctx, refs)
		case placeLast:
			err = c.d.Svc.Append(ctx, refs)
		default:
			err = c.d.Svc.Insert(ctx, where.slot, refs)
		}
		if err != nil {
			fail(i18n.T("Couldn't add to the queue"), err)
			return
		}
		what := DescribeItems(items)
		if where.kind == placeNext {
			ShowToast(ToastSpec{Title: i18n.T("{count, plural, one {{what} plays next} other {{what} play next}}", "count", len(items), "what", what)})
			return
		}
		ShowToast(ToastSpec{Title: i18n.T("Added {what} to the queue", "what", what)})
	}()
}

// refs resolves items to track refs in order; albums and playlists load
// concurrently.
func (c *QueueCommands) refs(items []Item) ([]services.TrackRef, error) {
	lists := make([][]services.TrackRef, len(items))
	errs := make([]error, len(items))
	var wg sync.WaitGroup
	for i, it := range items {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch it.Kind {
			case KindMusicVideo:
				lists[i] = []services.TrackRef{VideoRef(it.MusicVideo())}
			case KindSong:
				lists[i], _ = PlayableRefs([]services.Song{it.Song()}, 0)
			case KindAlbum, KindPlaylist:
				songs, err := c.d.Tracks(context.Background(), it)
				errs[i] = err
				lists[i], _ = PlayableRefs(songs, 0)
			}
		}()
	}
	wg.Wait()
	var out []services.TrackRef
	for i := range items {
		if errs[i] != nil {
			return nil, errs[i]
		}
		out = append(out, lists[i]...)
	}
	return out, nil
}

// QueueUnavailable is why the queue takes nothing in state s (a live station
// has no queue to add to; neither has a station of songs), "" when it does.
func QueueUnavailable(s services.PlaybackState) string {
	if s.Controls.Queue {
		return ""
	}
	if s.Current != nil && s.Current.Live {
		return i18n.T("Not available while listening to a live station")
	}
	if s.Station != nil && !s.Station.AutoPlay {
		return i18n.T("Not available while listening to a station")
	}
	return ""
}

// NewQueueCommandsFor is the commands over the services of core: what the
// shell hands every feature as Env.Queue.
func NewQueueCommandsFor(core *services.Core, play *store.Playback, post store.Poster) *QueueCommands {
	pb := services.Service[services.PlaybackService](core)
	sys := services.Service[services.SystemService](core)
	return NewQueueCommands(QueueDeps{
		Svc:         pb,
		Play:        play,
		Post:        post,
		ReplaceMode: func() string { return sys.Settings().Queue.ReplaceConfirm },
		Tracks:      func(ctx context.Context, it Item) ([]services.Song, error) { return TracksOf(ctx, core, it) },
		Unavailable: func() string { return QueueUnavailable(play.State()) },
	})
}
