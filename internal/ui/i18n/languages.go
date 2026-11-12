package i18n

import (
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	mygoi18n "github.com/egoist/mygo/i18n"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
	"github.com/coredev-uk/sona-go/internal/services"
)

// LanguageOption is one language the Language setting offers (besides
// Automatic).
type LanguageOption struct {
	// Tag is the setting's value: a BCP 47 tag ("en" for English).
	Tag string
	// Name is the language's name in itself ("Français (Canada)"), so
	// anyone finds their own.
	Name string
}

// CommunityShare is a community catalogue and the share of Sona's messages
// it translates.
type CommunityShare struct {
	Tag   string
	Share float64
}

// NativeName is a language's name in itself, capitalised as a list item
// starts ("Français (Canada)", as Intl.DisplayNames writes it with
// languageDisplay "standard": the language, then its region); the tag when
// CLDR has no name for it.
func NativeName(tag string) string {
	name := mygoi18n.Autonym(tag)
	if name == "" || name == tag {
		return tag
	}
	// MyGo gives CLDR's dialect names ("français canadien"); a region of a
	// language with no such name is written after the language, and so are
	// the ones that have: Sona's list shows regions the same way.
	if !strings.ContainsAny(name, "(（") {
		if t, err := language.Parse(tag); err == nil {
			if r, conf := t.Region(); conf == language.Exact {
				base := mygoi18n.Autonym(mygoi18n.Base(tag))
				if region := display.Regions(language.Make(mygoi18n.Base(tag))).Name(r); base != "" && region != "" {
					name = base + " (" + region + ")"
				}
			}
		}
	}
	_, size := utf8.DecodeRuneInString(name)
	// The first letter, upper-cased by the language's own rules.
	up := cases.Upper(language.Make(tag)).String(name[:size])
	return up + name[size:]
}

// LanguageOptions are the languages the Language setting offers: English,
// and every locale whose layers reach SelectableShare: Apple's locales (by
// their counted coverage, or the mapped share bounding it until one is
// downloaded) with the community catalogue of the same language added, and
// community catalogues Apple has no locale for. The chosen tag (current)
// stays listed even when it no longer qualifies, so it can be read and
// changed. Sorted by name in collation's rules.
func LanguageOptions(apple services.AppleLanguages, community []CommunityShare, current, collation string) []LanguageOption {
	communityTags := make([]string, len(community))
	for i, c := range community {
		communityTags[i] = c.Tag
	}
	tags := []string{SourceLocale}
	appleTags := make([]string, len(apple.Locales))
	for i, l := range apple.Locales {
		appleTags[i] = l.Tag
	}
	for _, l := range apple.Locales {
		sibling, _ := l10n.Match([]string{l.Tag}, communityTags)
		extra := 0.0
		for _, c := range community {
			if c.Tag == sibling {
				extra = c.Share
			}
		}
		if min(1, l.Coverage+extra) >= SelectableShare {
			tags = append(tags, l.Tag)
		}
	}
	for _, c := range community {
		if _, hasApple := l10n.Match([]string{c.Tag}, appleTags); c.Share >= SelectableShare && !hasApple {
			tags = append(tags, c.Tag)
		}
	}
	if current != "" {
		tags = append(tags, current)
	}

	seen := map[string]bool{}
	var out []LanguageOption
	for _, tag := range tags {
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, LanguageOption{Tag: tag, Name: NativeName(tag)})
	}
	col := collate.New(language.Make(collation))
	slices.SortStableFunc(out, func(a, b LanguageOption) int { return col.CompareString(a.Name, b.Name) })
	return out
}

// OffersLanguages reports whether the Language setting is offered: once a
// language besides English is selectable, or while one is chosen (so it can
// be changed back).
func OffersLanguages(options []LanguageOption, current string) bool {
	return current != "" || slices.ContainsFunc(options, func(o LanguageOption) bool { return o.Tag != SourceLocale })
}
