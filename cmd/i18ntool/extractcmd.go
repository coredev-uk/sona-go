package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n/extract"
	"github.com/coredev-uk/sona-go/internal/i18n/po"
)

// runExtract writes Go's English catalogue from the source, or with check
// only reports whether the committed one is up to date.
func runExtract(r repo, check bool) error {
	f, err := extract.Extract(string(r))
	if err != nil {
		return err
	}
	out := r.path(extract.SourcePath)
	want := po.Format(f)
	if check {
		got, err := os.ReadFile(out)
		if err != nil {
			return err
		}
		if bytes.Equal(got, want) {
			fmt.Printf("%s is up to date: %d messages\n", extract.SourcePath, len(f.Entries))
			return nil
		}
		old, err := po.Parse(got)
		if err != nil {
			return fmt.Errorf("%s: %w", extract.SourcePath, err)
		}
		diff := extract.Diff(old, f)
		if len(diff) == 0 {
			diff = []string{"~ formatting"}
		}
		return fmt.Errorf("%s is out of date (run `%s`):\n  %s", extract.SourcePath, extract.Command, strings.Join(diff, "\n  "))
	}
	if err := os.WriteFile(out, want, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s: %d messages\n", extract.SourcePath, len(f.Entries))
	return nil
}
