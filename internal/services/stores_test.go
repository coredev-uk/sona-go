package services

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/auth"
	"github.com/coredev-uk/sona-go/internal/db"
)

// TestAccountStoreKeepsContentLanguages: the storefront's supported
// languages survive a restart, for the l= choice before the next
// validation.
func TestAccountStoreKeepsContentLanguages(t *testing.T) {
	ctx := t.Context()
	d, err := db.Open(ctx, filepath.Join(t.TempDir(), "sona.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	s := AccountStore{DB: d}
	in := auth.Account{StorefrontID: "ca", Language: "en-CA", Languages: []string{"en-CA", "fr-CA"}}
	if err := s.Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.Language != "en-CA" || !slices.Equal(got.Languages, in.Languages) {
		t.Fatalf("loaded %+v, want the default and supported languages", got)
	}
	in.Languages = nil
	if err := s.Save(ctx, in); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Load(ctx); len(got.Languages) != 0 {
		t.Fatalf("languages after saving none = %q", got.Languages)
	}
}
