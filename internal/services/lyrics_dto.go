package services

// Lyrics are a song's lyrics from the provider that had the best ones.
// Provider is its config.Lyrics* ID ("apple", "musixmatch", "lrclib",
// "netease"). TranslationLanguage and TransliterationLanguage name the
// localizations carried on the lines ("" when there are none): a
// translation into the chosen language and a pronunciation in Latin
// script ("und-Latn" when the provider doesn't say which language it
// spells).
type Lyrics struct {
	Provider                string      `json:"provider"`
	Timing                  string      `json:"timing"` // "Word", "Line" or "None"
	Language                string      `json:"language"`
	Songwriters             []string    `json:"songwriters"`
	TranslationLanguage     string      `json:"translationLanguage"`     // e.g. "en-US"
	TransliterationLanguage string      `json:"transliterationLanguage"` // e.g. "ko-Latn"
	Lines                   []LyricLine `json:"lines"`
}

// LyricLine is one line; Words is set for word-timed lyrics. Background is
// the background vocals (their words are in Words, flagged Background);
// Agent the voice ("v1", "v2" for a duet's second voice, …) and AgentType
// its kind where the source declares it ("person", "group" for every voice
// together, "other"; "" otherwise). Translation and Transliteration are the
// line's localizations (nil when the lyrics have none, or none for this
// line).
type LyricLine struct {
	BeginMs         int64       `json:"beginMs"`
	EndMs           int64       `json:"endMs"`
	Text            string      `json:"text"`
	Background      string      `json:"backgroundText"`
	Part            string      `json:"part"`      // e.g. "Verse"
	Agent           string      `json:"agent"`     // "v1", "v2" (duet voice), …
	AgentType       string      `json:"agentType"` // "person", "group", "other" or ""
	Words           []LyricWord `json:"words"`
	Translation     *LyricText  `json:"translation"`
	Transliteration *LyricText  `json:"transliteration"`
}

// LyricText is a localized line. Words is set for word-timed
// transliterations (timed like the original syllables).
type LyricText struct {
	Text       string      `json:"text"`
	Background string      `json:"backgroundText"`
	Words      []LyricWord `json:"words"`
}

// LyricWord is one timed word or syllable (Text keeps its trailing space).
type LyricWord struct {
	BeginMs    int64  `json:"beginMs"`
	EndMs      int64  `json:"endMs"`
	Text       string `json:"text"`
	Background bool   `json:"background"`
}
