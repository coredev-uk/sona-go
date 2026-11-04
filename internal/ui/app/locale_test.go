package app

import (
	"slices"
	"testing"

	"github.com/coredev-uk/sona-go/internal/i18n/l10n"
)

func TestTheAccountLanguageIsPreferredFirst(t *testing.T) {
	l := &localizer{}
	sys := l10n.SystemLanguages()
	if got := l.preferred(); !slices.Equal(got, sys) {
		t.Errorf("signed out: %v, want the system's %v", got, sys)
	}
	l.account = "fr-CA"
	if got := l.preferred(); len(got) == 0 || got[0] != "fr-CA" || !slices.Equal(got[1:], sys) {
		t.Errorf("signed in: %v", got)
	}
	if got := l.regional(); got[len(got)-1] != "fr-CA" {
		t.Errorf("regional %v: the account refines the system's", got)
	}
}
