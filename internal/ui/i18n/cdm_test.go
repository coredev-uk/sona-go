package i18n

import (
	"strings"
	"testing"
)

var cdmCodes = []string{
	"network", "unsupported_platform", "manifest_invalid", "untrusted_url", "too_large", "hash_mismatch",
	"signature_invalid", "archive_invalid", "load_failed", "load_check_incomplete", "disk",
}

func TestEveryCDMFailureCodeHasItsOwnWords(t *testing.T) {
	general := CDMProblem("")
	if general != "Widevine couldn't be downloaded. Try again later." || CDMProblem("a_new_code") != general {
		t.Errorf("unknown codes say %q", general)
	}
	seen := map[string]string{}
	for _, code := range cdmCodes {
		if !HasCDMProblem(code) {
			t.Errorf("%s: HasCDMProblem is false", code)
		}
		s := CDMProblem(code)
		if s == general || s == "" {
			t.Errorf("%s has no wording of its own: %q", code, s)
		}
		if other, dup := seen[s]; dup {
			t.Errorf("%s and %s share %q", code, other, s)
		}
		seen[s] = code
		if !strings.HasSuffix(s, ".") {
			t.Errorf("%s: %q is not a sentence", code, s)
		}
	}
	if HasCDMProblem("a_new_code") || HasCDMProblem("") {
		t.Error("an unknown code has wording")
	}
}

func TestCDMFailedStateSaysInstallWhenOnlyLoadingFailed(t *testing.T) {
	for code, want := range map[string]string{
		"load_failed": "Couldn't install", "load_check_incomplete": "Couldn't install",
		"network": "Couldn't download", "disk": "Couldn't download", "": "Couldn't download",
	} {
		if got := CDMFailedState(code); got != want {
			t.Errorf("CDMFailedState(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestFormatPercentWritesWholePercentsInTheShownLanguage(t *testing.T) {
	SetLocale("en")
	if got := FormatPercent(0.42); got != "42%" {
		t.Errorf("0.42 = %q", got)
	}
	if got := FormatPercent(1); got != "100%" {
		t.Errorf("1 = %q", got)
	}
	if got := FormatPercent(0.425); got != "43%" && got != "42%" {
		t.Errorf("0.425 = %q", got)
	}
	SetLocale("de")
	defer SetLocale("en")
	if got := FormatPercent(0.42); got != "42\u00a0%" {
		t.Errorf("German 0.42 = %q", got)
	}
}
