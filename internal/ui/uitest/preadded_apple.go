package uitest

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/ui/uitest/fixture"
)

// PreAddedApple is a fake Apple Music library holding the album PreAdded
// stores, for a test that syncs it (Apple, and the services' sync over it).
// Before Publish it lists the album as Apple does before the release: flagged
// pre-release on its catalogue album, the songs already out (the first, with a
// date, and the single) with names and playParams, the others placeholders
// ("Track 3", the artist "unknown", no playParams, no artwork). After it the
// album is out: every song has its name, playParams, a catalogue ID and a
// cover. Everything else in the library is empty.
type PreAddedApple struct {
	release string

	mu        sync.Mutex
	published bool
	lists     map[string]int // page requests by path: how many syncs read each list
}

// NewPreAddedApple returns the fake, the album coming out on release
// ("YYYY-MM-DD").
func NewPreAddedApple(release string) *PreAddedApple {
	return &PreAddedApple{release: release, lists: map[string]int{}}
}

// Publish is Apple publishing the album: from now on its songs are out.
func (a *PreAddedApple) Publish() {
	a.mu.Lock()
	a.published = true
	a.mu.Unlock()
}

// Syncs is how many syncs have read the library: every one reads the music
// videos, once, whole, in a full run and in an incremental one.
func (a *PreAddedApple) Syncs() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lists["/v1/me/library/music-videos"]
}

// The songs of the published album, in track order.
var preAddedPublished = []string{"Out already", "Rising", "Falling", "Single"}

func (a *PreAddedApple) album(published bool) map[string]any {
	return map[string]any{
		"id": PreAddedID, "type": "library-albums",
		"attributes": map[string]any{
			"name": PreAddedName, "artistName": PreAddedArtist, "dateAdded": "2000-01-01T00:00:00Z", "trackCount": len(preAddedPublished),
			"artwork": map[string]any{"url": fixture.Template(900)},
		},
		"relationships": map[string]any{"catalog": map[string]any{"data": []any{map[string]any{
			"id": "7", "type": "albums",
			"attributes": map[string]any{"releaseDate": a.release, "isPrerelease": !published},
		}}}},
	}
}

func (a *PreAddedApple) song(i int, published bool) map[string]any {
	out := i == 0 || i == 3 // out before the album: the first and the single
	id := "i.soon" + strconv.Itoa(i)
	attrs := map[string]any{
		"name": preAddedPublished[i], "artistName": PreAddedArtist, "albumName": PreAddedName,
		"durationInMillis": 200000, "trackNumber": i + 1, "discNumber": 1,
	}
	if !out && !published {
		attrs["name"], attrs["artistName"] = "Track "+strconv.Itoa(i+1), "unknown"
	} else {
		attrs["artwork"] = map[string]any{"url": fixture.Template(900)}
		attrs["playParams"] = map[string]any{"id": id, "kind": "song", "isLibrary": true, "catalogId": "7s" + strconv.Itoa(i)}
	}
	if i == 0 {
		attrs["releaseDate"] = "1999-12-01"
	}
	return map[string]any{
		"id": id, "type": "library-songs", "attributes": attrs,
		"relationships": map[string]any{"albums": map[string]any{"data": []any{map[string]any{"id": PreAddedID, "type": "library-albums"}}}},
	}
}

func (a *PreAddedApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	published := a.published
	q := r.URL.Query()
	if q.Get("ids") == "" && q.Has("limit") {
		a.lists[r.URL.Path]++
	}
	a.mu.Unlock()

	var items []map[string]any
	switch p := r.URL.Path; {
	case p == "/v1/me/library/albums":
		items = []map[string]any{a.album(published)}
	case p == "/v1/me/library/songs":
		for i := range preAddedPublished {
			items = append(items, a.song(i, published))
		}
	case p == "/v1/me/library/artists", p == "/v1/me/library/playlists", p == "/v1/me/library/music-videos":
	case strings.HasPrefix(p, "/v1/me/ratings/"):
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
		return
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if ids := q.Get("ids"); ids != "" { // a lookup by ID
		data := []map[string]any{}
		for _, id := range strings.Split(ids, ",") {
			for _, it := range items {
				if it["id"] == id {
					data = append(data, it)
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
		return
	}
	off, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	page := []map[string]any{}
	if off < len(items) {
		page = items[off:min(off+limit, len(items))]
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"data": page, "meta": map[string]any{"total": len(items)}})
}
