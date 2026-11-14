package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/privatemode"
)

// fakeCatalogSongs is Apple's catalog song endpoints for the songs "s1"…:
// every ID is known but unknown (not served in the storefront) and future
// (releases in 2999). It counts the requests that reach it and holds each
// for latency, as a real round trip does.
type fakeCatalogSongs struct {
	latency       time.Duration
	unknown       []string
	future        []string
	holdUntilDone string // a batch naming this ID hangs until its request ends

	mu      sync.Mutex
	batches [][]string // the IDs of each batched request
	singles []string   // the IDs asked for one at a time
	entered chan struct{}
	dropped chan struct{} // closed when a held request was cancelled
}

func newFakeCatalogSongs(latency time.Duration) *fakeCatalogSongs {
	return &fakeCatalogSongs{latency: latency, entered: make(chan struct{}, 1), dropped: make(chan struct{})}
}

func (f *fakeCatalogSongs) requests() (batches [][]string, singles []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.batches), slices.Clone(f.singles)
}

func (f *fakeCatalogSongs) song(id string) string {
	date := "2020-01-01"
	if slices.Contains(f.future, id) {
		date = "2999-01-01"
	}
	return fmt.Sprintf(`{"id":%[1]q,"type":"songs","attributes":{"name":"Song %[1]s","artistName":"Artist","albumName":"Album",
		"durationInMillis":180000,"releaseDate":%[2]q,"playParams":{"id":%[1]q,"kind":"song"}},
		"relationships":{"albums":{"data":[{"id":"alb.%[1]s","type":"albums"}]},"artists":{"data":[{"id":"art.%[1]s","type":"artists"}]}}}`, id, date)
}

func (f *fakeCatalogSongs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if id, ok := strings.CutPrefix(r.URL.Path, "/v1/catalog/gb/songs/"); ok {
		f.mu.Lock()
		f.singles = append(f.singles, id)
		f.mu.Unlock()
		time.Sleep(f.latency)
		if slices.Contains(f.unknown, id) {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"data":[%s]}`, f.song(id))
		return
	}
	ids := strings.Split(r.URL.Query().Get("ids"), ",")
	f.mu.Lock()
	f.batches = append(f.batches, ids)
	f.mu.Unlock()
	if f.holdUntilDone != "" && slices.Contains(ids, f.holdUntilDone) {
		f.entered <- struct{}{}
		select {
		case <-r.Context().Done():
			close(f.dropped)
		case <-time.After(10 * time.Second):
		}
		return
	}
	time.Sleep(f.latency)
	var data []string
	for _, id := range ids {
		if !slices.Contains(f.unknown, id) {
			data = append(data, f.song(id))
		}
	}
	_, _ = fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(data, ","))
}

func catalogRefs(n int) []TrackRef {
	refs := make([]TrackRef, n)
	for i := range refs {
		refs[i] = TrackRef{ID: fmt.Sprintf("s%d", i+1), Source: SourceCatalog}
	}
	return refs
}

// Playing a chart (or search results, or a room) asks Apple for the songs the
// library doesn't hold yet. That is one batched request, not one per song:
// 50 songs cost 8.8 s of sequential round trips (175 ms each, measured against
// the real API) before the first one could start. The queue is what it was.
func TestPlayItemsFetchesUnstoredCatalogSongsInOneRequest(t *testing.T) {
	const latency = 40 * time.Millisecond
	ce := newCommandEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	ce.signIn(t)
	fake := newFakeCatalogSongs(latency)
	fake.unknown, fake.future = []string{"s7"}, []string{"s9"}
	ce.songs = fake.ServeHTTP
	refs := catalogRefs(60)

	t0 := time.Now()
	if err := ce.play.PlayItems(t.Context(), refs, 12, PlayFrom{ID: "Top songs"}); err != nil {
		t.Fatal(err)
	}
	took := time.Since(t0)
	t.Logf("PlayItems of %d unstored songs took %v (%v per request)", len(refs), took, latency)

	batches, singles := fake.requests()
	if len(batches) != 1 || len(batches[0]) != 60 || len(singles) != 0 {
		t.Fatalf("requests: %d batched (%v), %d single %v; want one batch of 60 ids", len(batches), batches, len(singles), singles)
	}
	// 60 requests in a row would take 2.4 s; the bound is a margin for slow CI,
	// far under that.
	if limit := 15 * latency; took > limit {
		t.Errorf("PlayItems took %v, want about one round trip (%v), under %v", took, latency, limit)
	}
	if got := ce.eng.played(); !slices.Equal(got, []string{"s13"}) {
		t.Fatalf("played %v, want the chosen song s13", got)
	}
	q := ce.play.Queue()
	var want []string
	for _, r := range refs {
		if r.ID != "s7" && r.ID != "s9" {
			want = append(want, r.ID)
		}
	}
	var got []string
	for _, it := range q.Items {
		got = append(got, it.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("queue %v, want %v (unknown and unreleased songs dropped, order kept)", got, want)
	}
	if cur := q.Items[q.Index]; cur.ID != "s13" || cur.Name != "Song s13" || cur.AlbumID != "alb.s13" || cur.ArtistID != "art.s13" || cur.Source != SourceCatalog {
		t.Fatalf("current item %+v", cur)
	}

	// Fetching stored the songs: playing the chart again asks only for the one
	// song Apple doesn't serve (nothing is stored for it).
	if err := ce.play.PlayItems(t.Context(), refs, 0, PlayFrom{ID: "Top songs"}); err != nil {
		t.Fatal(err)
	}
	if batches, singles = fake.requests(); len(batches) != 2 || !slices.Equal(batches[1], []string{"s7"}) || len(singles) != 0 {
		t.Fatalf("second play asked Apple for %v and %v, want only s7", batches[1:], singles)
	}
}

// The chosen song itself missing from the catalog fails the play (as it did
// song by song), and a lookup that fails fails it too.
func TestPlayItemsFailsWhenTheChosenSongIsNotServed(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	ce.signIn(t)
	fake := newFakeCatalogSongs(0)
	fake.unknown, fake.future = []string{"s2"}, []string{"s3"}
	ce.songs = fake.ServeHTTP
	refs := catalogRefs(4)

	if err := ce.play.PlayItems(t.Context(), refs, 1, PlayFrom{}); !errors.Is(err, applemusic.ErrNotFound) {
		t.Fatalf("unknown start song: %v, want ErrNotFound", err)
	}
	if err := ce.play.PlayItems(t.Context(), refs, 2, PlayFrom{}); !errors.Is(err, ErrNotReleased) {
		t.Fatalf("unreleased start song: %v, want ErrNotReleased", err)
	}
	if got := ce.eng.played(); len(got) != 0 {
		t.Fatalf("played %v", got)
	}

	ce.songs = func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "down", http.StatusBadRequest) }
	if err := ce.play.PlayItems(t.Context(), catalogRefs(3), 0, PlayFrom{}); err == nil {
		t.Fatal("PlayItems succeeded with the catalog failing")
	}
}

// A play that is still looking its songs up when the user plays something else
// is dropped at once, its request cancelled: it neither fails nor replaces the
// queue later. (Eight clicks on a chart used to play the last of them to
// finish, after twelve seconds, not the last chosen.)
func TestPlayItemsSupersededWhileResolvingIsDropped(t *testing.T) {
	ce := newCommandEnv(t, privatemode.Options{Override: privatemode.OverrideOn})
	ce.signIn(t)
	fake := newFakeCatalogSongs(0)
	fake.holdUntilDone = "s1"
	ce.songs = fake.ServeHTTP

	first := make(chan error, 1)
	go func() { first <- ce.play.PlayItems(context.Background(), catalogRefs(3), 0, PlayFrom{}) }()
	select {
	case <-fake.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the first play never asked Apple")
	}

	if err := ce.play.PlayItems(t.Context(), []TrackRef{{ID: "s50", Source: SourceCatalog}}, 0, PlayFrom{}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-first:
		if err != nil {
			t.Fatalf("superseded play = %v, want no failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the superseded play kept waiting for Apple")
	}
	select {
	case <-fake.dropped:
	case <-time.After(5 * time.Second):
		t.Fatal("the superseded play's request was not cancelled")
	}
	if got := ce.eng.played(); !slices.Equal(got, []string{"s50"}) {
		t.Fatalf("played %v, want only the last chosen song", got)
	}
	if q := ce.play.Queue(); len(q.Items) != 1 || q.Items[0].ID != "s50" {
		t.Fatalf("queue %+v", q.Items)
	}
}
