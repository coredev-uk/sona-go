package coordinator

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

type activityLog struct {
	mu  sync.Mutex
	got []string
}

func (l *activityLog) add(a Activity) {
	l.mu.Lock()
	defer l.mu.Unlock()
	switch a.Kind {
	case ActivityStart:
		l.got = append(l.got, fmt.Sprintf("start %s %s", a.Item.ID, a.Position))
	case ActivityEnd:
		l.got = append(l.got, fmt.Sprintf("end %s %s %s", a.Item.ID, a.Position, a.Reason))
	case ActivitySeek:
		l.got = append(l.got, fmt.Sprintf("seek %s %s→%s", a.Item.ID, a.From, a.Position))
	}
}

func (l *activityLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.got
	l.got = nil
	return out
}

func expect(t *testing.T, l *activityLog, want ...string) {
	t.Helper()
	synctest.Wait()
	if got := l.take(); !slices.Equal(got, want) {
		t.Fatalf("activity\n got %q\nwant %q", got, want)
	}
}

func TestActivity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := &activityLog{}
		h := newHarness(t, func(o *Options) { o.OnActivity = log.add })
		ctx := t.Context()

		must(t, h.c.SetQueue(ctx, items("a", "b", "c"), 0, ""))
		expect(t, log, "start a 0s")

		time.Sleep(10 * time.Second)
		must(t, h.c.Pause(ctx))
		expect(t, log, "end a 10s paused")
		must(t, h.c.Pause(ctx)) // already paused: nothing more
		must(t, h.c.Resume(ctx))
		expect(t, log, "start a 10s")

		time.Sleep(5 * time.Second)
		must(t, h.c.Seek(ctx, 50*time.Second))
		expect(t, log, "seek a 15s→50s") // a seek keeps the listen open

		must(t, h.c.Next(ctx))
		expect(t, log, "end a 50s skip-forward", "start b 0s")

		h.eng.end()
		expect(t, log, "end b 1m40s natural", "start c 0s")

		time.Sleep(time.Second)
		must(t, h.c.Previous(ctx))
		expect(t, log, "end c 1s skip-backward", "start b 0s")

		time.Sleep(2 * time.Second)
		h.eng.fail("decoder died")
		expect(t, log, "end b 2s failed", "start c 0s")

		must(t, h.c.Jump(ctx, 0))
		expect(t, log, "end c 0s selected", "start a 0s")

		time.Sleep(4 * time.Second)
		must(t, h.c.Previous(ctx)) // past the threshold: restart = seek to 0
		expect(t, log, "seek a 4s→0s")

		must(t, h.c.Stop(ctx))
		expect(t, log, "end a 0s stopped")

		must(t, h.c.SetQueue(ctx, items("d"), 0, ""))
		time.Sleep(3 * time.Second)
		must(t, h.c.Close())
		expect(t, log, "start d 0s", "end d 3s exited")
	})
}
