package services

import (
	"cmp"
	"context"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/catalog"
)

// Detail pages (album, artist, song, music video), on CatalogService.
// Everything comes from amp-api through the catalog cache.

// Album returns a catalog album page: its tracks (disc and track order)
// and its sections.
func (s *CatalogService) Album(ctx context.Context, id string) (CatalogAlbum, error) {
	a, err := s.c.Album(ctx, id)
	if err != nil {
		return CatalogAlbum{}, s.fail(err)
	}
	return s.albumPage(ctx, a), nil
}

func (s *CatalogService) albumPage(ctx context.Context, a applemusic.Album) CatalogAlbum {
	at := a.Attributes
	tracks := mapSlice(a.Relationships.Tracks.Data, s.m.amSong)
	album := s.m.amAlbum(a)
	for i := range tracks {
		if tracks[i].AlbumID == "" {
			tracks[i].AlbumID = album.ID
		}
		if tracks[i].ArtistID == "" {
			tracks[i].ArtistID = album.ArtistID
		}
	}
	if album.Prerelease {
		markUnreleased(tracks, s.m.clock())
	}
	notes, short := "", ""
	if n := at.EditorialNotes; n != nil {
		notes, short = plainText(n.Standard), plainText(n.Short)
	}
	if notes == short {
		short = ""
	}
	v := a.Views
	p := CatalogAlbum{
		AlbumDetail: AlbumDetail{
			Album:      album,
			Tracks:     tracks,
			DurationMs: totalMs(tracks),
			Label:      at.RecordLabel,
			Copyright:  at.Copyright,
			Notes:      notes,
			Header:     s.m.header(ctx, at.Artwork.Template()),
		},
		NotesShort:    short,
		Artists:       mapSlice(a.Relationships.Artists.Data, s.m.amArtist),
		PopularIDs:    popularTracks(a.Relationships.Tracks.Data),
		URL:           at.URL,
		ArtworkColors: artworkColors(at.Artwork),
		Genres:        albumGenres(at.GenreNames, a.Relationships.Genres.Data),
		MoreByArtist:  mapSlice(v.MoreByArtist.Data, s.m.amAlbum),
		Similar:       mapSlice(v.YouMightAlsoLike.Data, s.m.amAlbum),
		OtherVersions: mapSlice(v.OtherVersions.Data, s.m.amAlbum),
		AppearsOn:     mapSlice(v.AppearsOn.Data, s.m.amPlaylist),
		Videos:        mapSlice(v.RelatedVideos.Data, s.m.amMusicVideo),
		MoreToHear:    mapSlice(v.AudioExtras.Data, s.m.amStation),
	}
	p.Header.Motion = s.m.motionOf(at.EditorialVideo)
	p.Album.LibraryID, p.InLibrary = s.e.members.lookup(ctx, kindAlbum, album.CatalogID)
	return p
}

// artworkColors returns Apple's colours for a cover.
func artworkColors(a *applemusic.Artwork) ArtworkColors {
	if a == nil {
		return ArtworkColors{}
	}
	return ArtworkColors{
		Background: hexColor(a.BgColor),
		Text1:      hexColor(a.TextColor1),
		Text2:      hexColor(a.TextColor2),
		Text3:      hexColor(a.TextColor3),
		Text4:      hexColor(a.TextColor4),
	}
}

// albumGenres returns an album's genre names in Apple's order without the
// root genre ("Music", localised), which the genres relationship names as
// the one without a parent. Names repeated by Apple appear once.
func albumGenres(names []string, genres []applemusic.Resource[applemusic.GenreAttributes]) []string {
	root := ""
	for _, g := range genres {
		if g.Attributes.ParentID == "" {
			root = g.Attributes.Name
		}
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if n != "" && n != root && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// Popular-track policy: Apple scores an album's tracks 0–1 (1 = its most
// played); Sona marks up to maxPopular tracks scoring at least
// popularFloor.
const (
	popularFloor = 0.8
	maxPopular   = 3
)

// popularTracks returns the IDs of an album's most played tracks.
func popularTracks(tracks []applemusic.Song) []string {
	type scored struct {
		id string
		p  float64
	}
	if len(tracks) < 4 {
		return []string{} // marking most of a single or EP says nothing
	}
	var top []scored
	for _, t := range tracks {
		if t.Meta.Popularity >= popularFloor {
			top = append(top, scored{t.ID, t.Meta.Popularity})
		}
	}
	slices.SortStableFunc(top, func(a, b scored) int { return cmp.Compare(b.p, a.p) })
	out := make([]string, 0, maxPopular)
	for _, t := range top[:min(len(top), maxPopular)] {
		out = append(out, t.id)
	}
	return out
}

// artistShelves are the artist page's sections after Top songs, in display
// order. A video shelf falls back to the next view of the same kind when
// it is empty (Top videos → Music videos).
var artistShelves = []string{
	ViewFeaturedAlbums, ViewFullAlbums, ViewSingles, ViewLiveAlbums, ViewCompilations,
	ViewAppearsOn, ViewTopMusicVideos, ViewPlaylists, ViewSimilarArtists,
}

// viewKinds maps every artist view Sona shows to its item kind.
var viewKinds = map[string]ShelfKind{
	ViewTopSongs:       ShelfSongs,
	ViewFeaturedAlbums: ShelfAlbums,
	ViewFullAlbums:     ShelfAlbums,
	ViewSingles:        ShelfAlbums,
	ViewLiveAlbums:     ShelfAlbums,
	ViewCompilations:   ShelfAlbums,
	ViewAppearsOn:      ShelfAlbums,
	ViewPlaylists:      ShelfPlaylists,
	ViewMusicVideos:    ShelfVideos,
	ViewTopMusicVideos: ShelfVideos,
	ViewSimilarArtists: ShelfArtists,
}

// Artist returns a catalog artist page: biography, latest release, top
// songs and the discography, video, playlist and similar-artist shelves.
func (s *CatalogService) Artist(ctx context.Context, id string) (CatalogArtist, error) {
	a, err := s.c.Artist(ctx, id)
	if err != nil {
		return CatalogArtist{}, s.fail(err)
	}
	return s.artistPage(ctx, a), nil
}

func (s *CatalogService) artistPage(ctx context.Context, a applemusic.Artist) CatalogArtist {
	v, at := a.Views, a.Attributes
	p := CatalogArtist{
		Artist:       s.m.amArtist(a),
		Header:       s.m.header(ctx, at.Artwork.Template()),
		Bio:          plainText(at.ArtistBio),
		Origin:       at.Origin,
		BornOrFormed: at.BornOrFormed,
		Genres:       nonNil(at.GenreNames),
		URL:          at.URL,
		TopSongs:     mapSlice(v.TopSongs.Data, s.m.amSong),
		TopSongsMore: v.TopSongs.Next != "",
		Shelves:      []ArtistShelf{},
	}
	p.Header.Motion = s.m.motionOf(at.EditorialVideo)
	if d := v.LatestRelease.Data; len(d) > 0 {
		al := s.m.amAlbum(d[0])
		p.LatestRelease = &al
	}
	for _, name := range artistShelves {
		sh := s.shelf(v, name)
		if name == ViewTopMusicVideos && len(sh.Videos) == 0 {
			sh = s.shelf(v, ViewMusicVideos)
		}
		if len(sh.Albums)+len(sh.Playlists)+len(sh.Videos)+len(sh.Artists) > 0 {
			p.Shelves = append(p.Shelves, sh)
		}
	}
	return p
}

// shelf maps artist view name of v.
func (s *CatalogService) shelf(v applemusic.ArtistViews, name string) ArtistShelf {
	sh := ArtistShelf{View: name, Kind: viewKinds[name], Albums: []Album{}, Playlists: []Playlist{}, Videos: []MusicVideo{}, Artists: []Artist{}}
	albums := func(x applemusic.View[applemusic.Album]) {
		sh.Albums, sh.More = mapSlice(x.Data, s.m.amAlbum), x.Next != ""
	}
	switch name {
	case ViewFeaturedAlbums:
		albums(v.FeaturedAlbums)
	case ViewFullAlbums:
		albums(v.FullAlbums)
	case ViewSingles:
		albums(v.Singles)
	case ViewLiveAlbums:
		albums(v.LiveAlbums)
	case ViewCompilations:
		albums(v.CompilationAlbums)
	case ViewAppearsOn:
		albums(v.AppearsOnAlbums)
	case ViewPlaylists:
		sh.Playlists, sh.More = mapSlice(v.Playlists.Data, s.m.amPlaylist), v.Playlists.Next != ""
	case ViewTopMusicVideos:
		sh.Videos, sh.More = mapSlice(v.TopMusicVideos.Data, s.m.amMusicVideo), v.TopMusicVideos.Next != ""
	case ViewMusicVideos:
		sh.Videos, sh.More = mapSlice(v.MusicVideos.Data, s.m.amMusicVideo), v.MusicVideos.Next != ""
	case ViewSimilarArtists:
		sh.Artists, sh.More = mapSlice(v.SimilarArtists.Data, s.m.amArtist), v.SimilarArtists.Next != ""
	}
	return sh
}

// ArtistView returns one page of an artist's view (a see-all list): view is
// one of the View* names, offset comes from the previous page's Next, and
// limit is at most 100 (0 means 50).
func (s *CatalogService) ArtistView(ctx context.Context, id, view string, offset, limit int) (ArtistViewPage, error) {
	kind, ok := viewKinds[view]
	if !ok {
		return ArtistViewPage{}, fmt.Errorf("%w: artist view %q", ErrInvalidArgument, view)
	}
	if limit <= 0 {
		limit = 50
	}
	limit, offset = min(limit, 100), max(offset, 0)
	p := ArtistViewPage{
		View: view, Kind: kind, Next: -1,
		Songs: []Song{}, Albums: []Album{}, Playlists: []Playlist{}, Videos: []MusicVideo{}, Artists: []Artist{},
	}
	var (
		next string
		err  error
	)
	switch kind {
	case ShelfSongs:
		var r applemusic.View[applemusic.Song]
		r, err = catalog.ArtistView[applemusic.Song](ctx, s.c, id, view, offset, limit)
		p.Songs, next = mapSlice(r.Data, s.m.amSong), r.Next
	case ShelfAlbums:
		var r applemusic.View[applemusic.Album]
		r, err = catalog.ArtistView[applemusic.Album](ctx, s.c, id, view, offset, limit)
		p.Albums, next = mapSlice(r.Data, s.m.amAlbum), r.Next
	case ShelfPlaylists:
		var r applemusic.View[applemusic.Playlist]
		r, err = catalog.ArtistView[applemusic.Playlist](ctx, s.c, id, view, offset, limit)
		p.Playlists, next = mapSlice(r.Data, s.m.amPlaylist), r.Next
	case ShelfVideos:
		var r applemusic.View[applemusic.MusicVideo]
		r, err = catalog.ArtistView[applemusic.MusicVideo](ctx, s.c, id, view, offset, limit)
		p.Videos, next = mapSlice(r.Data, s.m.amMusicVideo), r.Next
	case ShelfArtists:
		var r applemusic.View[applemusic.Artist]
		r, err = catalog.ArtistView[applemusic.Artist](ctx, s.c, id, view, offset, limit)
		p.Artists, next = mapSlice(r.Data, s.m.amArtist), r.Next
	}
	if err != nil {
		return ArtistViewPage{}, s.fail(err)
	}
	p.Next = applemusic.NextOffset(next)
	return p, nil
}

// SongPage returns a catalog song page: the song with its album, artists,
// credits, lyrics excerpt, music videos and more albums by its artist.
func (s *CatalogService) SongPage(ctx context.Context, id string) (CatalogSong, error) {
	so, err := s.c.SongPage(ctx, id)
	if err != nil {
		return CatalogSong{}, s.fail(err)
	}
	at, rel := so.Attributes, so.Relationships
	p := CatalogSong{
		Song:                s.m.amSong(so),
		Artists:             mapSlice(rel.Artists.Data, s.m.amArtist),
		Composer:            at.ComposerName,
		ISRC:                at.ISRC,
		URL:                 at.URL,
		HasTimeSyncedLyrics: at.HasTimeSyncedLyrics,
		LyricsExcerpt:       strings.TrimSpace(at.LyricsExcerpt),
		Credits:             s.m.creditGroups(rel.Credits.Data),
		Videos:              mapSlice(rel.MusicVideos.Data, s.m.amMusicVideo),
		MoreByArtist:        []Album{},
	}
	if d := rel.Albums.Data; len(d) > 0 {
		al := s.m.amAlbum(d[0])
		p.Album = &al
	}
	var wg sync.WaitGroup
	wg.Go(func() { p.Header = s.m.header(ctx, at.Artwork.Template()) })
	if p.Song.ArtistID != "" {
		wg.Go(func() {
			// "More by" comes from the artist page (usually cached already);
			// a failure only drops the shelf.
			a, err := s.c.Artist(ctx, p.Song.ArtistID)
			if err != nil {
				s.e.Report(err)
				return
			}
			for _, al := range mapSlice(a.Views.FullAlbums.Data, s.m.amAlbum) {
				if p.Album == nil || al.ID != p.Album.ID {
					p.MoreByArtist = append(p.MoreByArtist, al)
				}
			}
		})
	}
	wg.Wait()
	return p, nil
}

// creditGroups maps Apple's role categories, dropping empty ones. A person
// has a picture when Apple sends artwork that is not its generated
// initials image; their pictures come with the song page, so the credits
// cost no request of their own and the images load lazily from /art/.
func (m mapper) creditGroups(in []applemusic.CreditCategory) []CreditGroup {
	out := make([]CreditGroup, 0, len(in))
	for _, c := range in {
		g := CreditGroup{Kind: c.Attributes.Kind, Title: c.Attributes.Title, Credits: []Credit{}}
		for _, a := range c.Relationships.CreditArtists.Data {
			if a.Attributes.Name != "" {
				cr := Credit{Name: a.Attributes.Name, Roles: nonNil(a.Attributes.RoleNames)}
				if !a.Attributes.Artwork.Generated() {
					cr.Art = m.centreSquare(a.Attributes.Artwork)
				}
				g.Credits = append(g.Credits, cr)
			}
		}
		if len(g.Credits) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// MusicVideo returns a catalog music video page. Videos cannot be played
// in Sona; OpenInAppleMusic opens Video.URL.
func (s *CatalogService) MusicVideo(ctx context.Context, id string) (CatalogVideo, error) {
	v, err := s.c.MusicVideo(ctx, id)
	if err != nil {
		return CatalogVideo{}, s.fail(err)
	}
	rel := v.Relationships
	p := CatalogVideo{
		Video:        s.m.amMusicVideo(v),
		Header:       s.m.header(ctx, v.Attributes.Artwork.Template()),
		Notes:        plainText(v.Attributes.EditorialNotes.Text()),
		Artists:      mapSlice(rel.Artists.Data, s.m.amArtist),
		MoreByArtist: mapSlice(v.Views.MoreByArtist.Data, s.m.amMusicVideo),
		MoreInGenre:  mapSlice(v.Views.MoreInGenre.Data, s.m.amMusicVideo),
	}
	if d := rel.Songs.Data; len(d) > 0 && d[0].Attributes.Name != "" {
		so := s.m.amSong(d[0])
		p.Song = &so
	}
	return p, nil
}

// OpenInAppleMusic opens a music.apple.com page (an item's url) in the
// system browser, e.g. a music video Sona cannot play.
func (s *CatalogService) OpenInAppleMusic(url string) error {
	if !strings.HasPrefix(url, "https://music.apple.com/") {
		return ErrNotAppleMusicLink
	}
	return s.e.OpenURL(url)
}

var (
	reBreak  = regexp.MustCompile(`(?i)<br\s*/?>`)
	rePara   = regexp.MustCompile(`(?i)</p\s*>`)
	reTag    = regexp.MustCompile(`<[^>]*>`)
	reBlanks = regexp.MustCompile(`\n{3,}`)
)

// plainText turns Apple's lightly marked-up notes and biographies (<i>,
// <b>, <br>, <p>) into plain text: line breaks kept, paragraphs separated
// by one blank line.
func plainText(s string) string {
	if s == "" {
		return ""
	}
	s = reBreak.ReplaceAllString(s, "\n")
	s = rePara.ReplaceAllString(s, "\n\n")
	s = html.UnescapeString(reTag.ReplaceAllString(s, ""))
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimSpace(ln)
	}
	return strings.TrimSpace(reBlanks.ReplaceAllString(strings.Join(lines, "\n"), "\n\n"))
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
