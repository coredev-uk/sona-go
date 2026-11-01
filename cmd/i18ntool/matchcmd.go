package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// missingKeys lists the mapped keys a bundle lacks (a plural key lacks its
// `.other` form).
func missingKeys(ms []mapping, b appleBundle) []string {
	var missing []string
	for _, m := range ms {
		if _, ok := appleMessage(b, m.Key, m.Plural); !ok {
			missing = append(missing, m.Key)
		}
	}
	return missing
}

func runMatch(ctx context.Context, r repo, f *fetcher) error {
	sona, err := r.loadSona()
	if err != nil {
		return err
	}
	ms, err := r.loadMappings(sona)
	if err != nil {
		return err
	}
	rejected, err := readJSON[rejection](r.path(rejectedPath))
	if err != nil {
		return err
	}
	urls, err := f.discoverBundles(ctx)
	if err != nil {
		return err
	}
	bundle, err := f.bundle(ctx, urls[appleSourceLocale])
	if err != nil {
		return err
	}
	if missing := missingKeys(ms, bundle); len(missing) > 0 {
		return fmt.Errorf("mapped keys missing from Apple's %s bundle: %s", appleSourceLocale, strings.Join(missing, ", "))
	}
	ix := indexBundle(bundle)
	res := matchAll(sona, ix, ms, rejected)
	for _, m := range res.drifted {
		fmt.Printf("note: %q no longer matches %s's en-GB text; review the mapping\n", m.Message, m.Key)
	}
	if err := os.MkdirAll(r.path(appleDir), 0o755); err != nil {
		return err
	}
	out := r.path(appleDir + "/proposals.json")
	if err := writeJSON(out, res.proposals); err != nil {
		return err
	}
	fmt.Printf("Apple %s bundle: %d strings (%d plain keys, %d plural keys)\n", appleSourceLocale, len(bundle), len(ix.plain), len(ix.plural))
	fmt.Printf("Sona messages: %d (%d plain or single-plural, so mappable)\n", len(sona.entries), len(sona.parsed))
	fmt.Printf("Messages with a match: %d exact, %d normalised\n", res.exact, res.normalised)
	fmt.Printf("Mapped: %d; rejected pairs: %d; matches left out by a rejection: %d\n", len(ms), len(rejected), res.rejectedHits)
	fmt.Printf("New proposals: %d → %s\n", len(res.proposals), out)
	if len(res.proposals) > 0 {
		fmt.Printf("Review: set \"decision\" to \"accept\" or \"reject\" (with a \"reason\"), then: i18ntool match -apply %s\n", out)
	}
	return nil
}

type matchResult struct {
	exact, normalised int // messages whose best match is of that kind
	rejectedHits      int // messages whose every match was rejected
	proposals         []proposal
	drifted           []mapping
}

// matchAll matches every mappable Sona message; messages already mapped
// are counted, not proposed, and rejected pairs are skipped.
func matchAll(sona *sonaCatalogue, ix *appleIndex, ms []mapping, rejected []rejection) matchResult {
	var res matchResult
	mapped := map[msgKey]mapping{}
	for _, m := range ms {
		mapped[m.msgKey()] = m
	}
	isRejected := map[msgKey]map[string]bool{}
	for _, rj := range rejected {
		k := msgKey{rj.Message, rj.Context}
		if isRejected[k] == nil {
			isRejected[k] = map[string]bool{}
		}
		isRejected[k][rj.Key] = true
	}
	for _, e := range sona.entries {
		k := e.key()
		msg, ok := sona.parsed[k]
		if !ok {
			continue
		}
		cands := ix.match(msg)
		if m, ok := mapped[k]; ok {
			if !hasKey(cands, m.Key) {
				res.drifted = append(res.drifted, m)
			}
		}
		if len(cands) > 0 {
			if cands[0].Kind == matchExact {
				res.exact++
			} else {
				res.normalised++
			}
		}
		if _, ok := mapped[k]; ok {
			continue
		}
		var open []candidate
		for _, c := range cands {
			if !isRejected[k][c.Key] {
				open = append(open, c)
			}
		}
		if len(open) == 0 {
			if len(cands) > 0 {
				res.rejectedHits++
			}
			continue
		}
		best := open[0]
		p := proposal{
			mapping: mapping{Message: k.ID, Context: k.Context, Key: best.Key, Plural: best.Plural, Placeholders: best.Placeholders},
			Match:   best.Kind,
			Apple:   appleText(ix.bundle, best.Key, best.Plural),
		}
		if len(p.Placeholders) == 0 {
			p.Placeholders = nil
		}
		for _, c := range open[1:] {
			p.Alternatives = append(p.Alternatives, c.Key)
		}
		res.proposals = append(res.proposals, p)
	}
	return res
}

func hasKey(cs []candidate, key string) bool {
	for _, c := range cs {
		if c.Key == key {
			return true
		}
	}
	return false
}

// appleText shows a key's en-GB text for review ("one: …; other: …" for a
// plural).
func appleText(b appleBundle, key string, plural bool) string {
	if !plural {
		return b[key]
	}
	var parts []string
	for _, c := range pluralCategories {
		if s, ok := b[key+"."+c]; ok {
			parts = append(parts, c+": "+s)
		}
	}
	return strings.Join(parts, "; ")
}

// applyReview moves accepted proposals into the mapping and rejected ones
// into the rejection list; undecided ones stay out of both.
func applyReview(r repo, path string) error {
	sona, err := r.loadSona()
	if err != nil {
		return err
	}
	ms, err := r.loadMappings(sona)
	if err != nil {
		return err
	}
	rejected, err := readJSON[rejection](r.path(rejectedPath))
	if err != nil {
		return err
	}
	ps, err := readJSON[proposal](path)
	if err != nil {
		return err
	}
	have := map[msgKey]bool{}
	for _, m := range ms {
		have[m.msgKey()] = true
	}
	var errs []error
	accepted, rejectedN, undecided := 0, 0, 0
	for _, p := range ps {
		switch p.Decision {
		case "accept":
			if have[p.msgKey()] {
				errs = append(errs, fmt.Errorf("already mapped: %q", p.Message))
				continue
			}
			have[p.msgKey()] = true
			ms = append(ms, p.mapping)
			accepted++
		case "reject":
			if strings.TrimSpace(p.Reason) == "" {
				errs = append(errs, fmt.Errorf("rejection without a reason: %q", p.Message))
				continue
			}
			rejected = append(rejected, rejection{Message: p.Message, Context: p.Context, Key: p.Key, Reason: p.Reason})
			rejectedN++
		case "":
			undecided++
		default:
			errs = append(errs, fmt.Errorf("decision %q for %q: want accept or reject", p.Decision, p.Message))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	if err := validateMappings(ms, sona.parsed); err != nil {
		return err
	}
	sortMappings(ms)
	sortRejections(rejected)
	if err := writeJSON(r.path(mappingPath), ms); err != nil {
		return err
	}
	if err := writeJSON(r.path(rejectedPath), rejected); err != nil {
		return err
	}
	fmt.Printf("accepted %d, rejected %d, undecided %d\n", accepted, rejectedN, undecided)
	return nil
}
