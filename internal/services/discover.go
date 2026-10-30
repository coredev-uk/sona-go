package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/db"
)

// Home cache policy.
const (
	homeKey = "home"
	// homeMinLife and homeMaxLife clamp Apple's nextUpdateDate. Apple often
	// sends dates already in the past (most shelves say "due" right after a
	// fetch), so a shelf is never due sooner than homeMinLife after it was
	// fetched; a missing date still expires.
	homeMinLife = 5 * time.Minute
	homeMaxLife = 12 * time.Hour
	// homeShelfRefreshMax: when at most this many shelves are due (Recently
	// Played, Favourite Artists) they are refreshed one by one; more means
	// the recommendations themselves rolled over and Home is refetched.
	homeShelfRefreshMax = 3
	// discoverKeep drops cached pages nobody opened for this long.
	discoverKeep = 30 * 24 * time.Hour
	chartsTTL    = time.Hour
	genresTTL    = 7 * 24 * time.Hour
)

// DiscoverService serves Home (personal recommendations), New, editorial
// rooms, multirooms, groupings and curators, and Charts.
type DiscoverService struct {
	e *Env
	m mapper

	mu         sync.Mutex
	refreshing bool // a background Home refresh is running
}

// newDiscoverService refreshes Home once the network opens (when Apple's
// shelves are due) and prunes pages not opened for a month.
func newDiscoverService(e *Env) *DiscoverService {
	s := &DiscoverService{e: e, m: e.m}
	e.OnNetwork(func() {
		go func() {
			ctx := context.Background()
			if n, err := e.DB.Q.DeleteOldDiscoverCache(ctx, db.Millis(e.Now().Add(-discoverKeep))); err != nil {
				e.Logger.Warn("discover cache prune", "err", err)
			} else if n > 0 {
				e.Logger.Info("discover cache pruned", "entries", n)
			}
			s.refreshHomeIfDue(ctx)
		}()
	})
	return s
}

// signedIn: a Music User Token is available (Home needs one).
func (s *DiscoverService) signedIn() bool {
	return s.e.API.UserToken != nil && s.e.API.UserToken() != ""
}

// timezone is the local UTC offset Apple's recommendations take ("+01:00").
func (s *DiscoverService) timezone() string { return s.e.Now().Format("-07:00") }

// homeEntry reads the cached Home response.
func (s *DiscoverService) homeEntry(ctx context.Context) (applemusic.MapResponse, int64, bool, error) {
	sf := s.e.Storefront()
	row, err := s.e.DB.RQ.GetDiscoverCache(ctx, sf+"|"+homeKey)
	if errors.Is(err, sql.ErrNoRows) {
		return applemusic.MapResponse{}, 0, false, nil
	}
	if err != nil {
		return applemusic.MapResponse{}, 0, false, fmt.Errorf("discover cache: %w", err)
	}
	var r applemusic.MapResponse
	if err := json.Unmarshal(row.Body, &r); err != nil {
		s.e.Logger.Warn("discover cache: undecodable home", "err", err)
		return applemusic.MapResponse{}, 0, false, nil
	}
	return r, row.FetchedAt, true, nil
}

// Home returns Home from the cache at once (no network before the first
// paint; a refresh that is due runs in the background and arrives as a
// "discover:home" event). Without a cached copy it asks Apple. Signed out
// it is empty.
func (s *DiscoverService) Home(ctx context.Context) (HomePage, error) {
	if !s.signedIn() {
		return HomePage{Sections: []Section{}}, nil
	}
	start := time.Now()
	r, fetched, ok, err := s.homeEntry(ctx)
	if err != nil {
		return HomePage{}, err
	}
	if ok {
		page := s.homePage(r, fetched)
		s.e.Logger.Info("home served from cache", "shelves", len(page.Sections), "took", time.Since(start).String(), "network", s.e.Gate.IsOpen())
		if s.e.Gate.IsOpen() {
			go s.refreshHomeIfDue(context.Background())
		}
		return page, nil
	}
	return s.RefreshHome(ctx)
}

// RefreshHome refetches every Home shelf from Apple.
func (s *DiscoverService) RefreshHome(ctx context.Context) (HomePage, error) {
	if !s.signedIn() {
		return HomePage{Sections: []Section{}}, nil
	}
	r, err := applemusic.Recommendations(ctx, s.e.API, s.timezone())
	if err != nil {
		return HomePage{}, s.e.Fail(err)
	}
	now := s.e.Now()
	s.putHome(ctx, r, now)
	s.e.Logger.Info("home fetched", "shelves", len(r.Data))
	return s.homePage(r, now.UnixMilli()), nil
}

func (s *DiscoverService) putHome(ctx context.Context, r applemusic.MapResponse, now time.Time) {
	putPage(ctx, s.e, s.e.Storefront()+"|"+homeKey, s.e.Storefront(), true, r, now, 0,
		func(r applemusic.MapResponse, now time.Time) time.Time { return homeExpiry(r, now) })
}

// homeExpiry is the earliest shelf nextUpdateDate, clamped.
func homeExpiry(r applemusic.MapResponse, now time.Time) time.Time {
	end := now.Add(homeMaxLife)
	for _, shelf := range r.Primary() {
		var a applemusic.RecommendationAttributes
		if shelf.Attrs(&a) != nil {
			continue
		}
		if t, err := time.Parse(time.RFC3339, a.NextUpdateDate); err == nil && t.Before(end) {
			end = t
		}
	}
	if min := now.Add(homeMinLife); end.Before(min) {
		end = min
	}
	return end
}

// dueShelves lists the shelves whose nextUpdateDate has passed, counting
// no shelf due before fetched + homeMinLife.
func dueShelves(r applemusic.MapResponse, fetched, now time.Time) []string {
	if now.Before(fetched.Add(homeMinLife)) {
		return nil
	}
	var due []string
	for _, shelf := range r.Primary() {
		var a applemusic.RecommendationAttributes
		if shelf.Attrs(&a) != nil {
			continue
		}
		if t, err := time.Parse(time.RFC3339, a.NextUpdateDate); err == nil && !t.After(now) {
			due = append(due, shelf.ID)
		}
	}
	return due
}

// refreshHomeIfDue renews the shelves Apple said are due: a few one by one
// (POST …?action=refresh), or all of Home when most are. The new page is
// emitted as "discover:home".
func (s *DiscoverService) refreshHomeIfDue(ctx context.Context) {
	if !s.signedIn() {
		return
	}
	s.mu.Lock()
	if s.refreshing {
		s.mu.Unlock()
		return
	}
	s.refreshing = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.refreshing = false
		s.mu.Unlock()
	}()

	r, fetched, ok, err := s.homeEntry(ctx)
	if err != nil {
		s.e.Logger.Warn("home refresh", "err", err)
		return
	}
	now := s.e.Now()
	due := dueShelves(r, time.UnixMilli(fetched), now)
	if ok && len(due) == 0 {
		return
	}
	if !ok || len(due) > homeShelfRefreshMax {
		page, err := s.RefreshHome(ctx)
		if err != nil {
			s.e.Logger.Warn("home refresh", "err", err)
			return
		}
		s.e.Emit.Emit(EventHome, page)
		return
	}
	for _, id := range due {
		fresh, err := applemusic.RefreshRecommendation(ctx, s.e.API, s.timezone(), id)
		if err != nil {
			s.e.Report(err)
			s.e.Logger.Warn("home shelf refresh", "shelf", id, "err", err)
			continue
		}
		mergeShelf(&r, fresh)
	}
	s.putHome(ctx, r, now)
	s.e.Logger.Info("home shelves refreshed", "shelves", len(due))
	s.e.Emit.Emit(EventHome, s.homePage(r, now.UnixMilli()))
}

// mergeShelf puts a refreshed shelf (and the resources it brought) into
// the cached Home response, in place.
func mergeShelf(r *applemusic.MapResponse, fresh applemusic.MapResponse) {
	if r.Resources == nil {
		r.Resources = applemusic.Resources{}
	}
	r.Resources.Merge(fresh.Resources)
}

// RefreshHomeShelf asks Apple to recompute one Home shelf and returns it.
func (s *DiscoverService) RefreshHomeShelf(ctx context.Context, id string) (Section, error) {
	fresh, err := applemusic.RefreshRecommendation(ctx, s.e.API, s.timezone(), id)
	if err != nil {
		return Section{}, s.e.Fail(err)
	}
	if r, _, ok, err := s.homeEntry(ctx); err == nil && ok {
		mergeShelf(&r, fresh)
		s.putHome(ctx, r, s.e.Now())
	}
	for _, sec := range s.homeSections(fresh) {
		return sec, nil
	}
	return Section{}, fmt.Errorf("%w: shelf %s", applemusic.ErrNotFound, id)
}

// HomeShelf returns Home shelf id's "See all" view (up to 100 items).
func (s *DiscoverService) HomeShelf(ctx context.Context, id string) (Section, error) {
	r, _, err := cachedPage(ctx, s.e, "home-shelf|"+id, true, 30*time.Minute,
		func(ctx context.Context, _ string) (applemusic.MapResponse, error) {
			return applemusic.RecommendationSeeAll(ctx, s.e.API, s.timezone(), id)
		}, nil)
	if err != nil {
		return Section{}, err
	}
	for _, sec := range s.homeSections(r) {
		sec.Kind = SectionShelf
		return sec, nil
	}
	return Section{}, fmt.Errorf("%w: shelf %s", applemusic.ErrNotFound, id)
}

func (s *DiscoverService) homePage(r applemusic.MapResponse, fetched int64) HomePage {
	p := HomePage{Sections: s.homeSections(r), FetchedAt: fetched}
	p.Stale = s.e.Gate.IsOpen() && len(dueShelves(r, time.UnixMilli(fetched), s.e.Now())) > homeShelfRefreshMax
	return p
}

// homeSections maps Home shelves; concerts and web-only cards are left out.
func (s *DiscoverService) homeSections(r applemusic.MapResponse) []Section {
	out := []Section{}
	for _, shelf := range r.Primary() {
		var a applemusic.RecommendationAttributes
		if shelf.Attrs(&a) != nil {
			continue
		}
		sec := Section{ID: shelf.ID, Title: a.Heading(), SeeAll: a.HasSeeAll, Kind: SectionShelf}
		switch a.Display.Kind {
		case "MusicConcertsEmptyShelf":
			continue
		case "MusicSuperHeroShelf":
			sec.Kind = SectionHero
		case "MusicNotesHeroShelf":
			sec.Kind = SectionFeature
		}
		for _, it := range s.m.items(r.Resources, shelf.Rel("contents")) {
			if it.Kind != ItemLink {
				sec.Items = append(sec.Items, it)
			}
		}
		if len(sec.Items) > 0 {
			out = append(out, sec)
		}
	}
	return out
}

// New returns the New page (the "music" editorial grouping).
func (s *DiscoverService) New(ctx context.Context) (EditorialPage, error) {
	return s.e.editorialGrouping(ctx, "music")
}

// Grouping returns editorial grouping id (a hub such as Music Videos).
func (s *DiscoverService) Grouping(ctx context.Context, id string) (EditorialPage, error) {
	r, info, err := cachedPage(ctx, s.e, "grouping-id|"+id, false, groupingTTL,
		func(ctx context.Context, sf string) (applemusic.MapResponse, error) {
			return applemusic.GroupingByID(ctx, s.e.API, sf, id)
		}, nil)
	if err != nil {
		return EditorialPage{}, err
	}
	p := s.m.groupingPage(r, info)
	if p.ID == "" {
		return EditorialPage{}, fmt.Errorf("%w: grouping %s", applemusic.ErrNotFound, id)
	}
	return p, nil
}

// Curator returns a curator page: a genre or category, a radio show, or an
// editorial or brand curator, with its sections (or its playlists when it
// has none).
func (s *DiscoverService) Curator(ctx context.Context, id string) (EditorialPage, error) {
	r, info, err := cachedPage(ctx, s.e, "curator|"+id, false, curatorTTL,
		func(ctx context.Context, sf string) (applemusic.MapResponse, error) {
			return applemusic.CuratorPage(ctx, s.e.API, sf, id)
		}, nil)
	if err != nil {
		return EditorialPage{}, err
	}
	nodes := r.Primary()
	if len(nodes) == 0 {
		return EditorialPage{}, fmt.Errorf("%w: curator %s", applemusic.ErrNotFound, id)
	}
	c := nodes[0]
	var a applemusic.CuratorAttributes
	_ = c.Attrs(&a)
	var ea editorialArt
	_ = c.Attrs(&ea)
	p := EditorialPage{
		Kind: ItemCurator, ID: c.ID, Title: a.Name, Subtitle: curatorKind(a), URL: a.URL,
		Art: s.m.artOf(a.Artwork.Template()), Sections: []Section{},
		FetchedAt: info.FetchedAt, Stale: info.Stale,
	}
	p.Wide, p.Color = s.m.wideOf(ea)
	if n := a.EditorialNotes; n != nil {
		p.Notes = plainText(firstNonEmpty(n.Standard, n.Short, n.Tagline))
	}
	for _, g := range r.Resources.Resolve(c.Rel("grouping")) {
		for _, tab := range r.Resources.Resolve(g.Rel("tabs")) {
			p.Sections = append(p.Sections, s.m.sections(r.Resources, tab)...)
		}
	}
	if pls := s.m.items(r.Resources, c.Rel("playlists")); len(pls) > 0 && !hasPlaylistShelf(p.Sections) {
		p.Sections = append(p.Sections, Section{ID: c.ID + "-playlists", Kind: SectionShelf, Title: "Playlists", Items: pls})
	}
	return p, nil
}

func hasPlaylistShelf(secs []Section) bool {
	for _, s := range secs {
		if s.Kind == SectionShelf && allKind(s.Items, ItemPlaylist) {
			return true
		}
	}
	return false
}

// Multiroom returns a campaign page: its banner and its rooms and
// editorial sections.
func (s *DiscoverService) Multiroom(ctx context.Context, id string) (EditorialPage, error) {
	r, info, err := cachedPage(ctx, s.e, "multiroom|"+id, false, roomTTL,
		func(ctx context.Context, sf string) (applemusic.MapResponse, error) {
			return applemusic.Multiroom(ctx, s.e.API, sf, id)
		}, nil)
	if err != nil {
		return EditorialPage{}, err
	}
	nodes := r.Primary()
	if len(nodes) == 0 {
		return EditorialPage{}, fmt.Errorf("%w: multiroom %s", applemusic.ErrNotFound, id)
	}
	mr := nodes[0]
	var a applemusic.MultiroomAttributes
	_ = mr.Attrs(&a)
	p := EditorialPage{Kind: ItemMultiroom, ID: mr.ID, Title: a.Title, Sections: []Section{}, FetchedAt: info.FetchedAt, Stale: info.Stale}
	if u := a.Uber; u != nil {
		if u.MasterArt != nil {
			p.Wide, p.Color = s.m.artOf(u.MasterArt.Template()), hexColor(u.MasterArt.BgColor)
		}
		if u.Description != nil {
			p.Notes = plainText(*u.Description)
		}
	}
	for _, child := range r.Resources.Resolve(mr.Rel("children")) {
		switch child.Type {
		case applemusic.TypeRooms:
			var ra applemusic.RoomAttributes
			_ = child.Attrs(&ra)
			sec := Section{ID: child.ID, Kind: SectionShelf, Title: ra.Title, Room: child.ID, Items: s.m.items(r.Resources, child.Rel("contents"))}
			if allKind(sec.Items, ItemSong) {
				sec.Kind = SectionTracks
			}
			if len(sec.Items) > 0 {
				p.Sections = append(p.Sections, sec)
			}
		case applemusic.TypeEditorialElements:
			if sec, ok := s.m.section(r.Resources, child); ok {
				p.Sections = append(p.Sections, sec)
			}
		}
	}
	return p, nil
}

// Room returns an editorial room (a section's "See all" grid) in sort
// order sort ("" is Apple's default).
func (s *DiscoverService) Room(ctx context.Context, id, sort string) (RoomPage, error) {
	r, _, err := cachedPage(ctx, s.e, "room|"+id+"|"+sort, false, roomTTL,
		func(ctx context.Context, sf string) (applemusic.MapResponse, error) {
			return applemusic.Room(ctx, s.e.API, sf, id, sort)
		}, nil)
	if err != nil {
		return RoomPage{}, err
	}
	nodes := r.Primary()
	if len(nodes) == 0 {
		return RoomPage{}, fmt.Errorf("%w: room %s", applemusic.ErrNotFound, id)
	}
	room := nodes[0]
	var a applemusic.RoomAttributes
	_ = room.Attrs(&a)
	p := RoomPage{
		ID: room.ID, Title: a.Title, Kind: SectionShelf, Sort: firstNonEmpty(sort, a.DefaultSort),
		Sorts: a.Sorts, Items: s.m.items(r.Resources, room.Rel("contents")),
		Next: nextOffset(room.Relationships["contents"].Next),
	}
	if p.Sorts == nil {
		p.Sorts = []string{}
	}
	if a.Kind == applemusic.ElementTracks || allKind(p.Items, ItemSong) {
		p.Kind = SectionTracks
	}
	return p, nil
}

// RoomItems returns the page of room id's items at offset (RoomPage.Next).
func (s *DiscoverService) RoomItems(ctx context.Context, id, sort string, offset int) (ItemPage, error) {
	if offset <= 0 {
		return ItemPage{}, fmt.Errorf("%w: offset %d", ErrInvalidArgument, offset)
	}
	r, _, err := cachedPage(ctx, s.e, fmt.Sprintf("room|%s|%s|%d", id, sort, offset), false, roomTTL,
		func(ctx context.Context, sf string) (applemusic.MapResponse, error) {
			return applemusic.RoomContents(ctx, s.e.API, sf, id, sort, offset)
		}, nil)
	if err != nil {
		return ItemPage{}, err
	}
	return ItemPage{Items: s.m.items(r.Resources, r.Data), Next: nextOffset(r.Next)}, nil
}

// Charts returns the charts for genre ("" is all music): top songs,
// albums, playlists and music videos, and the city and daily top charts.
func (s *DiscoverService) Charts(ctx context.Context, genre string) (ChartsPage, error) {
	r, info, err := cachedPage(ctx, s.e, "charts|"+genre, false, chartsTTL,
		func(ctx context.Context, sf string) (applemusic.ChartsResponse, error) {
			return applemusic.Charts(ctx, s.e.API, sf, applemusic.ChartsQuery{
				Genre: genre, Types: []string{ChartSongs, ChartAlbums, ChartPlaylists, ChartMusicVideos}, Cities: true,
			})
		}, nil)
	if err != nil {
		return ChartsPage{}, err
	}
	p := ChartsPage{Genre: genre, Charts: []Chart{}, FetchedAt: info.FetchedAt, Stale: info.Stale}
	p.Genres, err = s.genres(ctx)
	if err != nil {
		s.e.Logger.Warn("chart genres", "err", err)
		p.Genres = []Genre{}
	}
	for _, key := range chartOrder(r) {
		for _, c := range r.Results[key] {
			ch := Chart{Key: key, ChartID: c.ChartID(), Title: c.Name, Items: s.m.items(r.Resources, c.Data), Next: nextOffset(c.Next)}
			if len(ch.Items) > 0 {
				p.Charts = append(p.Charts, ch)
			}
		}
	}
	return p, nil
}

// chartOrder is meta.results.order's groups ("most-played:songs" → "songs"),
// then any group it left out.
func chartOrder(r applemusic.ChartsResponse) []string {
	var keys []string
	seen := map[string]bool{}
	for _, o := range r.Meta.Results.Order {
		_, key, _ := strings.Cut(o, ":")
		if _, ok := r.Results[key]; ok && !seen[key] {
			keys, seen[key] = append(keys, key), true
		}
	}
	for _, key := range []string{ChartSongs, ChartAlbums, ChartPlaylists, ChartMusicVideos, ChartCities, ChartDailyGlobal} {
		if _, ok := r.Results[key]; ok && !seen[key] {
			keys, seen[key] = append(keys, key), true
		}
	}
	return keys
}

// ChartItems returns the page of a chart at offset (Chart.Next): key and
// chartID as in Chart, genre as in Charts.
func (s *DiscoverService) ChartItems(ctx context.Context, key, chartID, genre string, offset int) (ItemPage, error) {
	q := applemusic.ChartsQuery{Genre: genre, Offset: offset}
	switch key {
	case ChartSongs, ChartAlbums, ChartPlaylists, ChartMusicVideos:
		q.Types, q.Chart = []string{key}, "most-played"
	case ChartCities, ChartDailyGlobal:
		if chartID == "" {
			return ItemPage{}, fmt.Errorf("%w: chart %s needs a chart ID", ErrInvalidArgument, key)
		}
		q.ChartID = chartID
	default:
		return ItemPage{}, fmt.Errorf("%w: chart %q", ErrInvalidArgument, key)
	}
	r, _, err := cachedPage(ctx, s.e, fmt.Sprintf("chart|%s|%s|%s|%d", key, chartID, genre, offset), false, chartsTTL,
		func(ctx context.Context, sf string) (applemusic.ChartsResponse, error) {
			return applemusic.Charts(ctx, s.e.API, sf, q)
		}, nil)
	if err != nil {
		return ItemPage{}, err
	}
	out := ItemPage{Items: []Item{}}
	for _, c := range r.Results[key] {
		out.Items = append(out.Items, s.m.items(r.Resources, c.Data)...)
		out.Next = nextOffset(c.Next)
	}
	return out, nil
}

// genres lists the chart genres, all music first (as ID "").
func (s *DiscoverService) genres(ctx context.Context) ([]Genre, error) {
	gs, _, err := cachedPage(ctx, s.e, "genres", false, genresTTL,
		func(ctx context.Context, sf string) ([]applemusic.Resource[applemusic.GenreAttributes], error) {
			return applemusic.Genres(ctx, s.e.API, sf)
		}, nil)
	if err != nil {
		return nil, err
	}
	out := []Genre{{ID: "", Name: "All genres"}}
	for _, g := range gs {
		if g.ID != applemusic.GenreAllMusic {
			out = append(out, Genre{ID: g.ID, Name: g.Attributes.Name})
		}
	}
	return out, nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
