package config

import (
	"slices"
	"strings"
)

// Lyrics providers, in their default order (internal/lyrics).
const (
	LyricsApple      = "apple"
	LyricsMusixmatch = "musixmatch"
	LyricsLRCLIB     = "lrclib"
	LyricsNetEase    = "netease"
)

// lyricsDefaults are the known providers in their default order, with
// whether each is on by default.
var lyricsDefaults = []LyricsProvider{
	{ID: LyricsApple, Enabled: true},
	{ID: LyricsMusixmatch, Enabled: true},
	{ID: LyricsLRCLIB, Enabled: true},
	{ID: LyricsNetEase, Enabled: false},
}

// Lyrics chooses where lyrics come from. Every enabled provider is asked
// and the result with the best timing wins (word over line over plain
// text); among equal results the one earlier in Providers wins.
type Lyrics struct {
	// Providers are every known provider, in the user's order.
	Providers []LyricsProvider `json:"providers"`
	// PreferWordTiming lets word-timed lyrics win over line-timed ones from
	// a provider earlier in the order. Off, any synced lyrics count alike
	// and the order decides.
	PreferWordTiming bool `json:"preferWordTiming"`
	// TranslationLanguage is the BCP 47 tag translations are asked in
	// (e.g. "es", "pt-BR"); "" uses the Apple Music account's language.
	TranslationLanguage string `json:"translationLanguage"`
}

// LyricsProvider is one provider's place in the order and its switch.
type LyricsProvider struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

func defaultLyrics() Lyrics {
	return Lyrics{Providers: slices.Clone(lyricsDefaults), PreferWordTiming: true}
}

// Enabled returns the enabled providers' IDs in order.
func (l Lyrics) Enabled() []string {
	var out []string
	for _, p := range l.Providers {
		if p.Enabled {
			out = append(out, p.ID)
		}
	}
	return out
}

// normalized keeps each known provider once, in the saved order, drops
// unknown ones and appends any missing provider (one added in a newer
// version) at the end with its default switch. A translation language
// that isn't a plausible language tag becomes "" (the account's).
func (l Lyrics) normalized() Lyrics {
	out := make([]LyricsProvider, 0, len(lyricsDefaults))
	for _, p := range l.Providers {
		if knownLyricsProvider(p.ID) && !slices.ContainsFunc(out, func(q LyricsProvider) bool { return q.ID == p.ID }) {
			out = append(out, p)
		}
	}
	for _, d := range lyricsDefaults {
		if !slices.ContainsFunc(out, func(q LyricsProvider) bool { return q.ID == d.ID }) {
			out = append(out, d)
		}
	}
	l.Providers = out
	l.TranslationLanguage = languageTag(l.TranslationLanguage)
	return l
}

func knownLyricsProvider(id string) bool {
	return slices.ContainsFunc(lyricsDefaults, func(d LyricsProvider) bool { return d.ID == id })
}

// languageTag returns tag trimmed when it looks like a BCP 47 language tag
// (a 2–3 letter language and optional subtags of 1–8 letters or digits,
// separated by hyphens; underscores are read as hyphens), else "".
func languageTag(tag string) string {
	tag = strings.ReplaceAll(strings.TrimSpace(tag), "_", "-")
	if tag == "" || len(tag) > 35 {
		return ""
	}
	for i, sub := range strings.Split(tag, "-") {
		if i == 0 && (len(sub) < 2 || len(sub) > 3) || len(sub) < 1 || len(sub) > 8 {
			return ""
		}
		for _, r := range sub {
			letter := 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z'
			digit := '0' <= r && r <= '9'
			if !letter && (i == 0 || !digit) {
				return ""
			}
		}
	}
	return tag
}
