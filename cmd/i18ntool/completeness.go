package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// completeness is completeness.json, which the frontend reads to decide
// which locales it offers (`selectableLocales` in shared/i18n/locale.ts).
// A locale's messages come from Apple's catalogue and then the community
// catalogue of the same language; Share is the fraction of Sona's messages
// the two translate together.
type completeness struct {
	Messages  int                       `json:"messages"`
	Threshold float64                   `json:"threshold"`
	Locales   map[string]localeCoverage `json:"locales"`
}

type localeCoverage struct {
	Apple      int     `json:"apple"`
	Community  int     `json:"community"`
	Share      float64 `json:"share"`
	Selectable bool    `json:"selectable"`
}

// language is a tag's primary language subtag.
func language(tag string) string {
	l, _, _ := strings.Cut(strings.ToLower(tag), "-")
	return l
}

// matchTag mirrors the frontend's matchLocale for one tag: the exact tag,
// else the bare language, else another tag of the same language.
func matchTag(tag string, available []string) (string, bool) {
	want := strings.ToLower(tag)
	for _, try := range []func(string) bool{
		func(a string) bool { return strings.ToLower(a) == want },
		func(a string) bool { return strings.ToLower(a) == language(want) },
		func(a string) bool { return language(a) == language(want) },
	} {
		for _, a := range available {
			if try(a) {
				return a, true
			}
		}
	}
	return "", false
}

// translatedKeys reads each `<tag>.po` in dir: the messages of Sona's
// catalogue it translates (fuzzy and empty entries do not count).
func translatedKeys(dir string, sona map[msgKey]bool) (map[string]map[msgKey]bool, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.po"))
	if err != nil {
		return nil, err
	}
	out := map[string]map[msgKey]bool{}
	for _, p := range paths {
		f, err := readPO(p)
		if err != nil {
			return nil, err
		}
		keys := map[msgKey]bool{}
		for _, e := range f.Entries {
			if !e.Obsolete && e.Str != "" && !e.fuzzy() && sona[e.key()] {
				keys[e.key()] = true
			}
		}
		out[strings.TrimSuffix(filepath.Base(p), ".po")] = keys
	}
	return out, nil
}

// computeCompleteness counts every locale with an Apple or a community
// catalogue.
func computeCompleteness(messages int, apple, community map[string]map[msgKey]bool) completeness {
	c := completeness{Messages: messages, Threshold: selectableShare, Locales: map[string]localeCoverage{}}
	communityTags := sortedKeys(community)
	tags := sortedKeys(apple)
	for _, t := range communityTags {
		if _, ok := apple[t]; !ok {
			tags = append(tags, t)
		}
	}
	for _, tag := range tags {
		union := map[msgKey]bool{}
		for k := range apple[tag] {
			union[k] = true
		}
		cov := localeCoverage{Apple: len(apple[tag])}
		if ct, ok := matchTag(tag, communityTags); ok {
			for k := range community[ct] {
				if !union[k] {
					union[k] = true
					cov.Community++
				}
			}
		}
		if messages > 0 {
			cov.Share = math.Round(float64(len(union))/float64(messages)*1000) / 1000
			cov.Selectable = float64(len(union)) >= selectableShare*float64(messages)
		}
		c.Locales[tag] = cov
	}
	return c
}

func writeCompleteness(r repo, sona *sonaCatalogue) error {
	keys := map[msgKey]bool{}
	for _, e := range sona.entries {
		keys[e.key()] = true
	}
	apple, err := translatedKeys(r.path(appleDir), keys)
	if err != nil {
		return err
	}
	community, err := translatedKeys(r.path(communityDir), keys)
	if err != nil {
		return err
	}
	c := computeCompleteness(len(keys), apple, community)
	if err := writeJSON(r.path(completenessPath), c); err != nil {
		return err
	}
	selectable := 0
	for _, l := range c.Locales {
		if l.Selectable {
			selectable++
		}
	}
	fmt.Printf("\n%s: %d locales, %d at least %.0f%% complete\n", completenessPath, len(c.Locales), selectable, selectableShare*100)
	return nil
}
