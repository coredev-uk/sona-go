package theme

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// docs/themes.md carries a complete example: it has to be a theme Sona
// accepts, and the page has to list every built-in.
func TestThemesDocExampleIsAValidThemeAndListsTheBuiltIns(t *testing.T) {
	doc, err := os.ReadFile("../../../docs/themes.md")
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("(?s)```json\n(.*?)\n```").FindSubmatch(doc)
	if m == nil {
		t.Fatal("no JSON example in docs/themes.md")
	}
	th, problems := ParseTheme(m[1])
	if th == nil {
		t.Fatalf("the documented example is refused: %v", problems)
	}
	if !th.HasLight() || !th.HasDark() {
		t.Error("the example should show both variants")
	}
	for _, b := range builtinOrder {
		if !strings.Contains(string(doc), "| `"+b+"` |") {
			t.Errorf("docs/themes.md does not list %s", b)
		}
	}
}
