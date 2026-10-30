package services

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// SearchService is search: Apple Music results by group (the catalog plus
// the social search's profiles and user playlists), "see all" pages per
// group, hints as the user types, the browse-categories landing page, the
// local library scope, recent searches and profile pages.
type SearchService struct {
	e *Env
	m mapper
}

func newSearchService(e *Env) *SearchService { return &SearchService{e: e, m: e.m} }

const (
	// firstPage is the per-group size of the first results page (the web
	// player's); groupPage the size of a "see all" page.
	firstPage = 21
	groupPage = applemusic.MaxSearchLimit
	// hintTerms and hintItems size the suggestions.
	hintTerms = 5
	hintItems = 6
	// keepRecent is how many recent searches are kept.
	keepRecent = 30
)

func (s *SearchService) storefront() (string, error) {
	sf := s.e.Storefront()
	if sf == "" {
		return "", catalog.ErrNoStorefront
	}
	return sf, nil
}

func (s *SearchService) signedIn() bool {
	return s.e.API.UserToken != nil && s.e.API.UserToken() != ""
}

// Catalog searches Apple Music: every result group in Apple's order, then
// profiles and user-shared playlists (signed in only; a failed social
// search leaves them out rather than failing the search).
func (s *SearchService) Catalog(ctx context.Context, term string) (SearchResults, error) {
	term = strings.TrimSpace(term)
	out := SearchResults{Term: term, Sections: []SearchSection{}}
	if term == "" {
		return out, nil
	}
	var (
		wg        sync.WaitGroup
		social    applemusic.SearchResults
		socialErr error
	)
	if s.signedIn() {
		if sf, err := s.storefront(); err == nil {
			wg.Go(func() {
				social, socialErr = applemusic.SocialSearch(ctx, s.e.API, sf, term, applemusic.SearchOptions{Limit: firstPage})
			})
		}
	}
	res, err := s.e.Catalog.Search(ctx, term, applemusic.SearchOptions{Limit: firstPage})
	wg.Wait()
	if err != nil {
		return SearchResults{}, s.e.Fail(err)
	}
	for _, g := range res.Order() {
		if sec := s.section(g, res.Groups[g], res.Resources); len(sec.Items) > 0 {
			out.Sections = append(out.Sections, sec)
		}
	}
	if socialErr != nil {
		s.e.Report(socialErr)
		s.e.Logger.Warn("social search", "err", socialErr)
		return out, nil
	}
	for _, g := range []string{applemusic.GroupProfile, applemusic.GroupPlaylist} {
		if sec := s.socialSection(g, social.Groups[g], social.Resources); len(sec.Items) > 0 {
			out.Sections = append(out.Sections, sec)
		}
	}
	return out, nil
}

// Group returns one page of a single result group at offset (the Next of
// its section or previous page): "see all".
func (s *SearchService) Group(ctx context.Context, term, group string, offset int) (SearchGroupPage, error) {
	term = strings.TrimSpace(term)
	if term == "" || group == "" || group == applemusic.GroupTop {
		return SearchGroupPage{}, fmt.Errorf("%w: search group %q for %q", ErrInvalidArgument, group, term)
	}
	opt := applemusic.SearchOptions{Group: group, Limit: groupPage, Offset: max(offset, 0)}
	var sec SearchSection
	switch group {
	case GroupProfiles, GroupSharedPlaylists:
		sf, err := s.storefront()
		if err != nil {
			return SearchGroupPage{}, err
		}
		apple := applemusic.GroupProfile
		if group == GroupSharedPlaylists {
			apple = applemusic.GroupPlaylist
		}
		opt.Group = apple
		res, err := applemusic.SocialSearch(ctx, s.e.API, sf, term, opt)
		if err != nil {
			return SearchGroupPage{}, s.e.Fail(err)
		}
		sec = s.socialSection(apple, res.Groups[apple], res.Resources)
	default:
		res, err := s.e.Catalog.Search(ctx, term, opt)
		if err != nil {
			return SearchGroupPage{}, s.e.Fail(err)
		}
		sec = s.section(group, res.Groups[group], res.Resources)
	}
	return SearchGroupPage(sec), nil
}

// Hints returns completions and top results for a partial term (call it
// debounced as the user types).
func (s *SearchService) Hints(ctx context.Context, term string) (SearchHints, error) {
	out := SearchHints{Terms: []SearchTerm{}, Items: []Item{}}
	term = strings.TrimSpace(term)
	if term == "" {
		return out, nil
	}
	sf, err := s.storefront()
	if err != nil {
		return out, err
	}
	res, err := applemusic.SearchSuggestions(ctx, s.e.API, sf, term, hintTerms, hintItems)
	if err != nil {
		return out, s.e.Fail(err)
	}
	for _, sg := range res.Results.Suggestions {
		switch {
		case sg.Kind == "terms" && sg.SearchTerm != "":
			d := sg.DisplayTerm
			if d == "" {
				d = sg.SearchTerm
			}
			out.Terms = append(out.Terms, SearchTerm{Term: sg.SearchTerm, Display: d})
		case sg.Kind == "topResults" && sg.Content != nil:
			out.Items = append(out.Items, s.items([]applemusic.Ref{*sg.Content}, res.Resources)...)
		}
	}
	return out, nil
}

// Browse returns the browse categories of the search landing page (genre
// and mood curators, campaigns, the charts), in Apple's order. Apple
// splits them over several shelves; they read as one set.
func (s *SearchService) Browse(ctx context.Context) ([]Item, error) {
	res, err := s.e.Catalog.SearchLanding(ctx)
	if err != nil {
		return nil, s.e.Fail(err)
	}
	// Every tile here is a category: saying so under each one adds nothing.
	category := curatorKind(applemusic.CuratorAttributes{Kind: "Genre"})
	out := []Item{}
	for _, shelf := range res.Primary() {
		for _, it := range s.items(shelf.Rel("contents"), res.Resources) {
			if it.Kind == ItemCurator && it.Subtitle == category {
				it.Subtitle = ""
			}
			out = append(out, it)
		}
	}
	return out, nil
}

// Library searches the library on this computer (prefix match on every
// word of names, artists and albums): up to perKind matches of each kind.
func (s *SearchService) Library(ctx context.Context, term string, perKind int) (LibrarySearch, error) {
	if perKind <= 0 || perKind > 100 {
		perKind = 25
	}
	r, err := s.e.Library.SearchAll(ctx, term, perKind)
	if err != nil {
		return LibrarySearch{}, err
	}
	out := LibrarySearch{
		Songs: s.m.libSongs(r.Songs), Albums: s.m.libAlbums(r.Albums),
		Artists: make([]Artist, len(r.Artists)), Playlists: make([]Playlist, len(r.Playlists)),
		SongsMore: r.SongsMore, AlbumsMore: r.AlbumsMore, ArtistsMore: r.ArtistsMore, PlaylistsMore: r.PlaylistsMore,
	}
	for i, a := range r.Artists {
		out.Artists[i] = s.m.libArtist(a)
	}
	for i, p := range r.Playlists {
		out.Playlists[i] = s.m.libPlaylist(p)
	}
	return out, nil
}

// Profile returns an Apple Music profile ("sp.…") with its public playlists.
func (s *SearchService) Profile(ctx context.Context, id string) (ProfileDetail, error) {
	sf, err := s.storefront()
	if err != nil {
		return ProfileDetail{}, err
	}
	n, res, err := applemusic.SocialProfile(ctx, s.e.API, sf, id)
	if err != nil {
		return ProfileDetail{}, s.e.Fail(err)
	}
	out := ProfileDetail{Profile: s.profile(n), Playlists: []Playlist{}}
	for _, pn := range res.Resolve(n.Rel("shared-playlists")) {
		if p, err := applemusic.As[applemusic.Playlist](pn); err == nil {
			out.Playlists = append(out.Playlists, s.m.amPlaylist(p))
		}
	}
	return out, nil
}

// Recent returns the recent searches, newest first.
func (s *SearchService) Recent(ctx context.Context) ([]RecentSearch, error) {
	rows, err := s.e.DB.RQ.ListRecentSearches(ctx, keepRecent)
	if err != nil {
		return nil, err
	}
	out := make([]RecentSearch, len(rows))
	for i, r := range rows {
		out[i] = recentDTO(r)
	}
	return out, nil
}

// RememberTerm records term as searched in scope ("catalog" or "library").
func (s *SearchService) RememberTerm(ctx context.Context, term, scope string) error {
	term = strings.Join(strings.Fields(term), " ")
	if term == "" {
		return nil
	}
	if scope != ScopeCatalog && scope != ScopeLibrary {
		return fmt.Errorf("%w: search scope %q", ErrInvalidArgument, scope)
	}
	return s.remember(ctx, dbq.PutRecentSearchParams{
		Key: "term:" + scope + ":" + strings.ToLower(term), Kind: RecentTerm, Scope: scope, Term: term, Name: term,
	})
}

// RememberItem records an item opened from search.
func (s *SearchService) RememberItem(ctx context.Context, it RecentItem) error {
	if it.ID == "" || it.Kind == "" || it.Kind == RecentTerm {
		return fmt.Errorf("%w: recent item %q %q", ErrInvalidArgument, it.Kind, it.ID)
	}
	if it.Source != SourceLibrary {
		it.Source = SourceCatalog
	}
	if !strings.HasPrefix(it.Art, "/art/") {
		it.Art = "" // only Go-served artwork
	}
	return s.remember(ctx, dbq.PutRecentSearchParams{
		Key: "item:" + it.Kind + ":" + string(it.Source) + ":" + it.ID, Kind: it.Kind, Scope: string(it.Source),
		ItemID: it.ID, Name: it.Name, Subtitle: it.Subtitle, ArtUrl: it.Art,
	})
}

func (s *SearchService) remember(ctx context.Context, p dbq.PutRecentSearchParams) error {
	p.SearchedAt = s.e.Now().UnixMilli()
	return s.e.DB.Tx(ctx, func(q *dbq.Queries) error {
		if err := q.PutRecentSearch(ctx, p); err != nil {
			return err
		}
		return q.TrimRecentSearches(ctx, keepRecent)
	})
}

// ForgetRecent removes one recent search by its Key.
func (s *SearchService) ForgetRecent(ctx context.Context, key string) error {
	return s.e.DB.Q.DeleteRecentSearch(ctx, key)
}

// ClearRecent removes every recent search.
func (s *SearchService) ClearRecent(ctx context.Context) error {
	return s.e.DB.Q.ClearRecentSearches(ctx)
}

func recentDTO(r dbq.RecentSearch) RecentSearch {
	out := RecentSearch{
		Key: r.Key, Kind: r.Kind, Term: r.Term, ItemID: r.ItemID, Name: r.Name, Subtitle: r.Subtitle,
		Art: r.ArtUrl, SearchedAt: r.SearchedAt,
	}
	if r.Kind == RecentTerm {
		out.Scope = r.Scope
	} else {
		out.Source = Source(r.Scope)
	}
	return out
}

// section maps one catalog result group.
func (s *SearchService) section(group string, g applemusic.SearchGroup, res applemusic.Resources) SearchSection {
	return SearchSection{Group: group, Items: s.items(g.Data, res), Next: nextOffset(g.Next)}
}

// socialSection maps a social search group ("profile" or "playlist").
func (s *SearchService) socialSection(group string, g applemusic.SearchGroup, res applemusic.Resources) SearchSection {
	if group == applemusic.GroupPlaylist {
		return s.section(GroupSharedPlaylists, g, res)
	}
	sec := SearchSection{Group: GroupProfiles, Items: []Item{}, Next: nextOffset(g.Next)}
	for _, n := range res.Resolve(g.Data) {
		if n.Type == "social-profiles" {
			sec.Items = append(sec.Items, profileItem(s.profile(n)))
		}
	}
	return sec
}

// items maps result references to cards, dropping kinds Sona has no page
// for (ItemLink: uploaded videos, music movies, …).
func (s *SearchService) items(refs []applemusic.Ref, res applemusic.Resources) []Item {
	all := s.m.items(res, refs)
	out := all[:0]
	for _, it := range all {
		if it.Kind != ItemLink {
			out = append(out, it)
		}
	}
	return out
}

// profileItem is a profile as a search card.
func profileItem(p SocialProfile) Item {
	return Item{Kind: ItemProfile, ID: p.ID, Name: p.Name, Subtitle: "@" + p.Handle, Art: p.Art, URL: p.URL}
}

func (s *SearchService) profile(n applemusic.Node) SocialProfile {
	var a struct {
		Name       string              `json:"name"`
		Handle     string              `json:"handle"`
		IsVerified bool                `json:"isVerified"`
		IsPrivate  bool                `json:"isPrivate"`
		URL        string              `json:"url"`
		Artwork    *applemusic.Artwork `json:"artwork"`
	}
	_ = n.Attrs(&a)
	return SocialProfile{
		ID: n.ID, Name: a.Name, Handle: a.Handle, Verified: a.IsVerified, Private: a.IsPrivate, URL: a.URL,
		Art: s.m.artOf(a.Artwork.Template()),
	}
}

// nextOffset is the offset of a group's next page, 0 when there is none.
func nextOffset(next string) int { return max(applemusic.NextOffset(next), 0) }
