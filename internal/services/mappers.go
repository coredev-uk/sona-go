package services

import (
	"cmp"
	"context"
	"strings"
	"time"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/artwork"
	"github.com/coredev-uk/sona-go/internal/library"
	"github.com/coredev-uk/sona-go/internal/motion"
)

// Shared DTO mappers. Every feature turns Apple resources (applemusic.*)
// and local library rows (library.*) into model.go DTOs through mapper, so
// artwork URLs, sources and IDs are filled the same way everywhere. New
// resource kinds get their mapper here (not in a feature file) as soon as a
// second feature needs them.

// colorsTimeout bounds dominant-colour extraction in detail calls; the
// colours usually come from a source already cached by the list view.
const colorsTimeout = 1500 * time.Millisecond

// mapper turns applemusic resources and library rows into DTOs. Methods:
// artwork (Apple artwork descriptor → Art at its own aspect ratio), artOf
// (bare template → square Art), header (template → backdrop + colours),
// motionOf (editorialVideo → motion cover URL),
// libSong/libSongs/libAlbum/libAlbums/libArtist/libPlaylist (library rows),
// amSong/amSongs/amAlbum/amAlbums/amArtist/amArtists/amPlaylist/amPlaylists
// (Apple resources, catalog or library-typed).
type mapper struct {
	art    *artwork.Cache
	motion *motion.Cache
}

// artwork maps an Apple artwork descriptor: its URLs carry the shape Apple
// serves it at (artwork.Proportions), so the UI can size wide editorial
// art by its aspect ratio.
func (m mapper) artwork(a *applemusic.Artwork) Art {
	if a == nil {
		return Art{}
	}
	w, h := artwork.Proportions(a.URL, a.Width, a.Height)
	return m.sized(a.URL, w, h)
}

// artOf maps a bare template (library rows and the queue keep no size),
// which is shown square.
func (m mapper) artOf(template string) Art { return m.sized(template, 1, 1) }

// sized returns template's Art for a w:h shape, each URL's long edge at
// the size the UI renders it.
func (m mapper) sized(template string, w, h int) Art {
	if template == "" || m.art == nil {
		return Art{}
	}
	at := func(long int) string {
		l := max(w, h)
		return m.art.URL(template, (w*long+l/2)/l, (h*long+l/2)/l)
	}
	return Art{Small: at(artSmall), Medium: at(artMedium), Large: at(artLarge)}
}

// header computes a detail page's backdrop and colours.
func (m mapper) header(ctx context.Context, template string) Header {
	if template == "" || m.art == nil {
		return Header{Colors: []string{}}
	}
	h := Header{Backdrop: m.art.BackdropURL(template), Colors: []string{}}
	ctx, cancel := context.WithTimeout(ctx, colorsTimeout)
	defer cancel()
	if cols, err := m.art.DominantColors(ctx, template); err == nil {
		for _, c := range cols {
			h.Colors = append(h.Colors, c.Hex())
		}
	}
	return h
}

// motionOf returns the Go-served URL of an item's square motion cover, ""
// when it has none.
func (m mapper) motionOf(v applemusic.EditorialVideo) string {
	return m.motion.URL(v.Square())
}

func unixMs(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// parseMs parses an RFC 3339 timestamp from Apple to Unix ms (0 if absent).
func parseMs(s string) int64 {
	if s == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UnixMilli()
	}
	return 0
}

func firstOf(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// Library models.

func (m mapper) libSong(s library.Song) Song {
	return Song{
		ID:          s.LibraryID,
		Source:      SourceLibrary,
		CatalogID:   s.CatalogID,
		LibraryID:   s.LibraryID,
		Name:        s.Name,
		Artist:      s.ArtistName,
		Album:       s.AlbumName,
		AlbumID:     s.AlbumLibraryID,
		ArtistID:    s.ArtistLibraryID,
		DurationMs:  s.DurationMS,
		TrackNumber: s.TrackNumber,
		DiscNumber:  s.DiscNumber,
		ReleaseDate: s.ReleaseDate,
		Genre:       s.Genre,
		Explicit:    s.Explicit,
		HasLyrics:   s.HasLyrics,
		Playable:    s.CatalogID != "" || s.LibraryID != "", // uploads play from their library ID
		Art:         m.artOf(s.ArtworkTemplate),
	}
}

func (m mapper) libSongs(in []library.Song) []Song {
	out := make([]Song, len(in))
	for i, s := range in {
		out[i] = m.libSong(s)
	}
	return out
}

func (m mapper) libAlbum(a library.Album) Album {
	return Album{
		ID:          a.LibraryID,
		Source:      SourceLibrary,
		CatalogID:   a.CatalogID,
		LibraryID:   a.LibraryID,
		Name:        a.Name,
		Artist:      a.ArtistName,
		ArtistID:    a.ArtistLibraryID,
		ReleaseDate: a.ReleaseDate,
		Genre:       a.Genre,
		TrackCount:  a.TrackCount,
		Explicit:    a.Explicit,
		DateAdded:   unixMs(a.DateAdded),
		Art:         m.artOf(a.ArtworkTemplate),
	}
}

func (m mapper) libAlbums(in []library.Album) []Album {
	out := make([]Album, len(in))
	for i, a := range in {
		out[i] = m.libAlbum(a)
	}
	return out
}

func (m mapper) libArtist(a library.Artist) Artist {
	return Artist{
		ID:        a.LibraryID,
		Source:    SourceLibrary,
		CatalogID: a.CatalogID,
		LibraryID: a.LibraryID,
		Name:      a.Name,
		Art:       m.artOf(a.ArtworkTemplate),
	}
}

func (m mapper) libPlaylist(p library.Playlist) Playlist {
	return Playlist{
		ID:           p.LibraryID,
		Source:       SourceLibrary,
		CatalogID:    p.CatalogID,
		LibraryID:    p.LibraryID,
		Name:         p.Name,
		Description:  p.Description,
		Curator:      p.CuratorName,
		CanEdit:      p.CanEdit,
		TrackCount:   p.TrackCount,
		LastModified: parseMs(p.LastModified),
		DateAdded:    unixMs(p.DateAdded),
		Art:          m.artOf(p.ArtworkTemplate),
	}
}

// Catalog (Apple) models. They can also be library resources (search over
// the library, recently played), told apart by their type.

func isLibraryType(typ string) bool { return strings.HasPrefix(typ, "library-") }

// traitsOf reads Apple's audioTraits and Apple Digital Master flag
// (isMasteredForItunes is its former name).
func traitsOf(audioTraits []string, adm bool) Traits {
	t := Traits{AppleDigitalMaster: adm}
	for _, a := range audioTraits {
		switch a {
		case "lossless":
			t.Lossless = true
		case "hi-res-lossless":
			t.Lossless, t.HiResLossless = true, true
		case "atmos":
			t.Atmos = true
		case "spatial":
			t.Spatial = true
		}
	}
	return t
}

func (m mapper) amSong(s applemusic.Song) Song {
	a := s.Attributes
	out := Song{
		ID:          s.ID,
		Source:      SourceCatalog,
		CatalogID:   s.CatalogID(),
		Name:        a.Name,
		Artist:      a.ArtistName,
		Album:       a.AlbumName,
		DurationMs:  a.DurationInMillis,
		TrackNumber: a.TrackNumber,
		DiscNumber:  a.DiscNumber,
		ReleaseDate: a.ReleaseDate,
		Genre:       firstOf(a.GenreNames),
		Explicit:    a.ContentRating == "explicit",
		HasLyrics:   a.HasLyrics,
		Playable:    a.PlayParams != nil,
		Traits:      traitsOf(a.AudioTraits, a.IsAppleDigitalMaster || a.IsMasteredForItunes),
		Art:         m.artwork(a.Artwork),
	}
	if isLibraryType(s.Type) {
		out.Source, out.LibraryID = SourceLibrary, s.ID
	}
	if d := s.Relationships.Albums.Data; len(d) > 0 {
		out.AlbumID = d[0].ID
	}
	if d := s.Relationships.Artists.Data; len(d) > 0 {
		out.ArtistID = d[0].ID
	}
	return out
}

func (m mapper) amSongs(in []applemusic.Song) []Song {
	out := make([]Song, 0, len(in))
	for _, s := range in {
		out = append(out, m.amSong(s))
	}
	return out
}

func (m mapper) amAlbum(a applemusic.Album) Album {
	at := a.Attributes
	out := Album{
		ID:          a.ID,
		Source:      SourceCatalog,
		CatalogID:   a.CatalogID(),
		Name:        at.Name,
		Artist:      at.ArtistName,
		ReleaseDate: at.ReleaseDate,
		Genre:       firstOf(at.GenreNames),
		TrackCount:  at.TrackCount,
		Explicit:    at.ContentRating == "explicit",
		Single:      at.IsSingle,
		Traits:      traitsOf(at.AudioTraits, at.IsAppleDigitalMaster || at.IsMasteredForItunes),
		Art:         m.artwork(at.Artwork),
	}
	if isLibraryType(a.Type) {
		out.Source, out.LibraryID = SourceLibrary, a.ID
		out.DateAdded = parseMs(at.DateAdded)
	}
	if d := a.Relationships.Artists.Data; len(d) > 0 {
		out.ArtistID = d[0].ID
	}
	return out
}

func (m mapper) amAlbums(in []applemusic.Album) []Album {
	out := make([]Album, 0, len(in))
	for _, a := range in {
		out = append(out, m.amAlbum(a))
	}
	return out
}

func (m mapper) amArtist(a applemusic.Artist) Artist {
	out := Artist{
		ID:        a.ID,
		Source:    SourceCatalog,
		CatalogID: a.CatalogID(),
		Name:      a.Attributes.Name,
		Genre:     firstOf(a.Attributes.GenreNames),
		Art:       m.artwork(a.Attributes.Artwork),
	}
	if isLibraryType(a.Type) {
		out.Source, out.LibraryID = SourceLibrary, a.ID
	}
	return out
}

func (m mapper) amPlaylist(p applemusic.Playlist) Playlist {
	at := p.Attributes
	out := Playlist{
		ID:           p.ID,
		Source:       SourceCatalog,
		CatalogID:    p.CatalogID(),
		Name:         at.Name,
		Description:  at.Description.Text(),
		Curator:      at.CuratorName,
		CanEdit:      at.CanEdit,
		TrackCount:   len(p.Relationships.Tracks.Data),
		LastModified: parseMs(at.LastModifiedDate),
		Art:          m.artwork(at.Artwork),
		Traits:       traitsOf(at.AudioTraits, false),
	}
	if isLibraryType(p.Type) {
		out.Source, out.LibraryID = SourceLibrary, p.ID
		out.DateAdded = parseMs(at.DateAdded)
	}
	return out
}

func totalMs(songs []Song) int64 {
	var t int64
	for _, s := range songs {
		t += s.DurationMs
	}
	return t
}

func (m mapper) amArtists(in []applemusic.Artist) []Artist {
	out := make([]Artist, 0, len(in))
	for _, a := range in {
		out = append(out, m.amArtist(a))
	}
	return out
}

func (m mapper) amPlaylists(in []applemusic.Playlist) []Playlist {
	out := make([]Playlist, 0, len(in))
	for _, p := range in {
		out = append(out, m.amPlaylist(p))
	}
	return out
}

func (m mapper) amMusicVideo(v applemusic.MusicVideo) MusicVideo {
	a := v.Attributes
	out := MusicVideo{
		ID:          v.ID,
		Source:      SourceCatalog,
		CatalogID:   v.CatalogID(),
		Name:        a.Name,
		Artist:      a.ArtistName,
		Album:       a.AlbumName,
		DurationMs:  a.DurationInMillis,
		ReleaseDate: a.ReleaseDate,
		Genre:       firstOf(a.GenreNames),
		Explicit:    a.ContentRating == "explicit",
		Has4K:       a.Has4K,
		HasHDR:      a.HasHDR,
		URL:         a.URL,
		Art:         m.artwork(a.Artwork),
	}
	if isLibraryType(v.Type) {
		out.Source, out.LibraryID = SourceLibrary, v.ID
	}
	if d := v.Relationships.Artists.Data; len(d) > 0 && !isLibraryType(d[0].Type) {
		out.ArtistID = d[0].ID
	}
	return out
}

func (m mapper) amMusicVideos(in []applemusic.MusicVideo) []MusicVideo {
	out := make([]MusicVideo, 0, len(in))
	for _, v := range in {
		out = append(out, m.amMusicVideo(v))
	}
	return out
}

// amStation maps a catalog station; playability comes from
// stationPlayability (radio.go).
func (m mapper) amStation(s applemusic.Station) Station {
	a := s.Attributes
	out := Station{
		ID:         s.ID,
		Name:       a.Name,
		Kind:       stationKind(s),
		Provider:   a.StationProviderName,
		DurationMs: a.DurationInMillis,
		URL:        a.URL,
		Art:        m.artwork(a.Artwork),
	}
	if n := a.EditorialNotes; n != nil {
		// Short is a label ("Live Station") when a tagline exists, else
		// the one-line description (mood stations).
		out.Tagline = cmp.Or(n.Tagline, n.Short)
		if n.Standard != out.Tagline {
			out.Notes = n.Standard
		}
	}
	if a.AirTime != nil {
		out.AirTimeMs = parseMs(a.AirTime.Start)
	}
	if d := s.Relationships.RadioShow.Data; len(d) > 0 {
		out.ShowID, out.ShowName, out.HostName = d[0].ID, d[0].Attributes.Name, d[0].Attributes.ShowHostName
	}
	out.Playable, out.Unavailable = stationPlayability(s)
	return out
}

func (m mapper) amStations(in []applemusic.Station) []Station {
	out := make([]Station, 0, len(in))
	for _, s := range in {
		out = append(out, m.amStation(s))
	}
	return out
}
