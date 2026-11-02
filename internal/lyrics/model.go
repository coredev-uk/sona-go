// Package lyrics finds a song's lyrics across providers: Apple Music,
// Musixmatch, LRCLIB and NetEase Cloud Music (config.Lyrics* IDs).
//
// A Resolver asks every enabled provider in parallel, each with its own
// timeout, rate limit and backoff, and keeps the result with the best
// timing (word over line over plain text; config.Lyrics.PreferWordTiming).
// It answers as soon as no provider still running could beat the best
// result so far. Every provider's answer, lyrics or "none", is cached in
// SQLite (lyrics_cache) per song, so changing the providers' order or
// switches costs no requests. All requests are made from Go; lyrics text
// is plain data to the frontend, never markup.
package lyrics

import "time"

// Timing values, as Apple's itunes:timing names them.
const (
	TimingWord = "Word" // every word (or syllable) has its own time
	TimingLine = "Line" // every line has its own time
	TimingNone = "None" // plain text
)

// Lyrics are one provider's lyrics for a song.
type Lyrics struct {
	Provider    string   `json:"provider"` // config.Lyrics* ID
	Timing      string   `json:"timing"`   // TimingWord, TimingLine or TimingNone
	Language    string   `json:"language"` // the lyrics' language, "" when unknown
	Songwriters []string `json:"songwriters,omitempty"`
	// TranslationLanguage and TransliterationLanguage name the
	// localizations carried on the lines ("" when there are none).
	TranslationLanguage     string `json:"translationLanguage,omitempty"`
	TransliterationLanguage string `json:"transliterationLanguage,omitempty"`
	Lines                   []Line `json:"lines"`
	// partial marks a result missing a part that failed to load (a
	// translation); it is cached only briefly so the next play retries.
	partial bool
}

// Line is one line. For plain-text lyrics Begin and End are zero; Words is
// set for word-timed lyrics only.
type Line struct {
	Begin time.Duration `json:"begin"`
	End   time.Duration `json:"end"`
	Text  string        `json:"text"`
	// Background is the background vocals sung over the line (their words
	// are in Words, flagged Background).
	Background string `json:"background,omitempty"`
	Part       string `json:"part,omitempty"`  // song part, e.g. "Verse"
	Agent      string `json:"agent,omitempty"` // the voice: "v1", "v2" (a duet's second voice), …
	Words      []Word `json:"words,omitempty"`
	// Translation and Transliteration are the line's localizations (nil
	// when there is none for the line).
	Translation     *Text `json:"translation,omitempty"`
	Transliteration *Text `json:"transliteration,omitempty"`
}

// Text is a localized line. Words is set for word-timed
// transliterations (timed like the original).
type Text struct {
	Text       string `json:"text"`
	Background string `json:"background,omitempty"`
	Words      []Word `json:"words,omitempty"`
}

// Word is one timed word or syllable. Text keeps a trailing space when one
// follows, so concatenating a line's words rebuilds it.
type Word struct {
	Begin      time.Duration `json:"begin"`
	End        time.Duration `json:"end"`
	Text       string        `json:"text"`
	Background bool          `json:"background,omitempty"`
}

// rank orders results by timing quality: word 3, line 2, plain text 1.
// With preferWord off, word-timed lyrics rank as line-timed ones.
func (l Lyrics) rank(preferWord bool) int {
	switch l.Timing {
	case TimingWord:
		if preferWord {
			return 3
		}
		return 2
	case TimingLine:
		return 2
	}
	return 1
}

// maxRank is the best rank a result can have.
func maxRank(preferWord bool) int {
	if preferWord {
		return 3
	}
	return 2
}

// empty reports whether l has no text at all.
func (l Lyrics) empty() bool {
	for _, ln := range l.Lines {
		if ln.Text != "" || ln.Background != "" {
			return false
		}
	}
	return true
}

// endLines sets each timed line's End, where the source gave none, to the
// next line's Begin (the last line keeps its Begin plus a few seconds, or
// the song's end when known).
func endLines(lines []Line, songEnd time.Duration) {
	for i := range lines {
		if lines[i].End > lines[i].Begin {
			continue
		}
		switch {
		case i+1 < len(lines):
			lines[i].End = lines[i+1].Begin
		case songEnd > lines[i].Begin:
			lines[i].End = songEnd
		default:
			lines[i].End = lines[i].Begin + 5*time.Second
		}
	}
}
