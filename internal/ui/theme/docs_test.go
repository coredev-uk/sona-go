package theme

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// The theme file format page carries a complete example: it has to be a theme
// Sona accepts. The themes guide has to list every built-in.
func TestThemesDocExampleIsAValidThemeAndListsTheBuiltIns(t *testing.T) {
	const format = "../../../site/src/content/docs/reference/theme-format.md"
	const guide = "../../../site/src/content/docs/guide/themes.md"
	doc, err := os.ReadFile(format)
	if err != nil {
		t.Fatal(err)
	}
	list, err := os.ReadFile(guide)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("(?s)```json\n(.*?)\n```").FindSubmatch(doc)
	if m == nil {
		t.Fatal("no JSON example in " + format)
	}
	th, problems := ParseTheme(m[1])
	if th == nil {
		t.Fatalf("the documented example is refused: %v", problems)
	}
	if !th.HasLight() || !th.HasDark() {
		t.Error("the example should show both variants")
	}
	for _, b := range builtinOrder {
		if !strings.Contains(string(list), "| `"+b+"` |") {
			t.Errorf("%s does not list %s", guide, b)
		}
	}
}
