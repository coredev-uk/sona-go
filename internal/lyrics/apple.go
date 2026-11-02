package lyrics

import (
	"context"
	"errors"

	"github.com/coredev-uk/sona-go/internal/applemusic"
	"github.com/coredev-uk/sona-go/internal/config"
)

// Apple is Apple Music's own lyrics: word-timed ("syllable-lyrics") where
// Apple has them, else line-timed, with Apple's translation and Latin
// transliteration. It needs the catalog ID and a signed-in account.
type Apple struct {
	API        *applemusic.Client
	Storefront func() string
	// Report passes a failed request to auth triage (services.Env.Report).
	Report func(error)
}

func (a *Apple) ID() string      { return config.LyricsApple }
func (a *Apple) Localized() bool { return true }

func (a *Apple) Fetch(ctx context.Context, req Request) (Lyrics, error) {
	if req.CatalogID == "" {
		return Lyrics{}, ErrNotFound
	}
	sf := a.Storefront()
	if sf == "" || a.API.UserToken == nil || a.API.UserToken() == "" {
		return Lyrics{}, ErrUnavailable
	}
	l, err := a.API.Lyrics(ctx, sf, req.CatalogID, applemusic.LyricsOptions{Syllable: true, Language: req.Language})
	if errors.Is(err, applemusic.ErrNotFound) {
		return Lyrics{}, ErrNotFound
	}
	if err != nil {
		if a.Report != nil {
			a.Report(err)
		}
		return Lyrics{}, err
	}
	return fromApple(l, req.Language), nil
}

// fromApple maps parsed TTML, attaching to each line its translation
// (preferring lang) and transliteration.
func fromApple(l applemusic.Lyrics, lang string) Lyrics {
	out := Lyrics{Provider: config.LyricsApple, Timing: l.Timing, Language: l.Language, Songwriters: l.Songwriters, Lines: make([]Line, len(l.Lines))}
	switch out.Timing {
	case TimingWord, TimingLine:
	default:
		out.Timing = TimingNone
	}
	tr := l.Localization(applemusic.LocalizationTranslation, lang)
	tl := l.Localization(applemusic.LocalizationTransliteration, lang)
	if tr != nil {
		out.TranslationLanguage = tr.Language
	}
	if tl != nil {
		out.TransliterationLanguage = tl.Language
	}
	for i, ln := range l.Lines {
		out.Lines[i] = Line{
			Begin: ln.Begin, End: ln.End, Text: ln.Text, Background: ln.BackgroundText, Part: ln.Part, Agent: ln.Agent,
			Words:           appleWords(ln.Words),
			Translation:     appleLocalized(tr, ln.Key),
			Transliteration: appleLocalized(tl, ln.Key),
		}
	}
	return out
}

func appleLocalized(loc *applemusic.LyricLocalization, key string) *Text {
	if loc == nil || key == "" {
		return nil
	}
	ln, ok := loc.Lines[key]
	if !ok || (ln.Text == "" && ln.BackgroundText == "") {
		return nil
	}
	return &Text{Text: ln.Text, Background: ln.BackgroundText, Words: appleWords(ln.Words)}
}

func appleWords(in []applemusic.LyricWord) []Word {
	if len(in) == 0 {
		return nil
	}
	out := make([]Word, len(in))
	for i, w := range in {
		out[i] = Word{Begin: w.Begin, End: w.End, Text: w.Text, Background: w.Background}
	}
	return out
}
