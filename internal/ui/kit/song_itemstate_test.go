package kit

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
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

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(2 * time.Millisecond) {
		if ok() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestLoadsInTheSameFrameAreOneStatesCall(t *testing.T) {
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
	s, api, q := newStates()
	api.gate, api.entered = make(chan struct{}), make(chan struct{}, 1)
	item := StateItem{ID: "late"}
	fetch := func() store.Result[services.ItemState] { return s.Use(StateSongs, item) }
	fetch()
	select {
	case <-api.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the refetch never started")
	}
	go func() {
		if _, err := s.Change(context.Background(), StateSongs, item, StateChange(RateFavorite)); err != nil {
			t.Error(err)
		}
	}()
	waitFor(t, "the rating", func() bool {
		r := store.Use(q, StateKey(StateSongs, "late"), func(context.Context) (services.ItemState, error) {
			return services.ItemState{}, errors.New("not cached")
		})
		return r.HasData && r.Data.Rating == 1
	})
	close(api.gate)
	time.Sleep(100 * time.Millisecond)
	if r := fetch(); !r.HasData || r.Data.Rating != 1 {
		t.Errorf("the late answer overwrote the rating: %+v", r)
	}
}

func TestBulkRateAddRemove(t *testing.T) {
	s, api, _ := newStates()
	items := []KindedItem{{StateSongs, StateItem{ID: "1", CatalogID: "c1"}}, {StateAlbums, StateItem{ID: "2", CatalogID: "c2"}}, {StateSongs, StateItem{ID: "3"}}}
	s.RateMany(items, RateFavorite, "3 items")
	waitFor(t, "the ratings", func() bool { return len(api.log()) == 3 })
	if got := api.log(); !slices.Equal(got, []string{"favorite=true 1", "favorite=true 2", "favorite=true 3"}) {
		t.Errorf("%v", got)
	}
	s.AddMany(items, "3 items")
	waitFor(t, "the adds", func() bool { return len(api.log()) == 5 })
	if got := api.log(); !slices.Contains(got, "add songs [c1 3]") || !slices.Contains(got, "add albums [c2]") {
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
	waitFor(t, "the removals", func() bool { return len(api.log()) == 8 })
	if got := api.log(); !slices.Contains(got, "remove 1") || !slices.Contains(got, "remove 3") {
		t.Errorf("%v", got)
	}
	// Cancelling removes nothing.
	s.RemoveMany(items, "3 items")
	settleConfirm(false)
	time.Sleep(20 * time.Millisecond)
	if len(api.log()) != 8 {
		t.Errorf("a cancelled removal ran: %v", api.log())
	}
}
