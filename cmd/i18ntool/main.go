// Command i18ntool keeps Sona's message catalogues: it extracts Go's own
// messages (internal/i18n/l10n) into their English catalogue, collects the
// interface's messages (internal/ui: i18n.T and i18n.TC calls) into the
// UI's catalogue, and maps the UI's messages to Apple Music's string keys
// (docs/ui-architecture.md, "Strings and locales"; CONTRIBUTING.md,
// "Strings and translations").
//
//	i18ntool extract          write both English catalogues: Go's
//	                          (internal/i18n/l10n/locales/en.po) and the UI's
//	                          (internal/ui/i18n/locales/en.po, en-XA.po),
//	                          each exactly the messages the code asks for
//	i18ntool extract -check   fail when either is out of date, a mapped
//	                          message is gone from the UI, or a call's text
//	                          is not a string literal
//	i18ntool match            propose Apple keys for unmapped messages
//	i18ntool match -apply F   move reviewed proposals into the mapping
//	i18ntool import           convert Apple's translations for review (gitignored)
//
// Apple's bundles are fetched from music.apple.com on every run and never
// committed; the mapping holds only message IDs, keys and placeholder
// names. Sona itself fetches the translations at run time
// (internal/i18n/applestrings); `import` runs the same conversion and
// checks and writes the result where a reviewer can read it. Run it from
// the repository root (or pass -root).
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/coredev-uk/sona-go/internal/i18n/applestrings"
	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

// Paths under the repository root.
const (
	i18nDir      = "internal/ui/i18n/locales"
	sourcePOPath = i18nDir + "/en.po"
	pseudoPOPath = i18nDir + "/en-XA.po"
	mappingPath  = i18nDir + "/apple-keys.json"
	rejectedPath = i18nDir + "/apple-keys-rejected.json"
	appleDir     = i18nDir + "/apple" // gitignored: Apple's text, for review
	uiDir        = "internal/ui"      // the Go sources whose i18n.T calls extract collects
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: i18ntool [-root DIR] extract [-check] | match [-apply FILE] | import")
	os.Exit(2)
}

func newFetcher() *applestrings.Fetcher {
	return &applestrings.Fetcher{HTTP: &http.Client{Timeout: 60 * time.Second}}
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
	case "extract":
		fs := flag.NewFlagSet("extract", flag.ExitOnError)
		check := fs.Bool("check", false, "fail when the catalogue is out of date instead of writing it")
		_ = fs.Parse(args)
		err = runExtract(r, *check)
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
	entries []*po.Entry
	parsed  map[po.Key]applestrings.Message
}

func (r repo) loadSona() (*sonaCatalogue, error) {
	f, err := po.Read(r.path(sourcePOPath))
	if err != nil {
		return nil, err
	}
	c := &sonaCatalogue{parsed: map[po.Key]applestrings.Message{}}
	for _, e := range f.Entries {
		if e.Obsolete {
			continue
		}
		c.entries = append(c.entries, e)
		if m, ok := applestrings.ParseSona(e.ID); ok {
			c.parsed[e.Key()] = m
		}
	}
	return c, nil
}

func (r repo) loadMappings(sona *sonaCatalogue) ([]applestrings.Mapping, error) {
	ms, err := readJSON[applestrings.Mapping](r.path(mappingPath))
	if err != nil {
		return nil, err
	}
	if err := applestrings.ValidateMappings(ms, sona.parsed); err != nil {
		return nil, fmt.Errorf("%s: %w", mappingPath, err)
	}
	return ms, nil
}
