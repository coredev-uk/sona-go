package services

// Shared editorial mapping and caching (not a service): Apple's normalised
// editorial and recommendation responses → Item and Section DTOs, and the
// discover_cache read-through used by Discover (Home, New, rooms, curators,
// charts) and Radio (the radio grouping). Features call these through Env.

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/catalog"
	"github.com/coredev-uk/sona-go/internal/db"
	"github.com/coredev-uk/sona-go/internal/db/dbq"
)

// Editorial cache lifetimes. Expired entries are served while a refetch is
// impossible (offline, before the first paint).
const (
	groupingTTL = time.Hour
	roomTTL     = 6 * time.Hour
	curatorTTL  = 6 * time.Hour
)

// cacheInfo says where a cached page came from.
type cacheInfo struct {
	FetchedAt int64 // Unix ms of the Apple response
	Stale     bool  // past its lifetime (served because Apple could not be asked)
	Expired   bool  // past its lifetime, whether or not it was refetched
}

// cacheScope is the storefront part of a discover_cache key, with the
// content language Apple is asked in (catalog.CacheScope).
func (e *Env) cacheScope(sf string) string {
	return catalog.CacheScope(sf, e.API.ContentLanguage())
}

// cachedPage reads key from discover_cache and refetches it when it has
// expired. Before the first paint an existing entry is served as is (cold
// start never waits for the network); when Apple fails, an expired entry
// is served with Stale set. expires returns the entry's lifetime end (nil:
// fetched + ttl). Personal entries are wiped on sign-out.
func cachedPage[T any](ctx context.Context, e *Env, key string, personal bool, ttl time.Duration,
	fetch func(ctx context.Context, sf string) (T, error), expires func(T, time.Time) time.Time,
) (T, cacheInfo, error) {
	var zero T
	sf := e.Storefront()
	if sf == "" {
		return zero, cacheInfo{}, catalog.ErrNoStorefront
	}
	key = e.cacheScope(sf) + "|" + key
	now := e.Now()
	var stale *T
	var info cacheInfo
	row, err := e.DB.RQ.GetDiscoverCache(ctx, key)
	switch {
	case err == nil:
		var v T
		if jerr := json.Unmarshal(row.Body, &v); jerr != nil {
			e.Logger.Warn("discover cache: undecodable entry", "key", key, "err", jerr)
			break
		}
		info = cacheInfo{FetchedAt: row.FetchedAt, Expired: now.UnixMilli() >= row.ExpiresAt}
		if !info.Expired || !e.Gate.IsOpen() {
			return v, info, nil
		}
		stale = &v
	case !errors.Is(err, sql.ErrNoRows):
		return zero, cacheInfo{}, fmt.Errorf("discover cache: %w", err)
	}
	v, err := fetch(ctx, sf)
	if err != nil {
		e.Report(err)
		if stale != nil && ctx.Err() == nil && !errors.Is(err, applemusic.ErrUnauthorized) && !errors.Is(err, applemusic.ErrNotFound) {
			e.Logger.Warn("discover: serving stale page", "key", key, "err", err)
			info.Stale = true
			return *stale, info, nil
		}
		return zero, cacheInfo{}, err
	}
	putPage(ctx, e, key, sf, personal, v, now, ttl, expires)
	return v, cacheInfo{FetchedAt: now.UnixMilli()}, nil
}

// putPage stores v under the full key (storefront-prefixed).
func putPage[T any](ctx context.Context, e *Env, key, sf string, personal bool, v T, now time.Time, ttl time.Duration, expires func(T, time.Time) time.Time) {
	switch r := any(v).(type) {
	case applemusic.MapResponse:
		slimResources(r.Resources)
	case applemusic.ChartsResponse:
		slimResources(r.Resources)
	}
	body, err := json.Marshal(v)
	if err != nil {
		e.Logger.Warn("discover cache: encode", "key", key, "err", err)
		return
	}
	end := now.Add(ttl)
	if expires != nil {
		end = expires(v, now)
	}
	err = e.DB.Q.PutDiscoverCache(ctx, dbq.PutDiscoverCacheParams{
		Key: key, Storefront: sf, Personal: db.Bool(personal), Body: body,
		FetchedAt: db.Millis(now), ExpiresAt: db.Millis(end),
	})
	if err != nil {
		e.Logger.Warn("discover cache: write", "key", key, "err", err)
	}
}

// slimResources drops, in place, the attributes Sona never reads
// (editorial card variants, unused editorial artwork flavours, motion
// artwork other than the square video's playlist URL) so cached pages are
// a fraction of Apple's size and decode fast on cold start.
func slimResources(res applemusic.Resources) {
	keep := map[string]bool{}
	for _, k := range wideKeys {
		keep[k] = true
	}
	for typ, byID := range res {
		for id, n := range byID {
			var obj map[string]json.RawMessage
			if json.Unmarshal(n.Raw, &obj) != nil {
				continue
			}
			var attrs map[string]json.RawMessage
			if json.Unmarshal(obj["attributes"], &attrs) != nil {
				continue
			}
			changed := false
			if _, ok := attrs["plainEditorialCard"]; ok {
				delete(attrs, "plainEditorialCard")
				changed = true
			}
			if raw, ok := attrs["editorialVideo"]; ok {
				var ev applemusic.EditorialVideo
				_ = json.Unmarshal(raw, &ev)
				if sq := ev.Square(); sq != "" {
					attrs["editorialVideo"], _ = json.Marshal(applemusic.EditorialVideo{"motionSquareVideo1x1": {Video: sq}})
				} else {
					delete(attrs, "editorialVideo")
				}
				changed = true
			}
			if raw, ok := attrs["editorialArtwork"]; ok {
				var ea map[string]json.RawMessage
				if json.Unmarshal(raw, &ea) == nil {
					for k := range ea {
						if !keep[k] {
							delete(ea, k)
							changed = true
						}
					}
					attrs["editorialArtwork"], _ = json.Marshal(ea)
				}
			}
			if !changed {
				continue
			}
			obj["attributes"], _ = json.Marshal(attrs)
			b, err := json.Marshal(obj)
			if err != nil {
				continue
			}
			var slim applemusic.Node
			if json.Unmarshal(b, &slim) == nil {
				res[typ][id] = slim
			}
		}
	}
}

// editorialGrouping returns a named editorial grouping ("music" is New,
// "radio" is Radio) as a page.
func (e *Env) editorialGrouping(ctx context.Context, name string) (EditorialPage, error) {
	r, info, err := cachedPage(ctx, e, "grouping|"+name, false, groupingTTL,
		func(ctx context.Context, sf string) (applemusic.MapResponse, error) {
			return applemusic.Grouping(ctx, e.API, sf, name)
		}, nil)
	if err != nil {
		return EditorialPage{}, err
	}
	return e.m.groupingPage(r, info), nil
}

// groupingPage maps a groupings response (data[0] is the grouping).
func (m mapper) groupingPage(r applemusic.MapResponse, info cacheInfo) EditorialPage {
	p := EditorialPage{Kind: ItemGrouping, Sections: []Section{}, FetchedAt: info.FetchedAt, Stale: info.Stale}
	nodes := r.Primary()
	if len(nodes) == 0 {
		return p
	}
	g := nodes[0]
	var a applemusic.GroupingAttributes
	_ = g.Attrs(&a)
	p.ID, p.Title = g.ID, a.Name
	for _, tab := range r.Resources.Resolve(g.Rel("tabs")) {
		p.Sections = append(p.Sections, m.sections(r.Resources, tab)...)
	}
	return p
}

// sections maps a page root (or any container element) to its sections.
func (m mapper) sections(res applemusic.Resources, root applemusic.Node) []Section {
	out := []Section{}
	for _, el := range res.Resolve(root.Rel("children")) {
		if el.Type != applemusic.TypeEditorialElements {
			continue
		}
		if s, ok := m.section(res, el); ok {
			out = append(out, s)
		}
	}
	return out
}

// section maps one editorial element; ok is false for empty sections.
func (m mapper) section(res applemusic.Resources, el applemusic.Node) (Section, bool) {
	var a applemusic.EditorialElementAttributes
	_ = el.Attrs(&a)
	s := Section{ID: el.ID, Title: a.Heading(), Items: []Item{}}
	if room := el.Rel("room"); len(room) > 0 {
		s.Room = room[0].ID
	}
	switch a.Kind {
	case applemusic.ElementText:
		s.Kind, s.Text = SectionText, strings.TrimSpace(a.Description)
		return s, s.Text != ""
	case applemusic.ElementHero:
		s.Kind = SectionHero
		for _, card := range res.Resolve(el.Rel("children")) {
			if it, ok := m.heroCard(res, card); ok {
				s.Items = append(s.Items, it)
			}
		}
	case applemusic.ElementBrick:
		s.Kind = SectionBricks
		for _, tile := range res.Resolve(el.Rel("children")) {
			if it, ok := m.brick(res, tile); ok {
				s.Items = append(s.Items, it)
			}
		}
	case applemusic.ElementLinks, applemusic.ElementLinksAlt:
		s.Kind = SectionLinks
		for _, l := range a.Links {
			s.Items = append(s.Items, linkItem(l.Label, l.URL))
		}
	default:
		s.Kind = SectionShelf
		if a.Kind == applemusic.ElementLive {
			s.Kind = SectionLive
		}
		s.Items = m.items(res, el.Rel("contents"))
		switch {
		case a.Kind == applemusic.ElementTracks || allKind(s.Items, ItemSong):
			s.Kind = SectionTracks
		case allBroadcasters(s.Items):
			s.Kind = SectionStations
		}
		if len(s.Items) == 0 {
			// A container without contents of its own (nested sections).
			nested := m.sections(res, el)
			if len(nested) == 1 {
				return nested[0], true
			}
			for _, n := range nested {
				s.Items = append(s.Items, n.Items...)
			}
		}
	}
	return s, len(s.Items) > 0
}

// allBroadcasters reports a section of local broadcasters only (Apple's
// "Local Broadcasters" and "International Broadcasters" shelves), which
// read as a list of stations rather than a row of covers.
func allBroadcasters(items []Item) bool {
	if len(items) == 0 {
		return false
	}
	for _, it := range items {
		if it.Station == nil || it.Station.Kind != StationBroadcast {
			return false
		}
	}
	return true
}

func allKind(items []Item, kind string) bool {
	for _, it := range items {
		if it.Kind != kind {
			return false
		}
	}
	return len(items) > 0
}

// items maps references, dropping kinds Sona cannot show.
func (m mapper) items(res applemusic.Resources, refs []applemusic.Ref) []Item {
	out := make([]Item, 0, len(refs))
	for _, ref := range refs {
		n, ok := res.Get(ref)
		if !ok {
			continue
		}
		if it, ok := m.amItem(n); ok {
			it.Caption = cmp.Or(sentenceCase(ref.Reason()), it.Caption)
			out = append(out, it)
		}
	}
	return out
}

// heroCard maps a hero card (317: its content's editorial art; 320: its own
// art and tagline).
func (m mapper) heroCard(res applemusic.Resources, card applemusic.Node) (Item, bool) {
	var a applemusic.EditorialElementAttributes
	_ = card.Attrs(&a)
	var it Item
	ok := false
	if refs := card.Rel("contents"); len(refs) > 0 {
		if n, found := res.Get(refs[0]); found {
			it, ok = m.amItem(n)
		}
	}
	if !ok && a.Link != nil {
		it, ok = linkItem(a.DesignTag, a.Link.URL), true
	}
	if !ok {
		return Item{}, false
	}
	if a.Artwork != nil {
		it.Wide, it.Color = m.artwork(a.Artwork), hexColor(a.Artwork.BgColor)
	}
	if a.DesignTag != "" && a.Kind == applemusic.ElementHeroArt {
		it.Subtitle = a.DesignTag
	}
	it.Caption = cmp.Or(sentenceCase(a.DesignBadge), it.Caption)
	return it, it.Wide.Large != "" || it.Art.Large != ""
}

// brick maps a brick tile (386: its content; 394: own art with a link or
// content).
func (m mapper) brick(res applemusic.Resources, tile applemusic.Node) (Item, bool) {
	var a applemusic.EditorialElementAttributes
	_ = tile.Attrs(&a)
	var it Item
	ok := false
	if refs := tile.Rel("contents"); len(refs) > 0 {
		if n, found := res.Get(refs[0]); found {
			it, ok = m.amItem(n)
		}
	}
	if !ok && a.Link != nil {
		it, ok = linkItem(a.DesignTag, a.Link.URL), true
	}
	if !ok {
		return Item{}, false
	}
	if a.Artwork != nil {
		it.Wide, it.Color = m.artwork(a.Artwork), hexColor(a.Artwork.BgColor)
	}
	if a.DesignTag != "" {
		it.Name = a.DesignTag
	}
	return it, it.Kind != ItemLink || it.URL != ""
}

// editorialArt is the artwork part of any resource's attributes.
type editorialArt struct {
	Artwork          *applemusic.Artwork           `json:"artwork"`
	EditorialArtwork map[string]applemusic.Artwork `json:"editorialArtwork"`
	URL              string                        `json:"url"`
	ArtistNames      string                        `json:"artistNames"`
}

// wideKeys are the editorial artwork flavours usable as a wide banner, in
// order of preference. subscriptionHero (4:1) is what music.apple.com's hero
// shelf ("flowcaseLockup") shows: its centre is the safe area, and the web
// player crops it to the card ("sr"). storeFlowcase is the same 4:1 shape;
// superHeroWide (2.4:1) and subscriptionCover (4:1 or square) only stand in.
var wideKeys = []string{"subscriptionHero", "storeFlowcase", "superHeroWide", "subscriptionCover"}

func (m mapper) wideOf(ea editorialArt) (Art, string) {
	for _, k := range wideKeys {
		if a, ok := ea.EditorialArtwork[k]; ok && a.URL != "" {
			return m.artwork(&a), hexColor(a.BgColor)
		}
	}
	return Art{}, ""
}

func hexColor(bg string) string {
	if len(bg) != 6 {
		return ""
	}
	return "#" + strings.ToLower(bg)
}

// amItem maps any resource to an Item; ok is false for kinds Sona does not
// show (social profiles, upsells, activities).
func (m mapper) amItem(n applemusic.Node) (Item, bool) {
	var ea editorialArt
	_ = n.Attrs(&ea)
	it := Item{ID: n.ID, URL: ea.URL}
	it.Wide, it.Color = m.wideOf(ea)
	if it.Color == "" && ea.Artwork != nil {
		it.Color = hexColor(ea.Artwork.BgColor)
	}
	switch n.Type {
	case applemusic.TypeAlbums, applemusic.TypeLibraryAlbums:
		v, err := applemusic.As[applemusic.Album](n)
		if err != nil {
			return Item{}, false
		}
		a := m.amAlbum(v)
		it.Kind, it.ID, it.Name, it.Subtitle, it.Art, it.Explicit, it.Album = ItemAlbum, a.ID, a.Name, a.Artist, a.Art, a.Explicit, &a
	case applemusic.TypePlaylists, applemusic.TypeLibraryPlaylists:
		v, err := applemusic.As[applemusic.Playlist](n)
		if err != nil {
			return Item{}, false
		}
		p := m.amPlaylist(v)
		it.Kind, it.ID, it.Name, it.Art, it.Playlist = ItemPlaylist, p.ID, p.Name, p.Art, &p
		it.Subtitle = cmp.Or(p.Curator, ea.ArtistNames)
	case applemusic.TypeSongs, applemusic.TypeLibrarySongs:
		v, err := applemusic.As[applemusic.Song](n)
		if err != nil {
			return Item{}, false
		}
		s := m.amSong(v)
		it.Kind, it.ID, it.Name, it.Subtitle, it.Art, it.Explicit, it.Song = ItemSong, s.ID, s.Name, s.Artist, s.Art, s.Explicit, &s
	case applemusic.TypeArtists, applemusic.TypeLibraryArtists:
		v, err := applemusic.As[applemusic.Artist](n)
		if err != nil {
			return Item{}, false
		}
		a := m.amArtist(v)
		it.Kind, it.ID, it.Name, it.Subtitle, it.Art, it.Artist = ItemArtist, a.ID, a.Name, a.Genre, a.Art, &a
	case applemusic.TypeStations:
		v, err := applemusic.As[applemusic.Station](n)
		if err != nil {
			return Item{}, false
		}
		s := m.amStation(v)
		it.Kind, it.Name, it.Art, it.Station = ItemStation, s.Name, s.Art, &s
		it.Subtitle = cmp.Or(s.ShowName, s.Tagline)
	case applemusic.TypeMusicVideos, applemusic.TypeLibraryMusicVideos:
		v, err := applemusic.As[applemusic.MusicVideo](n)
		if err != nil {
			return Item{}, false
		}
		mv := m.amMusicVideo(v)
		it.Kind, it.ID, it.Name, it.Subtitle, it.Art, it.Explicit, it.MusicVideo = ItemMusicVideo, mv.ID, mv.Name, mv.Artist, mv.Art, mv.Explicit, &mv
	case applemusic.TypeAppleCurators, applemusic.TypeCurators:
		var a applemusic.CuratorAttributes
		_ = n.Attrs(&a)
		it.Kind, it.Name, it.Subtitle = ItemCurator, cmp.Or(a.ShortName, a.Name), curatorKind(a)
		it.Art = m.artwork(a.Artwork)
	case applemusic.TypeRooms:
		var a applemusic.RoomAttributes
		_ = n.Attrs(&a)
		it.Kind, it.Name = ItemRoom, a.Title
	case applemusic.TypeMultirooms:
		var a applemusic.MultiroomAttributes
		_ = n.Attrs(&a)
		it.Kind, it.Name = ItemMultiroom, a.Title
		if a.Uber != nil && a.Uber.MasterArt != nil {
			it.Wide, it.Color = m.artwork(a.Uber.MasterArt), hexColor(a.Uber.MasterArt.BgColor)
		}
	case applemusic.TypeGroupings:
		var a applemusic.GroupingAttributes
		_ = n.Attrs(&a)
		it.Kind, it.Name = ItemGrouping, a.Name
	case applemusic.TypeEditorialItems:
		var a applemusic.EditorialItemAttributes
		_ = n.Attrs(&a)
		target := a.URL
		if a.Link != nil && a.Link.URL != "" {
			target = a.Link.URL
		}
		li := linkItem("", target)
		it.Kind, it.ID, it.URL = li.Kind, li.ID, li.URL
		if a.PlainEditorialNote != nil {
			it.Name, it.Subtitle = a.PlainEditorialNote.Name, a.PlainEditorialNote.Short
		}
		if c, ok := a.EditorialArtwork["subscriptionCover"]; ok {
			it.Wide, it.Color = m.artwork(&c), hexColor(c.BgColor)
		}
	case applemusic.TypeUploadedVideos, applemusic.TypeMusicMovies:
		var a struct {
			Name       string              `json:"name"`
			ArtistName string              `json:"artistName"`
			Artwork    *applemusic.Artwork `json:"artwork"`
		}
		_ = n.Attrs(&a)
		if ea.URL == "" {
			return Item{}, false
		}
		it.Kind, it.Name, it.Subtitle, it.Art = ItemLink, a.Name, a.ArtistName, m.artwork(a.Artwork)
	default:
		return Item{}, false
	}
	return it, it.Name != "" || it.Kind == ItemLink
}

func curatorKind(a applemusic.CuratorAttributes) string {
	switch a.Kind {
	case "Show":
		if a.ShowHostName != "" {
			return "Radio show with " + a.ShowHostName
		}
		return "Radio show"
	case "Genre":
		return "Category"
	default:
		return "Curator"
	}
}

// linkItem maps an Apple link to the page it opens in Sona, or to an
// ItemLink for pages that exist only on the web.
func linkItem(label, raw string) Item {
	kind, id := parseAppleLink(raw)
	if kind == "" {
		return Item{Kind: ItemLink, Name: label, URL: raw}
	}
	return Item{Kind: kind, ID: id, Name: label, URL: raw}
}

// parseAppleLink resolves an editorial link to an Item kind and ID:
// music.apple.com pages (/curator/…/id, /room/id, /multi-room/id,
// /grouping/id, /album/…/id, /playlist/…/id, /artist/…/id, /station/…/id,
// /music-video/…/id, /new/top-charts, /concerts and a concert's page, which
// open Concerts; a concert's artist page, /concerts/artist/id, opens the
// artist) and legacy store URLs
// (…/wa/viewMultiRoom?fcId=, viewRoom?fcId=, viewGrouping?id=,
// viewTop?genreId=, /collection/…?fcId=). kind is "" for anything else.
func parseAppleLink(raw string) (kind, id string) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	host := strings.ToLower(u.Hostname())
	if host != "music.apple.com" && host != "itunes.apple.com" && host != "beta.music.apple.com" {
		return "", ""
	}
	q := u.Query()
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	last := parts[len(parts)-1]
	switch {
	case strings.HasSuffix(u.Path, "/wa/viewMultiRoom"):
		return ItemMultiroom, q.Get("fcId")
	case strings.HasSuffix(u.Path, "/wa/viewRoom"):
		return ItemRoom, q.Get("fcId")
	case strings.HasSuffix(u.Path, "/wa/viewGrouping"):
		return ItemGrouping, q.Get("id")
	case strings.HasSuffix(u.Path, "/wa/viewTop"):
		return ItemCharts, normaliseGenre(q.Get("genreId"))
	}
	if q.Get("fcId") != "" && slices.Contains(parts, "collection") {
		return ItemRoom, q.Get("fcId")
	}
	// Concerts: /concerts, /{sf}/concerts and /{sf}/concerts/{id} (a
	// concert: the page lists them; its artist form is matched below).
	if ci := slices.Index(parts, "concerts"); ci >= 0 && ci <= 1 && !slices.Contains(parts, "artist") {
		return ItemConcerts, ""
	}
	// Path forms: /{sf}/{kind}/{slug?}/{id}.
	if len(parts) < 2 {
		return "", ""
	}
	switch {
	case slices.Contains(parts, "top-charts"):
		if len(parts) > 0 && isDigits(last) && last != "top-charts" {
			return ItemCharts, normaliseGenre(last)
		}
		return ItemCharts, ""
	}
	kinds := map[string]string{
		"curator": ItemCurator, "room": ItemRoom, "multi-room": ItemMultiroom, "grouping": ItemGrouping,
		"album": ItemAlbum, "playlist": ItemPlaylist, "artist": ItemArtist, "station": ItemStation,
		"music-video": ItemMusicVideo,
	}
	for i, p := range parts {
		if k, ok := kinds[p]; ok && i < len(parts)-1 {
			if k == ItemAlbum && q.Get("i") != "" {
				return ItemAlbum, last // the song's album; the song itself plays from there
			}
			return k, last
		}
	}
	return "", ""
}

func normaliseGenre(g string) string {
	if g == applemusic.GenreAllMusic {
		return ""
	}
	return g
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// sentenceCase turns Apple's upper-case badges ("NEW ALBUM", "ONLY ON
// APPLE MUSIC") into sentence case, keeping a few proper names.
func sentenceCase(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ToUpper(s) != s {
		return s
	}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		switch w {
		case "dj", "ep", "uk", "us":
			words[i] = strings.ToUpper(w)
		case "apple":
			words[i] = "Apple"
		case "music":
			if i > 0 && words[i-1] == "Apple" {
				words[i] = "Music"
			}
		}
	}
	out := strings.Join(words, " ")
	return strings.ToUpper(out[:1]) + out[1:]
}
