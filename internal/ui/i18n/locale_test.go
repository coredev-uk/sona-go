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

func TestFormatLocales(t *testing.T) {
	for _, tc := range []struct {
		locale    string
		preferred []string
		want      []string
	}{
		{"en", []string{"en-GB", "de-DE", "en-US", "en-GB"}, []string{"en-GB", "en-US", "en"}},
		{"en-XA", []string{"en-GB"}, []string{"en-XA"}},
		{"fr", []string{"en-GB", "", ""}, []string{"fr"}},
	} {
		if got := FormatLocales(tc.locale, tc.preferred); !slices.Equal(got, tc.want) {
			t.Errorf("FormatLocales(%q, %q) = %q, want %q", tc.locale, tc.preferred, got, tc.want)
		}
	}
}
