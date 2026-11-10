package theme

import (
	"testing"

	"github.com/egoist/mygo/plugins/sfsymbols"
	"github.com/egoist/mygo/ui"
)

// The macOS the app runs on has every symbol the map names: one it lacks
// would draw its Lucide fallback, which is no failure but is a wrong map.
func TestEveryMappedSymbolIsOnThisMac(t *testing.T) {
	if !sfsymbols.Available() {
		t.Fatal("this macOS draws no SF Symbols")
	}
	for name, sym := range symbolOf {
		if _, ok := sfsymbols.Image(sym, 32, ui.RGB(0, 0, 0), sfsymbols.Options{}); !ok {
			t.Errorf("%s: macOS has no symbol %q", name, sym)
		}
	}
	// The favourite star's Filled form.
	if _, ok := sfsymbols.Image("star.fill", 32, ui.RGB(0, 0, 0), sfsymbols.Options{}); !ok {
		t.Error("macOS has no star.fill")
	}
}
