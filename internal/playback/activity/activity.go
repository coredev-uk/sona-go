// Package activity reports Sona's listening to Apple's play-activity
// service, the way MusicKit JS does, so plays show up in the user's Recently
// Played, history and recommendations. It turns coordinator.Activity
// transitions into MusicKit's PLAY_START/PLAY_END events and delivers them
// in order, retrying transient failures. Events that are still undelivered
// at Close are saved (without credentials) and sent on the next start.
package activity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
)

// Options configures a Reporter.
type Options struct {
	Client *applemusic.Client // required
	// Storefront returns the user's storefront country code ("gb"); nil or
	// "" makes the Reporter look it up once through Client.
	Storefront func() string
	// StatePath stores undelivered events across restarts; "" disables it.
	StatePath  string
	AppVersion string // reported in build-version ("Sona/<version> …")
	Logger     *slog.Logger
}

// Limits.
const (
	maxQueue   = 500 // events kept while Apple is unreachable
	maxBatch   = 20
	minBackoff = 5 * time.Second
	maxBackoff = 5 * time.Minute
)

// Reporter is safe for concurrent use. Observe never blocks.
type Reporter struct {
	api        *applemusic.Client
	storefront func() string
	path       string
	log        *slog.Logger
	build      string
	userAgent  string
	now        func() time.Time
	backoff    func(attempt int) time.Duration

	mu     sync.Mutex
	queue  []applemusic.PlayActivityEvent
	sf     string // storefront looked up through api
	prev   *last  // previous event of the open (or last) listen
	open   bool   // a PLAY_START awaits its PLAY_END
	closed bool

	wake chan struct{}
	stop chan struct{}
	done chan struct{}
}

// last is what MusicKit's next event needs from the previous one.
type last struct {
	item     coordinator.Item
	position time.Duration
	duration time.Duration
}

// New returns a running Reporter, re-queueing events saved by a previous
// Close.
func New(o Options) (*Reporter, error) { return newReporter(o, defaultBackoff) }

func newReporter(o Options, backoff func(attempt int) time.Duration) (*Reporter, error) {
	if o.Client == nil {
		return nil, errors.New("activity: Client is required")
	}
	log := o.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	version := o.AppVersion
	if version == "" {
		version = "1.0"
	}
	r := &Reporter{
		api:        o.Client,
		storefront: o.Storefront,
		path:       o.StatePath,
		log:        log.With("component", "play-activity"),
		build:      buildVersion(version),
		userAgent:  fmt.Sprintf("Mozilla/5.0 (X11; Linux %s) Sona/%s", machine(), version),
		now:        time.Now,
		backoff:    backoff,
		wake:       make(chan struct{}, 1),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	if err := r.load(); err != nil {
		r.log.Warn("play activity: discarding saved events", "err", err)
	}
	go r.run()
	return r, nil
}

// buildVersion follows MusicKit: "<App>/<version> <OS>/<version> model/<platform> build/<build>".
func buildVersion(version string) string {
	return fmt.Sprintf("Sona/%s Linux/0.0 model/Linux %s build/0.0.0", version, machine())
}

func machine() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}
	return runtime.GOARCH
}

func defaultBackoff(attempt int) time.Duration {
	d := minBackoff << min(attempt, 10)
	return min(d, maxBackoff)
}

// Observe turns one coordinator activity into play-activity events, like
// MusicKit's PlayActivity: play() → PLAY_START, pause()/stop()/skip() →
// PLAY_END with the matching end reason, seek → PLAY_END SCRUB_BEGIN at the
// old position then PLAY_END SCRUB_END at the new one. Use it as
// coordinator.Options.OnActivity.
func (r *Reporter) Observe(a coordinator.Activity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if strings.HasPrefix(a.Item.ID, "ra.") {
		// Live radio and episodes are not songs: MusicKit reports them with
		// a radio event shape Sona does not produce, so they are not reported.
		return
	}
	switch a.Kind {
	case coordinator.ActivityStart:
		r.open = true
		r.prev = &last{item: a.Item, position: a.Position, duration: duration(a)}
		r.enqueueLocked(r.eventLocked(a.At, applemusic.PlayEventStart, a.Position, a.Position, nil))
	case coordinator.ActivitySeek:
		if !r.open || r.prev == nil || r.prev.item.ID != a.Item.ID {
			return
		}
		begin, end := applemusic.EndReasonScrubBegin, applemusic.EndReasonScrubEnd
		r.endLocked(a.At, a.From, begin)
		r.endLocked(a.At, a.Position, end)
	case coordinator.ActivityEnd:
		if !r.open || r.prev == nil || r.prev.item.ID != a.Item.ID {
			return
		}
		r.open = false
		r.endLocked(a.At, a.Position, endReason(a.Reason))
	}
}

// endLocked emits a PLAY_END from the previous position to pos.
func (r *Reporter) endLocked(at time.Time, pos time.Duration, reason applemusic.PlayEndReason) {
	start := r.prev.position
	r.enqueueLocked(r.eventLocked(at, applemusic.PlayEventEnd, start, pos, &reason))
	r.prev.position = pos
}

func duration(a coordinator.Activity) time.Duration {
	if a.Duration > 0 {
		return a.Duration
	}
	return a.Item.Duration
}

// endReason maps coordinator reasons to MusicKit's. A plain stop is
// NATURAL_END_OF_TRACK, as MusicKit's stop() defaults to.
func endReason(r coordinator.EndReason) applemusic.PlayEndReason {
	switch r {
	case coordinator.EndSkipForward:
		return applemusic.EndReasonSkippedForwards
	case coordinator.EndSkipBackward:
		return applemusic.EndReasonSkippedBackwards
	case coordinator.EndSelected:
		return applemusic.EndReasonSelectedOtherItem
	case coordinator.EndPaused:
		return applemusic.EndReasonManuallyPaused
	case coordinator.EndFailed:
		return applemusic.EndReasonFailedToLoad
	case coordinator.EndExited:
		return applemusic.EndReasonExitedApplication
	}
	return applemusic.EndReasonNaturalEnd // natural, stopped
}

// libraryID matches MusicKit's isLibraryType.
var libraryID = regexp.MustCompile(`^[ailp]\.[a-zA-Z0-9]+$`)

// eventLocked builds one event for r.prev's item.
func (r *Reporter) eventLocked(at time.Time, kind int, start, pos time.Duration, reason *applemusic.PlayEndReason) applemusic.PlayActivityEvent {
	if at.IsZero() {
		at = r.now()
	}
	_, offset := at.Zone()
	dur := r.prev.duration.Milliseconds()
	e := applemusic.PlayActivityEvent{
		EventType:        kind,
		Type:             applemusic.PlayItemTypeStoreContent,
		ContainerType:    applemusic.PlayContainerUnknown,
		FeatureName:      applemusic.PlayFeatureMusicKit,
		MediaType:        applemusic.PlayMediaTypeAudio,
		MediaDurationMS:  dur,
		StartPositionMS:  start.Milliseconds(),
		Timestamp:        at.UnixMilli(),
		PersistentID:     persistentID(),
		BuildVersion:     r.build,
		UserAgent:        r.userAgent,
		UTCOffsetSeconds: offset,
		SourceType:       applemusic.PlaySourceMusicKit,
		Subscribed:       true,
	}
	if id := r.prev.item.ID; libraryID.MatchString(id) {
		e.IDs = map[string]string{"cloud-id": id}
	} else {
		e.IDs = map[string]string{"subscription-adam-id": id}
	}
	if kind == applemusic.PlayEventStart {
		hint := 0 // NOT_SPECIFIED: no container
		e.EventReasonHintType = &hint
		return e
	}
	end := pos.Milliseconds()
	e.EndPositionMS, e.EndReasonType = &end, reason
	e.MediaDurationMS = max(dur, end)
	return e
}

// persistentID is MusicKit's generateUUID: 16 random hex digits.
func persistentID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (r *Reporter) enqueueLocked(e applemusic.PlayActivityEvent) {
	if len(r.queue) >= maxQueue {
		// Dropping the newest keeps a batch in flight aligned with the queue.
		r.log.Warn("play activity: queue full, dropping event")
		return
	}
	r.queue = append(r.queue, e)
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

// run delivers queued events until Close.
func (r *Reporter) run() {
	defer close(r.done)
	attempt := 0
	for {
		var retry <-chan time.Time
		if attempt > 0 {
			retry = time.After(r.backoff(attempt - 1))
		}
		select {
		case <-r.stop:
			return
		case <-r.wake:
		case <-retry:
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		go func() {
			select {
			case <-r.stop:
				cancel()
			case <-ctx.Done():
			}
		}()
		err := r.flush(ctx)
		cancel()
		switch {
		case err == nil:
			attempt = 0
		case errors.Is(err, context.Canceled):
			return
		default:
			attempt++
			r.log.Warn("play activity: delivery failed; will retry", "err", err, "attempt", attempt)
		}
	}
}

// flush sends the queue in batches. Events Apple rejects as malformed
// (4xx other than 401/403/429) are dropped so they cannot block the queue.
func (r *Reporter) flush(ctx context.Context) error {
	for {
		r.mu.Lock()
		n := min(len(r.queue), maxBatch)
		batch := append([]applemusic.PlayActivityEvent(nil), r.queue[:n]...)
		r.mu.Unlock()
		if n == 0 {
			return nil
		}
		if err := r.fillStorefront(ctx, batch); err != nil {
			return err
		}
		err := r.api.SendPlayActivity(ctx, batch)
		var se *applemusic.StatusError
		if errors.As(err, &se) && se.Code >= 400 && se.Code < 500 && se.Code != http.StatusTooManyRequests && !errors.Is(err, applemusic.ErrUnauthorized) {
			r.log.Error("play activity: Apple rejected events; dropping them", "status", se.Code, "body", se.Body, "events", n)
			err = nil
		}
		if err != nil {
			return err
		}
		r.mu.Lock()
		r.queue = r.queue[n:]
		r.mu.Unlock()
		r.log.Debug("play activity: delivered", "events", n)
	}
}

// fillStorefront stamps events queued before the storefront was known.
func (r *Reporter) fillStorefront(ctx context.Context, batch []applemusic.PlayActivityEvent) error {
	sf := ""
	if r.storefront != nil {
		sf = r.storefront()
	}
	if sf == "" {
		r.mu.Lock()
		sf = r.sf
		r.mu.Unlock()
	}
	if sf == "" {
		res, err := r.api.Storefront(ctx)
		if err != nil {
			return fmt.Errorf("storefront: %w", err)
		}
		sf = res.ID
		r.mu.Lock()
		r.sf = sf
		r.mu.Unlock()
	}
	for i := range batch {
		if batch[i].StoreFront == "" {
			batch[i].StoreFront = sf
		}
	}
	return nil
}

// Close stops reporting: it delivers what it can before ctx ends (Close
// the coordinator first so its final EndExited is included) and saves the
// rest to StatePath.
func (r *Reporter) Close(ctx context.Context) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()
	close(r.stop)
	<-r.done
	err := r.flush(ctx)
	if err != nil {
		r.log.Warn("play activity: undelivered at exit", "err", err)
	}
	return r.save()
}

// save writes the undelivered events (no credentials) to StatePath.
func (r *Reporter) save() error {
	r.mu.Lock()
	q := r.queue
	r.mu.Unlock()
	if r.path == "" {
		return nil
	}
	if len(q) == 0 {
		if err := os.Remove(r.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := json.Marshal(q)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

// load queues events saved by a previous Close and removes the file.
func (r *Reporter) load() error {
	if r.path == "" {
		return nil
	}
	b, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer os.Remove(r.path)
	var q []applemusic.PlayActivityEvent
	if err := json.Unmarshal(b, &q); err != nil {
		return err
	}
	r.mu.Lock()
	r.queue = append(q, r.queue...)
	if len(r.queue) > maxQueue {
		r.queue = r.queue[len(r.queue)-maxQueue:]
	}
	r.mu.Unlock()
	if len(q) > 0 {
		select {
		case r.wake <- struct{}{}:
		default:
		}
	}
	return nil
}
