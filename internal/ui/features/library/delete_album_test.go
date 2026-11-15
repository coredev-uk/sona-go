package library

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/coredev-uk/sona-go/internal/ui/uitest"
)

// stingyApple answers the library-state endpoints the way Apple does for
// albums it only partly knows: the catalogue says inLibrary:false for every
// album, and the library answer lists nothing. It records the DELETEs.
type stingyApple struct {
	mu      sync.Mutex
	deleted []string
	asked   int
}

func (a *stingyApple) asks() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.asked
}

func (a *stingyApple) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.asked++
	a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodDelete:
		a.mu.Lock()
		a.deleted = append(a.deleted, r.URL.Path)
		a.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.URL.Path == "/v1/catalog/gb":
		var data []string
		for _, id := range strings.Split(r.URL.Query().Get("ids[albums]"), ",") {
			if id != "" {
				data = append(data, fmt.Sprintf(`{"id":%q,"type":"albums","attributes":{"inLibrary":false},"relationships":{"library":{"data":[]}}}`, id))
			}
		}
		_, _ = fmt.Fprintf(w, `{"data":[%s]}`, strings.Join(data, ","))
	case r.URL.Path == "/v1/me/library":
		_, _ = fmt.Fprint(w, `{"data":[]}`)
	default:
		http.NotFound(w, r)
	}
}

// #336: every album under Library offers Delete from library, whatever
// Apple's catalogue says about it and even when the library answer leaves it
// out.
func TestEveryRecentlyAddedAlbumOffersDeleteFromLibrary(t *testing.T) {
	inBubble(t, everyRecentlyAddedAlbumOffersDeleteFromLibrary)
}

func everyRecentlyAddedAlbumOffersDeleteFromLibrary(t *testing.T) {
	apple := &stingyApple{}
	r := newGridRig(t, recentPath, nil, uitest.Albums(sevenAlbums...), uitest.Apple(apple))
	r.tt.Until("the entries", r.tt.Shows("Paper Planes", "Zebra Crossing", "7 albums"))
	for _, a := range sevenAlbums {
		if err := r.tt.RightClick(a.Name); err != nil {
			t.Fatalf("%s: %v", a.Name, err)
		}
		before := apple.asks()
		r.tt.Until("the menu of "+a.Name, r.tt.Shows("Favourite", "Suggest less"))
		r.tt.Until("Apple asked about "+a.Name, func() bool { return apple.asks() > before })
		// The menu's state has loaded once the rating rows are enabled; Apple's
		// answer must not have taken Delete away.
		r.tt.Quiesce()
		if !r.tt.HasText("Delete from library") {
			t.Errorf("the menu of %q has no Delete from library; texts %q", a.Name, r.tt.Texts())
		}
		r.tt.Key(0, ui.KeyEscape)
		r.tt.Until("the menu closed", func() bool { return !r.tt.HasText("Suggest less") })
	}
}

// Running it removes the album: Apple gets the DELETE of its library copy.
func TestDeleteFromLibraryRemovesAnAlbumAppleCallsNotInTheLibrary(t *testing.T) {
	apple := &stingyApple{}
	r := newGridRig(t, recentPath, nil, uitest.Albums(sevenAlbums...), uitest.Apple(apple))
	r.tt.Until("the entries", r.tt.Shows("Blue Hour", "7 albums"))
	if err := r.tt.RightClick("Blue Hour"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("Apple asked", func() bool { return apple.asks() > 0 })
	r.settle()
	if err := r.tt.Click("Delete from library"); err != nil {
		t.Fatal(err)
	}
	r.tt.Drain()
	if err := r.tt.Click("Delete"); err != nil {
		t.Fatal(err)
	}
	r.tt.Until("Apple's DELETE", func() bool {
		apple.mu.Lock()
		defer apple.mu.Unlock()
		return len(apple.deleted) == 1 && strings.HasPrefix(apple.deleted[0], "/v1/me/library/albums/")
	})
}
