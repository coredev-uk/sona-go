package library

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
)

// historyStub answers History from a function and notes the days asked for.
type historyStub struct {
	libraryAPI
	mu     sync.Mutex
	asked  []int
	answer func(days, call int) (services.ListeningHistory, error)
}

func (s *historyStub) History(ctx context.Context, days int) (services.ListeningHistory, error) {
	s.mu.Lock()
	s.asked = append(s.asked, days)
	call := len(s.asked)
	s.mu.Unlock()
	return s.answer(days, call)
}

func (s *historyStub) calls() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.asked)
}

// historyPlayRec notes the plays the page started.
type historyPlayRec struct {
	playbackAPI
	mu    sync.Mutex
	refs  [][]services.TrackRef
	start []int
	from  []services.PlayFrom
}

func (p *historyPlayRec) PlayItems(_ context.Context, refs []services.TrackRef, start int, from services.PlayFrom) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refs, p.start, p.from = append(p.refs, refs), append(p.start, start), append(p.from, from)
	return nil
}

func (p *historyPlayRec) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.refs)
}

func historySong(id, name string) services.Song {
	return services.Song{ID: id, CatalogID: id, Source: services.SourceCatalog, Name: name, Artist: "Art", Album: "Alb", Playable: true}
}

func historyEntry(at int64, s services.Song) services.HistoryEntry {
	return services.HistoryEntry{PlayedAt: at, Song: s, PlayedMs: 1}
}

func localMidnight(daysAgo int) time.Time {
	n := time.Now()
	return time.Date(n.Year(), n.Month(), n.Day()-daysAgo, 0, 0, 0, 0, time.Local)
}

// historyAccount answers the account state a test chose.
type historyAccount string

func (a historyAccount) Status() services.AuthStatus { return services.AuthStatus{State: string(a)} }

// historyRig is the History page for a signed-in account.
func historyRig(t *testing.T, answer func(days, call int) (services.ListeningHistory, error)) (*pageRig, *historyStub, *historyPlayRec) {
	t.Helper()
	return historyRigAuth(t, answer, "signed_in")
}

func historyRigAuth(t *testing.T, answer func(days, call int) (services.ListeningHistory, error), state string) (*pageRig, *historyStub, *historyPlayRec) {
	t.Helper()
	stub := &historyStub{answer: answer}
	rec := &historyPlayRec{}
	r := newPageRig(t, historyPath, 1132, 1000, func(l *library) {
		stub.libraryAPI = l.lib
		l.lib = stub
		rec.playbackAPI = l.play
		l.play = rec
		if state != "" {
			fakeAuth(l, historyAccount(state))
		}
	})
	return r, stub, rec
}

// historySignedOut is historyRig for an account that is not signed in: the
// uitest harness has no account, so the signed-in tests say it is.
func historySignedOut(t *testing.T, answer func(days, call int) (services.ListeningHistory, error)) (*pageRig, *historyStub, *historyPlayRec) {
	t.Helper()
	return historyRigAuth(t, answer, "")
}

func fixed(h services.ListeningHistory) func(int, int) (services.ListeningHistory, error) {
	return func(int, int) (services.ListeningHistory, error) { return h, nil }
}

func twoDays() services.ListeningHistory {
	today, yesterday := localMidnight(0).UnixMilli(), localMidnight(1).UnixMilli()
	a, b := historySong("1", "Alpha"), historySong("2", "Beta")
	return services.ListeningHistory{Apple: true, Since: yesterday, Days: []services.HistoryDay{
		{Day: today, Entries: []services.HistoryEntry{historyEntry(today+5000, a), {Song: b}, historyEntry(today+1000, a)}},
		{Day: yesterday, Entries: []services.HistoryEntry{historyEntry(yesterday+1, b)}},
	}}
}

func TestHistoryListsEachDaysSongsAndPlaysFromTheChosenOne(t *testing.T) {
	r, stub, rec := historyRig(t, fixed(twoDays()))
	r.tt.Until("the days", r.tt.Shows("History", "Today", "Yesterday", "Alpha", "Beta",
		"2 songs, 3 plays", "1 from other devices", "1 song", "On this computer and your other devices"))
	if got := stub.calls(); !slices.Equal(got, []int{30}) {
		t.Errorf("asked for days %v, want [30]", got)
	}
	if _, ok := r.tt.Find("Played Today"); !ok {
		t.Error("the day's list is not named Played Today")
	}
	// Alpha's repeat folded: Alpha shows once in today's list.
	n := 0
	for _, s := range r.tt.Texts() {
		if s == "Alpha" {
			n++
		}
	}
	// A row names its song twice (its play button and its title).
	if n != 2 {
		t.Errorf("Alpha shows %d times, want one row (2 texts)", n)
	}
	rect, _ := r.tt.Find("Beta")
	x, y := rect.X+rect.W/2, rect.Y+rect.H/2
	r.tt.ClickAt(x, y)
	r.tt.ClickAt(x, y)
	r.tt.Until("the play", func() bool { return rec.count() > 0 })
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.refs[0]) != 2 || rec.refs[0][0].ID != "1" || rec.refs[0][1].ID != "2" || rec.start[0] != 1 {
		t.Errorf("played %v from %d, want [1 2] from 1", rec.refs[0], rec.start[0])
	}
	if rec.from[0].ID != "History" {
		t.Errorf("played from %+v", rec.from[0])
	}
}

func TestHistorySublinesSayWhereThePlaysCameFrom(t *testing.T) {
	one := func(apple bool) services.ListeningHistory {
		d := localMidnight(0).UnixMilli()
		return services.ListeningHistory{Apple: apple, Days: []services.HistoryDay{
			{Day: 0, Entries: []services.HistoryEntry{{Song: historySong("9", "Elsewhere")}}},
			{Day: d, Entries: []services.HistoryEntry{historyEntry(d+1, historySong("1", "Alpha"))}},
		}}
	}
	r, _, _ := historyRig(t, fixed(one(false)))
	r.tt.Until("offline", r.tt.Shows("On this computer only: Apple Music couldn't be reached for your other devices",
		"Recently, on your other devices", "Elsewhere"))
	if r.tt.HasText("1 from other devices") {
		t.Error("the undated group counts its songs as from other devices")
	}

	r2, _, _ := historySignedOut(t, fixed(one(false)))
	r2.tt.Until("signed out", r2.tt.Shows("On this computer"))
	if r2.tt.HasText("On this computer only: Apple Music couldn't be reached for your other devices") {
		t.Error("signed out reads as offline")
	}

	r3, _, _ := historyRig(t, fixed(one(true)))
	r3.tt.Until("merged", r3.tt.Shows("On this computer and your other devices"))
}

func TestHistoryShowEarlierDaysAsksSixtyAndKeepsTheListShowing(t *testing.T) {
	release := make(chan struct{})
	r, stub, _ := historyRig(t, func(days, _ int) (services.ListeningHistory, error) {
		h := twoDays()
		h.More = days == 30
		if days == 60 {
			<-release
			h.Days = append(h.Days, services.HistoryDay{
				Day:     localMidnight(40).UnixMilli(),
				Entries: []services.HistoryEntry{historyEntry(1, historySong("3", "Gamma"))},
			})
		}
		return h, nil
	})
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	r.tt.Until("the first days", r.tt.Shows("Alpha", "Show earlier days"))
	if r.tt.HasText("Plays on this computer are kept for a year.") {
		t.Error("the retention note shows while there are earlier days")
	}
	if err := r.tt.Click("Show earlier days"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("loading", r.tt.Shows("Loading…"))
	if !r.tt.HasText("Alpha") {
		t.Error("the list vanished while the earlier days load")
	}
	// The fetch runs on a goroutine: the question may land after the frame.
	r.tt.Until("the ask for sixty days", func() bool { return slices.Equal(stub.calls(), []int{30, 60}) })
	close(release)
	r.tt.Until("the earlier day", r.tt.Shows("Gamma", "Plays on this computer are kept for a year."))
	if r.tt.HasText("Loading…") {
		t.Error("still loading")
	}
}

func TestHistoryLoadingShowsABusySectionOfSkeletonRows(t *testing.T) {
	release := make(chan struct{})
	r, _, _ := historyRig(t, func(int, int) (services.ListeningHistory, error) {
		<-release
		return twoDays(), nil
	})
	t.Cleanup(func() { close(release) })
	r.tt.Until("the skeleton", func() bool {
		_, ok := r.tt.Find("Loading your listening history")
		return ok
	})
	if r.tt.HasText("On this computer and your other devices") {
		t.Error("a subline shows before there is data")
	}
}

func TestHistoryEmptyStates(t *testing.T) {
	empty := fixed(services.ListeningHistory{Apple: true})
	r, _, _ := historyRig(t, empty)
	r.tt.Until("nothing played", r.tt.Shows("Nothing played yet", "Songs you play here or in any Apple Music app are listed here by day."))

	var opened int
	r2, _, _ := historySignedOut(t, empty)
	r2.l.openAccount = func() { opened++ }
	r2.tt.Until("signed out", r2.tt.Shows("Nothing played on this computer yet",
		"Songs you play in Sona are listed here by day. Sign in to add what you play on your other devices.", "Sign in"))
	if err := r2.tt.Click("Sign in"); err != nil {
		t.Fatal(err)
	}
	if opened != 1 {
		t.Errorf("Sign in opened the account dialog %d times", opened)
	}
}

func TestHistoryErrorOffersTryAgain(t *testing.T) {
	r, stub, _ := historyRig(t, func(_, call int) (services.ListeningHistory, error) {
		if call == 1 {
			return services.ListeningHistory{}, errors.New("boom")
		}
		return twoDays(), nil
	})
	r.tt.Until("the error", r.tt.Shows("Couldn't read your listening history", "Try again"))
	if err := r.tt.Click("Try again"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("the days", r.tt.Shows("Alpha", "Today"))
	if got := stub.calls(); len(got) != 2 {
		t.Errorf("asked %v, want two reads", got)
	}
}

// The player invalidates ["library","recent"] when a new song starts; the
// history is under that key and is read again then (and only then: its
// queries are never stale).
func TestHistoryRefetchesWhenTheRecentKeyIsInvalidated(t *testing.T) {
	r, stub, _ := historyRig(t, func(_, call int) (services.ListeningHistory, error) {
		h := twoDays()
		if call > 1 {
			d := localMidnight(0).UnixMilli()
			h.Days[0].Entries = append(h.Days[0].Entries, historyEntry(d+9000, historySong("7", "Newcomer")))
		}
		return h, nil
	})
	r.tt.Until("the days", r.tt.Shows("Alpha"))
	if r.tt.HasText("Newcomer") {
		t.Fatal("seeded early")
	}
	for i := 0; i < 20; i++ {
		r.tt.Drain() // never stale: drawing again asks Go for nothing
	}
	if got := stub.calls(); len(got) != 1 {
		t.Fatalf("asked %v before any invalidation", got)
	}
	r.l.env.Queries.Invalidate(keyRecent)
	r.tt.Until("the new play", r.tt.Shows("Newcomer"))
	if got := stub.calls(); len(got) != 2 || got[1] != 30 {
		t.Errorf("asked %v, want a second read of 30 days", got)
	}
}

func TestHistoryArtistAndAlbumLinkWhenThosePagesExist(t *testing.T) {
	d := localMidnight(0).UnixMilli()
	song := historySong("1", "Alpha")
	song.Artist, song.ArtistID, song.Album, song.AlbumID = "Linked Artist", "ar1", "Linked Album", "al1"
	h := services.ListeningHistory{Apple: true, Days: []services.HistoryDay{{Day: d, Entries: []services.HistoryEntry{historyEntry(d+1, song)}}}}
	stub := &historyStub{answer: fixed(h)}
	r := newPageRig(t, historyPath, 1132, 1000, func(l *library) {
		stub.libraryAPI = l.lib
		l.lib = stub
		l.env.HasRoute = func(p string) bool { return p == "/artist/x" || p == "/album/x" }
	})
	r.tt.Until("the song", r.tt.Shows("Linked Artist", "Linked Album"))
	if err := r.tt.Click("Linked Artist"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/artist/ar1" {
		t.Errorf("artist link went to %q", got)
	}
	r.router.Replace(historyPath)
	r.tt.Until("the song again", r.tt.Shows("Linked Album"))
	if err := r.tt.Click("Linked Album"); err != nil {
		t.Fatal(err)
	}
	if got := r.router.Location(); got != "/album/al1" {
		t.Errorf("album link went to %q", got)
	}
}
