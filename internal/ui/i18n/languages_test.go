package i18n

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/services"
)

// The cases of v0.2.0's shared/i18n/languages.test.ts.

func appleLanguages(locales ...any) services.AppleLanguages {
	a := services.AppleLanguages{}
	for i := 0; i < len(locales); i += 2 {
		cov := locales[i+1].(float64)
		a.Locales = append(a.Locales, services.AppleLanguage{Tag: locales[i].(string), Coverage: cov, Measured: true})
		a.Mapped = max(a.Mapped, cov)
	}
	return a
}

func tags(o []LanguageOption) []string {
	out := make([]string, len(o))
	for i, x := range o {
		out[i] = x.Tag
	}
	return out
}

func TestLanguageOptions(t *testing.T) {
	t.Run("offers English alone while no locale reaches the selectable share", func(t *testing.T) {
		o := LanguageOptions(appleLanguages("fr-FR", 0.07, "de-DE", 0.07), nil, "", "en")
		if !slices.Equal(tags(o), []string{"en"}) || OffersLanguages(o, "") {
			t.Errorf("options %v", tags(o))
		}
		if got := tags(LanguageOptions(services.AppleLanguages{}, nil, "", "en")); !slices.Equal(got, []string{"en"}) {
			t.Errorf("no Apple locales: %v", got)
		}
	})
	t.Run("offers Apple's locales from 90%, named in themselves and sorted", func(t *testing.T) {
		o := LanguageOptions(appleLanguages("fr-FR", 0.95, "de-DE", 0.9, "ja-JP", 0.89), nil, "", "en")
		if !slices.Equal(tags(o), []string{"de-DE", "en", "fr-FR"}) {
			t.Fatalf("options %v", tags(o))
		}
		names := []string{o[0].Name, o[1].Name, o[2].Name}
		if !slices.Equal(names, []string{"Deutsch (Deutschland)", "English", "Français (France)"}) {
			t.Errorf("names %q", names)
		}
		if !OffersLanguages(o, "") {
			t.Error("not offered")
		}
	})
	t.Run("adds a community catalogue of the same language to Apple's coverage", func(t *testing.T) {
		o := LanguageOptions(appleLanguages("pt-BR", 0.6), []CommunityShare{{"pt", 0.35}}, "", "en")
		if !slices.Equal(tags(o), []string{"en", "pt-BR"}) {
			t.Errorf("options %v", tags(o))
		}
	})
	t.Run("offers a complete community catalogue Apple has no locale for, once", func(t *testing.T) {
		o := LanguageOptions(appleLanguages("fr-FR", 0.95),
			[]CommunityShare{{"cy", 0.92}, {"fr", 0.95}, {"eo", 0.5}}, "", "en")
		if !slices.Equal(tags(o), []string{"cy", "en", "fr-FR"}) {
			t.Errorf("options %v", tags(o))
		}
	})
	t.Run("keeps the chosen language listed, so the setting can be read and changed", func(t *testing.T) {
		o := LanguageOptions(appleLanguages("fr-FR", 0.07), nil, "fr-FR", "en")
		if !slices.Equal(tags(o), []string{"en", "fr-FR"}) || !OffersLanguages(o, "fr-FR") {
			t.Errorf("options %v", tags(o))
		}
		if got := tags(LanguageOptions(appleLanguages("fr-FR", 0.95), nil, "FR-fr", "en")); !slices.Equal(got, []string{"en", "fr-FR"}) {
			t.Errorf("case-different current tag: %v", got)
		}
	})
}

func TestNativeName(t *testing.T) {
	for tag, want := range map[string]string{
		"fr-CA":     "Français (Canada)",
		"en":        "English",
		"not a tag": "not a tag",
		"de":        "Deutsch",
		"ja":        "日本語",
		"zh-Hant":   "繁體中文",
		"en-GB":     "English (United Kingdom)",
	} {
		if got := NativeName(tag); got != want {
			t.Errorf("NativeName(%q) = %q, want %q", tag, got, want)
		}
	}
}
