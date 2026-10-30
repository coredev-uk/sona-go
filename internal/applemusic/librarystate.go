package applemusic

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

// ItemKey names one resource: its Apple type ("songs", "library-albums", …)
// and ID.
type ItemKey struct {
	Type string
	ID   string
}

// LibraryState is a resource's place in the user's library and its rating.
type LibraryState struct {
	InLibrary bool
	// LibraryID is the library copy's ID ("i.…", "l.…", "p.…"); "" when
	// the item is not in the library.
	LibraryID string
	// Rating is 1 (favourite), -1 (suggest less) or 0.
	Rating int
}

// stateBatch bounds the IDs of one library-state request (all types
// together).
const stateBatch = 100

// stateResource is one resource of a library-state response.
type stateResource struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		InLibrary      bool `json:"inLibrary"`
		PersonalRating int  `json:"personalRating"`
	} `json:"attributes"`
	Relationships struct {
		Library struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"library"`
	} `json:"relationships"`
	Meta struct {
		FormerIDs []string `json:"formerIds"`
	} `json:"meta"`
}

// CatalogLibraryStates looks up whether catalog resources are in the
// library, their library IDs and ratings, as the web player does after a
// page loads. ids is keyed by type (TypeSongs, TypeAlbums, TypePlaylists,
// TypeMusicVideos; artists carry no library state here: use Ratings).
// Requests hold up to 100 IDs across types and run four at a time.
// Results are keyed by the requested type and ID (a song answered under
// its new ID is matched through meta.formerIds); unknown IDs are absent.
func (c *Client) CatalogLibraryStates(ctx context.Context, sf string, ids map[string][]string) (map[ItemKey]LibraryState, error) {
	return c.libraryStates(ctx, CatalogPath(sf), ids, "inLibrary,personalRating")
}

// LibraryItemStates looks up library resources (TypeLibrarySongs,
// TypeLibraryAlbums, TypeLibraryPlaylists, TypeLibraryArtists,
// TypeLibraryMusicVideos) by ID: a resource Apple returns is in the
// library (InLibrary, LibraryID = its ID) with its rating; deleted IDs are
// absent.
func (c *Client) LibraryItemStates(ctx context.Context, ids map[string][]string) (map[ItemKey]LibraryState, error) {
	// A resource whose sparse fieldset comes back empty is dropped from
	// the response, so ask for the name too.
	out, err := c.libraryStates(ctx, "/v1/me/library", ids, "name,personalRating")
	for k, st := range out {
		st.InLibrary, st.LibraryID = true, k.ID
		out[k] = st
	}
	return out, err
}

func (c *Client) libraryStates(ctx context.Context, path string, ids map[string][]string, fields string) (map[ItemKey]LibraryState, error) {
	// Every requested key, in a stable order, deduplicated.
	var keys []ItemKey
	seen := map[ItemKey]bool{}
	types := make([]string, 0, len(ids))
	for typ := range ids {
		types = append(types, typ)
	}
	sort.Strings(types)
	for _, typ := range types {
		for _, id := range ids[typ] {
			k := ItemKey{typ, id}
			if id != "" && !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	out := make(map[ItemKey]LibraryState, len(keys))
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(defaultParallel)
	for start := 0; start < len(keys); start += stateBatch {
		batch := keys[start:min(len(keys), start+stateBatch)]
		g.Go(func() error {
			byType := map[string][]string{}
			for _, k := range batch {
				byType[k.Type] = append(byType[k.Type], k.ID)
			}
			// Empty ids[…] keys make Apple answer 400: only send used ones.
			q := url.Values{"platform": {"web"}, "relate": {"library"}, "omit[resource]": {"autos"}}
			for typ, list := range byType {
				q.Set("ids["+typ+"]", strings.Join(list, ","))
				q.Set("fields["+typ+"]", fields)
			}
			var body struct {
				Data []stateResource `json:"data"`
			}
			if err := c.Do(gctx, Request{Method: http.MethodGet, URL: c.ampBase() + path, Query: q}, &body); err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			for _, r := range body.Data {
				st := LibraryState{InLibrary: r.Attributes.InLibrary, Rating: r.Attributes.PersonalRating}
				if d := r.Relationships.Library.Data; len(d) > 0 {
					st.LibraryID = d[0].ID
				}
				for _, id := range append([]string{r.ID}, r.Meta.FormerIDs...) {
					if k := (ItemKey{r.Type, id}); seen[k] {
						out[k] = st
					}
				}
			}
			return nil
		})
	}
	return out, g.Wait()
}

// libraryCollections are the collections under /v1/me/library that
// DeleteFromLibrary accepts.
var libraryCollections = map[string]bool{
	TypeSongs: true, TypeAlbums: true, TypeArtists: true, TypePlaylists: true, TypeMusicVideos: true,
}

// DeleteFromLibrary removes library resource libraryID from the user's
// library. collection is TypeSongs, TypeAlbums, TypeArtists,
// TypePlaylists or TypeMusicVideos (the path under /v1/me/library).
// Removing what is already gone succeeds.
func (c *Client) DeleteFromLibrary(ctx context.Context, collection, libraryID string) error {
	if !libraryCollections[collection] || libraryID == "" {
		return fmt.Errorf("apple music: delete %q from library collection %q", libraryID, collection)
	}
	err := c.Do(ctx, Request{
		Method: http.MethodDelete,
		URL:    c.ampBase() + "/v1/me/library/" + url.PathEscape(collection) + "/" + url.PathEscape(libraryID),
	}, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}
