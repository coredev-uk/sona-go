package config

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"
)

func lyricsIDs(l Lyrics) []string {
	out := make([]string, len(l.Providers))
	for i, p := range l.Providers {
		out[i] = p.ID
	}
	return out
}

func TestLyricsDefaults(t *testing.T) {
	l := Defaults().Lyrics
	if !l.PreferWordTiming || l.TranslationLanguage != "" ||
		!slices.Equal(lyricsIDs(l), []string{LyricsApple, LyricsMusixmatch, LyricsLRCLIB, LyricsNetEase}) {
		t.Fatalf("defaults = %+v", l)
	}
	// NetEase is off by default.
	if !slices.Equal(l.Enabled(), []string{LyricsApple, LyricsMusixmatch, LyricsLRCLIB}) {
		t.Fatalf("enabled = %v", l.Enabled())
	}
}

func TestLyricsNormalized(t *testing.T) {
	l := Lyrics{Providers: []LyricsProvider{
		{ID: LyricsLRCLIB, Enabled: true},
		{ID: "somewhere", Enabled: true},
		{ID: LyricsApple, Enabled: false},
		{ID: LyricsLRCLIB, Enabled: false}, // a duplicate: the first stays
	}, TranslationLanguage: " pt_BR "}.normalized()
	// The saved order is kept, unknown and repeated IDs go, missing
	// providers join at the end with their default switch.
	if !slices.Equal(lyricsIDs(l), []string{LyricsLRCLIB, LyricsApple, LyricsMusixmatch, LyricsNetEase}) {
		t.Fatalf("order = %v", lyricsIDs(l))
	}
	if !slices.Equal(l.Enabled(), []string{LyricsLRCLIB, LyricsMusixmatch}) {
		t.Fatalf("enabled = %v", l.Enabled())
	}
	if l.TranslationLanguage != "pt-BR" {
		t.Fatalf("language = %q", l.TranslationLanguage)
	}
	for in, want := range map[string]string{
		"es": "es", "zh-Hant-TW": "zh-Hant-TW", "yue": "yue", "es-419": "es-419",
		"e": "", "english": "", "en-": "", "1a": "", "en-GB<script>": "", "fr-toolongsubtag": "", "ja-日本": "",
	} {
		if got := languageTag(in); got != want {
			t.Errorf("languageTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLyricsSettingsFromOlderFile(t *testing.T) {
	// A settings file from before lyrics settings existed gets the defaults.
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := writeFileAtomic(path, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Lyrics.Enabled(), Defaults().Lyrics.Enabled()) || !s.Lyrics.PreferWordTiming {
		t.Fatalf("lyrics = %+v", s.Lyrics)
	}
	// An Update writing into the provider list can't reach the Store's copy.
	st, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	before := st.Get()
	if _, err := st.Update(func(s *Settings) { s.Lyrics.Providers[0].Enabled = false }); err != nil {
		t.Fatal(err)
	}
	if !before.Lyrics.Providers[0].Enabled {
		t.Fatal("update changed an earlier copy")
	}
	b, _ := json.Marshal(st.Get().Lyrics)
	if string(b) != `{"providers":[{"id":"apple","enabled":false},{"id":"musixmatch","enabled":true},{"id":"lrclib","enabled":true},{"id":"netease","enabled":false}],"preferWordTiming":true,"translationLanguage":""}` {
		t.Fatalf("json = %s", b)
	}
}
