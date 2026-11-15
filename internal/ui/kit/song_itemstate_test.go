package kit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/coredev-uk/sona-go/internal/services"
	"github.com/coredev-uk/sona-go/internal/ui/store"
)

type fakeStateAPI struct {
	mu      sync.Mutex
	batches []int
	calls   []string
	failID  string
	// gate holds States answers until closed; entered says one has arrived.
	gate    chan struct{}
	entered chan struct{}
}

func (f *fakeStateAPI) States(_ context.Context, refs []services.ItemRef) ([]services.ItemState, error) {
	if f.gate != nil {
		select {
		case f.entered <- struct{}{}:
		default:
		}
		<-f.gate
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.batches = append(f.batches, len(refs))
	out := make([]services.ItemState, len(refs))
	for i, r := range refs {
		out[i] = services.ItemState{Kind: r.Kind, ID: r.ID, InLibrary: true}
	}
	return out, nil
}

func (f *fakeStateAPI) asked() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.batches)
}

func (f *fakeStateAPI) do(call, id string, st services.ItemState) (services.ItemState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call+" "+id)
	if id == f.failID {
		return services.ItemState{}, errors.New("no")
	}
	return st, nil
}

func (f *fakeStateAPI) Favorite(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	st := services.ItemState{Kind: r.Kind, ID: r.ID, InLibrary: true}
	if on {
		st.Rating = 1
	}
	return f.do(fmt.Sprintf("favorite=%v", on), r.ID, st)
}

func (f *fakeStateAPI) SuggestLess(_ context.Context, r services.ItemRef, on bool) (services.ItemState, error) {
	return f.do(fmt.Sprintf("less=%v", on), r.ID, services.ItemState{Kind: r.Kind, ID: r.ID, Rating: -1})
}

func (f *fakeStateAPI) Remove(_ context.Context, r services.ItemRef) (services.ItemState, error) {
	return f.do("remove", r.ID, services.ItemState{Kind: r.Kind, ID: r.ID})
}

func (f *fakeStateAPI) Add(_ context.Context, typ string, ids []string) error {
	f.mu.Lock()
	f.calls = append(f.calls, fmt.Sprintf("add %s %v", typ, ids))
	f.mu.Unlock()
	return nil
}

func (f *fakeStateAPI) log() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.calls)
	slices.Sort(out)
	return out
}

func newStates() (*ItemStates, *fakeStateAPI, *store.Client) {
	api := &fakeStateAPI{}
	q := store.NewClient(func(fn func()) { fn() })
	return NewItemStates(q, func(fn func()) { fn() }, api), api, q
}

// The loads run in bubbles where the batch's timer is on a fake clock that
// moves only when every goroutine waits: a wall-clock window of 16 ms could
// pass between two goroutines starting on a busy machine.
func TestLoadsInTheSameFrameAreOneStatesCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, api, _ := newStates()
		var wg sync.WaitGroup
		for _, id := range []string{"1", "2", "3"} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				st, ok := s.Load(context.Background(), StateSongs, StateItem{ID: id})
				if !ok || st.ID != id {
					t.Errorf("load %s: %+v %v", id, st, ok)
				}
			}()
		}
		wg.Wait()
		if len(api.batches) != 1 || api.batches[0] != 3 {
			t.Errorf("batches %v, want one of 3", api.batches)
		}
		// A state just learnt is answered from memory.
		if _, ok := s.Load(context.Background(), StateSongs, StateItem{ID: "1"}); !ok || len(api.batches) != 1 {
			t.Errorf("asked again: %v", api.batches)
		}
		if _, ok := s.Load(context.Background(), StateSongs, StateItem{}); ok {
			t.Error("an item without an ID loaded")
		}
	})
}

func TestChangeShowsAtOnceAndRestoresOnFailure(t *testing.T) {
	s, api, q := newStates()
	api.failID = "bad"
	good, bad := StateItem{ID: "good"}, StateItem{ID: "bad", Source: services.SourceLibrary}
	st, err := s.Change(context.Background(), StateSongs, good, StateChange(RateFavorite))
	if err != nil || st.Rating != 1 {
		t.Fatalf("%+v %v", st, err)
	}
	r := store.Use(q, StateKey(StateSongs, "good"), func(context.Context) (services.ItemState, error) {
		return services.ItemState{}, errors.New("not cached")
	})
	if !r.HasData || r.Data.Rating != 1 {
		t.Errorf("cache %+v", r)
	}
	// A failure puts the item's own state back: a library item stays in the library.
	if _, err := s.Change(context.Background(), StateSongs, bad, ChangeRemove); err == nil {
		t.Fatal("no error")
	}
	got, _ := s.base(StateSongs, bad)
	if !got.InLibrary {
		t.Errorf("no state was learnt, so the base is the item's own (a library item): %+v", got)
	}
}

// A refetch in flight when a rating is written answers late and must not put
// the older state back (v0.2.0's cancelQueries before setQueryData).
func TestLateRefetchDoesNotOverwriteAnOptimisticRating(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, api, q := newStates()
		api.gate, api.entered = make(chan struct{}), make(chan struct{}, 1)
		item := StateItem{ID: "late"}
		fetch := func() store.Result[services.ItemState] { return s.Use(StateSongs, item) }
		fetch()
		select {
		case <-api.entered:
		case <-time.After(hangGuard):
			t.Fatal("the refetch never started")
		}
		go func() {
			if _, err := s.Change(context.Background(), StateSongs, item, StateChange(RateFavorite)); err != nil {
				t.Error(err)
			}
		}()
		synctest.Wait() // the change has written its rating
		r := store.Use(q, StateKey(StateSongs, "late"), func(context.Context) (services.ItemState, error) {
			return services.ItemState{}, errors.New("not cached")
		})
		if !r.HasData || r.Data.Rating != 1 {
			t.Fatalf("the rating was not written: %+v", r)
		}
		close(api.gate)
		synctest.Wait() // the late answer has come in, and been dropped
		if r := fetch(); !r.HasData || r.Data.Rating != 1 {
			t.Errorf("the late answer overwrote the rating: %+v", r)
		}
	})
}

func TestBulkRateAddRemove(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, api, _ := newStates()
		items := []KindedItem{{StateSongs, StateItem{ID: "1", CatalogID: "c1"}}, {StateAlbums, StateItem{ID: "2", CatalogID: "c2"}}, {StateSongs, StateItem{ID: "3"}}}
		s.RateMany(items, RateFavorite, "3 items")
		synctest.Wait() // the calls the goroutines make have been made
		if got := api.log(); !slices.Equal(got, []string{"favorite=true 1", "favorite=true 2", "favorite=true 3"}) {
			t.Errorf("%v", got)
		}
		s.AddMany(items, "3 items")
		synctest.Wait()
		if got := api.log(); len(got) != 5 || !slices.Contains(got, "add songs [c1 3]") || !slices.Contains(got, "add albums [c2]") {
			t.Errorf("%v", got)
		}
		// Removal asks once, for all.
		s.RemoveMany(items, "3 items")
		if !ConfirmPending() {
			t.Fatal("no question")
		}
		if len(api.log()) != 5 {
			t.Fatal("removed before the answer")
		}
		settleConfirm(true)
		synctest.Wait()
		if got := api.log(); len(got) != 8 || !slices.Contains(got, "remove 1") || !slices.Contains(got, "remove 3") {
			t.Errorf("%v", got)
		}
		// Cancelling removes nothing.
		s.RemoveMany(items, "3 items")
		settleConfirm(false)
		synctest.Wait()
		if len(api.log()) != 8 {
			t.Errorf("a cancelled removal ran: %v", api.log())
		}
	})
}

func TestALateLoadDoesNotOverwriteAChange(t *testing.T) {
	s, api, q := newStates()
	api.gate, api.entered = make(chan struct{}), make(chan struct{}, 1)
	it := StateItem{ID: "1"}
	loaded := make(chan services.ItemState, 1)
	go func() {
		st, _ := s.Load(context.Background(), StateSongs, it)
		loaded <- st
	}()
	select {
	case <-api.entered:
	case <-time.After(hangGuard):
		t.Fatal("the load never reached Go")
	}
	// The user favourites while Apple's answer (rating 0) is on its way.
	if _, err := s.Change(context.Background(), StateSongs, it, StateChange(RateFavorite)); err != nil {
		t.Fatal(err)
	}
	close(api.gate)
	if st := <-loaded; st.Rating != 1 {
		t.Errorf("Load answered %+v, want the change's rating 1", st)
	}
	r := store.Use(q, StateKey(StateSongs, "1"), func(context.Context) (services.ItemState, error) {
		return services.ItemState{}, errors.New("not cached")
	})
	if !r.HasData || r.Data.Rating != 1 {
		t.Errorf("the cache holds %+v after the late answer", r)
	}
	if st, ok := s.known(StateSongs, "1"); !ok || st.Rating != 1 {
		t.Errorf("known %+v %v", st, ok)
	}
}

func TestAStateIsAskedAgainAfterItsStaleTimeEvenOnScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, api, _ := newStates()
		now := time.Now()
		s.now = func() time.Time { return now }
		it := StateItem{ID: "1"}
		use := func() store.Result[services.ItemState] { return s.Use(StateSongs, it) }
		// idle runs the bubble's clock past the batch's timer and lets every
		// goroutine that started finish: what a frame was going to ask has been
		// asked by now.
		idle := func() {
			time.Sleep(time.Second)
			synctest.Wait()
		}
		use()
		idle()
		if !use().HasData {
			t.Fatal("no first answer")
		}
		if api.asked() != 1 {
			t.Fatalf("asked %d times", api.asked())
		}
		// Frames inside the stale time ask nothing, however many.
		now = now.Add(stateStale - time.Second)
		for range 5 {
			use()
		}
		idle()
		if api.asked() != 1 {
			t.Fatalf("asked again inside the stale time: %d", api.asked())
		}
		// Past it, the next frame asks once; the per-frame call does not keep
		// the state young.
		now = now.Add(2 * time.Second)
		use()
		idle()
		if api.asked() != 2 {
			t.Fatalf("asked %d times after the stale time, want 2", api.asked())
		}
		for range 5 {
			use()
		}
		idle()
		if api.asked() != 2 {
			t.Errorf("asked %d times, want 2", api.asked())
		}
	})
}

// toastTitles drains the queued toasts' titles.
func toastTitles() []string {
	toasts.mu.Lock()
	defer toasts.mu.Unlock()
	var out []string
	for _, t := range toasts.queue {
		out = append(out, t.Title)
	}
	toasts.queue = nil
	return out
}

func TestAddToastsUseTheSingleItemWords(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _, _ := newStates()
		toastTitles()
		s.Add(StateSongs, StateItem{ID: "1", CatalogID: "c1"}, "Airbag")
		synctest.Wait() // the goroutine that adds has raised its toast
		if got := toastTitles(); !slices.Equal(got, []string{"Added “Airbag” to your library"}) {
			t.Errorf("toasts %q", got)
		}
		s.AddMany([]KindedItem{{StateSongs, StateItem{ID: "1"}}}, "3 songs")
		synctest.Wait()
		if got := toastTitles(); !slices.Equal(got, []string{"Added 3 songs to your library"}) {
			t.Errorf("toasts %q", got)
		}
	})
}

// A state Go announces (`sona like`) shows at once and outdates a load begun
// before it.
func TestRememberShowsAGoSideChange(t *testing.T) {
	s, _, q := newStates()
	s.Remember(services.ItemState{Kind: StateSongs, ID: "x", InLibrary: true, Rating: 1})
	r := store.Use(q, StateKey(StateSongs, "x"), func(context.Context) (services.ItemState, error) {
		return services.ItemState{}, errors.New("not cached")
	})
	if !r.HasData || r.Data.Rating != 1 || !r.Data.InLibrary {
		t.Fatalf("cache %+v", r)
	}
	if got, ok := s.known(StateSongs, "x"); !ok || got.Rating != 1 {
		t.Errorf("not remembered: %+v %v", got, ok)
	}
	s.Remember(services.ItemState{}) // no ID: nothing to write
}

// Follow takes the event from the bus.
func TestFollowRemembersTheItemStateEvent(t *testing.T) {
	s, _, q := newStates()
	bus := store.NewBus(nil)
	bus.SetPoster(func(fn func()) { fn() })
	s.Follow(bus)
	bus.Emit(services.EventItemState, services.ItemState{Kind: StateAlbums, ID: "a", Rating: -1})
	r := store.Use(q, StateKey(StateAlbums, "a"), func(context.Context) (services.ItemState, error) {
		return services.ItemState{}, errors.New("not cached")
	})
	if !r.HasData || r.Data.Rating != -1 {
		t.Errorf("cache %+v", r)
	}
}

// A state Go announced (`sona like`) changes the favourite songs list too.
func TestRememberRefreshesTheFavouriteSongs(t *testing.T) {
	s, _, q := newStates()
	key := store.Key{"library", "favorite-songs"}
	store.Set(q, key, 1)
	s.Remember(services.ItemState{Kind: StateSongs, ID: "x", Rating: 1})
	fetched := make(chan struct{}, 1)
	store.Use(q, key, func(context.Context) (int, error) {
		fetched <- struct{}{}
		return 2, nil
	}, store.Options{Stale: store.Forever})
	select {
	case <-fetched:
	case <-time.After(hangGuard):
		t.Error("the favourite songs were not refetched")
	}
}
