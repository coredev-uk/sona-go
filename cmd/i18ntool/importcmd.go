package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

// localeResult is one locale's import.
type localeResult struct {
	Tag     string // BCP 47
	Apple   string // Apple's tag
	Skip    string
	Entries []*po.Entry
	Missing []string
	Invalid []string
	Err     error
}

// importLocale converts each mapped key the bundle has into a Lingui
// message, tagged with its Apple key, with the checks Sona applies at run
// time (applestrings.Mapping.Translate).
func importLocale(b applestrings.Bundle, ms []applestrings.Mapping, sona *sonaCatalogue) (entries []*po.Entry, missing, invalid []string) {
	byKey := map[po.Key]*po.Entry{}
	for _, e := range sona.entries {
		byKey[e.Key()] = e
	}
	for _, m := range ms {
		text, err := m.Translate(b)
		switch {
		case errors.Is(err, applestrings.ErrMissing):
			missing = append(missing, m.Key)
			continue
		case err != nil:
			invalid = append(invalid, fmt.Sprintf("%s: %v", m.Key, err))
			continue
		}
		src := byKey[m.MsgKey()]
		entries = append(entries, &po.Entry{
			Extracted:  []string{"apple-key: " + m.Key},
			References: src.References,
			Context:    src.Context,
			HasContext: src.HasContext,
			ID:         src.ID,
			Str:        text,
		})
	}
	return entries, missing, invalid
}

// missingKeys lists the mapped keys a bundle lacks (a plural key lacks its
// `.other` form).
func missingKeys(ms []applestrings.Mapping, b applestrings.Bundle) []string {
	var missing []string
	for _, m := range ms {
		if _, ok := applestrings.AppleMessage(b, m.Key, m.Plural); !ok {
			missing = append(missing, m.Key)
		}
	}
	return missing
}

func runImport(ctx context.Context, r repo, f *applestrings.Fetcher) error {
	sona, err := r.loadSona()
	if err != nil {
		return err
	}
	ms, err := r.loadMappings(sona)
	if err != nil {
		return err
	}
	urls, err := f.Discover(ctx)
	if err != nil {
		return err
	}
	source, err := f.Bundle(ctx, urls[applestrings.SourceLocale])
	if err != nil {
		return err
	}
	if missing := missingKeys(ms, source); len(missing) > 0 {
		return fmt.Errorf("mapped keys disappeared from Apple's %s bundle (update the mapping): %s", applestrings.SourceLocale, strings.Join(missing, ", "))
	}

	tags := slices.Sorted(maps.Keys(urls))
	results := make([]localeResult, len(tags))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, tag := range tags {
		results[i] = localeResult{Tag: applestrings.BCP47(tag), Apple: tag, Skip: applestrings.Skip(tag)}
		if results[i].Skip != "" {
			continue
		}
		wg.Add(1)
		go func(res *localeResult, url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			b, err := f.Bundle(ctx, url)
			if err != nil {
				res.Err = err
				return
			}
			res.Entries, res.Missing, res.Invalid = importLocale(b, ms, sona)
		}(&results[i], urls[tag])
	}
	wg.Wait()

	dir := r.path(appleDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	old, _ := filepath.Glob(filepath.Join(dir, "*.po"))
	for _, p := range old {
		if err := os.Remove(p); err != nil {
			return err
		}
	}
	var errs []error
	for _, res := range results {
		if res.Err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", res.Apple, res.Err))
			continue
		}
		if res.Skip != "" {
			continue
		}
		f := &po.File{
			Header: po.Header(
				[2]string{"MIME-Version", "1.0"},
				[2]string{"Content-Type", "text/plain; charset=utf-8"},
				[2]string{"Content-Transfer-Encoding", "8bit"},
				[2]string{"Language", res.Tag},
				[2]string{"X-Source", "Apple Music on the web, locales/" + res.Apple + "/translations.json"},
				[2]string{"X-Generator", "i18ntool import"},
			),
			Entries: res.Entries,
		}
		if err := os.WriteFile(filepath.Join(dir, res.Tag+".po"), po.Format(f), 0o644); err != nil {
			return err
		}
	}
	printImportSummary(results, ms, len(sona.entries))
	fmt.Printf("\nReview copies (gitignored): %s/<locale>.po\n", appleDir)
	return errors.Join(errs...)
}

func percent(n, total int) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(total))
}

func printImportSummary(results []localeResult, ms []applestrings.Mapping, total int) {
	fmt.Printf("Apple locales: %d; mapped keys: %d; Sona messages: %d\n\n", len(results), len(ms), total)
	fmt.Println("| Locale | Apple tag | Mapped keys found | Missing or refused | Coverage of Sona's messages |")
	fmt.Println("| ------ | --------- | ----------------- | ------------------ | --------------------------- |")
	for _, res := range results {
		switch {
		case res.Skip != "":
			fmt.Printf("| %s | %s | skipped: %s | | |\n", res.Tag, res.Apple, res.Skip)
		case res.Err != nil:
			fmt.Printf("| %s | %s | failed: %v | | |\n", res.Tag, res.Apple, res.Err)
		default:
			n := len(res.Entries)
			fmt.Printf("| %s | %s | %d | %d | %s |\n", res.Tag, res.Apple, n, len(res.Missing)+len(res.Invalid), percent(n, total))
		}
	}
	for _, res := range results {
		for _, s := range res.Invalid {
			fmt.Printf("%s: refused: %s\n", res.Tag, s)
		}
	}
}
