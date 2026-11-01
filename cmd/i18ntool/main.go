// Command i18ntool maps Sona's messages to Apple Music's string keys and
// builds the catalogues around them (docs/frontend-architecture.md,
// "Strings and locales"; CONTRIBUTING.md, "Strings and translations").
//
//	i18ntool match            propose Apple keys for unmapped messages
//	i18ntool match -apply F   move reviewed proposals into the mapping
//	i18ntool import           write Apple's translations (gitignored)
//	i18ntool completeness     recount how complete each locale is
//
// Apple's bundles are fetched from music.apple.com on every run and never
// committed; the mapping holds only message IDs, keys and placeholder
// names. Run it from the repository root (or pass -root).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
)

// Paths under the repository root.
const (
	i18nDir      = "frontend/src/shared/i18n"
	sourcePOPath = i18nDir + "/locales/en.po"
	mappingPath  = i18nDir + "/apple-keys.json"
	rejectedPath = i18nDir + "/apple-keys-rejected.json"
	appleDir     = i18nDir + "/apple" // gitignored: Apple's text
	communityDir = i18nDir + "/community"
	// completenessPath is gitignored while it counts Apple's text.
	completenessPath = i18nDir + "/completeness.json"
)

// selectableShare is how complete a locale's catalogues must be before
// Sona offers the locale (frontend `selectableShare`).
const selectableShare = 0.9

func usage() {
	fmt.Fprintln(os.Stderr, "usage: i18ntool [-root DIR] match [-apply FILE] | import | completeness")
	os.Exit(2)
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() < 1 {
		usage()
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	r := repo(*root)
	cmd, args := flag.Arg(0), flag.Args()[1:]
	var err error
	switch cmd {
	case "match":
		fs := flag.NewFlagSet("match", flag.ExitOnError)
		apply := fs.String("apply", "", "reviewed proposals file to apply")
		_ = fs.Parse(args)
		if *apply != "" {
			err = applyReview(r, *apply)
		} else {
			err = runMatch(ctx, r, newFetcher())
		}
	case "import":
		err = runImport(ctx, r, newFetcher())
	case "completeness":
		var sona *sonaCatalogue
		if sona, err = r.loadSona(); err == nil {
			err = writeCompleteness(r, sona)
		}
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "i18ntool:", err)
		os.Exit(1)
	}
}

// repo resolves the tool's paths under a repository root.
type repo string

func (r repo) path(p string) string { return filepath.Join(string(r), filepath.FromSlash(p)) }

// sonaCatalogue is en.po: every message, and the ones that can map to an
// Apple key, parsed.
type sonaCatalogue struct {
	entries []*poEntry
	parsed  map[msgKey]message
}

func (r repo) loadSona() (*sonaCatalogue, error) {
	f, err := readPO(r.path(sourcePOPath))
	if err != nil {
		return nil, err
	}
	c := &sonaCatalogue{parsed: map[msgKey]message{}}
	for _, e := range f.Entries {
		if e.Obsolete {
			continue
		}
		c.entries = append(c.entries, e)
		if m, ok := parseSona(e.ID); ok {
			c.parsed[e.key()] = m
		}
	}
	return c, nil
}

func (r repo) loadMappings(sona *sonaCatalogue) ([]mapping, error) {
	ms, err := readJSON[mapping](r.path(mappingPath))
	if err != nil {
		return nil, err
	}
	if err := validateMappings(ms, sona.parsed); err != nil {
		return nil, fmt.Errorf("%s: %w", mappingPath, err)
	}
	return ms, nil
}
