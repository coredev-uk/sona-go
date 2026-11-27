package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
)

// recentRig answers /v1/me/recent/played/tracks from pages (songs per
// offset; a missing offset is an empty page, a negative count a 403) and
// notes the offsets asked, in order.
type recentRig struct {
	s     *LibraryService
	mu    sync.Mutex
	asked []int
}

func newRecentRig(t *testing.T, pages map[int]int) *recentRig {
	t.Helper()
	r := &recentRig{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/v1/me/recent/played/tracks" {
			http.NotFound(w, req)
			return
		}
		off, _ := strconv.Atoi(req.URL.Query().Get("offset"))
		r.mu.Lock()
		r.asked = append(r.asked, off)
		r.mu.Unlock()
		n := pages[off]
		if n < 0 {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		var songs []string
		for i := range n {
			songs = append(songs, fmt.Sprintf(`{"id":"%d","type":"songs","attributes":{"name":"S%d"}}`, off+i, off+i))
		}
		_, _ = w.Write([]byte(`{"data":[` + strings.Join(songs, ",") + `]}`))
	}))
	t.Cleanup(srv.Close)
	api := applemusic.New(srv.Client(), staticToken{}, func() string { return "mut" })
	api.Base, api.AMPBase = srv.URL, srv.URL
	api.Sleep = func(context.Context, time.Duration) error { return nil }
	r.s = &LibraryService{o: &Env{Deps: Deps{API: api, Logger: discard()}}}
	return r
}

func (r *recentRig) offsets() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.asked)
}

func ids(songs []applemusic.Song) []string {
	out := make([]string, len(songs))
	for i, s := range songs {
		out[i] = s.ID
	}
	return out
}

// Apple's list is read a page at a time up to a short page, which is its end.
func TestRecentlyPlayedAllStopsAtAShortPage(t *testing.T) {
	r := newRecentRig(t, map[int]int{0: 30, 30: 30, 60: 10, 90: 30})
	got, err := r.s.recentlyPlayedAll(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 70 || got[0].ID != "0" || got[69].ID != "69" {
		t.Errorf("read %d songs (%v ...), want 70 in order", len(got), ids(got[:min(3, len(got))]))
	}
	if want := []int{0, 30, 60}; !slices.Equal(r.offsets(), want) {
		t.Errorf("asked offsets %v, want %v", r.offsets(), want)
	}
}

// A full list is at most recentPages requests, and an empty page ends it.
func TestRecentlyPlayedAllReadsAtMostRecentPages(t *testing.T) {
	full := map[int]int{}
	for i := range recentPages + 2 {
		full[i*30] = 30
	}
	r := newRecentRig(t, full)
	got, err := r.s.recentlyPlayedAll(t.Context())
	if err != nil || len(got) != recentPages*30 {
		t.Fatalf("read %d songs, err %v; want %d", len(got), err, recentPages*30)
	}
	if n := len(r.offsets()); n != recentPages {
		t.Errorf("%d requests, want %d", n, recentPages)
	}

	empty := newRecentRig(t, map[int]int{})
	got, err = empty.s.recentlyPlayedAll(t.Context())
	if err != nil || len(got) != 0 || len(empty.offsets()) != 1 {
		t.Errorf("empty list: %d songs, err %v, asked %v", len(got), err, empty.offsets())
	}
}

// A page that fails does not drop the ones before it: they come back with
// the error, and nothing after the failure is asked.
func TestRecentlyPlayedAllKeepsThePagesThatArrived(t *testing.T) {
	r := newRecentRig(t, map[int]int{0: 30, 30: 30, 60: -1, 90: 30})
	got, err := r.s.recentlyPlayedAll(t.Context())
	if err == nil {
		t.Fatal("no error for the failed page")
	}
	if len(got) != 60 || got[59].ID != "59" {
		t.Errorf("kept %d songs, want the first 60", len(got))
	}
	if want := []int{0, 30, 60}; !slices.Equal(r.offsets(), want) {
		t.Errorf("asked offsets %v, want %v", r.offsets(), want)
	}

	first := newRecentRig(t, map[int]int{0: -1})
	if got, err := first.s.recentlyPlayedAll(t.Context()); err == nil || len(got) != 0 {
		t.Errorf("failed first page: %d songs, err %v", len(got), err)
	}
}
