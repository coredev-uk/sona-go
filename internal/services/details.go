package services

import (
	"context"
	"errors"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/catalog"
)

// Detail pages (album, artist, song, music video) and lyrics, on
// CatalogService. Everything comes from amp-api through the catalog cache.

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
	tracks := s.m.amSongs(a.Relationships.Tracks.Data)
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
	notes := ""
	if n := at.EditorialNotes; n != nil {
		notes = plainText(n.Standard)
	}
	short := ""
	if n := at.EditorialNotes; n != nil {
		short = plainText(n.Short)
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
		Artists:       s.m.amArtists(a.Relationships.Artists.Data),
		PopularIDs:    popularTracks(a.Relationships.Tracks.Data),
		URL:           at.URL,
		MoreByArtist:  s.m.amAlbums(v.MoreByArtist.Data),
		Similar:       s.m.amAlbums(v.YouMightAlsoLike.Data),
		OtherVersions: s.m.amAlbums(v.OtherVersions.Data),
		AppearsOn:     s.m.amPlaylists(v.AppearsOn.Data),
		Videos:        s.m.amMusicVideos(v.RelatedVideos.Data),
	}
	p.Header.Motion = s.m.motionOf(at.EditorialVideo)
	p.Album.LibraryID, p.InLibrary = s.e.members.lookup(ctx, kindAlbum, album.CatalogID)
	return p
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
	var top []scored
	for _, t := range tracks {
		if t.Meta.Popularity >= popularFloor {
			top = append(top, scored{t.ID, t.Meta.Popularity})
		}
	}
	if len(tracks) < 4 {
		top = nil // marking most of a single or EP says nothing
	}
	slices.SortStableFunc(top, func(a, b scored) int {
		switch {
		case a.p > b.p:
			return -1
		case a.p < b.p:
			return 1
		}
		return 0
	})
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
		TopSongs:     s.m.amSongs(v.TopSongs.Data),
		TopSongsMore: v.TopSongs.Next != "",
		Shelves:      []ArtistShelf{},
	}
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
		sh.Albums, sh.More = s.m.amAlbums(x.Data), x.Next != ""
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
		sh.Playlists, sh.More = s.m.amPlaylists(v.Playlists.Data), v.Playlists.Next != ""
	case ViewTopMusicVideos:
		sh.Videos, sh.More = s.m.amMusicVideos(v.TopMusicVideos.Data), v.TopMusicVideos.Next != ""
	case ViewMusicVideos:
		sh.Videos, sh.More = s.m.amMusicVideos(v.MusicVideos.Data), v.MusicVideos.Next != ""
	case ViewSimilarArtists:
		sh.Artists, sh.More = s.m.amArtists(v.SimilarArtists.Data), v.SimilarArtists.Next != ""
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
		p.Songs, next = s.m.amSongs(r.Data), r.Next
	case ShelfAlbums:
		var r applemusic.View[applemusic.Album]
		r, err = catalog.ArtistView[applemusic.Album](ctx, s.c, id, view, offset, limit)
		p.Albums, next = s.m.amAlbums(r.Data), r.Next
	case ShelfPlaylists:
		var r applemusic.View[applemusic.Playlist]
		r, err = catalog.ArtistView[applemusic.Playlist](ctx, s.c, id, view, offset, limit)
		p.Playlists, next = s.m.amPlaylists(r.Data), r.Next
	case ShelfVideos:
		var r applemusic.View[applemusic.MusicVideo]
		r, err = catalog.ArtistView[applemusic.MusicVideo](ctx, s.c, id, view, offset, limit)
		p.Videos, next = s.m.amMusicVideos(r.Data), r.Next
	case ShelfArtists:
		var r applemusic.View[applemusic.Artist]
		r, err = catalog.ArtistView[applemusic.Artist](ctx, s.c, id, view, offset, limit)
		p.Artists, next = s.m.amArtists(r.Data), r.Next
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
		Artists:             s.m.amArtists(rel.Artists.Data),
		Composer:            at.ComposerName,
		ISRC:                at.ISRC,
		URL:                 at.URL,
		HasTimeSyncedLyrics: at.HasTimeSyncedLyrics,
		LyricsExcerpt:       strings.TrimSpace(at.LyricsExcerpt),
		Credits:             creditGroups(rel.Credits.Data),
		Videos:              s.m.amMusicVideos(rel.MusicVideos.Data),
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
			for _, al := range s.m.amAlbums(a.Views.FullAlbums.Data) {
				if p.Album == nil || al.ID != p.Album.ID {
					p.MoreByArtist = append(p.MoreByArtist, al)
				}
			}
		})
	}
	wg.Wait()
	return p, nil
}

// creditGroups maps Apple's role categories, dropping empty ones.
func creditGroups(in []applemusic.CreditCategory) []CreditGroup {
	out := make([]CreditGroup, 0, len(in))
	for _, c := range in {
		g := CreditGroup{Kind: c.Attributes.Kind, Title: c.Attributes.Title, Credits: []Credit{}}
		for _, a := range c.Relationships.CreditArtists.Data {
			if a.Attributes.Name != "" {
				g.Credits = append(g.Credits, Credit{Name: a.Attributes.Name, Roles: nonNil(a.Attributes.RoleNames)})
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
		Artists:      s.m.amArtists(rel.Artists.Data),
		MoreByArtist: s.m.amMusicVideos(v.Views.MoreByArtist.Data),
		MoreInGenre:  s.m.amMusicVideos(v.Views.MoreInGenre.Data),
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
		return fmt.Errorf("%w: not an Apple Music link", ErrInvalidArgument)
	}
	return s.e.OpenURL(url)
}

// Lyrics returns the lyrics of catalog song songID, word-timed where
// Apple has them, with a translation into the account's language and a
// Latin-script pronunciation when Apple has them. Songs without lyrics
// fail with code not_found.
func (s *CatalogService) Lyrics(ctx context.Context, songID string) (Lyrics, error) {
	lang := s.e.Auth.Status().Language
	l, err := s.c.Lyrics(ctx, songID, applemusic.LyricsOptions{Syllable: true, Language: lang})
	if err != nil {
		if errors.Is(err, applemusic.ErrNotFound) {
			return Lyrics{}, err
		}
		return Lyrics{}, s.fail(err)
	}
	return lyricsDTO(l, lang), nil
}

// lyricsDTO maps parsed lyrics, attaching to each line its translation
// (preferring lang) and transliteration.
func lyricsDTO(l applemusic.Lyrics, lang string) Lyrics {
	out := Lyrics{Timing: l.Timing, Language: l.Language, Songwriters: nonNil(l.Songwriters), Lines: make([]LyricLine, len(l.Lines))}
	tr := l.Localization(applemusic.LocalizationTranslation, lang)
	tl := l.Localization(applemusic.LocalizationTransliteration, lang)
	if tr != nil {
		out.TranslationLanguage = tr.Language
	}
	if tl != nil {
		out.TransliterationLanguage = tl.Language
	}
	for i, ln := range l.Lines {
		out.Lines[i] = LyricLine{
			BeginMs: ln.Begin.Milliseconds(), EndMs: ln.End.Milliseconds(),
			Text: ln.Text, Background: ln.BackgroundText, Part: ln.Part, Agent: ln.Agent,
			Words:           lyricWords(ln.Words),
			Translation:     localizedLine(tr, ln.Key),
			Transliteration: localizedLine(tl, ln.Key),
		}
	}
	return out
}

func localizedLine(loc *applemusic.LyricLocalization, key string) *LyricText {
	if loc == nil || key == "" {
		return nil
	}
	ln, ok := loc.Lines[key]
	if !ok || (ln.Text == "" && ln.BackgroundText == "") {
		return nil
	}
	return &LyricText{Text: ln.Text, Background: ln.BackgroundText, Words: lyricWords(ln.Words)}
}

func lyricWords(in []applemusic.LyricWord) []LyricWord {
	out := make([]LyricWord, len(in))
	for i, w := range in {
		out[i] = LyricWord{BeginMs: w.Begin.Milliseconds(), EndMs: w.End.Milliseconds(), Text: w.Text, Background: w.Background}
	}
	return out
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
