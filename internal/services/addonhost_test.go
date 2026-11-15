package services

import (
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
	nowPlaying(t, topic) // idle
	playLibrary(t, ce, "i.100")
	awaitStatus(t, topic, func(np cli.NowPlaying) bool { return np.Track != nil })

	if _, err := ce.call(t, "private", "on"); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, topic, func(np cli.NowPlaying) bool { return np.Track == nil }) // the listen ends for the add-on
	if _, err := ce.call(t, "private", "off"); err != nil {
		t.Fatal(err)
	}
	quiet(t, topic) // off again: still the private listen, still no event

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

// A 30-second preview is hidden like a private listen.
func TestAddonHostHidesPreviews(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Save: func(bool) error { return nil }})
	h := ce.srv.addon
	cur := &coordinator.Entry{UID: 7}
	h.mu.Lock()
	_, hidden := h.listenLocked(coordinator.Snapshot{Current: cur, Preview: true})
	_, shown := h.listenLocked(coordinator.Snapshot{Current: cur})
	h.mu.Unlock()
	if !hidden || shown {
		t.Errorf("preview hidden = %v, the same listen not a preview hidden = %v", hidden, shown)
	}
}
