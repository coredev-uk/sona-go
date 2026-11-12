package services

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/desktop/notify"
	"github.com/coredev-uk/sona-go/internal/discord"
	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
	"github.com/coredev-uk/sona-go/internal/playback"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

const (
	// integrationsCallTimeout bounds each D-Bus or Discord call.
	integrationsCallTimeout = 5 * time.Second
	// presenceArtSize is the artwork edge Discord is pointed at.
	presenceArtSize = 512
	// presenceInterval spaces SET_ACTIVITY calls (Discord allows 5 per 20 s).
	presenceInterval = 4 * time.Second
	// presenceRetry is how often an unreachable Discord is retried while
	// a track plays.
	presenceRetry = 30 * time.Second

	actionNext = "next"
)

// Go's own text in the integrations, in the user's language
// (Deps.Language).
const (
	msgShowSona      l10n.Message = "Show Sona"     // a track notification's button: brings the window back
	msgNextSong      l10n.Message = "Next"          // a track notification's button: plays the next song
	msgInhibitReason l10n.Message = "Playing music" // why Sona keeps the computer from sleeping (the system's inhibitor list)
)

// IntegrationsService drives the desktop integrations from playback:
// track-change notifications, the suspend inhibitor while playing and the
// Discord presence. It starts after the first paint.
type IntegrationsService struct {
	e *Env

	changed chan struct{} // integration settings changed

	mu      sync.Mutex
	status  IntegrationsStatus
	started bool
	stopped bool
	quit    chan struct{}
	done    chan struct{}

	// Notifications: notifyMu orders Show/Dismiss across the per-track
	// goroutines; seq drops a track's notification once a newer one is due.
	notifyMu  sync.Mutex
	notifier  Notifier
	notifySeq uint64
	closing   bool
}

func newIntegrationsService(e *Env) *IntegrationsService {
	s := &IntegrationsService{
		e:       e,
		changed: make(chan struct{}, 1),
		quit:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	s.status.Discord.Configured = discord.ClientID() != ""
	poke := func() {
		select {
		case s.changed <- struct{}{}:
		default:
		}
	}
	e.Settings.OnChange(func(prev, next config.Settings) {
		if prev.Integrations != next.Integrations {
			poke()
		}
	})
	if e.Private != nil {
		e.Private.OnChange(func(privatemode.State) { poke() })
	}
	// D-Bus and the Discord socket are local, but nothing extra touches
	// them before the first paint.
	e.OnNetwork(func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.stopped || s.started {
			return
		}
		s.started = true
		e.Go("integrations", s.run)
	})
	e.OnClose(s.stop)
	return s
}

// Status reports the integrations' state.
func (s *IntegrationsService) Status() IntegrationsStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *IntegrationsService) setStatus(fn func(*IntegrationsStatus)) {
	s.mu.Lock()
	prev := s.status
	fn(&s.status)
	next := s.status
	s.mu.Unlock()
	if next != prev && s.e.Emit != nil {
		s.e.Emit.Emit(EventIntegrationsStatus, next)
	}
}

func (s *IntegrationsService) stop() {
	s.mu.Lock()
	s.stopped = true
	started := s.started
	s.mu.Unlock()
	if started {
		close(s.quit)
		<-s.done
	}
}

// run follows playback until stop.
func (s *IntegrationsService) run() {
	defer close(s.done)
	snaps, cancel := s.e.Coord.Subscribe()
	defer cancel()

	var inh SleepInhibitor
	if s.e.Platform.Inhibitor != nil {
		inh = s.e.Platform.Inhibitor(func() string { return s.e.Language.Printer().T(msgInhibitReason) })
	}
	var keeper RunningKeeper
	if s.e.Platform.KeepRunning != nil {
		keeper = s.e.Platform.KeepRunning(func() string { return s.e.Language.Printer().T(msgInhibitReason) })
	}
	var presence *presenceWorker
	if id := discord.ClientID(); id != "" && discord.Supported {
		presence = newPresenceWorker(s, discord.New(id))
		s.e.Go("discord presence", presence.run)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), integrationsCallTimeout)
		defer cancel()
		s.closeNotifier(ctx)
		if inh != nil {
			if err := inh.Close(); err != nil {
				s.e.Logger.Warn("inhibit: release", "err", err)
			}
		}
		if keeper != nil {
			keeper.Close()
		}
		if presence != nil {
			presence.stop()
		}
	}()

	var (
		last coordinator.Snapshot
		have bool
		st   integrationsState
	)
	apply := func() {
		p := st.plan(last, s.e.Settings.Get().Integrations, s.e.Private.Since, s.e.Now())
		if p.announce != nil && s.e.Platform.Notifications != nil {
			s.announce(*p.announce)
		}
		if p.inhibit != nil {
			s.setStatus(func(st *IntegrationsStatus) { st.Inhibit = s.inhibit(inh, *p.inhibit) })
		}
		if keeper != nil && p.running != nil {
			if *p.running {
				keeper.Hold()
			} else {
				keeper.Release()
			}
		}
		if presence != nil && p.presence != nil {
			presence.set(*p.presence)
		}
	}

	for {
		select {
		case <-s.quit:
			return
		case snap, ok := <-snaps:
			if !ok {
				return
			}
			last, have = snap, true
			apply()
		case <-s.changed:
			if have {
				apply()
			}
		}
	}
}

// inhibit takes (want) or drops the sleep inhibitor inh (nil: none on
// this platform) and returns IntegrationsStatus.Inhibit.
func (s *IntegrationsService) inhibit(inh SleepInhibitor, want bool) string {
	if inh == nil {
		if want {
			return IntegrationUnavailable
		}
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), integrationsCallTimeout)
	defer cancel()
	if want {
		if err := inh.Acquire(ctx); err != nil {
			s.e.Logger.Warn("inhibit: acquire", "err", err)
		}
	} else if err := inh.Release(); err != nil {
		s.e.Logger.Warn("inhibit: release", "err", err)
	}
	backend := inh.Backend()
	if want && backend == "" {
		backend = IntegrationUnavailable
	}
	return backend
}

// integrationsState is what the integrations last did, so each snapshot
// only acts on changes.
type integrationsState struct {
	notified  uint64 // queue slot last announced (UIDs start at 1)
	inhibited bool
	running   bool // the app is held at full speed (Platform.KeepRunning)
	// playing is the queue slot last seen playing and playingAt when it
	// started, which is when its listen began for private mode.
	playing   uint64
	playingAt time.Time
}

// integrationsPlan is what to do for one snapshot.
type integrationsPlan struct {
	announce *coordinator.Item // notify this track
	inhibit  *bool             // take (true) or drop (false) the inhibitor
	running  *bool             // hold (true) or release (false) the full-speed hold
	presence *presenceWant     // the Discord presence; nil leaves it as is
}

// plan decides the integrations' actions for snap under set at now;
// private is privatemode.Mode.Since (nil: never private).
func (st *integrationsState) plan(snap coordinator.Snapshot, set config.Integrations, private func(time.Time) bool, now time.Time) integrationsPlan {
	var p integrationsPlan
	// Notifications: once per queue slot, when it starts playing (a slot
	// restored paused is announced when it is played). The slot counts as
	// announced even with notifications off, so turning them on does not
	// announce a track already playing.
	if cur := snap.Current; cur != nil && snap.State == playback.StatePlaying && cur.UID != st.notified {
		st.notified = cur.UID
		if set.Notifications {
			p.announce = &cur.Item
		}
	}
	// Suspend inhibitor: held while playing, also across the loading gap
	// between tracks.
	awake := snap.Current != nil && (snap.State == playback.StatePlaying || snap.State == playback.StateLoading)
	if want := set.InhibitSleep && awake; want != st.inhibited {
		st.inhibited = want
		p.inhibit = &want
	}
	// The same span keeps the app itself at full speed, whatever the
	// setting: the computer may still sleep when idle.
	if awake != st.running {
		st.running = awake
		p.running = &awake
	}
	// Discord: the track while it plays, cleared otherwise; left as is
	// while the next track loads so it does not flicker. A private listen
	// (private mode on at any point since its slot started playing) shows
	// nothing.
	if cur := snap.Current; cur != nil && snap.State == playback.StatePlaying && cur.UID != st.playing {
		st.playing, st.playingAt = cur.UID, now
	}
	if snap.State != playback.StateLoading {
		w := presenceWant{enabled: set.DiscordPresence}
		hidden := private != nil && snap.Current != nil && private(st.playingAt)
		if set.DiscordPresence && !hidden {
			w.act = presenceFor(snap)
		}
		p.presence = &w
	}
	return p
}

// announce shows item's notification off the playback loop: it resolves
// the artwork to a local file first, then skips the notification if a
// newer track came meanwhile or the main window has focus.
func (s *IntegrationsService) announce(item coordinator.Item) {
	s.notifyMu.Lock()
	s.notifySeq++
	seq := s.notifySeq
	s.notifyMu.Unlock()
	s.e.Go("track notification", func() {
		var image string
		if item.Artwork != "" && s.e.Art != nil {
			ctx, cancel := context.WithTimeout(context.Background(), integrationsCallTimeout)
			p, err := s.e.Art.LocalPath(ctx, item.Artwork, mprisArtSize, mprisArtSize)
			cancel()
			if err != nil {
				s.e.Logger.Debug("notification artwork", "err", err)
			} else {
				image = p
			}
		}
		if s.e.Focused != nil && s.e.Focused() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), integrationsCallTimeout)
		defer cancel()
		s.notifyMu.Lock()
		defer s.notifyMu.Unlock()
		if s.closing || seq != s.notifySeq {
			return
		}
		n, err := s.notifierLocked(ctx)
		if err != nil {
			if s.Status().Notifications != IntegrationUnavailable {
				s.e.Logger.Info("notifications unavailable", "err", err)
			}
			s.setStatus(func(st *IntegrationsStatus) { st.Notifications = IntegrationUnavailable })
			return
		}
		switch err := n.Show(ctx, trackNotification(s.e.Language.Printer(), item, image)); {
		case errors.Is(err, ErrNotificationsDenied):
			if s.Status().Notifications != IntegrationDenied {
				s.e.Logger.Info("notifications denied by the system", "err", err)
			}
			s.setStatus(func(st *IntegrationsStatus) { st.Notifications = IntegrationDenied })
		case err != nil:
			s.e.Logger.Warn("notification", "err", err)
		default:
			// Allowed again (System Settings): the status was "denied".
			s.setStatus(func(st *IntegrationsStatus) {
				if st.Notifications == IntegrationDenied {
					st.Notifications = IntegrationReady
				}
			})
		}
	})
}

// notifierLocked connects to the notification server on first use, and
// again after a failure (the server may have started since).
func (s *IntegrationsService) notifierLocked(ctx context.Context) (Notifier, error) {
	if s.notifier != nil {
		return s.notifier, nil
	}
	n, err := s.e.Platform.Notifications(ctx, s.onAction)
	if err != nil {
		return nil, err
	}
	s.notifier = n
	s.setStatus(func(st *IntegrationsStatus) { st.Notifications = IntegrationReady })
	return n, nil
}

// closeNotifier removes the notification (its Next button would outlive
// the app) and unsubscribes; later announce goroutines do nothing.
func (s *IntegrationsService) closeNotifier(ctx context.Context) {
	s.notifyMu.Lock()
	defer s.notifyMu.Unlock()
	s.closing = true
	if s.notifier == nil {
		return
	}
	if err := s.notifier.Dismiss(ctx); err != nil {
		s.e.Logger.Debug("notification dismiss", "err", err)
	}
	s.notifier.Close()
	s.notifier = nil
}

// onAction handles the notification's buttons (on the notifier's signal
// goroutine).
func (s *IntegrationsService) onAction(key string) {
	switch key {
	case actionNext:
		s.e.Go("notification next", func() {
			ctx, cancel := context.WithTimeout(context.Background(), integrationsCallTimeout)
			defer cancel()
			if err := s.e.Coord.Next(ctx); err != nil {
				s.e.Logger.Warn("notification next", "err", err)
			}
		})
	case notify.DefaultAction:
		if s.e.Focus != nil {
			s.e.Focus()
		}
	}
}

// trackNotification is the notification for item, its buttons in p's
// language; image is a local artwork file or "".
func trackNotification(p *l10n.Printer, item coordinator.Item, image string) notify.Notification {
	body := item.Artist
	if item.Album != "" {
		if body != "" {
			body += " — "
		}
		body += item.Album
	}
	actions := []notify.Action{{Key: notify.DefaultAction, Label: p.T(msgShowSona)}}
	if !item.Live() { // a live station has no next song
		actions = append(actions, notify.Action{Key: actionNext, Label: p.T(msgNextSong)})
	}
	return notify.Notification{Summary: item.Title, Body: body, Image: image, Actions: actions}
}

// presenceFor is the Discord activity for snap: the current track while it
// plays, nil otherwise.
func presenceFor(snap coordinator.Snapshot) *discord.Activity {
	cur := snap.Current
	if cur == nil || snap.State != playback.StatePlaying {
		return nil
	}
	a := &discord.Activity{
		Title:      cur.Item.Title,
		Artist:     cur.Item.Artist,
		Album:      cur.Item.Album,
		ArtworkURL: publicArtURL(cur.Item.Artwork, presenceArtSize),
	}
	if !snap.At.IsZero() {
		a.Start = snap.At.Add(-snap.Position)
		dur := snap.Duration
		if dur == 0 {
			dur = cur.Item.Duration
		}
		if dur > 0 {
			a.End = a.Start.Add(dur)
		}
	}
	return a
}

// publicArtURL fills an Apple artwork template to size×size for a client
// outside Sona (Discord fetches it itself). Only https URLs on Apple's
// public artwork CDN qualify: never Sona's local /art/ URLs or anything
// that could carry credentials.
func publicArtURL(tmpl string, size int) string {
	tmpl = strings.TrimSpace(tmpl)
	u, err := url.Parse(tmpl)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if !strings.HasSuffix(host, ".mzstatic.com") || host == ".mzstatic.com" || (u.Port() != "" && u.Port() != "443") {
		return ""
	}
	n := strconv.Itoa(size)
	return strings.NewReplacer("{w}", n, "{h}", n, "{c}", "bb", "{f}", "jpg").Replace(tmpl)
}

// presenceWorker sends the wanted Discord activity, coalescing changes to
// Discord's rate limit and retrying while Discord is unreachable.
type presenceWorker struct {
	s      *IntegrationsService
	client *discord.Client
	want   chan presenceWant // latest only
	quit   chan struct{}
	done   chan struct{}
}

type presenceWant struct {
	enabled bool
	act     *discord.Activity
}

func newPresenceWorker(s *IntegrationsService, c *discord.Client) *presenceWorker {
	return &presenceWorker{
		s: s, client: c,
		want: make(chan presenceWant, 1),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// set replaces the wanted presence.
func (w *presenceWorker) set(want presenceWant) {
	for {
		select {
		case w.want <- want:
			return
		default:
			select {
			case <-w.want:
			default:
			}
		}
	}
}

// stop clears the presence and disconnects.
func (w *presenceWorker) stop() {
	close(w.quit)
	<-w.done
}

func (w *presenceWorker) run() {
	defer close(w.done)
	var (
		want    presenceWant
		sent    *discord.Activity
		synced  = true // Discord shows want (nothing wanted at start)
		next    time.Time
		timer   = time.NewTimer(0)
		pending bool // timer armed
	)
	<-timer.C
	defer func() {
		timer.Stop()
		if w.client.Connected() {
			ctx, cancel := context.WithTimeout(context.Background(), integrationsCallTimeout)
			_ = w.client.SetActivity(ctx, nil)
			cancel()
		}
		_ = w.client.Close()
	}()
	arm := func(at time.Time) {
		if pending {
			timer.Stop()
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(time.Until(at))
		pending = true
	}
	for {
		select {
		case <-w.quit:
			return
		case nw := <-w.want:
			if nw.enabled == want.enabled && sameActivity(nw.act, sent) && synced {
				want = nw
				continue
			}
			want, synced = nw, false
			if time.Now().Before(next) {
				arm(next)
				continue
			}
		case <-timer.C:
			pending = false
		}
		if synced {
			continue
		}
		if !want.enabled && !w.client.Connected() {
			sent, synced = nil, true
			w.s.setStatus(func(st *IntegrationsStatus) { st.Discord.Connected = false })
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), integrationsCallTimeout)
		err := w.client.SetActivity(ctx, want.act)
		cancel()
		next = time.Now().Add(presenceInterval)
		if err == nil {
			sent, synced = want.act, true
			if !want.enabled {
				_ = w.client.Close()
			}
		} else if want.act != nil {
			arm(time.Now().Add(presenceRetry))
		} else {
			// Nothing to show and Discord gone: nothing left to clear.
			sent, synced = nil, true
		}
		if err != nil {
			w.s.e.Logger.Debug("discord presence", "err", err)
		}
		connected := w.client.Connected()
		w.s.setStatus(func(st *IntegrationsStatus) { st.Discord.Connected = connected })
	}
}

// sameActivity reports whether a and b show the same thing; the start and
// end may drift by the snapshot extrapolation.
func sameActivity(a, b *discord.Activity) bool {
	if a == nil || b == nil {
		return a == b
	}
	near := func(x, y time.Time) bool {
		d := x.Sub(y)
		return d < 2*time.Second && d > -2*time.Second
	}
	return a.Title == b.Title && a.Artist == b.Artist && a.Album == b.Album &&
		a.ArtworkURL == b.ArtworkURL && near(a.Start, b.Start) && near(a.End, b.End)
}
