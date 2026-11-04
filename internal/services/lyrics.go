package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coredev-uk/sona-go/internal/auth/store"
	"github.com/coredev-uk/sona-go/internal/config"
	"github.com/coredev-uk/sona-go/internal/lyrics"
)

// musixmatchSecret is the keyring key of Musixmatch's anonymous user
// token (file fallback: <config>/credentials-musixmatch).
const musixmatchSecret = "musixmatch"

// LyricsService finds a song's lyrics across the providers chosen in
// Settings → Playback (config.Lyrics): Apple Music, Musixmatch, LRCLIB and
// NetEase, through internal/lyrics. Every request is made from Go, after
// the first paint (the shared gate), and each provider's answer is cached
// in SQLite.
type LyricsService struct {
	e *Env
	r *lyrics.Resolver
}

func newLyricsService(e *Env) *LyricsService {
	httpc := &http.Client{Transport: e.gated(), Timeout: 30 * time.Second, CheckRedirect: lyrics.HTTPSOnly}
	// LRCLIB and NetEase get Sona's name and home page; Musixmatch leaves
	// word timing out of its answers for a User-Agent carrying a URL.
	ua := "Sona/" + e.Version + " (+https://github.com/coredev-uk/sona-go)"
	short := "Sona/" + e.Version
	return newLyrics(e,
		&lyrics.Apple{API: e.API, Storefront: e.Storefront, Report: e.Report},
		&lyrics.Musixmatch{HTTP: httpc, UserAgent: short, Secret: func() (store.Secret, error) {
			return e.openSecret(musixmatchSecret)
		}},
		&lyrics.LRCLIB{HTTP: httpc, UserAgent: ua},
		&lyrics.NetEase{HTTP: httpc, UserAgent: ua},
	)
}

// newLyrics builds the service over the given providers.
func newLyrics(e *Env, providers ...lyrics.Provider) *LyricsService {
	return &LyricsService{e: e, r: lyrics.New(lyrics.Options{
		DB:        e.DB,
		Providers: providers,
		Settings:  func() config.Lyrics { return e.Settings.Get().Lyrics },
		Logger:    e.Logger.With("component", "lyrics"),
		Now:       e.Now,
	})}
}

// Lyrics returns the lyrics of catalog song songID from the provider with
// the best ones (word-timed over line-timed over plain text; Settings →
// Playback → Lyrics), with a translation into the chosen language (the
// account's by default) and a Latin-script pronunciation where the
// provider has them. Songs without lyrics fail with code not_found.
func (s *LyricsService) Lyrics(ctx context.Context, songID string) (Lyrics, error) {
	if songID == "" {
		return Lyrics{}, fmt.Errorf("%w: no song", ErrInvalidArgument)
	}
	lang := s.e.Settings.Get().Lyrics.TranslationLanguage
	if lang == "" && s.e.Auth != nil {
		lang = s.e.Auth.Status().Language
	}
	l, err := s.r.Lyrics(ctx, lyrics.Request{Track: s.track(ctx, songID), Language: lang})
	if err != nil {
		return Lyrics{}, err
	}
	return lyricsDTO(l), nil
}

// track is what the providers match songID by: its title, artist, album,
// length and ISRC from the catalog cache. Without them (the catalog
// can't be reached) only Apple, which needs just the ID, can answer.
func (s *LyricsService) track(ctx context.Context, songID string) lyrics.Track {
	t := lyrics.Track{CatalogID: songID}
	so, err := s.e.Catalog.Song(ctx, songID)
	if err != nil {
		if !errors.Is(err, context.Canceled) {
			s.e.Logger.Warn("lyrics: song details", "id", songID, "err", err)
		}
		return t
	}
	a := so.Attributes
	t.Title, t.Artist, t.Album, t.ISRC = a.Name, a.ArtistName, a.AlbumName, a.ISRC
	t.Duration = time.Duration(a.DurationInMillis) * time.Millisecond
	return t
}

// lyricsDTO maps lyrics for the UI (empty lists as []).
func lyricsDTO(l lyrics.Lyrics) Lyrics {
	out := Lyrics{
		Provider: l.Provider, Timing: l.Timing, Language: l.Language, Songwriters: nonNil(l.Songwriters),
		TranslationLanguage: l.TranslationLanguage, TransliterationLanguage: l.TransliterationLanguage,
		Lines: make([]LyricLine, len(l.Lines)),
	}
	for i, ln := range l.Lines {
		out.Lines[i] = LyricLine{
			BeginMs: ln.Begin.Milliseconds(), EndMs: ln.End.Milliseconds(),
			Text: ln.Text, Background: ln.Background, Part: ln.Part, Agent: ln.Agent, AgentType: ln.AgentType,
			Words:           lyricWords(ln.Words),
			Translation:     lyricText(ln.Translation),
			Transliteration: lyricText(ln.Transliteration),
		}
	}
	return out
}

func lyricText(t *lyrics.Text) *LyricText {
	if t == nil {
		return nil
	}
	return &LyricText{Text: t.Text, Background: t.Background, Words: lyricWords(t.Words)}
}

func lyricWords(in []lyrics.Word) []LyricWord {
	out := make([]LyricWord, len(in))
	for i, w := range in {
		out[i] = LyricWord{BeginMs: w.Begin.Milliseconds(), EndMs: w.End.Milliseconds(), Text: w.Text, Background: w.Background}
	}
	return out
}
