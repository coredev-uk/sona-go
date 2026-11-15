package addons

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The manifest examples of docs/addons.md are what ParseManifest accepts
// (internal/cli's tests check them against the page's JSON schema), and the
// limits the page states are the code's.
func TestDocManifestExamplesParse(t *testing.T) {
	b, err := os.ReadFile("../../docs/addons.md")
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	n := 0
	for _, m := range regexp.MustCompile("(?s)```json example Manifest\n(.*?)\n```").FindAllStringSubmatch(text, -1) {
		n++
		if _, err := ParseManifest([]byte(m[1])); err != nil {
			t.Errorf("example %s: %v", m[1], err)
		}
	}
	if n == 0 {
		t.Error("no manifest example in the page")
	}
	for _, want := range []string{
		"The manifest is at most 64 KiB",
		"1 to 60 characters",
		"At most 300 characters",
		"at most 200 bytes",
		"At most 100 folders",
		"rotated at 1 MiB with one older copy",
		"cut at 4 KiB",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	if MaxManifestBytes != 64<<10 || MaxNameRunes != 60 || MaxDescRunes != 300 || MaxCommandBytes != 200 || MaxScanned != 100 || LogBytes != 1<<20 || logBackups != 1 || MaxLogLine != 4096 {
		t.Error("a limit changed; update docs/addons.md")
	}
	for _, code := range []string{ProblemUnreadable, ProblemTooLarge, ProblemManifest, ProblemID, ProblemReservedID, ProblemIDMismatch, ProblemName,
		ProblemVersion, ProblemCommand, ProblemPermission, ProblemNoExecutable, ProblemEscapes, ProblemUnsafe, ProblemTooMany} {
		if !strings.Contains(text, "`"+code+"`") {
			t.Errorf("the page doesn't list the problem %q", code)
		}
	}
}
