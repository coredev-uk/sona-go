package i18n

import (
	"slices"
	"testing"
)

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
