package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// bcp47 writes Apple's lower-case tags in BCP 47 case: "zh-hans-cn" →
// "zh-Hans-CN", "es-419" stays.
func bcp47(tag string) string {
	parts := strings.Split(strings.ToLower(tag), "-")
	for i, p := range parts {
		if i == 0 {
			continue
		}
		switch len(p) {
		case 2:
			parts[i] = strings.ToUpper(p)
		case 4:
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, "-")
}

// skipLocale says why an Apple locale is not imported ("" to import it).
func skipLocale(tag string) string {
	switch {
	case tag == "dev":
		return "Apple's development strings"
	case tag == "en" || strings.HasPrefix(tag, "en-"):
		return "English: Sona's source text"
	}
	return ""
}

// localeResult is one locale's import.
type localeResult struct {
	Tag     string // BCP 47
	Apple   string // Apple's tag
	Skip    string
	Entries []*poEntry
	Missing []string
	Invalid []string
	Err     error
}

// importLocale converts each mapped key the bundle has into a Lingui
// message, tagged with its Apple key.
func importLocale(b appleBundle, ms []mapping, sona *sonaCatalogue) (entries []*poEntry, missing, invalid []string) {
	byKey := map[msgKey]*poEntry{}
	for _, e := range sona.entries {
		byKey[e.key()] = e
	}
	for _, m := range ms {
		apple, ok := appleMessage(b, m.Key, m.Plural)
		if !ok {
			missing = append(missing, m.Key)
			continue
		}
		renames := m.Placeholders
		if renames == nil {
			renames = map[string]string{}
		}
		text, err := toICU(apple, sona.parsed[m.msgKey()], renames)
		if err != nil {
			invalid = append(invalid, fmt.Sprintf("%s: %v", m.Key, err))
			continue
		}
		src := byKey[m.msgKey()]
		entries = append(entries, &poEntry{
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

func runImport(ctx context.Context, r repo, f *fetcher) error {
	sona, err := r.loadSona()
	if err != nil {
		return err
	}
	ms, err := r.loadMappings(sona)
	if err != nil {
		return err
	}
	urls, err := f.discoverBundles(ctx)
	if err != nil {
		return err
	}
	source, err := f.bundle(ctx, urls[appleSourceLocale])
	if err != nil {
		return err
	}
	if missing := missingKeys(ms, source); len(missing) > 0 {
		return fmt.Errorf("mapped keys disappeared from Apple's %s bundle (update the mapping): %s", appleSourceLocale, strings.Join(missing, ", "))
	}

	tags := sortedKeys(urls)
	results := make([]localeResult, len(tags))
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for i, tag := range tags {
		results[i] = localeResult{Tag: bcp47(tag), Apple: tag, Skip: skipLocale(tag)}
		if results[i].Skip != "" {
			continue
		}
		wg.Add(1)
		go func(res *localeResult, url string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			b, err := f.bundle(ctx, url)
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
		po := &poFile{
			Header: poHeader(
				[2]string{"MIME-Version", "1.0"},
				[2]string{"Content-Type", "text/plain; charset=utf-8"},
				[2]string{"Content-Transfer-Encoding", "8bit"},
				[2]string{"Language", res.Tag},
				[2]string{"X-Source", "Apple Music on the web, locales/" + res.Apple + "/translations.json"},
				[2]string{"X-Generator", "i18ntool import"},
			),
			Entries: res.Entries,
		}
		if err := os.WriteFile(filepath.Join(dir, res.Tag+".po"), formatPO(po), 0o644); err != nil {
			return err
		}
	}
	printImportSummary(results, ms, len(sona.entries))
	if err := writeCompleteness(r, sona); err != nil {
		return err
	}
	return errors.Join(errs...)
}

func percent(n, total int) string {
	if total == 0 {
		return "-"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(total))
}

func printImportSummary(results []localeResult, ms []mapping, total int) {
	fmt.Printf("Apple locales: %d; mapped keys: %d; Sona messages: %d; a locale is offered from %.0f%% complete\n\n", len(results), len(ms), total, selectableShare*100)
	fmt.Println("| Locale | Apple tag | Mapped keys found | Missing | Coverage of Sona's messages |")
	fmt.Println("| ------ | --------- | ----------------- | ------- | --------------------------- |")
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
			fmt.Printf("%s: not imported: %s\n", res.Tag, s)
		}
	}
}
