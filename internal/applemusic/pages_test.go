package applemusic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// pagedServer serves n items {"id":"<i>"} at /v1/items with Apple's
// limit/offset/meta.total shape. Each page request sleeps a random moment so
// completions arrive out of order.
type pagedServer struct {
	n        int
	noTotal  bool
	mu       sync.Mutex
	offsets  []int
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	failAt   int // offset answering 400 (0 = never)
}

func (s *pagedServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cur := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	for {
		m := s.maxSeen.Load()
		if cur <= m || s.maxSeen.CompareAndSwap(m, cur) {
			break
		}
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit > 1 {
		s.mu.Lock()
		s.offsets = append(s.offsets, off)
		s.mu.Unlock()
		time.Sleep(time.Duration(rand.IntN(5)) * time.Millisecond)
	}
	if s.failAt > 0 && off == s.failAt {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	type item struct {
		ID string `json:"id"`
	}
	var p struct {
		Data []item `json:"data"`
		Next string `json:"next,omitempty"`
		Meta *struct {
			Total int `json:"total"`
		} `json:"meta,omitempty"`
	}
	p.Data = []item{}
	for i := off; i < min(off+limit, s.n); i++ {
		p.Data = append(p.Data, item{strconv.Itoa(i)})
	}
	if off+limit < s.n {
		p.Next = fmt.Sprintf("/v1/items?offset=%d", off+limit)
	}
	if !s.noTotal {
		p.Meta = &struct {
			Total int `json:"total"`
		}{s.n}
	}
	json.NewEncoder(w).Encode(p)
}

type idItem struct {
	ID string `json:"id"`
}

func collect(t *testing.T, c *Client, opt PageOptions) ([]string, []int, int, error) {
	t.Helper()
	var ids []string
	var offs []int
	total, err := FetchPages(context.Background(), c, "/v1/items", nil, opt, func(off int, items []idItem) error {
		offs = append(offs, off)
		for _, it := range items {
			ids = append(ids, it.ID)
		}
		return nil
	})
	return ids, offs, total, err
}

func TestFetchPagesOrderedParallel(t *testing.T) {
	srv := &pagedServer{n: 1234}
	c, _, _ := newTestClient(t, srv)
	var gotTotal int
	ids, offs, total, err := collect(t, c, PageOptions{Parallel: 4, OnTotal: func(n int) { gotTotal = n }})
	if err != nil || total != 1234 || gotTotal != 1234 {
		t.Fatalf("total=%d onTotal=%d err=%v", total, gotTotal, err)
	}
	if len(ids) != 1234 {
		t.Fatalf("got %d items", len(ids))
	}
	for i, id := range ids {
		if id != strconv.Itoa(i) {
			t.Fatalf("item %d = %s: pages delivered out of order (offsets %v)", i, id, offs)
		}
	}
	if len(srv.offsets) != 13 {
		t.Fatalf("page requests = %d, want 13", len(srv.offsets))
	}
	if m := srv.maxSeen.Load(); m > 4 {
		t.Fatalf("max concurrent requests %d > 4", m)
	}
}

func TestFetchPagesResumesFromStart(t *testing.T) {
	srv := &pagedServer{n: 450}
	c, _, _ := newTestClient(t, srv)
	ids, offs, _, err := collect(t, c, PageOptions{Start: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 250 || ids[0] != "200" || offs[0] != 200 {
		t.Fatalf("ids=%d first=%s offs=%v", len(ids), ids[0], offs)
	}
}

func TestFetchPagesErrorStopsInOrder(t *testing.T) {
	srv := &pagedServer{n: 1000, failAt: 500}
	c, _, _ := newTestClient(t, srv)
	ids, offs, _, err := collect(t, c, PageOptions{Parallel: 3})
	if err == nil {
		t.Fatal("want error")
	}
	// Everything before the failing page may be delivered, nothing after.
	if len(ids) > 500 {
		t.Fatalf("delivered %d items past the failing page (offsets %v)", len(ids), offs)
	}
	for i, off := range offs {
		if off != i*100 {
			t.Fatalf("offsets %v not contiguous", offs)
		}
	}
}

func TestFetchPagesCallbackError(t *testing.T) {
	srv := &pagedServer{n: 1000}
	c, _, _ := newTestClient(t, srv)
	stop := errors.New("stop")
	calls := 0
	_, err := FetchPages(context.Background(), c, "/v1/items", nil, PageOptions{}, func(off int, items []idItem) error {
		calls++
		if off == 200 {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || calls != 3 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
}

func TestFetchPagesWithoutTotalFollowsNext(t *testing.T) {
	srv := &pagedServer{n: 250, noTotal: true}
	c, _, _ := newTestClient(t, srv)
	ids, _, total, err := collect(t, c, PageOptions{})
	if err != nil || len(ids) != 250 || total != 250 {
		t.Fatalf("ids=%d total=%d err=%v", len(ids), total, err)
	}
}

func TestFetchPagesEmpty(t *testing.T) {
	c, _, _ := newTestClient(t, &pagedServer{n: 0})
	ids, _, total, err := collect(t, c, PageOptions{})
	if err != nil || len(ids) != 0 || total != 0 {
		t.Fatalf("ids=%v total=%d err=%v", ids, total, err)
	}
}

func TestFollowNext(t *testing.T) {
	srv := &pagedServer{n: 260}
	c, _, _ := newTestClient(t, srv)
	rel := Relationship[idItem]{Data: []idItem{{"0"}, {"1"}}, Next: "/v1/items?offset=2&limit=2"}
	if err := FollowNext(context.Background(), c, &rel); err != nil {
		t.Fatal(err)
	}
	if len(rel.Data) != 260 || rel.Next != "" || rel.Data[259].ID != "259" {
		t.Fatalf("len=%d next=%q", len(rel.Data), rel.Next)
	}
}
