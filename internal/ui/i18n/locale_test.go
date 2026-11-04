package i18n

import (
	"slices"
	"testing"
)

// The cases of v0.2.0's shared/i18n/locale.test.ts.

func TestMatchLocale(t *testing.T) {
	available := []string{"en", "pt-PT", "pt-BR", "zh-Hant"}
	for _, tc := range []struct {
		preferred []string
		want      string
		ok        bool
	}{
		{[]string{"pt-br"}, "pt-BR", true},
		{[]string{"en-GB"}, "en", true},
		{[]string{"pt-AO"}, "pt-PT", true},
		{[]string{"zh-Hant-TW"}, "zh-Hant", true},
		{[]string{"", "de-DE", "pt-BR", "en"}, "pt-BR", true},
		{[]string{"de-DE", "fr"}, "", false},
		{nil, "", false},
	} {
		got, ok := MatchLocale(tc.preferred, available)
		if got != tc.want || ok != tc.ok {
			t.Errorf("MatchLocale(%q) = %q, %v; want %q, %v", tc.preferred, got, ok, tc.want, tc.ok)
		}
	}
}

func TestTranslationCandidates(t *testing.T) {
	for _, tc := range []struct {
		in, want []string
	}{
		{[]string{"fr-FR", "de-DE", "en-GB", "ja"}, []string{"fr-FR", "de-DE"}},
		{[]string{"", "fr-FR", "fr-fr", "fr"}, []string{"fr-FR", "fr"}},
		{[]string{"en-GB", "fr-FR"}, nil},
	} {
		if got := TranslationCandidates(tc.in); !slices.Equal(got, tc.want) {
			t.Errorf("TranslationCandidates(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslatedShare(t *testing.T) {
	source := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	near := func(got, want float64) bool { return got > want-1e-9 && got < want+1e-9 }
	if got := TranslatedShare(0.2, map[string]string{"a": "A", "b": "B"}, map[string]string{"b": "b", "c": "c", "z": "z"}, source); !near(got, 0.3) {
		t.Errorf("Apple's coverage plus the community messages Apple lacks: %v, want 0.3", got)
	}
	if got := TranslatedShare(0.2, map[string]string{"a": "A", "b": "B"}, nil, source); got != 0.2 {
		t.Errorf("no community catalogue: %v", got)
	}
	if got := TranslatedShare(0, nil, map[string]string{"a": "a"}, nil); got != 0 {
		t.Errorf("no source messages: %v", got)
	}
}

func TestChooseTranslation(t *testing.T) {
	loader := func(shares map[string]float64, asked *[]string) func(string) (Translation, bool) {
		return func(tag string) (Translation, bool) {
			if asked != nil {
				*asked = append(*asked, tag)
			}
			return Translation{Locale: tag, Share: shares[tag]}, true
		}
	}
	t.Run("shows a locale only from the selectable share", func(t *testing.T) {
		if SelectableShare != 0.9 {
			t.Fatalf("selectable share %v", SelectableShare)
		}
		load := loader(map[string]float64{"fr-FR": 0.89, "de-DE": 0.9, "ja": 1}, nil)
		if got, ok := ChooseTranslation([]string{"fr-FR", "de-DE", "ja"}, load, SelectableShare); !ok || got.Locale != "de-DE" {
			t.Errorf("chose %+v, %v", got, ok)
		}
		if _, ok := ChooseTranslation([]string{"fr-FR"}, load, SelectableShare); ok {
			t.Error("a locale below the share was chosen")
		}
	})
	t.Run("stops at English: an English preference before a translation keeps English", func(t *testing.T) {
		var asked []string
		load := loader(map[string]float64{"en-GB": 1, "fr-FR": 1}, &asked)
		if _, ok := ChooseTranslation([]string{"", "en-GB", "fr-FR"}, load, SelectableShare); ok || len(asked) != 0 {
			t.Errorf("chose %v after asking %v", ok, asked)
		}
	})
	t.Run("asks for today's typical coverage and keeps English", func(t *testing.T) {
		load := loader(map[string]float64{"fr-FR": 98.0 / 1415, "fr": 98.0 / 1415}, nil)
		if _, ok := ChooseTranslation([]string{"fr-FR", "fr"}, load, SelectableShare); ok {
			t.Error("a 7% locale was chosen")
		}
	})
	t.Run("shows a chosen language however complete, once it translates anything", func(t *testing.T) {
		load := loader(map[string]float64{"fr-FR": 98.0 / 1415, "de-DE": 0}, nil)
		if got, ok := ChooseTranslation([]string{"fr-FR"}, load, 0); !ok || got.Locale != "fr-FR" {
			t.Errorf("chose %+v, %v", got, ok)
		}
		if _, ok := ChooseTranslation([]string{"de-DE"}, load, 0); ok {
			t.Error("a locale with nothing translated was chosen")
		}
		if _, ok := ChooseTranslation([]string{"en"}, load, 0); ok {
			t.Error("English was chosen as a translation")
		}
	})
	t.Run("skips a locale that has no layers", func(t *testing.T) {
		load := func(tag string) (Translation, bool) {
			if tag == "fr-FR" {
				return Translation{}, false
			}
			return Translation{Locale: tag, Share: 1}, true
		}
		if got, ok := ChooseTranslation([]string{"fr-FR", "de-DE"}, load, SelectableShare); !ok || got.Locale != "de-DE" {
			t.Errorf("chose %+v, %v", got, ok)
		}
	})
}

func TestFormatLocales(t *testing.T) {
	for _, tc := range []struct {
		locale    string
		preferred []string
		want      []string
	}{
		{"en", []string{"en-GB", "de-DE", "en-US", "en-GB"}, []string{"en-GB", "en-US", "en"}},
		{"en-XA", []string{"en-GB"}, []string{"en-GB", "en-XA"}},
		{"fr", []string{"en-GB", "", ""}, []string{"fr"}},
	} {
		if got := FormatLocales(tc.locale, tc.preferred); !slices.Equal(got, tc.want) {
			t.Errorf("FormatLocales(%q, %q) = %q, want %q", tc.locale, tc.preferred, got, tc.want)
		}
	}
}
