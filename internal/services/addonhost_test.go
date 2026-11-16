package services

import (
	"encoding/json"
	"errors"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/cli"
	"github.com/coredev-uk/sona-go/internal/playback/coordinator"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// nowPlaying waits for the next now-playing event of an add-on's topic.
func nowPlaying(t *testing.T, ch <-chan any) cli.NowPlaying {
	t.Helper()
	select {
	case v, ok := <-ch:
		if !ok {
			t.Fatal("the topic ended")
		}
		return v.(cli.NowPlaying)
	case <-time.After(5 * time.Second):
		t.Fatal("no now-playing event")
	}
	return cli.NowPlaying{}
}

// quiet asserts the topic has nothing to say for a moment.
func quiet(t *testing.T, ch <-chan any) {
	t.Helper()
	select {
	case v := <-ch:
		t.Fatalf("an event arrived: %+v", v)
	case <-time.After(150 * time.Millisecond):
	}
}

// awaitStatus drains the add-on feed until pred holds.
func awaitStatus(t *testing.T, ch <-chan any, pred func(cli.NowPlaying) bool) cli.NowPlaying {
	t.Helper()
	for {
		if np := nowPlaying(t, ch); pred(np) {
			return np
		}
	}
}

func playLibrary(t *testing.T, ce *commandEnv, id string) {
	t.Helper()
	if err := ce.play.PlayItems(t.Context(), []TrackRef{{ID: id, Source: SourceLibrary}}, 0, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
}

func addonCall(t *testing.T, ce *commandEnv, cmd string, args ...string) (any, error) {
	t.Helper()
	return ce.srv.addon.Call(t.Context(), cli.Request{Command: cmd, Args: args})
}

// A public listen reaches an add-on as an event, a status and a queue,
// with the flags that explain a hidden listen cleared.
func TestAddonHostShowsAPublicListen(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	topic, cancel, err := ce.srv.addon.Watch(cli.TopicNowPlaying)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	if np := nowPlaying(t, topic); np.Track != nil {
		t.Fatalf("first = %+v, want nothing playing", np)
	}
	playLibrary(t, ce, "i.100")
	np := awaitStatus(t, topic, func(np cli.NowPlaying) bool { return np.Track != nil })
	if np.Track.Title != "Come Together" || np.StartedAt == 0 {
		t.Errorf("now playing = %+v", np)
	}
	res, err := addonCall(t, ce, "status")
	if err != nil {
		t.Fatal(err)
	}
	if st := res.(cli.Status); st.Track == nil || st.Track.ID != "100" && st.Track.ID != "i.100" || st.Private || st.Preview {
		t.Errorf("status = %+v", st)
	}
	if res, err = addonCall(t, ce, "queue"); err != nil || len(res.(cli.Queue).Tracks) != 1 {
		t.Errorf("queue = %+v, %v", res, err)
	}
}

// The acceptance test of the private filter: private mode switched on in
// the middle of a listen and off again produces no event, and the reads
// (status, queue, lyrics, like) behave as if nothing played, until the
// next song, which is public.
func TestAddonHostHidesAListenPrivateAtAnyPoint(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	topic, cancel, err := ce.srv.addon.Watch(cli.TopicNowPlaying)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	statuses, cancelStatus, err := ce.srv.addon.Watch(cli.TopicStatus)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelStatus()
	queues, cancelQueue, err := ce.srv.addon.Watch(cli.TopicQueue)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelQueue()
	nowPlaying(t, topic) // idle
	playLibrary(t, ce, "i.100")
	awaitStatus(t, topic, func(np cli.NowPlaying) bool { return np.Track != nil })

	if _, err := ce.call(t, "private", "on"); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, topic, func(np cli.NowPlaying) bool { return np.Track == nil }) // the listen ends for the add-on
	// Its status goes idle and its queue empty, once.
	var lastStatus cli.Status
	for deadline := time.After(2 * time.Second); lastStatus.Track != nil || lastStatus.State != cli.StateIdle; {
		select {
		case v := <-statuses:
			lastStatus = v.(cli.Status)
		case <-deadline:
			t.Fatalf("status never went idle: %+v", lastStatus)
		}
	}
	for q := (<-queues).(cli.Queue); len(q.Tracks) != 0; q = (<-queues).(cli.Queue) {
	}
	if rest := drainStatus(statuses, 200*time.Millisecond); len(rest) > 0 { // repeats of the idle status
		if last := rest[len(rest)-1]; last.Track != nil || last.State != cli.StateIdle {
			t.Fatalf("status after going idle: %+v", last)
		}
	}
	if _, err := ce.call(t, "private", "off"); err != nil {
		t.Fatal(err)
	}
	quiet(t, topic) // off again: still the private listen, still no event
	quiet(t, statuses)
	quiet(t, queues)

	res, err := addonCall(t, ce, "status")
	if err != nil {
		t.Fatal(err)
	}
	if st := res.(cli.Status); st.Track != nil || st.State != cli.StateIdle || st.Private {
		t.Errorf("status of a hidden listen = %+v", st)
	}
	if res, err = addonCall(t, ce, "queue"); err != nil || len(res.(cli.Queue).Tracks) != 0 || res.(cli.Queue).Index != -1 {
		t.Errorf("queue of a hidden listen = %+v, %v", res, err)
	}
	for _, cmd := range []string{"lyrics", "like"} {
		if _, err := addonCall(t, ce, cmd); code(err) != cli.CodeNothingPlaying {
			t.Errorf("%s of a hidden listen: %v", cmd, err)
		}
	}
	// Sona's own v1 command line is not filtered: it is the user's.
	if st := ce.status(t, "status"); st.Track == nil {
		t.Errorf("the command line lost the song: %+v", st)
	}

	playLibrary(t, ce, "i.101") // the next listen is public
	np := awaitStatus(t, topic, func(np cli.NowPlaying) bool { return np.Track != nil })
	if np.Track.Title != "Something" {
		t.Errorf("next listen = %+v", np)
	}
}

// A listen that starts while private mode is on is never an event, and
// turning private mode off during it doesn't make it one.
func TestAddonHostNeverShowsAListenStartedPrivate(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	if _, err := ce.call(t, "private", "on"); err != nil {
		t.Fatal(err)
	}
	topic, cancel, err := ce.srv.addon.Watch(cli.TopicNowPlaying)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	nowPlaying(t, topic)
	playLibrary(t, ce, "i.100")
	quiet(t, topic)
	if _, err := ce.call(t, "private", "off"); err != nil {
		t.Fatal(err)
	}
	quiet(t, topic)
	if res, _ := addonCall(t, ce, "status"); res.(cli.Status).Track != nil {
		t.Errorf("status = %+v", res)
	}
	queue, cancelQueue, err := ce.srv.addon.Watch(cli.TopicQueue)
	if err != nil {
		t.Fatal(err)
	}
	defer cancelQueue()
	if q := (<-queue).(cli.Queue); len(q.Tracks) != 0 {
		t.Errorf("queue event = %+v", q)
	}
}

// A 30-second preview is hidden like a private listen, and a listen that
// has been a preview stays one, whatever a later snapshot says.
func TestAddonHostHidesPreviews(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	h := ce.srv.addon
	cur, next := &coordinator.Entry{UID: 7}, &coordinator.Entry{UID: 8}
	h.mu.Lock()
	_, early := h.listenLocked(coordinator.Snapshot{Current: cur})
	_, hidden := h.listenLocked(coordinator.Snapshot{Current: cur, Preview: true})
	_, still := h.listenLocked(coordinator.Snapshot{Current: cur})
	_, other := h.listenLocked(coordinator.Snapshot{Current: next})
	slots := maps.Clone(h.hiddenUIDs)
	h.mu.Unlock()
	if early || !hidden || !still || other {
		t.Errorf("hidden: before the report %v, preview %v, flag gone again %v, next listen %v", early, hidden, still, other)
	}
	if !slots[7] || slots[8] {
		t.Errorf("hidden slots = %v", slots)
	}
}

func drainStatus(ch <-chan any, d time.Duration) []cli.Status {
	var out []cli.Status
	for timer := time.After(d); ; {
		select {
		case v := <-ch:
			out = append(out, v.(cli.Status))
		case <-timer:
			return out
		}
	}
}

const appleURLError = `Post "https://play.itunes.apple.com/WebObjects/MZPlay.woa/wa/webPlayback?song=1440841363": dial tcp 17.253.1.1:443: connect: connection refused`

// The text of a playback failure holds Apple URLs and song IDs; an add-on
// gets none of it, in status events or in any command answer.
func TestAddonHostNeverShowsErrorText(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	st, cancel, err := ce.srv.addon.Watch(cli.TopicStatus)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	ce.eng.playErr = errors.New(appleURLError)
	_ = ce.play.PlayItems(t.Context(), []TrackRef{{ID: "i.100", Source: SourceLibrary}}, 0, PlayFrom{})
	if got := ce.srv.status(ce.e.Coord.Snapshot()).Error; !strings.Contains(got, "apple.com") {
		t.Fatalf("the player's own status error = %q: the test no longer produces one", got)
	}
	for _, s := range drainStatus(st, 300*time.Millisecond) {
		if s.Error != "" {
			t.Errorf("status event carries %q", s.Error)
		}
	}
	for _, cmd := range []string{"status", "play", "pause", "toggle", "next", "prev"} {
		res, err := addonCall(t, ce, cmd)
		if err != nil {
			continue // a refusal carries a code, covered by the session tests
		}
		if b, _ := json.Marshal(res); strings.Contains(string(b), "apple.com") || strings.Contains(string(b), "1440841363") {
			t.Errorf("%s answers %s", cmd, b)
		}
		if s, ok := res.(cli.Status); ok && s.Error != "" {
			t.Errorf("%s answers status error %q", cmd, s.Error)
		}
	}
}

// A hidden listen's failure doesn't show up in the next visible listen.
func TestAddonHostHiddenErrorDoesNotReachTheNextListen(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	if _, err := ce.call(t, "private", "on"); err != nil {
		t.Fatal(err)
	}
	ce.eng.playErr = errors.New(appleURLError)
	_ = ce.play.PlayItems(t.Context(), []TrackRef{{ID: "i.100", Source: SourceLibrary}}, 0, PlayFrom{})
	if got := ce.srv.status(ce.e.Coord.Snapshot()).Error; !strings.Contains(got, "apple.com") {
		t.Fatalf("the player's own status error = %q: the test no longer produces one", got)
	}
	if _, err := ce.call(t, "private", "off"); err != nil {
		t.Fatal(err)
	}
	st, cancel, err := ce.srv.addon.Watch(cli.TopicStatus)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	ce.eng.mu.Lock()
	ce.eng.playErr = nil
	ce.eng.mu.Unlock()
	playLibrary(t, ce, "i.101")
	res, err := addonCall(t, ce, "status")
	if err != nil {
		t.Fatal(err)
	}
	if s := res.(cli.Status); s.Track == nil || s.Error != "" {
		t.Errorf("status of the next listen = %+v", s)
	}
	for _, s := range drainStatus(st, 300*time.Millisecond) {
		if s.Error != "" {
			t.Errorf("status event carries %q", s.Error)
		}
	}
}

// Private mode switched on while lyrics or like run: the answer, judged
// again afterwards, is not given.
func TestAddonHostJudgesAgainAfterTheCall(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	ce.lyrics.by = map[string]Lyrics{"100": {Provider: "apple", Timing: "Line", Lines: []LyricLine{{BeginMs: 1, EndMs: 2, Text: "la"}}}}
	playLibrary(t, ce, "i.100")
	if res, err := addonCall(t, ce, "lyrics"); err != nil {
		t.Fatalf("lyrics of a public listen: %v (%+v)", err, res)
	}
	ce.lyrics.mu.Lock()
	ce.lyrics.during = func() {
		if _, err := ce.call(t, "private", "on"); err != nil {
			t.Error(err)
		}
	}
	ce.lyrics.mu.Unlock()
	if res, err := addonCall(t, ce, "lyrics"); code(err) != cli.CodeNothingPlaying || res != nil {
		t.Errorf("lyrics answered %+v, %v after private mode went on", res, err)
	}
	if _, err := addonCall(t, ce, "like"); code(err) != cli.CodeNothingPlaying {
		t.Errorf("like during private mode: %v", err)
	}
}

// The same slot is required too: the player moving to another listen
// during the call is not answered with that listen.
func TestAddonHostJudgesTheSameSlotAfterTheCall(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	ce.lyrics.by = map[string]Lyrics{"100": {Provider: "apple", Timing: "Line", Lines: []LyricLine{{Text: "la"}}}, "101": {Provider: "apple", Timing: "Line", Lines: []LyricLine{{Text: "lo"}}}}
	playLibrary(t, ce, "i.100")
	ce.lyrics.mu.Lock()
	ce.lyrics.during = func() { playLibrary(t, ce, "i.101") }
	ce.lyrics.mu.Unlock()
	if res, err := addonCall(t, ce, "lyrics"); code(err) != cli.CodeNothingPlaying || res != nil {
		t.Errorf("lyrics answered %+v, %v after the song changed", res, err)
	}
}

// Private on for P1 and P2, off at P3: the queue an add-on sees lists P3
// alone, at index 0, in the call and in the event.
func TestAddonHostQueueDropsHiddenSlots(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	if _, err := ce.call(t, "private", "on"); err != nil {
		t.Fatal(err)
	}
	refs := []TrackRef{{ID: "i.100", Source: SourceLibrary}, {ID: "i.100", Source: SourceLibrary}, {ID: "i.101", Source: SourceLibrary}, {ID: "i.100", Source: SourceLibrary}}
	if err := ce.play.PlayItems(t.Context(), refs, 0, PlayFrom{}); err != nil { // P1
		t.Fatal(err)
	}
	_, _ = addonCall(t, ce, "status")
	if _, err := ce.call(t, "next"); err != nil { // P2
		t.Fatal(err)
	}
	_, _ = addonCall(t, ce, "status")
	if _, err := ce.call(t, "private", "off"); err != nil {
		t.Fatal(err)
	}
	if _, err := ce.call(t, "next"); err != nil { // P3
		t.Fatal(err)
	}
	res, err := addonCall(t, ce, "queue")
	if err != nil {
		t.Fatal(err)
	}
	q := res.(cli.Queue)
	if q.Index != 0 || len(q.Tracks) != 2 || q.Tracks[0].Title != "Something" || q.Tracks[1].Title != "Come Together" {
		t.Errorf("queue = %+v, want P3 at index 0 and P4", q)
	}
	topic, cancel, err := ce.srv.addon.Watch(cli.TopicQueue)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	ev := (<-topic).(cli.Queue)
	for ev.Index != 0 || len(ev.Tracks) != 2 {
		select {
		case v := <-topic:
			ev = v.(cli.Queue)
		case <-time.After(2 * time.Second):
			t.Fatalf("queue event = %+v", ev)
		}
	}
	if ev.Index != 0 || ev.Tracks[0].Title != "Something" {
		t.Errorf("queue event = %+v", ev)
	}
}

// open through an add-on parses the link as the command line does.
func TestAddonHostOpenParsesTheLink(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	if _, err := addonCall(t, ce, "open", "https://example.com/album/1"); code(err) != cli.CodeBadLink {
		t.Errorf("foreign link: %v", err)
	}
	if ce.focused != 0 {
		t.Errorf("a foreign link raised the window")
	}
	res, err := addonCall(t, ce, "open", "https://music.apple.com/gb/album/abbey-road/1441164426?i=1441164430")
	if err != nil || res.(cli.Opened) != (cli.Opened{Kind: "song", ID: "1441164430", Storefront: "gb"}) {
		t.Fatalf("open = %+v, %v", res, err)
	}
}
